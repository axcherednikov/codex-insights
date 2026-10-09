# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-10-09

### Added

- A separate `quick` subagent report covering unique created agents, working
  turns, completed/aborted/incomplete runs, nested creations, logged roles,
  models, and reasoning settings. Repeated assignments remain working turns
  of the same agent rather than extra user tasks.
- Subagent token totals by logged role and observed model, preferring confirmed
  `token_usage_record` requests and separately labelling legacy `token_count`
  estimates. Cached input and reasoning output are included subsets, not
  additional totals. Missing data and counter disagreements are disclosed.
- Summed recorded subagent working time, including parallel execution and
  explicitly separate from user-task elapsed time.
- Console and HTML `analyze` comparisons for tasks with and without observed
  subagents, by task type, subagent role/model, and matching parent execution
  settings. Rows include user steering, token/time/tool averages, sample sizes,
  and resource coverage using existing semantic labels.
- Regression tests for nested and reused agents, repeated records and rollouts,
  inherited fork history, conflicting ownership/counters, old session formats,
  missing values, report rendering, and preservation of user-task statistics.
- Synthetic report examples and documentation of attribution, resource
  accounting, overlapping cohorts, and statistical limitations.

### Compatibility

- User-task statistics continue to include only user sessions. Historical
  guardian service sessions are excluded from ordinary subagents, and child
  sessions do not increase user-task counts.
- Existing CLI flags, semantic methodology, Judge model/prompt/schema, cache
  format, and golden workflow remain unchanged. Role analytics require no
  plugin or role-name registry and add no semantic Judge requests.
- Old histories remain readable; unavailable roles, request ownership, root
  links, or durations reduce coverage rather than inventing attribution.

### Limitations

- Resource means use observed values; partial coverage is not a complete cost
  estimate. No monetary pricing is inferred. `quick` lifecycle statuses and
  working time are aggregate totals, not per-role breakdowns.
- Effectiveness is descriptive, not causal. Actual difficulty and individual
  agent success are unavailable; no steering does not prove success. Role/model
  cohorts overlap, and the ten-sample reporting threshold does not establish
  statistical significance.

## [0.3.0] - 2026-10-05

### Added

- Strict `golangci-lint` checks in CI using the repository configuration.

### Changed

- Default reports to English while retaining explicit `--lang auto` locale
  detection and `--lang ru` Russian output.
- Advance the semantic methodology and Judge prompt to version 2. Existing
  semantic cache entries are not reused, and the first deep analysis recomputes
  classifications. The result schema remains unchanged.
- Report that historical semantic and follow-up reference metrics need
  recalibration for the new methodology while retaining strict checks for
  unaffected task counts, statuses, and averages.

### Fixed

- Propagate local report output and resource failures, and prevent Judge calls
  when the required privacy disclosure cannot be written.
- Bound semantic Judge requests by serialized size, truncate exceptionally long
  text fields, and split unexpected oversized batches instead of aborting the
  full analysis.
- Build follow-up pairs only from adjacent completed interactions so aborted
  or incomplete tasks cannot create false semantic steering relationships.
- Serve local HTML reports while the browser opens, cancel pending browser
  launches on server failure, and close idle connections during shutdown.
- Preserve analysis cancellation and propagate resource cleanup failures.
- Check file permission semantics correctly on Windows while retaining exact
  restrictive permission checks on Unix-like systems.

### Security

- Run Judge-backed analysis with `codex exec --ephemeral` so prompts, answers,
  and follow-ups are not persisted as Codex session rollout files.

## [0.2.1] - 2026-09-17

### Changed

- Expanded installation instructions with persistent `PATH` setup and the
  required Codex CLI installation and sign-in steps.

### Fixed

- Report a localized, actionable error before analysis when the Codex CLI is
  not available on `PATH`.

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

[Unreleased]: https://github.com/axcherednikov/codex-insights/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/axcherednikov/codex-insights/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/axcherednikov/codex-insights/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/axcherednikov/codex-insights/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/axcherednikov/codex-insights/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/axcherednikov/codex-insights/releases/tag/v0.1.0
