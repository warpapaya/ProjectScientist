package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPerformanceSmokeCommandExercisesConcurrentMutationsResultsAuditAndReports(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist-smoke.db")
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"project-scientist",
		"smoke", "performance",
		"--db", dbPath,
		"--samples", "4",
		"--concurrency", "2",
		"--reports", "2",
		"--json",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("performance smoke command failed: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}

	var summary performanceSmokeSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("summary should be JSON, got err=%v stdout=%s", err, stdout.String())
	}
	if summary.SamplesCreated != 4 {
		t.Fatalf("expected 4 concurrent sample mutations, got %#v", summary)
	}
	if summary.ResultsEntered != 4 || summary.ResultsAccepted != 4 {
		t.Fatalf("expected one entered+accepted result per sample, got %#v", summary)
	}
	if summary.ReportsGenerated != 2 {
		t.Fatalf("expected requested report generation count, got %#v", summary)
	}
	if summary.AuditEvents < 18 {
		t.Fatalf("expected audit writes for client/sample/result/review/release/report paths, got %#v", summary)
	}
	if summary.Concurrency != 2 || summary.Limits.Samples != 4 || summary.Limits.Reports != 2 {
		t.Fatalf("limits/concurrency should be echoed for repeatability, got %#v", summary)
	}
	if len(summary.Observations) == 0 {
		t.Fatalf("expected smoke observations to document limits/bottlenecks, got %#v", summary)
	}
}
