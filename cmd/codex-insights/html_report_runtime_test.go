package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

type trackingListener struct {
	net.Listener
	closed bool
}

type delayedAcceptListener struct {
	net.Listener
	accepted chan struct{}
	release  chan struct{}
	once     sync.Once
	closeErr error
	wrapConn func(net.Conn) net.Conn
}

func (listener *delayedAcceptListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	listener.accepted <- struct{}{}
	<-listener.release
	if listener.wrapConn != nil {
		connection = listener.wrapConn(connection)
	}
	return connection, nil
}

func (listener *delayedAcceptListener) Close() error {
	listener.once.Do(func() { close(listener.release) })
	return errors.Join(listener.Listener.Close(), listener.closeErr)
}

type sentinelCloseConn struct {
	net.Conn
	err error
}

func (connection sentinelCloseConn) Close() error {
	return errors.Join(connection.Conn.Close(), connection.err)
}

type failingAcceptListener struct{ err error }

func (listener failingAcceptListener) Accept() (net.Conn, error) { return nil, listener.err }
func (listener failingAcceptListener) Close() error              { return nil }
func (listener failingAcceptListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
}

type pendingOpenerFailureListener struct {
	err           error
	acceptStarted chan struct{}
	closed        chan struct{}
	closeOnce     sync.Once
}

func (listener *pendingOpenerFailureListener) Accept() (net.Conn, error) {
	close(listener.acceptStarted)
	return nil, listener.err
}

func (listener *pendingOpenerFailureListener) Close() error {
	listener.closeOnce.Do(func() { close(listener.closed) })
	return nil
}

func (listener *pendingOpenerFailureListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
}

func (listener *trackingListener) Close() error {
	listener.closed = true
	return listener.Listener.Close()
}

type failingHTTPResponseWriter struct {
	header http.Header
	err    error
}

func (writer *failingHTTPResponseWriter) Header() http.Header {
	if writer.header == nil {
		writer.header = make(http.Header)
	}
	return writer.header
}

func (writer *failingHTTPResponseWriter) WriteHeader(int) {}

func (writer *failingHTTPResponseWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type htmlContextKey struct{}

func TestAggregateHTMLLabelsIsDeterministic(t *testing.T) {
	labels := aggregateHTMLLabels(semanticReportResults{
		steering:        []analyze.SteeringResult{{Label: "steering"}, {Label: "continuation"}, {Label: "steering"}},
		reasons:         []analyze.SteeringReasonResult{{Reason: "z"}, {Reason: "a"}, {Reason: "z"}},
		promptQuality:   []analyze.PromptQualityResult{{Issue: "missing_scope"}, {Issue: "missing_scope"}},
		agentsRules:     []analyze.AgentsRuleResult{{Rule: "rule_b"}, {Rule: "rule_a"}},
		skillCandidates: []analyze.SkillCandidateResult{{Category: "workflow"}},
		validation:      []analyze.ValidationResult{{ValidationType: "tests"}, {ValidationType: "tests"}},
	})
	want := HTMLReportLabels{
		Steering:        []HTMLLabelCount{{Label: "steering", Count: 2}, {Label: "continuation", Count: 1}},
		Reasons:         []HTMLLabelCount{{Label: "z", Count: 2}, {Label: "a", Count: 1}},
		PromptQuality:   []HTMLLabelCount{{Label: "missing_scope", Count: 2}},
		AgentsRules:     []HTMLLabelCount{{Label: "rule_a", Count: 1}, {Label: "rule_b", Count: 1}},
		SkillCandidates: []HTMLLabelCount{{Label: "workflow", Count: 1}},
		Validation:      []HTMLLabelCount{{Label: "tests", Count: 2}},
	}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("aggregateHTMLLabels() = %#v, want %#v", labels, want)
	}
}

func TestCreateHTMLReportUsesPrivateTimestampedCollisionSafeDirectories(t *testing.T) {
	root := t.TempDir()
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 34, 56, 0, time.UTC) }
	first, err := createHTMLReport(root, now, "<html>first</html>")
	if err != nil {
		t.Fatal(err)
	}
	second, err := createHTMLReport(root, now, "<html>second</html>")
	if err != nil {
		t.Fatal(err)
	}
	if first.Dir == second.Dir || filepath.Dir(first.Path) != first.Dir || filepath.Dir(second.Path) != second.Dir {
		t.Fatalf("reports did not use distinct report directories: first=%q second=%q", first.Dir, second.Dir)
	}
	if !strings.Contains(first.Dir, filepath.Join("temp", "reports")) || !strings.Contains(second.Dir, filepath.Join("temp", "reports")) {
		t.Fatalf("reports were not persisted below temp/reports: %q %q", first.Dir, second.Dir)
	}
	for _, artifact := range []htmlReportArtifact{first, second} {
		info, err := os.Stat(artifact.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if mode := info.Mode().Perm(); mode != 0700 {
				t.Errorf("report directory mode = %o, want 0700", mode)
			}
		}
		info, err = os.Stat(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if mode := info.Mode().Perm(); mode != 0600 {
				t.Errorf("report file mode = %o, want 0600", mode)
			}
		}
	}
	content, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "<html>first</html>" {
		t.Fatalf("first report content = %q", content)
	}
}

func TestHTMLReportServerIsLoopbackOnlyAndStopsOnCancellation(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, func() time.Time { return time.Now().UTC() }, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var openedURL string
	server, err := startHTMLReportServer(ctx, artifact, func(_ context.Context, url string) error {
		openedURL = url
		return nil
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(server.URL, "http://127.0.0.1:") || openedURL != server.URL {
		t.Fatalf("server URL = %q, opened URL = %q", server.URL, openedURL)
	}
	for _, path := range []string{"/", "/index.html"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || string(body) != "<html>current</html>" {
			t.Fatalf("GET %s: status=%d body=%q", path, response.StatusCode, body)
		}
		if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("GET %s missing private response headers: %v", path, response.Header)
		}
	}
	response, err := http.Get(server.URL + "/other.txt")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /other.txt status=%d, want 404", response.StatusCode)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "spoofed.example"
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusMisdirectedRequest || strings.Contains(string(body), "<html>current</html>") {
		t.Fatalf("spoofed Host response: status=%d body=%q", response.StatusCode, body)
	}
	cancel()
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLShutdownWaitsForLateAcceptAdmissionBarrier(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, time.Now, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	underlying, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &delayedAcceptListener{Listener: underlying, accepted: make(chan struct{}, 1), release: make(chan struct{})}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := startHTMLReportServerWithListener(ctx, artifact, nil, io.Discard, func(string, string) (net.Listener, error) {
		return listener, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-listener.accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept the raw client connection")
	}
	cancel()
	waitResult := make(chan error, 1)
	go func() { waitResult <- server.Wait() }()
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not finish promptly after the accept-admission barrier")
	}
	assertHTMLLifecycleDone(t, server)
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("late raw client connection remained open after shutdown")
	}
}

func TestHTMLShutdownRetainsLateConnectionAndListenerCloseErrors(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, time.Now, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	underlying, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	connectionErr := errors.New("synthetic connection close failure")
	listenerErr := errors.New("synthetic listener close failure")
	listener := &delayedAcceptListener{
		Listener: underlying,
		accepted: make(chan struct{}, 1),
		release:  make(chan struct{}),
		closeErr: listenerErr,
		wrapConn: func(connection net.Conn) net.Conn { return sentinelCloseConn{Conn: connection, err: connectionErr} },
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := startHTMLReportServerWithListener(ctx, artifact, nil, io.Discard, func(string, string) (net.Listener, error) {
		return listener, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-listener.accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept the raw client connection")
	}
	cancel()
	result := make(chan error, 1)
	go func() { result <- server.Wait() }()
	select {
	case err := <-result:
		if !errors.Is(err, connectionErr) || !errors.Is(err, listenerErr) {
			t.Fatalf("Wait() error = %v, want connection and listener close failures", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not complete after joining late connection cleanup")
	}
	assertHTMLLifecycleDone(t, server)
}

func TestHTMLShutdownDrainsActiveRequest(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(response, "complete response")
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := syntheticHTMLServer(handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startHTMLServerLifecycle(ctx, server, listener)
	responseResult := make(chan struct {
		body string
		err  error
	}, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String() + "/")
		if requestErr != nil {
			responseResult <- struct {
				body string
				err  error
			}{err: requestErr}
			return
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		responseResult <- struct {
			body string
			err  error
		}{body: string(body), err: readErr}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("synthetic request handler did not start")
	}
	cancel()
	select {
	case <-server.serveDone:
	case <-time.After(time.Second):
		t.Fatal("Serve did not stop accepting after cancellation")
	}
	close(release)
	select {
	case response := <-responseResult:
		if response.err != nil || response.body != "complete response" {
			t.Fatalf("active request result = %q, %v", response.body, response.err)
		}
	case <-time.After(time.Second):
		t.Fatal("active request did not finish after release")
	}
	result := make(chan error, 1)
	go func() { result <- server.Wait() }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("graceful shutdown did not complete after active request drained")
	}
}

func TestHTMLServeFailureReturnsWithoutContextCancellation(t *testing.T) {
	serveErr := errors.New("synthetic accept failure")
	server := syntheticHTMLServer(http.NotFoundHandler())
	startHTMLServerLifecycle(context.Background(), server, failingAcceptListener{err: serveErr})
	result := make(chan error, 1)
	go func() { result <- server.Wait() }()
	select {
	case err := <-result:
		if !errors.Is(err, serveErr) {
			t.Fatalf("Wait() error = %v, want Serve failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait waited for parent cancellation after Serve failed")
	}
}

func TestHTMLServeFailureCancelsPendingOpenerAndReturnsServeError(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, time.Now, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := errors.New("synthetic early accept failure")
	listener := &pendingOpenerFailureListener{
		err: serveErr, acceptStarted: make(chan struct{}), closed: make(chan struct{}),
	}
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	openerStarted := make(chan struct{})
	openerCanceled := make(chan struct{})
	startupResult := make(chan struct {
		server *htmlReportServer
		err    error
	}, 1)
	go func() {
		server, startErr := startHTMLReportServerWithListener(parent, artifact, func(openerContext context.Context, _ string) error {
			close(openerStarted)
			<-openerContext.Done()
			close(openerCanceled)
			return fmt.Errorf("reap synthetic launcher: %w", openerContext.Err())
		}, io.Discard, func(string, string) (net.Listener, error) {
			return listener, nil
		})
		startupResult <- struct {
			server *htmlReportServer
			err    error
		}{server: server, err: startErr}
	}()
	for name, signal := range map[string]<-chan struct{}{"opener start": openerStarted, "Serve start": listener.acceptStarted} {
		select {
		case <-signal:
		case <-time.After(time.Second):
			cancelParent()
			t.Fatalf("timed out waiting for %s", name)
		}
	}
	var started struct {
		server *htmlReportServer
		err    error
	}
	select {
	case started = <-startupResult:
	case <-time.After(time.Second):
		cancelParent()
		started = <-startupResult
		if started.server != nil {
			_ = started.server.Wait()
		}
		t.Fatal("startup remained blocked after Serve failed while parent context was live")
	}
	if parent.Err() != nil {
		t.Fatalf("parent context was canceled: %v", parent.Err())
	}
	if started.server != nil || !errors.Is(started.err, serveErr) {
		t.Fatalf("server=%v startup error=%v, want Serve failure", started.server, started.err)
	}
	for name, signal := range map[string]<-chan struct{}{"opener reaped": openerCanceled, "listener closed": listener.closed} {
		select {
		case <-signal:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
}

func syntheticHTMLServer(handler http.Handler) *htmlReportServer {
	server := &htmlReportServer{
		server:         &http.Server{Handler: handler},
		serveDone:      make(chan struct{}),
		shutdownDone:   make(chan struct{}),
		newConnections: make(map[net.Conn]struct{}),
	}
	server.server.ConnState = func(connection net.Conn, state http.ConnState) {
		server.responseMu.Lock()
		defer server.responseMu.Unlock()
		if state == http.StateNew {
			server.newConnections[connection] = struct{}{}
		} else {
			delete(server.newConnections, connection)
		}
	}
	return server
}

func assertHTMLLifecycleDone(t *testing.T, server *htmlReportServer) {
	t.Helper()
	for label, done := range map[string]<-chan struct{}{"Serve": server.serveDone, "shutdown": server.shutdownDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Errorf("%s lifecycle was not joined", label)
		}
	}
}

func TestHTMLReportServerWarnsWhenBrowserOpenFails(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, func() time.Time { return time.Now().UTC() }, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	server, err := startHTMLReportServer(ctx, artifact, func(context.Context, string) error { return errors.New("browser unavailable") }, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Warning:") || !strings.Contains(output.String(), server.URL) {
		t.Fatalf("browser failure output = %q", output.String())
	}
	cancel()
	if err := server.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLReportServerServesWhileBrowserLauncherIsPending(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, time.Now, "<html>served before launcher exit</html>")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requestResult := make(chan error, 1)
	launcherRelease := make(chan struct{})
	startResult := make(chan struct {
		server *htmlReportServer
		err    error
	}, 1)
	go func() {
		server, startErr := startHTMLReportServer(ctx, artifact, func(_ context.Context, reportURL string) error {
			response, requestErr := http.Get(reportURL)
			if requestErr == nil {
				body, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if response.StatusCode != http.StatusOK || string(body) != "<html>served before launcher exit</html>" {
					requestErr = fmt.Errorf("HTTP response status=%d body=%q", response.StatusCode, body)
				}
				requestErr = errors.Join(requestErr, readErr, closeErr)
			}
			requestResult <- requestErr
			<-launcherRelease
			return requestErr
		}, io.Discard)
		startResult <- struct {
			server *htmlReportServer
			err    error
		}{server: server, err: startErr}
	}()
	select {
	case requestErr := <-requestResult:
		if requestErr != nil {
			close(launcherRelease)
			t.Fatalf("HTTP request while browser launcher was pending: %v", requestErr)
		}
	case <-time.After(time.Second):
		close(launcherRelease)
		t.Fatal("HTTP report was not served while browser launcher was pending")
	}
	close(launcherRelease)
	started := <-startResult
	if started.err != nil || started.server == nil {
		t.Fatalf("server=%v start error=%v", started.server, started.err)
	}
	cancel()
	if err := started.server.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLReportServerStartupOutputFailuresCloseListener(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, func() time.Time { return time.Now().UTC() }, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		opener htmlBrowserOpener
	}{
		{name: "report address"},
		{name: "browser warning", opener: func(context.Context, string) error { return errors.New("browser unavailable") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			tracked := &trackingListener{Listener: listener}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server, err := startHTMLReportServerWithListener(ctx, artifact, test.opener, failingProgressWriter{err: io.ErrClosedPipe}, func(string, string) (net.Listener, error) {
				return tracked, nil
			})
			if server != nil || !errors.Is(err, io.ErrClosedPipe) || !tracked.closed {
				t.Fatalf("server=%v error=%v listenerClosed=%v", server, err, tracked.closed)
			}
		})
	}
}

func TestHTMLReportServerWaitReturnsResponseWriteFailures(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, func() time.Time { return time.Now().UTC() }, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := startHTMLReportServer(ctx, artifact, nil, io.Discard)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, server.URL+"/", nil)
	request.Host = strings.TrimPrefix(server.URL, "http://")
	response := &failingHTTPResponseWriter{err: io.ErrClosedPipe}
	server.server.Handler.ServeHTTP(response, request)
	cancel()
	if err := server.Wait(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Wait() error = %v, want response writer failure", err)
	}
}

func TestHTMLShutdownContextRetainsValuesAfterCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), htmlContextKey{}, "retained"))
	cancel()
	shutdown, stop := newHTMLShutdownContext(parent)
	defer stop()
	if shutdown.Value(htmlContextKey{}) != "retained" || shutdown.Err() != nil {
		t.Fatalf("shutdown context value/error = %v/%v", shutdown.Value(htmlContextKey{}), shutdown.Err())
	}
}

func TestHTMLBrowserCommandSelection(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "darwin", name: "open", args: []string{"http://127.0.0.1:54321/"}},
		{goos: "linux", name: "xdg-open", args: []string{"http://127.0.0.1:54321/"}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "http://127.0.0.1:54321/"}},
	}
	for _, test := range tests {
		name, args, err := htmlBrowserCommand(test.goos, test.args[len(test.args)-1])
		if err != nil {
			t.Fatalf("htmlBrowserCommand(%q): %v", test.goos, err)
		}
		if name != test.name || !reflect.DeepEqual(args, test.args) {
			t.Errorf("htmlBrowserCommand(%q) = %q %#v, want %q %#v", test.goos, name, args, test.name, test.args)
		}
	}
	if _, _, err := htmlBrowserCommand("plan9", "http://127.0.0.1:54321/"); !errors.Is(err, errUnsupportedBrowserOS) {
		t.Error("unsupported OS did not return an error")
	}
}

func TestHTMLBrowserRejectsUnsafeURLsAndHonorsCanceledContext(t *testing.T) {
	for _, raw := range []string{
		"https://127.0.0.1:54321/",
		"http://example.com:54321/",
		"http://127.0.0.1/",
		"http://127.0.0.1:54321/private",
		"http://user@127.0.0.1:54321/",
		"http://127.0.0.1:54321/?secret=1",
		"http://127.0.0.1:54321/#fragment",
	} {
		if err := validateHTMLBrowserURL(raw); !errors.Is(err, errInvalidBrowserURL) {
			t.Errorf("validateHTMLBrowserURL(%q) error = %v", raw, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := openHTMLBrowser(ctx, "http://127.0.0.1:54321/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled browser launch error = %v", err)
	}
}

func TestHTMLReportInputNoDataPathContainsOnlyAggregates(t *testing.T) {
	tr, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	input := buildHTMLReportInput(tr, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), HTMLReportCounts{UserSessions: 2, Tasks: 3}, analyze.SemanticStats{}, analyze.EffectivenessAnalysis{}, semanticReportResults{})
	if input.Counts.Tasks != 3 || input.Counts.UserSessions != 2 || input.WindowStart.IsZero() || input.WindowEnd.IsZero() {
		t.Fatalf("no-data input lost aggregate window/counts: %#v", input)
	}
	if len(input.Labels.Steering) != 0 || len(input.Labels.Reasons) != 0 {
		t.Fatalf("no-data input unexpectedly contains labels: %#v", input.Labels)
	}
}

func TestAnalyzeHTMLFlagDefaultsFalseAndNoHTMLPathDoesNotLaunch(t *testing.T) {
	fs := newAnalyzeFlagSet()
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if fs.Lookup("html") == nil || fs.Lookup("html").DefValue != "false" {
		t.Fatal("analyze --html is not registered with false default")
	}
	launched := false
	if err := launchHTMLReportIfRequested(false, func() error {
		launched = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if launched {
		t.Fatal("no-html path launched report")
	}
}
