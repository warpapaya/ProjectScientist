package lab

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestAppendAuditTxSanitizesCallerProvidedDetails(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	err = store.withTx(func(tx *sql.Tx) error {
		return appendAuditTx(tx, auditWrite{
			Scope:    DefaultScope,
			Actor:    testActor("migration-bot"),
			Action:   "client.imported",
			Outcome:  AuditOutcomeAllowed,
			Resource: AuditResource{Type: "client", ID: "C-00001"},
			Details: map[string]any{
				"source":      "synthetic-clients.csv",
				"row":         7,
				"source_hash": "sha256:0123456789abcdef",
				"mapping_id":  "map-client-v1",
				"legacy_id":   "LEGACY-CUSTOMER-123",
				"name":        "Alpha Environmental",
				"raw_payload": map[string]any{"name": "Alpha Environmental", "api_key": "sk_live_123456789"},
				"token":       "Bearer super-secret-token",
			},
		})
	})
	if err != nil {
		t.Fatalf("append audit: %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	got := events[len(events)-1].Details
	for _, key := range []string{"source", "row", "source_hash", "mapping_id"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("expected safe provenance key %q to be preserved: %#v", key, got)
		}
	}
	for _, key := range []string{"legacy_id", "name", "raw_payload", "token"} {
		if _, ok := got[key]; ok {
			t.Fatalf("unsafe audit detail key %q was persisted: %#v", key, got)
		}
	}
}

func TestImportClientAuditPreservesProvenanceWithoutCustomerPayload(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	payload := []byte("name,email,legacy_id,api_token\nAlpha Environmental,alpha@example.test,SENAITE-001,sk_live_customer_secret\n")
	_, err = store.ImportForScope(DefaultScope, payload, ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients, Source: "synthetic-clients.csv?api_key=sk_live_customer_secret"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("import clients: %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var created AuditEvent
	for _, event := range events {
		if event.Action == "client.imported" {
			created = event
			break
		}
	}
	if created.EventID == "" {
		t.Fatalf("client.imported event not found: %#v", events)
	}
	if created.Details["row"] != float64(1) || created.Details["source_hash"] == "" || created.Details["source"] == "" {
		t.Fatalf("expected safe import provenance in audit details: %#v", created.Details)
	}
	for _, key := range []string{"name", "legacy_id", "email", "api_token", "raw_payload"} {
		if _, ok := created.Details[key]; ok {
			t.Fatalf("customer payload key %q was persisted in import audit details: %#v", key, created.Details)
		}
	}
	if created.Details["source"] == "synthetic-clients.csv?api_key=sk_live_customer_secret" {
		t.Fatalf("secret-like source value was persisted without minimization: %#v", created.Details)
	}
}
