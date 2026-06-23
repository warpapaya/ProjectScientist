package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleReleaseRequiresAcceptedResultsAndAcceptedQCBatches(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample to review: %v", err)
	}

	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err == nil || !strings.Contains(err.Error(), "unaccepted result") {
		t.Fatalf("expected unaccepted result to block release, got %v", err)
	}

	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "ready", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("accept result: %v", err)
	}

	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err == nil || !strings.Contains(err.Error(), "QC batch") {
		t.Fatalf("expected unaccepted QC batch to block release, got %v", err)
	}

	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready for QC decision", manager); err != nil {
		t.Fatalf("move QC batch to review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "QC accepted", manager); err != nil {
		t.Fatalf("accept QC batch: %v", err)
	}

	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err != nil {
		t.Fatalf("release after accepted result and QC: %v", err)
	}
}

func TestSampleReleaseQCOverrideRequiresPermissionReasonAndAuditsDecision(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample to review: %v", err)
	}
	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "ready", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready for QC decision", manager); err != nil {
		t.Fatalf("move QC batch to review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusRejected, "blank failed", manager); err != nil {
		t.Fatalf("reject QC batch: %v", err)
	}

	if err := store.TransitionSampleWithReleaseOverride(sample.ID, StatusReleased, ReleaseOverrideInput{OverrideQC: true}, manager); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("expected override reason to be required, got %v", err)
	}
	if err := store.TransitionSampleWithReleaseOverride(sample.ID, StatusReleased, ReleaseOverrideInput{OverrideQC: true, OverrideReason: "client-approved exception"}, manager); err == nil || !strings.Contains(err.Error(), "report release") {
		t.Fatalf("expected report release permission to be required, got %v", err)
	}

	releaser := testActorWithRoles("authorized-releaser", RoleLabManager, RoleReportReleaser)
	if err := store.TransitionSampleWithReleaseOverride(sample.ID, StatusReleased, ReleaseOverrideInput{OverrideQC: true, OverrideReason: "client-approved exception"}, releaser); err != nil {
		t.Fatalf("override rejected QC batch: %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditEventExists(events, "sample.release.qc_override", "sample", sample.ID) {
		t.Fatalf("expected QC override audit event")
	}
}

func seedReleaseReadinessFixture(t *testing.T) (*Store, Sample, QCBatch) {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Release QC", ClientSampleID: "REL-C-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(clientSample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected analysis request line, got %d", len(lines))
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 1.2, RawValue: "1.2 mg/L", Unit: "mg/L", Dilution: 1, AnalystID: "analyst-1"}, testActorWithRoles("analyst-1", RoleAnalyst)); err != nil {
		t.Fatalf("create result: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Release QC", ClientSampleID: "REL-MB-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}
	batch, err := store.CreateQCBatch(CreateQCBatchInput{Name: "Release QC batch", MethodID: method.ID, Matrix: "Water"}, actor)
	if err != nil {
		t.Fatalf("create QC batch: %v", err)
	}
	if _, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: clientSample.ID, Role: QCItemRoleClientSample}, actor); err != nil {
		t.Fatalf("add client QC item: %v", err)
	}
	qcItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindMethodBlank}, actor)
	if err != nil {
		t.Fatalf("add QC item: %v", err)
	}
	if _, err := store.CreateQCRelationship(CreateQCRelationshipInput{QCBatchID: batch.ID, QCItemID: qcItem.ID, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: clientSample.ID}, actor); err != nil {
		t.Fatalf("create QC relationship: %v", err)
	}
	return store, clientSample, batch
}

func advanceSampleToReview(store *Store, sampleID string, actor ActorContext) error {
	for _, status := range []SampleStatus{StatusInPrep, StatusInAnalysis, StatusInReview} {
		if err := store.TransitionSample(sampleID, status, actor); err != nil {
			return err
		}
	}
	return nil
}
