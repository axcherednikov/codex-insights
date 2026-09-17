package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

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
	server, err := startHTMLReportServer(ctx, artifact, func(url string) error {
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

func TestHTMLReportServerWarnsWhenBrowserOpenFails(t *testing.T) {
	root := t.TempDir()
	artifact, err := createHTMLReport(root, func() time.Time { return time.Now().UTC() }, "<html>current</html>")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	server, err := startHTMLReportServer(ctx, artifact, func(string) error { return errors.New("browser unavailable") }, &output)
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

func TestHTMLBrowserCommandSelection(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "darwin", name: "open", args: []string{"https://example.test/report"}},
		{goos: "linux", name: "xdg-open", args: []string{"https://example.test/report"}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "https://example.test/report"}},
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
	if _, _, err := htmlBrowserCommand("plan9", "https://example.test/report"); err == nil {
		t.Error("unsupported OS did not return an error")
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
