# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-09-17

### Added

- An optional `analyze --html` workflow that generates a responsive local HTML
  report after semantic analysis.
- Friendly English and Russian summaries with strengths, growth areas,
  prioritized recommendations, practical checklists, and clearly labeled
  synthetic before-and-after examples.
- Timestamped report storage under `temp/reports/`, with restrictive file
  permissions on Unix-like systems and retained reports for later review.
- A loopback-only HTTP server that prints the report URL, opens it in the
  default browser, and runs until interrupted with Ctrl+C.
- Regression coverage for deterministic rendering, HTML escaping, evidence
  thresholds, private report persistence, loopback serving, and Host-header
  validation.

### Security

- Kept prompts, answers, and session text out of HTML reports by accepting only
  aggregate counts, effectiveness statistics, and semantic label totals.
- Restricted report serving to `127.0.0.1`, rejected mismatched Host headers,
  and prevented access to files outside the current report.

## [0.1.0] - 2026-09-17

### Added

- Local summaries of Codex sessions, task outcomes, token usage, duration, tool
  calls, and model/reasoning combinations.
- Judge-backed semantic analysis of follow-ups, steering, prompt quality,
  validation gaps, task types, project rules, and reusable skill candidates.
- English and Russian report localization.
- Privacy-preserving semantic cache containing classifications rather than raw
  conversation text.
- Human-reviewed golden fixture export and evaluation workflow.
- Historical regression guard and deterministic aggregate reporting.
- Automated tests, cross-platform CI, and release packaging.
- A `--version` command for identifying installed release binaries.
- README, license, and changelog files in every release archive.

### Changed

- Updated GitHub Actions to their current Node.js 24-compatible major versions.
- Documented the Windows ACL behavior for privacy-sensitive output files.

### Fixed

- Kept the restrictive permission regression test meaningful on POSIX systems
  without failing on Windows, where `FileMode.Perm` does not represent ACLs.

[Unreleased]: https://github.com/axcherednikov/codex-insights/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/axcherednikov/codex-insights/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/axcherednikov/codex-insights/releases/tag/v0.1.0
