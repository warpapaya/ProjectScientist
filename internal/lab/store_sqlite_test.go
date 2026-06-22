package lab

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteMigrationAddsTenantLabColumnsBeforeScopedIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	legacySchema := []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
		`CREATE TABLE store_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
		`CREATE TABLE clients (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, created_at TEXT NOT NULL);`,
		`CREATE TABLE samples (id TEXT PRIMARY KEY, client_id TEXT NOT NULL REFERENCES clients(id), project TEXT NOT NULL, matrix TEXT NOT NULL, status TEXT NOT NULL, analyses_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE audit_events (sequence INTEGER PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, details_json TEXT NOT NULL, previous_hash TEXT NOT NULL, hash TEXT NOT NULL UNIQUE);`,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (1, '2026-06-01T00:00:00Z');`,
		`INSERT INTO store_meta(key, value) VALUES ('next_client', '2'), ('next_sample', '1'), ('next_audit', '1'), ('last_hash', '');`,
		`INSERT INTO clients(id, name, email, created_at) VALUES ('C-00001', 'Legacy Client', 'legacy@example.test', '2026-06-01T00:00:00Z');`,
	}
	for _, stmt := range legacySchema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("install legacy schema: %v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}
	defer store.Close()

	clients := store.ClientsForScope(DefaultScope)
	if len(clients) != 1 || clients[0].TenantID != DefaultScope.TenantID || clients[0].LabID != DefaultScope.LabID {
		t.Fatalf("legacy clients not backfilled into default scope: %#v", clients)
	}
}

func TestSQLiteMigrationAddsMissingAuditColumnsBeforeVerification(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	legacySchema := []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
		`CREATE TABLE store_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
		`CREATE TABLE clients (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, created_at TEXT NOT NULL);`,
		`CREATE TABLE samples (id TEXT PRIMARY KEY, client_id TEXT NOT NULL REFERENCES clients(id), project TEXT NOT NULL, matrix TEXT NOT NULL, status TEXT NOT NULL, analyses_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE audit_events (sequence INTEGER PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, previous_hash TEXT NOT NULL, hash TEXT NOT NULL UNIQUE);`,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (1, '2026-06-01T00:00:00Z');`,
		`INSERT INTO store_meta(key, value) VALUES ('next_client', '1'), ('next_sample', '1'), ('next_audit', '1'), ('last_hash', '');`,
	}
	for _, stmt := range legacySchema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("install legacy schema: %v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("migrate legacy audit store: %v", err)
	}
	defer store.Close()

	client, err := store.CreateClient("Migrated Audit Lab", "audit@example.test", "operator")
	if err != nil {
		t.Fatalf("create client after migration: %v", err)
	}
	if client.ID != "C-00001" {
		t.Fatalf("client ID after migration = %s", client.ID)
	}
}

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

	_, err = store.CreateClient("Atomic Lab", "atomic@example.test", "friday")
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
	client, err := store.CreateClient("Clearline Demo Lab", "qa@example.test", "friday")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Drinking Water Compliance", Matrix: "Water", Tests: []string{"pH", "Turbidity"}}, "friday")
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
	_, err = store.CreateClient("Tamper Lab", "tamper@example.test", "aegis")
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
