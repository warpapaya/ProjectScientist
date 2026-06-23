package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMVPVerificationSuiteCommandResetsRunsControlsAndWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "project-scientist.db")
	artifactsDir := filepath.Join(dir, "artifacts")

	var stdout, stderr bytes.Buffer
	err := run([]string{"project-scientist", "mvp", "verify-suite", "--db", dbPath, "--artifacts", artifactsDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("mvp verify-suite command failed: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"mvp verify-suite ok", "negative_controls=5", "artifact="} {
		if !strings.Contains(out, want) {
			t.Fatalf("command output missing %q: %s", want, out)
		}
	}

	artifactPath := filepath.Join(artifactsDir, "mvp-verification-suite.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var artifact struct {
		Status           string   `json:"status"`
		Command          string   `json:"command"`
		SampleID         string   `json:"sample_id"`
		ReportArtifactID string   `json:"report_artifact_id"`
		NegativeControls []string `json:"negative_controls"`
		AuditActions     []string `json:"audit_actions"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode artifact: %v content=%s", err, data)
	}
	if artifact.Status != "pass" || artifact.Command == "" || artifact.SampleID == "" || artifact.ReportArtifactID == "" {
		t.Fatalf("artifact missing pass metadata: %#v", artifact)
	}
	for _, want := range []string{"illegal_workflow_jump", "release_before_preconditions", "cross_tenant_attempt", "unauthorized_mutation", "mutate_released_artifact"} {
		if !stringSliceHasPrefix(artifact.NegativeControls, want+":") {
			t.Fatalf("artifact missing negative control %q: %#v", want, artifact.NegativeControls)
		}
	}
	for _, want := range []string{"client.created", "sample.created", "sample.label_artifact.generated", "worksheet.created", "result.accepted", "sample.release.blocked", "report.artifact.released"} {
		if !stringSliceContains(artifact.AuditActions, want) {
			t.Fatalf("artifact missing audit action %q: %#v", want, artifact.AuditActions)
		}
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSliceHasPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
