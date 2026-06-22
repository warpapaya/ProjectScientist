package lab

import (
	"path/filepath"
	"testing"
)

func TestQCBatchCompositionPersistsClientAndQCItems(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "C-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "LCS-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}

	batch, err := store.CreateQCBatch(CreateQCBatchInput{Name: "Metals batch 2026-001", MethodID: method.ID, Matrix: "Water", Notes: "synthetic lab-test batch"}, actor)
	if err != nil {
		t.Fatalf("create QC batch: %v", err)
	}
	if batch.Status != QCBatchStatusOpen {
		t.Fatalf("new batch status got %q want %q", batch.Status, QCBatchStatusOpen)
	}
	clientItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: clientSample.ID, Role: QCItemRoleClientSample, Notes: "production sample"}, actor)
	if err != nil {
		t.Fatalf("add client item: %v", err)
	}
	qcItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindLaboratoryControlSample, Notes: "LCS control"}, actor)
	if err != nil {
		t.Fatalf("add QC item: %v", err)
	}

	loaded, ok := store.QCBatchByID(batch.ID)
	if !ok {
		t.Fatalf("batch not found")
	}
	if loaded.MethodID != method.ID || loaded.Matrix != "Water" || len(loaded.Items) != 2 {
		t.Fatalf("batch composition not persisted: %#v", loaded)
	}
	if loaded.Items[0].ID != clientItem.ID || loaded.Items[1].ID != qcItem.ID {
		t.Fatalf("batch items returned in insertion order, got %#v", loaded.Items)
	}
}

func TestQCRelationshipLinksQCItemToClientSampleLineAndAudits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "C-2", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create client sample: %v", err)
	}
	qcSample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "DUP-1", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create qc sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(clientSample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected analysis request line, got %d", len(lines))
	}
	batch, err := store.CreateQCBatch(CreateQCBatchInput{Name: "Duplicate batch", MethodID: method.ID, Matrix: "Water"}, actor)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	qcItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindLabDuplicate}, actor)
	if err != nil {
		t.Fatalf("add QC item: %v", err)
	}

	rel, err := store.CreateQCRelationship(CreateQCRelationshipInput{QCBatchID: batch.ID, QCItemID: qcItem.ID, RelationshipType: QCRelationshipTypeDuplicateOf, RelatedSampleID: clientSample.ID, AnalysisRequestLineID: lines[0].ID, Notes: "duplicate precision check"}, actor)
	if err != nil {
		t.Fatalf("create QC relationship: %v", err)
	}
	loaded, ok := store.QCBatchByID(batch.ID)
	if !ok || len(loaded.Relationships) != 1 {
		t.Fatalf("relationship not returned with batch: %#v", loaded)
	}
	if loaded.Relationships[0].ID != rel.ID || loaded.Relationships[0].RelatedSampleID != clientSample.ID || loaded.Relationships[0].AnalysisRequestLineID != lines[0].ID {
		t.Fatalf("relationship links not persisted: %#v", loaded.Relationships[0])
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditEventExists(events, "qc_batch.relationship.created", "qc_relationship", rel.ID) {
		t.Fatalf("expected QC relationship creation audit event")
	}
}

func TestQCBatchStatusWorkflowRequiresReviewAndAuditsAcceptance(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	client, method, service := createQCClientMethodAndService(t, store, actor)
	clientSample, _ := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "C-3", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	qcSample, _ := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "QC batch", ClientSampleID: "MB-3", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	batch, err := store.CreateQCBatch(CreateQCBatchInput{Name: "Acceptance batch", MethodID: method.ID, Matrix: "Water"}, actor)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	qcItem, err := store.AddQCItemToBatch(batch.ID, CreateQCItemInput{SampleID: qcSample.ID, Role: QCItemRoleQCSample, QCSampleKind: QCSampleKindMethodBlank}, actor)
	if err != nil {
		t.Fatalf("add QC item: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "skipping review", actor); err == nil {
		t.Fatalf("expected direct acceptance from open to fail")
	}
	if _, err := store.CreateQCRelationship(CreateQCRelationshipInput{QCBatchID: batch.ID, QCItemID: qcItem.ID, RelationshipType: QCRelationshipTypeBatchControl, RelatedSampleID: clientSample.ID}, actor); err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready for QC review", actor); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	accepted, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "blank acceptable for synthetic fixture", actor)
	if err != nil {
		t.Fatalf("accept batch: %v", err)
	}
	if accepted.Status != QCBatchStatusAccepted || accepted.DecisionReason != "blank acceptable for synthetic fixture" {
		t.Fatalf("accepted state not persisted: %#v", accepted)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditEventExists(events, "qc_batch.status.changed", "qc_batch", batch.ID) {
		t.Fatalf("expected status change audit event")
	}
}

func auditEventExists(events []AuditEvent, action, resourceType, resourceID string) bool {
	for _, event := range events {
		if event.Action == action && event.Resource.Type == resourceType && event.Resource.ID == resourceID && event.Outcome == AuditOutcomeAllowed {
			return true
		}
	}
	return false
}
