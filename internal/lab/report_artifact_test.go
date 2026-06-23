package lab

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportArtifactReleaseCapturesImmutableHashProvenanceAndSupersession(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample to review: %v", err)
	}
	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "approved for COA", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready for QC decision", manager); err != nil {
		t.Fatalf("move QC batch to review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "QC accepted", manager); err != nil {
		t.Fatalf("accept QC batch: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}
	releaser := testActorWithRoles("report-releaser-1", RoleReportReleaser)

	releasedAtFloor := time.Now().UTC()
	first, err := store.ReleaseReportArtifact(ReportReleaseInput{
		SampleID:           sample.ID,
		TemplateID:         "coa-standard",
		TemplateVersion:    "2026.06",
		GenerationInputs:   map[string]string{"format": "pdf", "locale": "en-US"},
		ArtifactFormat:     "application/pdf",
		ArtifactContent:    []byte("synthetic COA artifact v1"),
		SupersessionReason: "initial release",
	}, releaser)
	if err != nil {
		t.Fatalf("release report artifact: %v", err)
	}
	if first.Snapshot.SampleID != sample.ID || first.Snapshot.TemplateID != "coa-standard" || first.Snapshot.TemplateVersion != "2026.06" {
		t.Fatalf("snapshot identity/provenance mismatch: %#v", first.Snapshot)
	}
	if first.Snapshot.ReviewedBy != "reviewer-1" || first.Snapshot.ReleasedBy != releaser.UserID || first.Snapshot.ReleasedAt.Before(releasedAtFloor) {
		t.Fatalf("review/release provenance not captured: %#v", first.Snapshot)
	}
	if first.Snapshot.DataSnapshot.Sample.ID != sample.ID || first.Snapshot.DataSnapshot.Sample.Status != StatusReleased || len(first.Snapshot.DataSnapshot.Results) != 1 || first.Snapshot.DataSnapshot.Results[0].Status != ResultStatusAccepted {
		t.Fatalf("data snapshot should freeze released sample and accepted results: %#v", first.Snapshot.DataSnapshot)
	}
	if first.Snapshot.GenerationInputs["format"] != "pdf" || first.Snapshot.GenerationInputs["locale"] != "en-US" {
		t.Fatalf("generation inputs not preserved: %#v", first.Snapshot.GenerationInputs)
	}
	if !strings.HasPrefix(first.Snapshot.ContentHash, "sha256:") || !strings.HasPrefix(first.Artifact.ContentHash, "sha256:") {
		t.Fatalf("snapshot and artifact should carry sha256 hashes: %#v %#v", first.Snapshot, first.Artifact)
	}
	if first.Artifact.SnapshotID != first.Snapshot.ID || first.Artifact.SampleID != sample.ID || first.Artifact.Format != "application/pdf" {
		t.Fatalf("artifact provenance mismatch: %#v", first.Artifact)
	}

	if _, err := store.DB().Exec(`UPDATE report_snapshots SET template_version = 'mutated' WHERE id = ?`, first.Snapshot.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected report snapshot update to be rejected as immutable, got %v", err)
	}
	if _, err := store.DB().Exec(`DELETE FROM report_artifacts WHERE id = ?`, first.Artifact.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected report artifact delete to be rejected as immutable, got %v", err)
	}

	second, err := store.ReleaseReportArtifact(ReportReleaseInput{
		SampleID:           sample.ID,
		TemplateID:         "coa-standard",
		TemplateVersion:    "2026.07",
		GenerationInputs:   map[string]string{"format": "pdf", "locale": "en-US", "reason": "amended narrative"},
		ArtifactFormat:     "application/pdf",
		ArtifactContent:    []byte("synthetic COA artifact v2"),
		SupersessionReason: "amended narrative",
	}, releaser)
	if err != nil {
		t.Fatalf("release superseding report artifact: %v", err)
	}
	if second.Snapshot.SupersedesSnapshotID != first.Snapshot.ID || second.Artifact.SupersedesArtifactID != first.Artifact.ID {
		t.Fatalf("second report should supersede first: first=%#v second=%#v", first, second)
	}
	refetchedFirst, ok := store.ReportSnapshot(first.Snapshot.ID)
	if !ok {
		t.Fatalf("refetch first report snapshot")
	}
	if refetchedFirst.SupersededBySnapshotID != second.Snapshot.ID {
		t.Fatalf("first snapshot should expose superseded-by link from append-only edge: %#v", refetchedFirst)
	}
	if first.Snapshot.ContentHash == second.Snapshot.ContentHash || first.Artifact.ContentHash == second.Artifact.ContentHash {
		t.Fatalf("template/input/content changes should change hashes: first=%#v second=%#v", first, second)
	}
}

func TestReportArtifactReleaseRequiresReleasedSampleAndReportReleaser(t *testing.T) {
	store, sample, _ := seedReleaseReadinessFixture(t)
	defer store.Close()

	input := ReportReleaseInput{SampleID: sample.ID, TemplateID: "coa-standard", TemplateVersion: "2026.06", ArtifactFormat: "text/plain", ArtifactContent: []byte("draft")}
	if _, err := store.ReleaseReportArtifact(input, testActorWithRoles("reviewer-only", RoleReviewer)); err == nil || !strings.Contains(err.Error(), "report release") {
		t.Fatalf("expected report release permission denial, got %v", err)
	}
	if _, err := store.ReleaseReportArtifact(input, testActorWithRoles("report-releaser-1", RoleReportReleaser)); err == nil || !strings.Contains(err.Error(), "released sample") {
		t.Fatalf("expected unreleased sample denial, got %v", err)
	}
}

func TestReportArtifactHashIsStableForEquivalentInputs(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	sample := Sample{ID: "S-123", TenantID: DefaultTenantID, LabID: DefaultLabID, Status: StatusReleased}
	results := []Result{{ID: "R-1", SampleID: sample.ID, Status: ResultStatusAccepted, ReviewedBy: "reviewer-1"}}
	left, err := buildReportSnapshotPayload(sample, results, "coa", "v1", map[string]string{"b": "2", "a": "1"}, "releaser", time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build left payload: %v", err)
	}
	right, err := buildReportSnapshotPayload(sample, results, "coa", "v1", map[string]string{"a": "1", "b": "2"}, "releaser", time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build right payload: %v", err)
	}
	if left.ContentHash != right.ContentHash || string(left.CanonicalJSON) != string(right.CanonicalJSON) {
		t.Fatalf("equivalent generation input maps should hash canonically\nleft=%s\nright=%s", left.CanonicalJSON, right.CanonicalJSON)
	}
}
