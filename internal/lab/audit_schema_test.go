package lab

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditEventsUseSchemaV1RequiredFields(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	client, err := store.CreateClient("Schema Lab", "schema@example.test", testActor("friday"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Audit Schema", Matrix: "Water", Tests: []string{"pH"}}, testActor("friday"))
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusInPrep, testActor("friday")); err != nil {
		t.Fatalf("transition sample: %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 audit events, got %d", len(events))
	}
	for _, event := range events {
		if err := ValidateAuditEvent(event); err != nil {
			t.Fatalf("event %d should be schema-valid: %v\n%#v", event.Sequence, err, event)
		}
		if event.TenantID != DefaultTenantID {
			t.Fatalf("event %d tenant = %q, want %q", event.Sequence, event.TenantID, DefaultTenantID)
		}
		if event.Actor != "friday" || event.ActorContext.UserID != "friday" || event.ActorContext.DisplayNameSnapshot == "" {
			t.Fatalf("event %d actor context incomplete: actor=%#v context=%#v", event.Sequence, event.Actor, event.ActorContext)
		}
		if event.Resource.Type == "" || event.Resource.ID == "" {
			t.Fatalf("event %d resource incomplete: %#v", event.Sequence, event.Resource)
		}
		if event.Outcome != AuditOutcomeAllowed {
			t.Fatalf("event %d outcome = %q, want %q", event.Sequence, event.Outcome, AuditOutcomeAllowed)
		}
		if event.CorrelationID == "" || event.EventID == "" || event.Hash == "" {
			t.Fatalf("event %d missing ids/hash: %#v", event.Sequence, event)
		}
	}
}

func TestDeniedProtectedTransitionIsAuditedWithSafeReason(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	client, _ := store.CreateClient("Denied Lab", "denied@example.test", testActor("analyst"))
	sample, _ := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Metals", Matrix: "Soil", Tests: []string{"Lead"}}, testActor("analyst"))

	err = store.TransitionSample(sample.ID, StatusReleased, testActor("analyst"))
	if err == nil {
		t.Fatalf("expected denied direct release transition")
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	last := events[len(events)-1]
	if last.Action != "sample.transition.requested" {
		t.Fatalf("denied event action = %q", last.Action)
	}
	if last.Outcome != AuditOutcomeDenied {
		t.Fatalf("denied event outcome = %q", last.Outcome)
	}
	if last.Reason != "transition_not_allowed" {
		t.Fatalf("denied event reason = %q", last.Reason)
	}
	if last.Resource.Type != "sample" || last.Resource.ID != sample.ID {
		t.Fatalf("denied event resource = %#v", last.Resource)
	}
	if _, ok := last.Details["full_payload"]; ok {
		t.Fatalf("denied event leaked full payload in details: %#v", last.Details)
	}
	if err := ValidateAuditEvent(last); err != nil {
		t.Fatalf("denied event should be schema-valid: %v", err)
	}
}

func TestAuditVerifierDetectsDatabaseTampering(t *testing.T) {
	cases := []struct {
		name   string
		tamper string
		want   string
	}{
		{name: "modify event body", tamper: `UPDATE audit_events SET action = 'client.renamed' WHERE sequence = 1`, want: "hash mismatch"},
		{name: "delete event", tamper: `DELETE FROM audit_events WHERE sequence = 2`, want: "sequence gap"},
		{name: "reorder event", tamper: `UPDATE audit_events SET sequence = 4 WHERE sequence = 2`, want: "sequence gap"},
		{name: "truncate tail", tamper: `DELETE FROM audit_events WHERE sequence = 3`, want: "checkpoint"},
		{name: "sequence gap", tamper: `UPDATE audit_events SET sequence = 5 WHERE sequence = 3`, want: "sequence gap"},
		{name: "malformed details", tamper: `UPDATE audit_events SET details_json = '{not-json}' WHERE sequence = 2`, want: "malformed"},
		{name: "stored hash mismatch", tamper: `UPDATE audit_events SET hash = 'bad-hash' WHERE sequence = 2`, want: "hash mismatch"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := buildAuditHistory(t)
			tamper, err := OpenSQLiteStoreWithoutVerification(dbPath)
			if err != nil {
				t.Fatalf("open tamper store: %v", err)
			}
			if _, err := tamper.db.Exec(tc.tamper); err != nil {
				t.Fatalf("tamper audit rows: %v", err)
			}
			if err := tamper.Close(); err != nil {
				t.Fatalf("close tamper store: %v", err)
			}

			_, err = OpenSQLiteStore(dbPath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected startup verifier error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAuditVerifierDetectsDuplicateSequenceInMalformedStream(t *testing.T) {
	dbPath := buildAuditHistory(t)
	tamper, err := OpenSQLiteStoreWithoutVerification(dbPath)
	if err != nil {
		t.Fatalf("open tamper store: %v", err)
	}
	events, err := tamper.AuditEvents(0)
	if err != nil {
		t.Fatalf("read audit events for duplicate tamper: %v", err)
	}
	dup := events[1]
	dup.EventID = dup.EventID + "-duplicate"
	details, _ := json.Marshal(dup.Details)
	actor, _ := json.Marshal(dup.ActorContext)
	resource, _ := json.Marshal(dup.Resource)
	if _, err := tamper.db.Exec(`INSERT INTO audit_events(event_id, tenant_id, lab_id, timestamp, actor, actor_json, resource_json, action, outcome, reason, correlation_id, sequence, details_json, previous_hash, hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, dup.EventID, dup.TenantID, dup.LabID, formatTime(dup.Timestamp), dup.Actor, string(actor), string(resource), dup.Action, string(dup.Outcome), dup.Reason, dup.CorrelationID, dup.Sequence, string(details), dup.PreviousHash, dup.Hash); err != nil {
		t.Fatalf("insert duplicate sequence: %v", err)
	}
	if err := tamper.Close(); err != nil {
		t.Fatalf("close tamper store: %v", err)
	}

	_, err = OpenSQLiteStore(dbPath)
	if err == nil || !strings.Contains(err.Error(), "duplicate sequence") {
		t.Fatalf("expected duplicate sequence verifier error, got %v", err)
	}
}

func buildAuditHistory(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.CreateClient("Tamper Lab", "tamper@example.test", testActor("aegis")); err != nil {
		t.Fatalf("create client 1: %v", err)
	}
	if _, err := store.CreateClient("Second Lab", "second@example.test", testActor("aegis")); err != nil {
		t.Fatalf("create client 2: %v", err)
	}
	if _, err := store.CreateClient("Third Lab", "third@example.test", testActor("aegis")); err != nil {
		t.Fatalf("create client 3: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return dbPath
}
