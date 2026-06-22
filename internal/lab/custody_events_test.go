package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustodyEventsAreTenantScopedAuditedAndVisibleOnSamples(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	alpha := Scope{TenantID: "tenant-alpha", LabID: "water-lab"}
	beta := Scope{TenantID: "tenant-beta", LabID: "water-lab"}
	alphaActor := testScopedActor("custodian-alpha", alpha.TenantID)
	betaActor := testScopedActor("custodian-beta", beta.TenantID)
	client, _ := store.CreateClientForScope(alpha, "Custody Client", "custody@example.test", alphaActor)
	sample, err := store.CreateSampleForScope(alpha, CreateSampleInput{ClientID: client.ID, Project: "COC-001", Matrix: "Water", Tests: []string{"pH"}}, alphaActor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	received, err := store.RecordCustodyEventForScope(alpha, CustodyEventInput{SampleID: sample.ID, Type: CustodyReceived, Location: "Receiving fridge A", Reason: "COC intake", OccurredAt: time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)}, alphaActor)
	if err != nil {
		t.Fatalf("record received custody event: %v", err)
	}
	transferred, err := store.RecordCustodyEventForScope(alpha, CustodyEventInput{SampleID: sample.ID, Type: CustodyTransferred, Location: "Prep bench 2", Reason: "Prep handoff"}, alphaActor)
	if err != nil {
		t.Fatalf("record transfer custody event: %v", err)
	}
	if received.ID == "" || transferred.Sequence != received.Sequence+1 || transferred.Actor.UserID != alphaActor.UserID || transferred.Location != "Prep bench 2" || transferred.Reason != "Prep handoff" {
		t.Fatalf("custody event metadata/sequence not persisted: received=%#v transferred=%#v", received, transferred)
	}

	loaded, ok := store.GetSampleForScope(alpha, sample.ID)
	if !ok {
		t.Fatalf("load sample %q", sample.ID)
	}
	if len(loaded.CustodyEvents) != 2 || loaded.CustodyEvents[0].Type != CustodyReceived || loaded.CustodyEvents[1].Type != CustodyTransferred {
		t.Fatalf("sample did not expose custody history: %#v", loaded.CustodyEvents)
	}
	if _, ok := store.GetSampleForScope(beta, sample.ID); ok {
		t.Fatalf("beta scope should not see alpha sample")
	}
	if _, err := store.RecordCustodyEventForScope(beta, CustodyEventInput{SampleID: sample.ID, Type: CustodyStored, Location: "Other lab", Reason: "cross tenant attempt"}, betaActor); err == nil || !strings.Contains(err.Error(), "unknown sample") || strings.Contains(err.Error(), "outside requested tenant/lab scope") {
		t.Fatalf("expected cross-tenant custody denial without existence leak, got %v", err)
	}

	events, err := store.AuditEventsForScope(alpha, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditContains(events, "sample.custody.recorded", sample.ID) {
		t.Fatalf("custody event was not audited in alpha scope: %#v", events)
	}
	betaEvents, _ := store.AuditEventsForScope(beta, 0)
	if !auditContains(betaEvents, "sample.custody.record.requested", sample.ID) {
		t.Fatalf("cross-tenant custody attempt was not audited in beta scope: %#v", betaEvents)
	}
}

func TestCustodyEventValidationAndIllegalEdits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("custody-validator")
	client, _ := store.CreateClient("Custody Validation Client", "custody-validation@example.test", actor)
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Custody Validation", Matrix: "Water", Tests: []string{"pH"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	for _, eventType := range []CustodyEventType{CustodyReceived, CustodyTransferred, CustodySplit, CustodyStored, CustodyDisposed, CustodyReturned} {
		if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: eventType, Location: "Bench", Reason: "exercise allowed type"}, actor); err != nil {
			t.Fatalf("record allowed custody type %s: %v", eventType, err)
		}
	}
	if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyEventType("edited"), Location: "Bench", Reason: "illegal type"}, actor); err == nil || !strings.Contains(err.Error(), "unsupported custody event type") {
		t.Fatalf("expected unsupported custody type error, got %v", err)
	}
	if _, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyStored, Location: "", Reason: "missing location"}, actor); err == nil || !strings.Contains(err.Error(), "location is required") {
		t.Fatalf("expected missing location error, got %v", err)
	}
	first, err := store.RecordCustodyEvent(CustodyEventInput{SampleID: sample.ID, Type: CustodyReceived, Location: "Bench", Reason: "immutable row"}, actor)
	if err != nil {
		t.Fatalf("record custody event for immutability check: %v", err)
	}
	if _, err := store.DB().Exec(`UPDATE custody_events SET location = ? WHERE id = ?`, "Changed", first.ID); err == nil || !strings.Contains(err.Error(), "custody history is immutable") {
		t.Fatalf("expected raw UPDATE to be blocked by immutable custody trigger, got %v", err)
	}
	if _, err := store.DB().Exec(`DELETE FROM custody_events WHERE id = ?`, first.ID); err == nil || !strings.Contains(err.Error(), "custody history is immutable") {
		t.Fatalf("expected raw DELETE to be blocked by immutable custody trigger, got %v", err)
	}
	if err := store.UpdateCustodyEvent(sample.ID, first.ID, CustodyEventInput{SampleID: sample.ID, Type: CustodyStored, Location: "Changed", Reason: "edit"}, actor); !errors.Is(err, ErrCustodyHistoryImmutable) {
		t.Fatalf("expected immutable history error, got %v", err)
	}
}

func auditContains(events []AuditEvent, action, resourceID string) bool {
	for _, event := range events {
		if event.Action == action && event.Resource.ID == resourceID {
			return true
		}
	}
	return false
}
