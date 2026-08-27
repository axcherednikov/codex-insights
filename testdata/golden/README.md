# Golden fixture workflow

Run `codex-insights golden export --output candidate.json` with the same
`--sessions`, `--days`, `--before`, and `--legacy-exclude-originator` options
used for the analysis window. Inspect the minimized, redacted candidate
locally. Do not commit a candidate as approved and do not add labels without a
human review record.

An approved fixture must retain `format_version: "golden-v1"`, all three
current methodology version fields, and provenance containing
`status: "approved"`, a reviewer, an RFC3339 timestamp, and a source. Every
case must have at least one expected field, but expected fields may be
partial when the reviewer checked only selected labels. Run
`codex-insights golden evaluate --fixture approved.json`; evaluation refuses
candidate/unapproved files, calls the current Judge without the production
cache, and reports deterministic per-field and multi-label prevention metrics.

This directory intentionally contains no approved fixture or accepted labels.
