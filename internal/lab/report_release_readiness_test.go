package lab

import (
	"strings"
	"testing"
)

func TestReportReleaseReadinessExplainsBlockersThenCurrentRelease(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample to review: %v", err)
	}

	blocked, ok := store.ReportReleaseReadinessForScope(DefaultScope, sample.ID)
	if !ok {
		t.Fatalf("expected readiness row for sample %s", sample.ID)
	}
	if blocked.ReadyForRelease || blocked.ReleaseAction != "blocked" {
		t.Fatalf("sample should be blocked before accepted result/QC/released status: %#v", blocked)
	}
	for _, want := range []string{"sample_status", "unaccepted_result", "qc_not_accepted", "no_current_report"} {
		if !readinessHasBlocker(blocked, want) {
			t.Fatalf("readiness missing blocker %q: %#v", want, blocked.Blockers)
		}
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

	ready, ok := store.ReportReleaseReadinessForScope(DefaultScope, sample.ID)
	if !ok {
		t.Fatalf("expected readiness row after release")
	}
	if !ready.ReadyForRelease || ready.ReleaseAction != "initial_release" || len(ready.Blockers) != 0 {
		t.Fatalf("released sample with accepted result/QC should be ready for initial report: %#v", ready)
	}

	released, err := store.GenerateCOAReportArtifact(COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: "coa-standard", Version: "2026.06", Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: "Demo Client"}}, testActorWithRoles("report-releaser-1", RoleReportReleaser))
	if err != nil {
		t.Fatalf("generate report artifact: %v", err)
	}
	current, ok := store.ReportReleaseReadinessForScope(DefaultScope, sample.ID)
	if !ok {
		t.Fatalf("expected readiness row with current report")
	}
	if !current.ReadyForRelease || current.ReleaseAction != "amendment" || current.CurrentArtifactID != released.Artifact.ID || current.CurrentSnapshotID != released.Snapshot.ID {
		t.Fatalf("current released artifact should switch CTA to amendment: %#v released=%#v", current, released)
	}
	if current.PreviewLabel == "" || !strings.Contains(current.PreviewLabel, sample.ID) {
		t.Fatalf("preview label should identify sample: %#v", current)
	}
}

func readinessHasBlocker(readiness ReportReleaseReadiness, code string) bool {
	for _, blocker := range readiness.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}
