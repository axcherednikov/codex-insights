package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/fileio"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

type htmlReportArtifact struct {
	Dir  string
	Path string
}

const (
	privateDirectoryMode  = 0700
	minimumHTMLPort       = 1
	maximumHTMLPort       = 65535
	htmlReadHeaderTimeout = 5 * time.Second
)

// aggregateHTMLLabels converts semantic result records into deterministic
// counts. The result deliberately contains no turn IDs or conversation text.
func aggregateHTMLLabels(results semanticReportResults) HTMLReportLabels {
	return HTMLReportLabels{
		Steering:        aggregateHTMLLabelValues(len(results.steering), func(index int) string { return results.steering[index].Label }),
		Reasons:         aggregateHTMLLabelValues(len(results.reasons), func(index int) string { return results.reasons[index].Reason }),
		PromptQuality:   aggregateHTMLLabelValues(len(results.promptQuality), func(index int) string { return results.promptQuality[index].Issue }),
		AgentsRules:     aggregateHTMLLabelValues(len(results.agentsRules), func(index int) string { return results.agentsRules[index].Rule }),
		SkillCandidates: aggregateHTMLLabelValues(len(results.skillCandidates), func(index int) string { return results.skillCandidates[index].Category }),
		Validation:      aggregateHTMLLabelValues(len(results.validation), func(index int) string { return results.validation[index].ValidationType }),
	}
}

func aggregateHTMLLabelValues(length int, labelAt func(int) string) []HTMLLabelCount {
	counts := make(map[string]int, length)
	for index := 0; index < length; index++ {
		label := strings.TrimSpace(labelAt(index))
		if label != "" {
			counts[label]++
		}
	}
	result := make([]HTMLLabelCount, 0, len(counts))
	for label, count := range counts {
		result = append(result, HTMLLabelCount{Label: label, Count: count})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Count != result[right].Count {
			return result[left].Count > result[right].Count
		}

		return result[left].Label < result[right].Label
	})

	return result
}

func buildHTMLReportInput(
	translator i18n.Translator,
	windowStart time.Time,
	windowEnd time.Time,
	counts HTMLReportCounts,
	semanticStats analyze.SemanticStats,
	effectiveness analyze.EffectivenessAnalysis,
	results semanticReportResults,
) HTMLReportInput {
	return HTMLReportInput{
		Translator:    translator,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		Counts:        counts,
		Semantic:      semanticStats,
		Effectiveness: effectiveness,
		Labels:        aggregateHTMLLabels(results),
	}
}

func createHTMLReport(baseDir string, now func() time.Time, content string) (htmlReportArtifact, error) {
	if now == nil {
		now = time.Now
	}
	reportsDir := filepath.Join(baseDir, "temp", "reports")
	if err := os.MkdirAll(reportsDir, privateDirectoryMode); err != nil {
		return htmlReportArtifact{}, fmt.Errorf("create HTML reports directory: %w", err)
	}
	stamp := now().UTC().Format("20060102T150405.000000000Z")
	for suffix := 0; ; suffix++ {
		name := stamp
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d", stamp, suffix)
		}
		directory := filepath.Join(reportsDir, name)
		if err := os.Mkdir(directory, privateDirectoryMode); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}

			return htmlReportArtifact{}, fmt.Errorf("create HTML report directory: %w", err)
		}
		path := filepath.Join(directory, "index.html")
		if err := fileio.WriteExclusive(path, []byte(content)); err != nil {
			writeErr := fmt.Errorf("create private HTML report file: %w", err)
			cleanupErr := os.RemoveAll(directory)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("remove incomplete HTML report directory: %w", cleanupErr)
			}

			return htmlReportArtifact{}, errors.Join(writeErr, cleanupErr)
		}

		return htmlReportArtifact{Dir: directory, Path: path}, nil
	}
}

const htmlShutdownTimeout = 5 * time.Second

type htmlReportServer struct {
	URL            string
	Path           string
	server         *http.Server
	serveDone      chan struct{}
	shutdownDone   chan struct{}
	responseMu     sync.Mutex
	serveErr       error
	shutdownErr    error
	responseErr    error
	newConnections map[net.Conn]struct{}
}

func startHTMLReportServer(ctx context.Context, artifact htmlReportArtifact, opener htmlBrowserOpener, output io.Writer) (*htmlReportServer, error) {
	return startHTMLReportServerWithListener(ctx, artifact, opener, output, net.Listen)
}

type htmlListenFunc func(network, address string) (net.Listener, error)
type htmlBrowserOpener func(context.Context, string) error

func startHTMLReportServerWithListener(ctx context.Context, artifact htmlReportArtifact, opener htmlBrowserOpener, output io.Writer, listen htmlListenFunc) (*htmlReportServer, error) {
	if ctx == nil {
		return nil, errNilHTMLContext
	}
	content, listener, reportURL, expectedHost, err := prepareHTMLListener(artifact, listen)
	if err != nil {
		return nil, err
	}
	var lifecycle *htmlReportServer
	server := newHTMLReportServer(reportURL, artifact.Path, expectedHost, content, &lifecycle)
	lifecycle = server
	if err := writeHTMLServerStartup(output, server); err != nil {
		return nil, errors.Join(err, closeHTMLListener(listener))
	}
	serverContext, cancelServer := context.WithCancel(ctx)
	startHTMLServerLifecycle(serverContext, server, listener)
	go func() {
		<-server.shutdownDone
		cancelServer()
	}()
	if err := writeHTMLBrowserWarning(serverContext, output, server, opener); err != nil {
		cancelServer()

		return nil, errors.Join(err, server.Wait())
	}
	select {
	case <-server.serveDone:
		if err := server.Wait(); err != nil {
			return nil, err
		}
	default:
	}

	return server, nil
}

func prepareHTMLListener(artifact htmlReportArtifact, listen htmlListenFunc) ([]byte, net.Listener, string, string, error) {
	content, err := fileio.ReadFile(artifact.Path)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("read current HTML report: %w", err)
	}
	listener, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("listen for HTML report: %w", err)
	}
	expectedHost := listener.Addr().String()
	reportURL := "http://" + expectedHost
	if err := validateHTMLBrowserURL(reportURL); err != nil {
		return nil, nil, "", "", errors.Join(fmt.Errorf("validate HTML report listener URL: %w", err), closeHTMLListener(listener))
	}

	return content, listener, reportURL, expectedHost, nil
}

func newHTMLReportServer(reportURL, path, expectedHost string, content []byte, lifecycle **htmlReportServer) *htmlReportServer {
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Host != expectedHost {
			response.WriteHeader(http.StatusMisdirectedRequest)
			(*lifecycle).writeResponse(response, "request Host does not match the local report listener\n")

			return
		}
		if request.URL.Path != "/" && request.URL.Path != "/index.html" {
			response.Header().Set("Content-Type", "text/plain; charset=utf-8")
			response.WriteHeader(http.StatusNotFound)
			(*lifecycle).writeResponse(response, "404 page not found\n")

			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		(*lifecycle).writeResponse(response, string(content))
	})
	result := &htmlReportServer{
		URL: reportURL, Path: path,
		server:    &http.Server{Handler: handler, ReadHeaderTimeout: htmlReadHeaderTimeout},
		serveDone: make(chan struct{}), shutdownDone: make(chan struct{}),
		newConnections: make(map[net.Conn]struct{}),
	}
	result.server.ConnState = func(connection net.Conn, state http.ConnState) {
		result.responseMu.Lock()
		defer result.responseMu.Unlock()
		if state == http.StateNew {
			result.newConnections[connection] = struct{}{}
		} else {
			delete(result.newConnections, connection)
		}
	}

	return result
}

func writeHTMLServerStartup(output io.Writer, server *htmlReportServer) error {
	reportLines := fmt.Sprintf("HTML report: %s\nReport path: %s\n", server.URL, server.Path)

	return writeHTMLStartup(output, reportLines, "write HTML report address")
}

func writeHTMLBrowserWarning(ctx context.Context, output io.Writer, server *htmlReportServer, opener htmlBrowserOpener) error {
	if opener == nil {
		return nil
	}
	if err := opener(ctx, server.URL); err != nil {
		warning := fmt.Sprintf("Warning: could not open the report in a browser: %v. Open %s manually.\n", err, server.URL)
		if writeErr := writeHTMLStartup(output, warning, "write browser warning"); writeErr != nil {
			return errors.Join(fmt.Errorf("open HTML report in browser: %w", err), writeErr)
		}
	}

	return nil
}

func startHTMLServerLifecycle(ctx context.Context, server *htmlReportServer, listener net.Listener) {
	go func() {
		err := server.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.responseMu.Lock()
		if err != nil {
			server.serveErr = fmt.Errorf("serve HTML report: %w", err)
		}
		server.responseMu.Unlock()
		close(server.serveDone)
	}()
	go func() {
		select {
		case <-server.serveDone:
		case <-ctx.Done():
			shutdownContext, cancel := newHTMLShutdownContext(ctx)
			shutdownResult := make(chan error, 1)
			go func() {
				shutdownResult <- server.server.Shutdown(shutdownContext)
			}()
			<-server.serveDone
			closeErr := closeUnstartedHTMLConnections(server)
			shutdownErr := <-shutdownResult
			cancel()
			if closeErr != nil || shutdownErr != nil {
				server.responseMu.Lock()
				if shutdownErr != nil {
					shutdownErr = fmt.Errorf("shut down HTML report server: %w", shutdownErr)
				}
				server.shutdownErr = errors.Join(closeErr, shutdownErr)
				server.responseMu.Unlock()
			}
		}
		close(server.shutdownDone)
	}()
}

func closeUnstartedHTMLConnections(server *htmlReportServer) error {
	server.responseMu.Lock()
	connections := make([]net.Conn, 0, len(server.newConnections))
	for connection := range server.newConnections {
		connections = append(connections, connection)
		delete(server.newConnections, connection)
	}
	server.responseMu.Unlock()
	var closeErr error
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close unstarted HTML report connection: %w", err))
		}
	}

	return closeErr
}

func newHTMLShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), htmlShutdownTimeout)
}

func (server *htmlReportServer) Wait() error {
	<-server.serveDone
	<-server.shutdownDone
	server.responseMu.Lock()
	defer server.responseMu.Unlock()

	return errors.Join(server.serveErr, server.shutdownErr, server.responseErr)
}

func (server *htmlReportServer) writeResponse(response http.ResponseWriter, text string) {
	written, err := io.WriteString(response, text)
	if err == nil && written != len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		server.responseMu.Lock()
		server.responseErr = errors.Join(server.responseErr, fmt.Errorf("write HTML report response: %w", err))
		server.responseMu.Unlock()
	}
}

func writeHTMLStartup(output io.Writer, text, operation string) error {
	written, err := io.WriteString(output, text)
	if err == nil && written != len(text) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func closeHTMLListener(listener net.Listener) error {
	if err := listener.Close(); err != nil {
		return fmt.Errorf("close HTML report listener: %w", err)
	}

	return nil
}

func htmlBrowserCommand(goos, url string) (string, []string, error) {
	if err := validateHTMLBrowserURL(url); err != nil {
		return "", nil, err
	}
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operating system %q for browser auto-open: %w", goos, errUnsupportedBrowserOS)
	}
}

func validateHTMLBrowserURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse browser report URL: %w", err)
	}
	if !isAllowedHTMLURL(parsed, raw) {
		return errInvalidBrowserURL
	}
	address := net.ParseIP(parsed.Hostname())
	if address == nil || !address.IsLoopback() {
		return errInvalidBrowserURL
	}

	return validateHTMLPort(parsed.Port())
}

func isAllowedHTMLURL(parsed *url.URL, raw string) bool {
	return parsed.Scheme == "http" && parsed.Opaque == "" && parsed.User == nil &&
		parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && !strings.Contains(raw, "#") &&
		isAllowedHTMLPath(parsed.Path) && parsed.Port() != ""
}

func isAllowedHTMLPath(path string) bool {
	return path == "" || path == "/" || path == "/index.html"
}

func validateHTMLPort(raw string) error {
	port, err := strconv.Atoi(raw)
	if err != nil || port < minimumHTMLPort || port > maximumHTMLPort {
		return errInvalidBrowserURL
	}

	return nil
}

func openHTMLBrowser(ctx context.Context, reportURL string) error {
	if ctx == nil {
		return errNilHTMLContext
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("browser launch canceled: %w", err)
	}
	if err := validateHTMLBrowserURL(reportURL); err != nil {
		return err
	}
	browserName, arguments, err := htmlBrowserCommand(runtime.GOOS, reportURL)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open")
		command.Args = append([]string{browserName}, arguments...)
	case "linux":
		command = exec.CommandContext(ctx, "xdg-open")
		command.Args = append([]string{browserName}, arguments...)
	case "windows":
		command = exec.CommandContext(ctx, "rundll32")
		command.Args = append([]string{browserName}, arguments...)
	default:
		return fmt.Errorf("unsupported operating system %q for browser auto-open: %w", runtime.GOOS, errUnsupportedBrowserOS)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start browser command: %w", err)
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf("wait for browser command: %w", err)
	}

	return nil
}

func launchHTMLReportIfRequested(enabled bool, launch func() error) error {
	if !enabled {
		return nil
	}

	return launch()
}

type analyzeFlagSet struct {
	*flag.FlagSet
	html *bool
}

func newAnalyzeFlagSet() *analyzeFlagSet {
	flags := flag.NewFlagSet("analyze", flag.ExitOnError)

	return &analyzeFlagSet{FlagSet: flags, html: flags.Bool("html", false, "generate and serve a local HTML report")}
}

func serveCurrentHTMLReport(
	translator i18n.Translator,
	windowStart time.Time,
	windowEnd time.Time,
	counts HTMLReportCounts,
	semanticStats analyze.SemanticStats,
	effectiveness analyze.EffectivenessAnalysis,
	results semanticReportResults,
) error {
	html, err := RenderHTMLReport(buildHTMLReportInput(translator, windowStart, windowEnd, counts, semanticStats, effectiveness, results))
	if err != nil {
		return fmt.Errorf("render HTML report: %w", err)
	}
	artifact, err := createHTMLReport(".", time.Now, html)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server, err := startHTMLReportServer(ctx, artifact, func(browserContext context.Context, reportURL string) error {
		return openHTMLBrowser(browserContext, reportURL)
	}, os.Stdout)
	if err != nil {
		return err
	}
	if err := server.Wait(); err != nil {
		return fmt.Errorf("serve HTML report: %w", err)
	}

	return nil
}
