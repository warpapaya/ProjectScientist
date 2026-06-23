package lab

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportClientsCSVValidatesAndAuditsCreatedRows(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	payload := []byte("name,email,legacy_id\nAlpha Environmental,alpha@example.test,SENAITE-001\nBeta Labs,beta@example.test,SENAITE-002\n")
	result, err := store.ImportForScope(DefaultScope, payload, ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients, Source: "synthetic-clients.csv"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("import clients: %v", err)
	}
	if result.TotalRows != 2 || result.ValidRows != 2 || result.CreatedRows != 2 || len(result.Errors) != 0 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	clients := store.Clients()
	if len(clients) != 2 || clients[0].Name != "Alpha Environmental" || clients[1].Email != "beta@example.test" {
		t.Fatalf("unexpected imported clients: %#v", clients)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	last := events[len(events)-1]
	if last.Action != "import.completed" || last.Resource.Type != "import" || last.Details["entity"] != ImportEntityClients || last.Details["source"] != "synthetic-clients.csv" {
		t.Fatalf("missing import audit provenance: %#v", last)
	}
}

func TestImportClientsDryRunDoesNotMutateOrAuditCreates(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	beforeEvents, _ := store.AuditEvents(0)
	result, err := store.ImportForScope(DefaultScope, []byte("name,email\nDry Run Only,dry@example.test\n"), ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients, DryRun: true, Source: "dry-run.csv"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if !result.DryRun || result.CreatedRows != 0 || result.ValidRows != 1 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	if got := store.Clients(); len(got) != 0 {
		t.Fatalf("dry-run mutated clients: %#v", got)
	}
	afterEvents, _ := store.AuditEvents(0)
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("dry-run wrote audit/create events: before=%d after=%d", len(beforeEvents), len(afterEvents))
	}
}

func TestImportClientsReportsInvalidRowsWithoutPartialMutation(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.ImportForScope(DefaultScope, []byte("name,email\nValid Client,valid@example.test\n,missing-name@example.test\n"), ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients}, testActor("migration-bot"))
	if err == nil || !strings.Contains(err.Error(), "row 2") || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected row validation error, got %v", err)
	}
	if got := store.Clients(); len(got) != 0 {
		t.Fatalf("invalid import partially mutated clients: %#v", got)
	}
}

func TestImportClientsJSONXLSXAndReconciliationHook(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, err = store.CreateClient("Existing Client", "existing@example.test", testActor("seed"))
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}

	jsonPayload, _ := json.Marshal([]map[string]string{{"name": "JSON Client", "email": "json@example.test"}})
	if _, err := store.ImportForScope(DefaultScope, jsonPayload, ImportOptions{Format: ImportFormatJSON, Entity: ImportEntityClients}, testActor("migration-bot")); err != nil {
		t.Fatalf("json import: %v", err)
	}

	xlsxPayload, err := EncodeRowsXLSX([]ImportRow{{"name": "XLSX Client", "email": "xlsx@example.test"}}, []string{"name", "email"})
	if err != nil {
		t.Fatalf("encode xlsx fixture: %v", err)
	}
	result, err := store.ImportForScope(DefaultScope, xlsxPayload, ImportOptions{
		Format: ImportFormatXLSX,
		Entity: ImportEntityClients,
		Reconciler: func(row ImportRow, existing ReconciliationSnapshot) ReconciliationDecision {
			if row["name"] == "XLSX Client" && len(existing.Clients) > 0 {
				return ReconciliationDecision{Action: ReconciliationActionSkip, ExistingID: existing.Clients[0].ID, Reason: "operator matched synthetic duplicate"}
			}
			return ReconciliationDecision{Action: ReconciliationActionCreate}
		},
	}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("xlsx import with reconciliation: %v", err)
	}
	if result.SkippedRows != 1 || result.CreatedRows != 0 || result.Rows[0].ExistingID == "" {
		t.Fatalf("reconciliation hook did not safely skip row: %#v", result)
	}
	if got := store.Clients(); len(got) != 2 {
		t.Fatalf("expected existing + json client only, got %#v", got)
	}
}

func TestExportClientsFixturesAsCSVJSONAndXLSX(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, _ = store.CreateClient("Alpha Environmental", "alpha@example.test", testActor("fixture"))
	_, _ = store.CreateClient("Beta Labs", "beta@example.test", testActor("fixture"))

	csvBytes, err := store.ExportForScope(DefaultScope, ExportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients})
	if err != nil {
		t.Fatalf("csv export: %v", err)
	}
	wantCSV, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "import_export_clients.csv"))
	if err != nil {
		t.Fatalf("read csv fixture: %v", err)
	}
	if string(csvBytes) != string(wantCSV) {
		t.Fatalf("csv export did not match fixture:\n%s", csvBytes)
	}
	records, err := csv.NewReader(bytes.NewReader(csvBytes)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv export: %v", err)
	}
	if len(records) != 3 || records[0][0] != "id" || records[1][1] != "Alpha Environmental" {
		t.Fatalf("unexpected csv fixture: %#v", records)
	}

	jsonBytes, err := store.ExportForScope(DefaultScope, ExportOptions{Format: ImportFormatJSON, Entity: ImportEntityClients})
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	wantJSON, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "import_export_clients.json"))
	if err != nil {
		t.Fatalf("read json fixture: %v", err)
	}
	if string(jsonBytes) != string(wantJSON) {
		t.Fatalf("json export did not match fixture:\n%s", jsonBytes)
	}
	var rows []map[string]string
	if err := json.Unmarshal(jsonBytes, &rows); err != nil || len(rows) != 2 || rows[1]["email"] != "beta@example.test" {
		t.Fatalf("unexpected json fixture rows=%#v err=%v", rows, err)
	}

	xlsxBytes, err := store.ExportForScope(DefaultScope, ExportOptions{Format: ImportFormatXLSX, Entity: ImportEntityClients})
	if err != nil {
		t.Fatalf("xlsx export: %v", err)
	}
	parsed, err := DecodeRowsXLSX(xlsxBytes)
	if err != nil {
		t.Fatalf("parse xlsx export: %v", err)
	}
	if len(parsed) != 2 || parsed[0]["name"] != "Alpha Environmental" || parsed[1]["email"] != "beta@example.test" {
		t.Fatalf("unexpected xlsx fixture: %#v", parsed)
	}
}

func TestExportForScopeAsActorRequiresExportPermission(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, _ = store.CreateClient("Alpha Environmental", "alpha@example.test", actorWithRoles("seed-manager", RoleLabManager))

	if _, err := store.ExportForScopeAsActor(DefaultScope, ExportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients}, actorWithRoles("analyst", RoleAnalyst)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected analyst export denial, got %v", err)
	}

	csvBytes, err := store.ExportForScopeAsActor(DefaultScope, ExportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients}, actorWithRoles("manager", RoleLabManager))
	if err != nil {
		t.Fatalf("expected lab manager export allowed: %v", err)
	}
	if !strings.Contains(string(csvBytes), "Alpha Environmental") {
		t.Fatalf("authorized export missing client row: %s", csvBytes)
	}

	events, err := store.AuditEventsForScope(DefaultScope, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if !auditDeniedEventExists(events, string(OperationExportRun), "export", ImportEntityClients) {
		t.Fatalf("missing denied export audit event: %#v", events)
	}
}
