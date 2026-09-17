package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

type htmlReportArtifact struct {
	Dir  string
	Path string
}

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
	if err := os.MkdirAll(reportsDir, 0700); err != nil {
		return htmlReportArtifact{}, fmt.Errorf("create HTML reports directory: %w", err)
	}
	stamp := now().UTC().Format("20060102T150405.000000000Z")
	for suffix := 0; ; suffix++ {
		name := stamp
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d", stamp, suffix)
		}
		directory := filepath.Join(reportsDir, name)
		if err := os.Mkdir(directory, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return htmlReportArtifact{}, fmt.Errorf("create HTML report directory: %w", err)
		}
		path := filepath.Join(directory, "index.html")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return htmlReportArtifact{}, fmt.Errorf("create HTML report file: %w", err)
		}
		if _, err := io.WriteString(file, content); err != nil {
			_ = file.Close()
			return htmlReportArtifact{}, fmt.Errorf("write HTML report file: %w", err)
		}
		if err := file.Close(); err != nil {
			return htmlReportArtifact{}, fmt.Errorf("close HTML report file: %w", err)
		}
		return htmlReportArtifact{Dir: directory, Path: path}, nil
	}
}

type htmlReportServer struct {
	URL    string
	Path   string
	server *http.Server
	done   chan error
}

func startHTMLReportServer(ctx context.Context, artifact htmlReportArtifact, opener func(string) error, output io.Writer) (*htmlReportServer, error) {
	return startHTMLReportServerWithListener(ctx, artifact, opener, output, net.Listen)
}

type htmlListenFunc func(network, address string) (net.Listener, error)

func startHTMLReportServerWithListener(ctx context.Context, artifact htmlReportArtifact, opener func(string) error, output io.Writer, listen htmlListenFunc) (*htmlReportServer, error) {
	content, err := os.ReadFile(artifact.Path)
	if err != nil {
		return nil, fmt.Errorf("read current HTML report: %w", err)
	}
	listener, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for HTML report: %w", err)
	}
	expectedHost := listener.Addr().String()
	url := "http://" + expectedHost
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Host != expectedHost {
			response.WriteHeader(http.StatusMisdirectedRequest)
			_, _ = io.WriteString(response, "request Host does not match the local report listener\n")
			return
		}
		if request.URL.Path != "/" && request.URL.Path != "/index.html" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(content)
	})
	server := &htmlReportServer{
		URL:  url,
		Path: artifact.Path,
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		done: make(chan error, 1),
	}
	fmt.Fprintf(output, "HTML report: %s\nReport path: %s\n", server.URL, server.Path)
	if opener != nil {
		if err := opener(server.URL); err != nil {
			fmt.Fprintf(output, "Warning: could not open the report in a browser: %v. Open %s manually.\n", err, server.URL)
		}
	}
	go func() {
		err := server.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.done <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.server.Shutdown(shutdownContext)
	}()
	return server, nil
}

func (server *htmlReportServer) Wait() error {
	return <-server.done
}

func htmlBrowserCommand(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operating system %q for browser auto-open", goos)
	}
}

func openHTMLBrowser(url string) error {
	name, args, err := htmlBrowserCommand(runtime.GOOS, url)
	if err != nil {
		return err
	}
	return exec.Command(name, args...).Start()
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
	server, err := startHTMLReportServer(ctx, artifact, openHTMLBrowser, os.Stdout)
	if err != nil {
		return err
	}
	if err := server.Wait(); err != nil {
		return fmt.Errorf("serve HTML report: %w", err)
	}
	return nil
}
