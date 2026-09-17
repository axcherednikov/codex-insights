package main

import (
	"strings"
	"testing"
	"time"
)

func TestVersionLineIncludesConfiguredVersion(t *testing.T) {
	original := version
	version = "0.1.0"
	t.Cleanup(func() { version = original })

	if got := versionLine(); got != "codex-insights 0.1.0" {
		t.Fatalf("version line = %q", got)
	}
	if strings.Contains(versionLine(), "v0.1.0") {
		t.Fatal("release version unexpectedly gained a second v prefix")
	}
}

func TestTimestampInWindowHonorsSnapshotBounds(t *testing.T) {
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 8, 26, 14, 56, 26, 0, time.UTC)

	if !timestampInWindow("2026-08-20T12:00:00Z", since, before) {
		t.Fatal("expected timestamp inside window")
	}
	if timestampInWindow("2026-08-26T15:00:00Z", since, before) {
		t.Fatal("timestamp after snapshot must be excluded")
	}
	if timestampInWindow("2026-07-31T23:59:59Z", since, before) {
		t.Fatal("timestamp before window must be excluded")
	}
	if timestampInWindow("invalid", since, before) {
		t.Fatal("invalid timestamp must be excluded")
	}
}
