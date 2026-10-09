# Golden semantic evaluation

`golden export` scans the same local rollout history and time window as
`analyze`, then writes a bounded `golden-v1` candidate fixture. It writes only
minimized prompts, follow-up prompts, and final answers after deterministic
redaction of emails, bearer/API tokens, local absolute paths, and fenced code.
Case IDs are anonymized. Export is local-only and never calls the Judge; the
output is atomically written with mode `0600`.

Review the candidate outside the program. Add only labels a human has actually
approved to every case (partial field coverage is allowed), set
`provenance.status` to `approved`, and record a non-empty approver, RFC3339
approval time, and source. Never infer labels from an old cache or report.
`golden evaluate --fixture FILE` refuses candidate or incompletely sourced
fixtures, runs the current unified methodology with no production
semantic-cache reuse, and reports a regression if approved labels do not
match. Version differences in fixture metadata are retained as provenance and
shown as a warning; they do not block evaluation.

The fixture records methodology, prompt, and schema versions independently.
Bump methodology when the classification definitions or evaluation procedure
changes; bump prompt when Judge instructions change; bump schema when the
wire/result shape changes. Current production versions are `semantic-v2`,
`semantic-judge-v2`, and `semantic-schema-v2`. These identities advance
independently. Version 0.3.0 changed the methodology and prompt while retaining
the result schema. Subagent reporting retains these versions and reuses existing
semantic labels. All three participate in production cache keys.
Model and reasoning effort also intentionally invalidate cache entries;
language does not, because labels are language-independent. The persistent
cache stores labels and confidence values, never raw conversation text. A cold
deep analysis and golden evaluation disclose that selected conversation text
is transmitted to the configured Judge.

Metrics are per-field sample/correct/accuracy for supplied labels. Prevention
mechanisms are multi-label and additionally report micro precision, recall, F1,
and exact-set matches. Mismatch/confusion lines are sorted for reproducibility.
Partially labelled records contribute only to fields that were supplied.

The historical guard applies only to the documented snapshot invocation:
`--before 2026-08-26T14:56:26Z --days 0 --legacy-exclude-originator
codex_exec`. It fails on deterministic aggregate mismatches (1046 tasks,
1023/20/3 statuses, 930 follow-ups, displayed averages 1652761 tokens, 258.3
seconds, and 15.6 tools). The follow-up and semantic reference values belong
to `semantic-v1`. The current CLI reports calibration-required warnings for
follow-up and semantic metrics under `semantic-v2` (and unknown methodologies),
even when observed values happen to equal the historical values. It continues
to check tasks, statuses, and averages strictly. Only an explicitly selected
`semantic-v1` run compares the historical follow-up count and semantic rates;
overall steering uses +/-1.5 percentage points, and refactor and architecture
cohorts use +/-8 points with at least 10 observations. A differing local
history source still fails strict unaffected fields; the historical figures
are not replaced with local observations.

Semantic input bounds each prompt, follow-up prompt, and previous answer to
24 KiB. Oversized text keeps roughly half from the beginning and half from the
end around a truncation marker, and omits the middle. Relevant evidence may be
lost. Synthetic long-input tests verify serialization, UTF-8 integrity, cues,
markers, and byte limits through the fake Judge seam. They verify input
integrity only; they do not establish classifier quality. Real long-case
quality remains unverified until a human-approved golden sample is evaluated.

Aggregates are associations, not causal effects. Cohort comparisons can be
confounded by task difficulty and routing, and subagent-use comparisons have
selection bias. Small cohorts are not interpreted. Golden cases are a reviewed
sample and need not represent all users or tasks.
