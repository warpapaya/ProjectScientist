package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestClientImportReconciliationReportFindsMissingExtraAndMismatchedRecords(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	result, err := store.ImportForScope(DefaultScope, []byte("legacy_id,name,email\nSRC-001,Alpha Environmental,alpha@example.test\nSRC-002,Beta Labs,beta@example.test\n"), ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients, Source: "senaite-clients.csv"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("import clients: %v", err)
	}
	if err := deleteClientForReconciliationTest(store, result.Rows[0].ID); err != nil {
		t.Fatalf("delete imported client fixture: %v", err)
	}
	if err := updateClientForReconciliationTest(store, result.Rows[1].ID, "Beta Labs", "changed@example.test"); err != nil {
		t.Fatalf("update imported client fixture: %v", err)
	}
	extra, err := store.CreateClient("Unexpected Extra", "extra@example.test", testActor("fixture"))
	if err != nil {
		t.Fatalf("create extra client: %v", err)
	}

	report, err := store.ClientImportReconciliationReportForScope(DefaultScope, result, testActor("reconciliation-bot"))
	if err != nil {
		t.Fatalf("reconciliation report: %v", err)
	}

	if report.Provenance.Source != "senaite-clients.csv" || report.Provenance.Entity != ImportEntityClients || report.Provenance.Format != ImportFormatCSV || report.Provenance.GeneratedBy != "reconciliation-bot" {
		t.Fatalf("missing source/entity/actor provenance: %#v", report.Provenance)
	}
	if report.SourceCount != 2 || report.ImportedCount != 2 || report.MatchedCount != 0 {
		t.Fatalf("unexpected counts: %#v", report)
	}
	if len(report.MissingRecords) != 1 || report.MissingRecords[0].SourceID != "SRC-001" || report.MissingRecords[0].ImportedID != result.Rows[0].ID {
		t.Fatalf("missing record not reported: %#v", report.MissingRecords)
	}
	if len(report.MismatchedRecords) != 1 || report.MismatchedRecords[0].SourceID != "SRC-002" || report.MismatchedRecords[0].FieldDiffs["email"].Imported != "changed@example.test" {
		t.Fatalf("mismatched record not reported: %#v", report.MismatchedRecords)
	}
	if len(report.ExtraRecords) != 1 || report.ExtraRecords[0].ImportedID != extra.ID {
		t.Fatalf("extra record not reported: %#v", report.ExtraRecords)
	}
	if report.SourceHash == "" || report.ImportedHash == "" || report.AuditEventID == "" {
		t.Fatalf("missing hashes/audit id: %#v", report)
	}
}

func deleteClientForReconciliationTest(store *Store, id string) error {
	_, err := store.db.Exec(`DELETE FROM clients WHERE id = ?`, id)
	return err
}

func updateClientForReconciliationTest(store *Store, id, name, email string) error {
	_, err := store.db.Exec(`UPDATE clients SET name = ?, email = ? WHERE id = ?`, name, email, id)
	return err
}

func TestClientImportReconciliationReportRendersReadableMarkdownAndAuditsProvenance(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	result, err := store.ImportForScope(DefaultScope, []byte("legacy_id,name,email\nSRC-001,Alpha Environmental,alpha@example.test\n"), ImportOptions{Format: ImportFormatCSV, Entity: ImportEntityClients, Source: "senaite-clients.csv"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("import clients: %v", err)
	}

	report, err := store.ClientImportReconciliationReportForScope(DefaultScope, result, testActor("reconciliation-bot"))
	if err != nil {
		t.Fatalf("reconciliation report: %v", err)
	}
	markdown := report.Markdown()
	for _, want := range []string{"# Import Reconciliation Report", "Source: senaite-clients.csv", "Entity: clients", "Matched: 1", "Source SHA-256:", "Imported SHA-256:", "Audit event:"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}

	events, err := store.AuditEventsForScope(DefaultScope, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	last := events[len(events)-1]
	if last.EventID != report.AuditEventID || last.Action != "import.reconciliation_reported" || last.Resource.Type != "import_reconciliation" || last.Details["source_hash"] != report.SourceHash {
		t.Fatalf("missing reconciliation audit provenance: event=%#v report=%#v", last, report)
	}
}
