package lab

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteStoreMigratesLegacyStoreBeforeCreatingScopedIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "project-scientist.db")
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE store_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE clients (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL CHECK (length(trim(name)) > 0),
			email TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE samples (
			id TEXT PRIMARY KEY,
			client_id TEXT NOT NULL REFERENCES clients(id),
			project TEXT NOT NULL CHECK (length(trim(project)) > 0),
			matrix TEXT NOT NULL,
			status TEXT NOT NULL,
			analyses_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			details_json TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			previous_hash TEXT NOT NULL,
			hash TEXT NOT NULL
		)`,
		`CREATE TABLE audit_checkpoints (
			name TEXT PRIMARY KEY,
			sequence INTEGER NOT NULL,
			hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO store_meta(key, value) VALUES ('next_client', '1'), ('next_sample', '1'), ('next_audit', '1'), ('last_hash', '')`,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (1, '2026-06-22T00:00:00.000Z')`,
	} {
		if _, err := legacy.Exec(stmt); err != nil {
			t.Fatalf("apply legacy stmt %q: %v", stmt, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open migrated legacy store: %v", err)
	}
	defer store.Close()

	for _, table := range []string{"clients", "samples", "audit_events"} {
		columns, err := tableColumns(t.Context(), store.db, table)
		if err != nil {
			t.Fatalf("inspect %s columns: %v", table, err)
		}
		if !columns["tenant_id"] || !columns["lab_id"] {
			t.Fatalf("%s missing scoped columns after migration: %#v", table, columns)
		}
	}
	if _, err := store.CreateClient("Migrated Legacy Lab", "legacy@example.test", demoSeedActorForTest()); err != nil {
		t.Fatalf("expected migrated legacy audit schema to accept new audit writes: %v", err)
	}
}
