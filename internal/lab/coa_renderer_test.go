package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCOARendererMatchesSyntheticTindallFixtureAndHash(t *testing.T) {
	sample := Sample{
		ID:             "S-000042",
		TenantID:       DefaultTenantID,
		LabID:          DefaultLabID,
		Project:        "Synthetic Drinking Water Compliance",
		ClientSampleID: "TW-2026-001",
		LabSampleID:    "TINDALL-2026-00042",
		Matrix:         "Drinking Water",
		Status:         StatusReleased,
	}
	results := []Result{
		{ID: "R-1", SampleID: sample.ID, Status: ResultStatusAccepted, RawValue: "7.12", Value: 7.12, Unit: "pH", Qualifier: "", AnalystID: "analyst-1", ReviewedBy: "reviewer-1"},
		{ID: "R-2", SampleID: sample.ID, Status: ResultStatusAccepted, RawValue: "0.42", Value: 0.42, Unit: "NTU", Qualifier: "J", MDL: 0.1, RL: 0.3, AnalystID: "analyst-1", ReviewedBy: "reviewer-1"},
	}

	artifact, err := RenderCOAArtifact(COARenderInput{
		Template: COATemplate{ID: "coa-tindall-standard", Version: "2026.06", Style: COAStyleTindall, LabName: "Tindall Synthetic Lab", ClientName: "Tindall Synthetic Utilities"},
		Snapshot: ReportDataSnapshot{Sample: sample, Results: results},
	})
	if err != nil {
		t.Fatalf("render COA artifact: %v", err)
	}
	fixturePath := filepath.Join("..", "..", "fixtures", "coa_tindall_synthetic.txt")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}
	if string(artifact.Content) != string(fixture) {
		t.Fatalf("COA artifact mismatch\n--- got ---\n%s\n--- want ---\n%s", artifact.Content, fixture)
	}
	sum := sha256.Sum256(fixture)
	wantHash := "sha256:" + hex.EncodeToString(sum[:])
	hashFixture, err := os.ReadFile(fixturePath + ".sha256")
	if err != nil {
		t.Fatalf("read fixture hash: %v", err)
	}
	if !strings.Contains(string(hashFixture), wantHash) {
		t.Fatalf("fixture hash file mismatch: got %q want %s", hashFixture, wantHash)
	}
	if artifact.Format != COAArtifactFormatText || artifact.ContentHash != wantHash {
		t.Fatalf("artifact format/hash mismatch: %#v want hash %s", artifact, wantHash)
	}
	if !strings.Contains(string(artifact.Content), "CERTIFICATE OF ANALYSIS") || !strings.Contains(string(artifact.Content), "TINDALL-2026-00042") {
		t.Fatalf("artifact should include COA title and lab sample id: %s", artifact.Content)
	}
}

func TestGenerateCOAReportArtifactReleasesHashAndAudits(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()
	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample: %v", err)
	}
	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "approved for COA", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("review result: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready", manager); err != nil {
		t.Fatalf("QC review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "accepted", manager); err != nil {
		t.Fatalf("QC accept: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}

	releaser := testActorWithRoles("report-releaser-1", RoleReportReleaser)
	released, err := store.GenerateCOAReportArtifact(COAGenerationInput{
		SampleID: sample.ID,
		Template: COATemplate{ID: "coa-cenla-standard", Version: "2026.06", Style: COAStyleCENLA, LabName: "CENLA Synthetic Lab", ClientName: "CENLA Synthetic Client"},
	}, releaser)
	if err != nil {
		t.Fatalf("generate COA report artifact: %v", err)
	}
	if released.Snapshot.TemplateID != "coa-cenla-standard" || released.Snapshot.TemplateVersion != "2026.06" {
		t.Fatalf("snapshot template provenance mismatch: %#v", released.Snapshot)
	}
	if released.Artifact.Format != COAArtifactFormatText || !strings.HasPrefix(released.Artifact.ContentHash, "sha256:") || !strings.Contains(string(released.Artifact.Content), "CENLA Synthetic Lab") {
		t.Fatalf("artifact format/hash/content mismatch: %#v content=%s", released.Artifact, released.Artifact.Content)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditContains(events, "report.artifact.released", released.Artifact.ID) {
		t.Fatalf("COA release audit event missing for %s: %#v", released.Artifact.ID, events)
	}
}
