# Codex Insights project context

## Purpose and stack

Codex Insights is a CLI that reads local Codex rollout history and reports usage,
task outcomes, steering, validation, and instruction/skill opportunities. `quick`
is local-only; `analyze` and `golden evaluate` use a Judge through `codex exec`.
The CLI also exports reviewed golden fixtures and serves local HTML reports.
Sources: [README](../README.md), [CLI entry point](../cmd/codex-insights/main.go).

The module is `github.com/axcherednikov/codex-insights`, with Go `1.27.0`
declared in [go.mod](../go.mod). There are no third-party Go dependencies.
Judge-backed commands require an installed and authenticated Codex CLI.
Supported platforms are macOS, Linux, and Windows; CI tests all three.
Sources: [CI](../.github/workflows/ci.yml), [contributing](../CONTRIBUTING.md).

## Module and domain map

| Location | Responsibility and relevant checks |
|---|---|
| `cmd/codex-insights/` | Subcommands, flags, session selection, reports, HTML serving, and historical guard. Tests live beside the commands. |
| `internal/sessions/` | Rollout discovery, metadata, turns, interactions, follow-ups, and Judge-session detection. Parser tests use synthetic rollouts. |
| `internal/analyze/` | Deterministic aggregates, semantic batches, classifications, and recommendation inputs. Semantic and batch tests use injected runners. |
| `internal/judge/` | Codex subprocess execution, JSON schemas, response decoding, and stderr privacy. Runner tests cover diagnostic sanitization. |
| `internal/cache/` | Concurrent semantic-label persistence and atomic writes. Store tests cover persistence, permissions, and concurrency. |
| `internal/golden/` | Fixture redaction, human approval validation, metrics, and historical baselines. Golden tests cover these contracts. |
| `internal/i18n/` | English/Russian report strings and language selection. Recommendation tests cover localized output. |
| `testdata/golden/`, `docs/` | Fixture guidance and durable user-facing workflow documentation. |

Package boundaries follow [AGENTS.md](../AGENTS.md): CLI wiring stays in `cmd/`;
reusable behavior stays in the narrowest relevant `internal/` package. Tests use
the standard `testing` package, live beside implementation, and should be
deterministic. Use standard Go formatting and focused Conventional Commits.

## Declared commands

From [AGENTS.md](../AGENTS.md) and [CONTRIBUTING.md](../CONTRIBUTING.md):

```sh
go run ./cmd/codex-insights quick
go run ./cmd/codex-insights analyze --days 30
go test ./internal/analyze -run TestName
go test ./...
go vet ./...
go build ./cmd/codex-insights
gofmt -w path/to/file.go
```

`TestName` and `path/to/file.go` are the documented selectors to replace with
the relevant test or changed file. Running `analyze` uses real session history
and can transmit conversation text; use deterministic tests for routine checks.

## Data and behavior contracts

- Session history is sensitive. Use minimal synthetic test data; never commit
  raw conversations, credentials, tokens, or private absolute paths. Follow
  [security guidance](../SECURITY.md) and [AGENTS.md](../AGENTS.md).
- `analyze` and `golden export` share session selection in
  [session_collection.go](../cmd/codex-insights/session_collection.go), including
  exclusion of identified Insights Judge sessions and malformed rollout files.
- Cache values contain labels and confidence, not conversation text. Methodology,
  prompt, schema, model, and reasoning effort participate in semantic cache keys.
  Their versions have independent lifecycles. Sources:
  [semantic.go](../internal/analyze/semantic.go),
  [golden workflow](golden-evaluation.md).
- Golden export is local-only, minimized, redacted, and atomically written.
  Evaluation requires human-approved labels and provenance, and bypasses the
  production semantic cache. Automatic redaction does not replace review.
  Sources: [golden implementation](../internal/golden/golden.go),
  [golden workflow](golden-evaluation.md).
- Exported fixtures and cache files use mode `0600` on Unix-like systems;
  Windows inherits directory ACLs. HTML reports serve on loopback, use no
  external assets, and omit raw session text. Source: [README](../README.md).

## DevCrew layout

The local helper records paths in `.codex/devcrew.json` and managed integration
state in `.codex/devcrew-state.json`. The complete shipped role set is installed
under `.codex/agents/`, with its model/effort defaults preserved. No project
role overrides are required by the current repository instructions.

Task cards and working artifacts belong in `.codex/tasks/`; session snapshots
and handoffs in `.codex/sessions/`; check reports in `.codex/checks/`. `.codex/`
is ignored by Git. Durable lessons belong in [memory/](../memory/README.md);
confirmed public behavior and decisions belong in existing `docs/` documents.
Project invariants remain in `AGENTS.md`; no separate rules tree is required.

Start a new Codex chat in this project to load the installed custom roles.
This context contains durable facts; task progress and temporary observations
belong in the ignored runtime directories.

## Open questions

None required for setup. Judge account/model access and evaluation against real
private history are not established by local initialization or unit tests.
