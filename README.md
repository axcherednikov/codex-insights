# Codex Insights

[![CI](https://github.com/axcherednikov/codex-insights/actions/workflows/ci.yml/badge.svg)](https://github.com/axcherednikov/codex-insights/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/axcherednikov/codex-insights)](https://github.com/axcherednikov/codex-insights/releases)
[![License](https://img.shields.io/github/license/axcherednikov/codex-insights)](LICENSE)

Codex Insights is a command-line tool for understanding how you use Codex. It
reads local Codex session history, summarizes workload and model usage, and can
run a deeper semantic analysis to identify steering, validation gaps, prompt
quality issues, and opportunities for better project instructions or reusable
skills.

## Features

- Fast, local-only usage summary with `quick`.
- Deeper Judge-backed analysis with actionable recommendations.
- English and Russian reports, selected automatically or with `--lang`.
- Configurable analysis windows and session locations.
- Local semantic cache that stores labels rather than raw conversations.
- Privacy-conscious golden fixture export and regression evaluation.

## Privacy

Codex session history can contain source code, prompts, answers, credentials,
and other sensitive information.

- `quick` reads session files locally and does not send conversation content to
  a model.
- `analyze` sends selected task prompts, final answers, and follow-up messages
  to the Judge configured by this project through `codex exec`.
- `golden export` writes minimized, automatically redacted data locally and
  does not call the Judge. Automatic redaction is not a guarantee of secrecy;
  review every fixture before sharing or committing it.
- The persistent semantic cache contains classifications and confidence values,
  not raw conversation text.
- On Unix-like systems, exported fixtures and cache files are restricted to the
  current user with mode `0600`. Windows uses the ACL inherited from the chosen
  directory, so export fixtures only to a private location.

Review the code and your organization's data-handling requirements before using
Judge-backed commands with sensitive session history.

## Requirements

- macOS, Linux, or Windows.
- Codex session history, normally stored in `~/.codex/sessions`.
- Go 1.27 or newer when installing or building from source.
- For `analyze` and `golden evaluate`: the `codex` CLI installed, available on
  `PATH`, and authenticated with access to the configured Judge model.

## Installation

### Go install

```bash
go install github.com/axcherednikov/codex-insights/cmd/codex-insights@latest
```

Make sure the Go binary directory (usually `$(go env GOPATH)/bin`) is on your
`PATH`.

### Prebuilt binaries

Download the archive for your operating system and architecture from
[GitHub Releases](https://github.com/axcherednikov/codex-insights/releases),
extract it, and move `codex-insights` (or `codex-insights.exe` on Windows) to a
directory on your `PATH`. Verify the archive with the published
`checksums.txt` before running it.

### From source

```bash
git clone https://github.com/axcherednikov/codex-insights.git
cd codex-insights
go test ./...
go build -o codex-insights ./cmd/codex-insights
```

## Quick start

Run the local report for the last 30 days:

```bash
codex-insights quick
```

Running `codex-insights` without a subcommand is equivalent to
`codex-insights quick`.

Analyze all available history locally:

```bash
codex-insights quick --days 0
```

Run the deeper Judge-backed report:

```bash
codex-insights analyze --days 30
```

Generate the deeper report as a local HTML page:

```bash
codex-insights analyze --days 30 --html
```

With `--html`, the console report is printed first, then a standalone HTML
report is written below `temp/reports/<UTC-date-time>/index.html` and served
from a loopback-only URL. The tool attempts to open that URL in the default
browser; if it cannot, the URL remains usable and is printed for manual use.
The server stays running until you press Ctrl+C. Existing report directories
are retained and `temp/` is ignored by Git.

Select Russian output explicitly:

```bash
codex-insights quick --lang ru
```

Use a non-default session directory:

```bash
codex-insights quick --sessions /path/to/codex/sessions
```

Verify the installed version:

```bash
codex-insights --version
```

## Example output

The values below are synthetic and do not contain session data:

```text
Codex Insights Quick
====================
Period: last 30 days — 2026-09-17T09:00:00Z
User sessions: 42
Tasks: 118

Status:
  Complete         112
  Aborted            4
  Incomplete         2

Completed averages:
  Tokens:                184320
  Duration:              96.4 s
  Tool calls:            12.8
```

## Commands

### `quick`

Produces a local summary of sessions, tasks, completion statuses, token use,
duration, tool calls, and model/reasoning combinations.

```text
codex-insights quick [options]
```

Common options:

| Option | Default | Description |
|---|---:|---|
| `--days` | `30` | Number of days to analyze; `0` means all history. |
| `--before` | now | Analyze state before an RFC3339 timestamp. |
| `--sessions` | `~/.codex/sessions` | Session history directory. |
| `--lang` | `auto` | Report language: `auto`, `en`, or `ru`. |

### `analyze`

Runs the local aggregation plus semantic classification through the Codex CLI.
The command displays a privacy notice before its first uncached Judge call.

```text
codex-insights analyze [options]
```

In addition to the common options, `analyze` supports:

| Option | Default | Description |
|---|---:|---|
| `--concurrency` | `6` | Concurrent Judge workers, from 1 to 32. |
| `--cache` | platform default | Custom semantic cache file. |
| `--verbose` | `false` | Show timings and methodology metadata. |
| `--html` | `false` | Write and serve a local HTML report after the console report. |

The HTML report is local-only: it serves over loopback HTTP on `127.0.0.1`,
makes no external network requests, loads no external assets, and contains
aggregate counts, semantic labels, and synthetic examples only. It does not
include prompts, answers, or session text. Report files are created with
restrictive permissions on Unix-like systems and should still be treated as
local analysis output.

### Golden evaluation

Golden fixtures support human-reviewed regression testing of semantic
classifications:

```bash
codex-insights golden export --output candidate.json
codex-insights golden evaluate --fixture approved.json
```

See [Golden semantic evaluation](docs/golden-evaluation.md) for the review,
approval, privacy, and evaluation workflow.

### Version

```text
codex-insights --version
```

Release binaries report their semantic version. Locally built development
binaries report `dev` unless a version is supplied at link time.

## Development

```bash
gofmt -w path/to/changed.go
go vet ./...
go test ./...
go build ./cmd/codex-insights
```

Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.
Security vulnerabilities should be reported according to
[SECURITY.md](SECURITY.md), not through a public issue.

## Changelog

Release history is available in [CHANGELOG.md](CHANGELOG.md).

## License

Codex Insights is released under the [MIT License](LICENSE).
