package lab

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenDatasetSampleCustodyRowsImportReconcileAndAudit(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActor("golden-sample-migration")
	clientRows := make([]ImportRow, 0, len(dataset.Clients))
	for _, client := range dataset.Clients {
		clientRows = append(clientRows, ImportRow{"legacy_id": client.LegacyID, "name": client.Name, "email": client.Email, "family_id": client.FamilyID})
	}
	clientPayload, err := json.Marshal(clientRows)
	if err != nil {
		t.Fatalf("marshal client rows: %v", err)
	}
	clientResult, err := store.ImportForScope(DefaultScope, clientPayload, ImportOptions{Format: ImportFormatJSON, Entity: ImportEntityClients, Source: "fixtures/golden_migration_dataset.json#clients"}, actor)
	if err != nil {
		t.Fatalf("import clients: %v", err)
	}
	clientIDs := map[string]string{}
	for _, row := range clientResult.Rows {
		clientIDs[row.Data["legacy_id"]] = row.ID
	}

	sampleRows := make([]ImportRow, 0, len(dataset.Samples))
	for _, sample := range dataset.Samples {
		if strings.TrimSpace(sample.Preservation) == "" {
			t.Fatalf("golden sample %s must declare preservation expectation", sample.LegacyID)
		}
		if strings.TrimSpace(sample.ReceivedCondition) == "" {
			t.Fatalf("golden sample %s must declare received condition expectation", sample.LegacyID)
		}
		containersJSON, err := json.Marshal(sample.Containers)
		if err != nil {
			t.Fatalf("marshal containers: %v", err)
		}
		analysesJSON, err := json.Marshal(sample.Analyses)
		if err != nil {
			t.Fatalf("marshal analyses: %v", err)
		}
		custodyJSON, err := json.Marshal(sample.CustodyEvents)
		if err != nil {
			t.Fatalf("marshal custody events: %v", err)
		}
		sampleRows = append(sampleRows, ImportRow{
			"legacy_id":           sample.LegacyID,
			"client_legacy_id":    sample.ClientLegacyID,
			"client_id":           clientIDs[sample.ClientLegacyID],
			"family_id":           sample.FamilyID,
			"client_sample_id":    sample.ClientSampleID,
			"lab_sample_id":       sample.LegacyID,
			"matrix":              sample.Matrix,
			"preservation":        sample.Preservation,
			"received_condition":  sample.ReceivedCondition,
			"containers_json":     string(containersJSON),
			"analyses_json":       string(analysesJSON),
			"custody_events_json": string(custodyJSON),
		})
	}
	samplePayload, err := json.Marshal(sampleRows)
	if err != nil {
		t.Fatalf("marshal sample rows: %v", err)
	}

	result, err := store.ImportForScope(DefaultScope, samplePayload, ImportOptions{Format: ImportFormatJSON, Entity: ImportEntitySamples, Source: "fixtures/golden_migration_dataset.json#samples"}, actor)
	if err != nil {
		t.Fatalf("import golden samples/custody: %v", err)
	}
	if result.TotalRows != len(dataset.Samples) || result.CreatedRows != len(dataset.Samples) || result.SkippedRows != 0 {
		t.Fatalf("unexpected sample import result: %#v", result)
	}

	samples := store.SamplesForScope(DefaultScope)
	if len(samples) != len(dataset.Samples) {
		t.Fatalf("expected %d imported samples, got %d", len(dataset.Samples), len(samples))
	}
	byLabID := map[string]Sample{}
	for _, sample := range samples {
		byLabID[sample.LabSampleID] = sample
	}
	for _, expected := range dataset.Samples {
		imported, ok := byLabID[expected.LegacyID]
		if !ok {
			t.Fatalf("missing imported sample for legacy/lab id %s", expected.LegacyID)
		}
		if imported.ClientSampleID != expected.ClientSampleID || imported.Matrix != expected.Matrix {
			t.Fatalf("sample metadata mismatch for %s: %#v", expected.LegacyID, imported)
		}
		if len(imported.Containers) != len(expected.Containers) {
			t.Fatalf("container count mismatch for %s: got %d want %d", expected.LegacyID, len(imported.Containers), len(expected.Containers))
		}
		if imported.ReceivedConditionID == "" {
			t.Fatalf("received condition not preserved for %s", expected.LegacyID)
		}
		if len(imported.Analyses) != len(expected.Analyses) {
			t.Fatalf("analysis expectation count mismatch for %s: got %d want %d", expected.LegacyID, len(imported.Analyses), len(expected.Analyses))
		}
		if len(imported.CustodyEvents) != len(expected.CustodyEvents) {
			t.Fatalf("custody event count mismatch for %s: got %d want %d", expected.LegacyID, len(imported.CustodyEvents), len(expected.CustodyEvents))
		}
		for i, event := range imported.CustodyEvents {
			if event.Sequence != int64(i+1) || !strings.Contains(event.Reason, expected.CustodyEvents[i]) {
				t.Fatalf("custody event %d mismatch for %s: %#v", i, expected.LegacyID, event)
			}
		}
	}

	report, err := store.SampleImportReconciliationReportForScope(DefaultScope, result, actor)
	if err != nil {
		t.Fatalf("reconcile samples: %v", err)
	}
	if report.SourceCount != len(dataset.Samples) || report.ImportedCount != len(dataset.Samples) || report.MatchedCount != len(dataset.Samples) || len(report.MissingRecords) != 0 || len(report.MismatchedRecords) != 0 {
		t.Fatalf("golden sample/custody reconciliation should be clean: %#v", report)
	}
	if report.AuditEventID == "" || report.SourceHash == "" || report.ImportedHash == "" {
		t.Fatalf("reconciliation should emit audit evidence and hashes: %#v", report)
	}
	events, err := store.AuditEventsForScope(DefaultScope, 0)
	if err != nil {
		t.Fatalf("read audit events: %v", err)
	}
	assertAuditActionPresent(t, events, "sample.imported")
	assertAuditActionPresent(t, events, "sample.custody.recorded")
	assertAuditActionPresent(t, events, "import.reconciliation_reported")
}

func assertAuditActionPresent(t *testing.T, events []AuditEvent, action string) {
	t.Helper()
	for _, event := range events {
		if event.Action == action {
			return
		}
	}
	t.Fatalf("missing audit action %s in %#v", action, events)
}
