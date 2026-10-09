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
- Separate subagent activity: created agents, working turns, lifecycle statuses,
  nesting, logged roles, models, and reasoning settings.
- Subagent token accounting by model and role, with recorded usage and legacy
  estimates shown separately, plus summed working time.
- Deeper Judge-backed analysis with actionable recommendations.
- Subagent comparisons by task type, logged role and model, with sample sizes,
  resource coverage, and explicit statistical limitations in console and HTML
  reports.
- English reports by default, with Russian or locale-based selection available
  through `--lang`.
- Configurable analysis windows and session locations.
- Local semantic cache that stores labels rather than raw conversations.
- Privacy-conscious golden fixture export and regression evaluation.

## Privacy

Codex session history can contain source code, prompts, answers, credentials,
and other sensitive information.

- `quick` reads session files locally and does not send conversation content to
  a model.
- `analyze` sends bounded excerpts of selected task prompts, final answers, and
  follow-up messages to the Judge configured by this project through
  `codex exec`. Judge runs use `--ephemeral`, so the Codex CLI does not persist
  their session rollout files.
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
  `PATH`, authenticated with access to the configured Judge model, and recent
  enough to support `codex exec --ephemeral`.

## Installation

### Go install

```bash
go install github.com/axcherednikov/codex-insights/cmd/codex-insights@latest
```

Go installs the executable into `GOBIN`, or into `$(go env GOPATH)/bin` when
`GOBIN` is not configured. Add the default directory to your current shell and
verify the installation:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
codex-insights --version
```

To keep the command available in new terminals, add the same `export` line to
your shell profile, normally `~/.zshrc` on macOS or `~/.bashrc` on Linux.

### Codex CLI for semantic analysis

The `analyze` and `golden evaluate` commands invoke `codex exec`, so they also
require the Codex CLI to be installed, available on `PATH`, and authenticated.
On macOS, install it with Homebrew:

```bash
brew install --cask codex
```

Alternatively, use the official standalone installer on macOS or Linux:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

Then start Codex once and complete sign-in:

```bash
codex
```

See the [official Codex CLI documentation](https://developers.openai.com/codex/cli)
for Windows, npm, update, and authentication instructions.

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

The examples below are synthetic. Model names and semantic labels are
illustrative; they are not measurements of model quality or agent benefit.

```text
Codex Insights Quick
====================
Period: last 30 days — 2026-10-09T00:00:00Z
User sessions: 1
Tasks: 21

Status:
  complete         21

Completed task averages:
  Tokens:                1357
  Duration:              6.0 sec
  Tool calls:            2.0

Subagents:
  Created: 20
  Working turns: 20
  Completed: 20
  Aborted: 0
  Incomplete: 0
  Nested created: 0
  Roles (created / working turns):
    reviewer: 10 / 10
    worker: 10 / 10
  Tokens (token_usage_record): 7400 (20 working turns)
  Estimated tokens (token_count): n/a (0 working turns)
  Tokens by logged role (token_usage_record / token_count):
    reviewer: 1400 / n/a
    worker: 6000 / n/a
  Tokens by observed model (unknown = attribution unavailable) (token_usage_record / token_count):
    model-b: 6000 / n/a
    model-c: 1400 / n/a
  Recorded working time: 60.000 sec (20 working turns)
```

This is an excerpt: model settings, token components, and explanatory notes are
omitted. The same synthetic data produces these `analyze` cohort headings;
resource rows are omitted here:

```text
Subagent effectiveness:
  All task types / With subagents — samples=10, steering=20.0%
  All task types / No observed subagents — samples=10, steering=30.0%
  Bug fix / With subagents — samples=10, steering=20.0%
  Bug fix / No observed subagents — samples=10, steering=30.0%
  Bug fix / Model model-b — samples=10, steering=20.0%
  Bug fix / Model model-c — samples=10, steering=20.0%
  Bug fix / Role reviewer — samples=10, steering=20.0%
  Bug fix / Role worker — samples=10, steering=20.0%
```

Both role cohorts contain the same ten user tasks. Their percentages are not
independent evidence about either role. See [Subagent reports](docs/subagent-reports.md)
for the full resource rows and accounting definitions.

## Commands

### `quick`

Produces a local summary of sessions, tasks, completion statuses, token use,
duration, tool calls, and model/reasoning combinations.

The separate Subagents section counts unique created agents and their working
turns, including repeat assignments and nested agents. It shows logged roles
and execution settings, token use by model and role, and summed working time.
Child sessions never become extra user tasks. Role names are read from Codex
records; no plugin installation or fixed role-name list is required. Lifecycle
statuses and working time in `quick` are totals across agents, while role rows
show created/working counts and token use.

```text
codex-insights quick [options]
```

Common options:

| Option | Default | Description |
|---|---:|---|
| `--days` | `30` | Number of days to analyze; `0` means all history. |
| `--before` | now | Analyze state before an RFC3339 timestamp. |
| `--sessions` | `~/.codex/sessions` | Session history directory. |
| `--lang` | `en` | Report language: `auto`, `en`, or `ru`. |

### `analyze`

Runs the local aggregation plus semantic classification through the Codex CLI.
The command displays a privacy notice before its first uncached Judge call.

Subagent effectiveness uses the same semantic task-type and follow-up labels as
the existing report, without an additional Judge pass. It compares user tasks
with and without observed subagents, then groups observations by task type,
logged subagent role/model, and matching parent model/reasoning settings.
Rows show sample size, user steering, and resource averages with known/missing
coverage. Parent resources and agent resources remain separate.

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

### Subagent data and comparison limits

- `token_usage_record` is preferred when request identity and ownership are
  confirmed. Repeated requests and inherited fork history are excluded.
  `token_count` differences are a separately labelled fallback estimate; the
  two sources are not silently added or substituted when records conflict.
- Cached input is already included in input tokens; reasoning output is
  already included in output tokens. Neither subset is added again. No
  monetary cost is calculated.
- Working time sums valid `task_complete.duration_ms` values, including
  parallel work. It is not user-task elapsed time. In `analyze`, agent token,
  time, and tool values are average sums per observed user task; parent elapsed
  time is reported separately.
- Missing values are `n/a`, not zero. Means use tasks with observed values;
  incomplete coverage gives partial totals. Counter disagreements and
  unverified attribution are disclosed. Older sessions remain readable, but
  missing roles, request ownership, or `root_turn_id` limit agent comparisons.
- Historical guardian service sessions are excluded from ordinary subagents.
  Unknown roles are not inferred from prompts, task names, or plugin settings.
- Comparisons use completed user tasks with an existing follow-up label.
  No follow-up or no steering does not prove success. At least ten observations
  per matched cohort are needed to remove the small-sample warning; this is
  a reporting threshold, not statistical significance.
- Task-type and parent-routing groups reduce some differences between cohorts,
  but actual task difficulty is unavailable. Associations do not prove that
  agents help or hurt. Role/model cohorts can overlap, and the outcome of a
  shared task cannot be attributed to one participant.

Existing user-task counts, session filtering, semantic cache format, and Judge
methodology are retained. A history without attributable child data still
produces the existing user report and reports the limits of agent evidence.

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
go test -count=1 ./...
golangci-lint run ./...
go build ./cmd/codex-insights
```

Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.
Security vulnerabilities should be reported according to
[SECURITY.md](SECURITY.md), not through a public issue.

## Changelog

Release history is available in [CHANGELOG.md](CHANGELOG.md).

## License

Codex Insights is released under the [MIT License](LICENSE).
