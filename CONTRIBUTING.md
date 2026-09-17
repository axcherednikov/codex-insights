# Contributing

Thank you for helping improve Codex Insights.

## Before you start

- Search existing issues and pull requests before proposing a change.
- Use a GitHub issue to discuss substantial features or behavior changes before
  investing in an implementation.
- Do not include Codex session history, prompts, credentials, access tokens,
  absolute local paths, or other private data in issues, tests, or fixtures.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md) in all project spaces.

## Development setup

Codex Insights requires Go 1.27 or newer.

```bash
git clone https://github.com/axcherednikov/codex-insights.git
cd codex-insights
go test ./...
```

Run the CLI against your local session history:

```bash
go run ./cmd/codex-insights quick
```

Judge-backed analysis also requires an installed and authenticated `codex` CLI.
It may transmit selected conversation text to the configured Judge.

## Making changes

- Keep changes focused and follow existing Go package boundaries and naming.
- Preserve stable CLI flags and report output unless the change explicitly
  requires an interface change.
- Add deterministic regression coverage for changed parsing, aggregation,
  localization, caching, or CLI behavior.
- Use Conventional Commit-style subjects, for example
  `fix: exclude judge sessions from quick report`.
- Add user-visible changes under `Unreleased` in [CHANGELOG.md](CHANGELOG.md).

## Verification

Format changed Go files and run the repository checks:

```bash
gofmt -w path/to/changed.go
go vet ./...
go test ./...
go build ./cmd/codex-insights
```

When report formatting changes, include representative terminal output in the
pull request. When privacy or cache behavior changes, call it out explicitly.

## Pull requests

A pull request should explain the behavior change, link relevant issues, list
the commands actually used for verification, and note any compatibility,
privacy, or cache-format impact. Small, reviewable pull requests are preferred.
