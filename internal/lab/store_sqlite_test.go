package lab

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteStoreCommitsDomainStateAndAuditAtomically(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenSQLiteStore(filepath.Join(dir, "project-scientist.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	if _, err := store.db.Exec(`CREATE TRIGGER reject_audit_insert BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END;`); err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}

	_, err = store.CreateClient("Atomic Lab", "atomic@example.test", testActor("friday"))
	if err == nil || !strings.Contains(err.Error(), "audit unavailable") {
		t.Fatalf("expected audit failure, got %v", err)
	}

	clients := store.Clients()
	if len(clients) != 0 {
		t.Fatalf("domain client committed without audit event: %#v", clients)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("audit events should be empty after rolled-back mutation: %#v", events)
	}
}

func TestSQLiteStorePersistsDomainStateAndHashChainedAudit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	client, err := store.CreateClient("Clearline Demo Lab", "qa@example.test", testActor("friday"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Drinking Water Compliance", Matrix: "Water", Tests: []string{"pH", "Turbidity"}}, testActor("friday"))
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer reopened.Close()

	persisted, ok := reopened.GetSample(sample.ID)
	if !ok {
		t.Fatalf("sample %s was not persisted", sample.ID)
	}
	if persisted.Status != StatusReceived || len(persisted.Analyses) != 2 {
		t.Fatalf("unexpected persisted sample: %#v", persisted)
	}

	events, err := reopened.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected client + sample audit events, got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			t.Fatalf("event %d previous hash mismatch: got %q want %q", i, events[i].PreviousHash, events[i-1].Hash)
		}
	}
}

func TestSQLiteStoreRefusesStartupWhenAuditHashChainIsDamaged(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	_, err = store.CreateClient("Tamper Lab", "tamper@example.test", testActor("aegis"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	tamper, err := OpenSQLiteStoreWithoutVerification(dbPath)
	if err != nil {
		t.Fatalf("open without verification: %v", err)
	}
	if _, err := tamper.db.Exec(`UPDATE audit_events SET details_json = ? WHERE sequence = 1`, json.RawMessage(`{"name":"rewritten"}`)); err != nil {
		t.Fatalf("tamper audit row: %v", err)
	}
	if err := tamper.Close(); err != nil {
		t.Fatalf("close tamper store: %v", err)
	}

	if _, err := OpenSQLiteStore(dbPath); err == nil || !strings.Contains(err.Error(), "audit verification") {
		t.Fatalf("expected startup audit verification failure, got %v", err)
	}
}
