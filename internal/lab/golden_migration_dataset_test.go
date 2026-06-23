package lab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type goldenMigrationDataset struct {
	DatasetID       string                 `json:"dataset_id"`
	Version         string                 `json:"version"`
	SyntheticOnly   bool                   `json:"synthetic_only"`
	Boundary        string                 `json:"boundary"`
	FixtureFamilies []goldenFixtureFamily  `json:"fixture_families"`
	Clients         []goldenClient         `json:"clients"`
	Samples         []goldenSample         `json:"samples"`
	Analyses        []goldenAnalysis       `json:"analyses"`
	QCBatches       []goldenQCBatch        `json:"qc_batches"`
	Reports         []goldenReport         `json:"reports"`
	ExpectedGaps    []goldenParityGap      `json:"expected_parity_gaps"`
	SENAITEMapping  map[string]string      `json:"senaite_mapping"`
	MigrationChecks []goldenMigrationCheck `json:"migration_checks"`
}

type goldenFixtureFamily struct {
	ID       string   `json:"id"`
	Style    string   `json:"style"`
	Workflow []string `json:"workflow"`
}

type goldenClient struct {
	LegacyID string `json:"legacy_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	FamilyID string `json:"family_id"`
}

type goldenSample struct {
	LegacyID       string   `json:"legacy_id"`
	ClientLegacyID string   `json:"client_legacy_id"`
	FamilyID       string   `json:"family_id"`
	ClientSampleID string   `json:"client_sample_id"`
	Matrix         string   `json:"matrix"`
	Containers     []string `json:"containers"`
	Analyses       []string `json:"analyses"`
	CustodyEvents  []string `json:"custody_events"`
}

type goldenAnalysis struct {
	SampleLegacyID string `json:"sample_legacy_id"`
	Service        string `json:"service"`
	Method         string `json:"method"`
	Unit           string `json:"unit"`
	Result         string `json:"result"`
	Qualifier      string `json:"qualifier"`
	QCRole         string `json:"qc_role"`
}

type goldenQCBatch struct {
	ID       string   `json:"id"`
	FamilyID string   `json:"family_id"`
	Method   string   `json:"method"`
	Samples  []string `json:"samples"`
	Checks   []string `json:"checks"`
}

type goldenReport struct {
	ID       string   `json:"id"`
	FamilyID string   `json:"family_id"`
	Outputs  []string `json:"outputs"`
	Includes []string `json:"includes"`
}

type goldenParityGap struct {
	ID              string   `json:"id"`
	Severity        string   `json:"severity"`
	CurrentBehavior string   `json:"current_behavior"`
	RequiredFor     []string `json:"required_for"`
}

type goldenMigrationCheck struct {
	ID         string   `json:"id"`
	Assertions []string `json:"assertions"`
}

func TestGoldenMigrationDatasetDefinesThreeSyntheticWorkflowFamilies(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	if dataset.DatasetID != "psc-rm-073-golden-migration-v1" {
		t.Fatalf("unexpected dataset id %q", dataset.DatasetID)
	}
	if !dataset.SyntheticOnly || !strings.Contains(strings.ToLower(dataset.Boundary), "lab-test") {
		t.Fatalf("golden dataset must be synthetic-only and lab-test bounded: %#v", dataset)
	}
	families := map[string]bool{}
	for _, family := range dataset.FixtureFamilies {
		families[family.ID] = len(family.Workflow) >= 4 && strings.TrimSpace(family.Style) != ""
	}
	for _, id := range []string{"precast-industrial", "municipal-water", "materials-forensics"} {
		if !families[id] {
			t.Fatalf("missing workflow family %s with style and executable workflow steps", id)
		}
	}
	if len(dataset.Clients) != 3 {
		t.Fatalf("expected one synthetic client per workflow family, got %d", len(dataset.Clients))
	}
}

func TestGoldenMigrationDatasetContainsNoCustomerSensitiveIdentifiers(t *testing.T) {
	raw := string(readGoldenMigrationDataset(t))
	for _, forbidden := range []string{"Tindall", "CENLA", "RJ Lee", "AmSpec", "Krishna", "@clearlinelims.com", "@tindall", "@cenla", "@rjlee"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(forbidden)) {
			t.Fatalf("golden fixture contains forbidden customer-sensitive identifier %q", forbidden)
		}
	}
	dataset := loadGoldenMigrationDataset(t)
	for _, client := range dataset.Clients {
		if !strings.HasSuffix(client.Email, "@example.test") && !strings.HasSuffix(client.Email, ".example.test") {
			t.Fatalf("client email must use reserved example.test domain: %#v", client)
		}
		if !strings.HasPrefix(client.LegacyID, "SYN-") {
			t.Fatalf("legacy ids must be synthetic: %#v", client)
		}
	}
}

func TestGoldenMigrationDatasetExercisesMigrationImportAndReconciliation(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	rows := make([]ImportRow, 0, len(dataset.Clients))
	for _, client := range dataset.Clients {
		rows = append(rows, ImportRow{"legacy_id": client.LegacyID, "name": client.Name, "email": client.Email, "family_id": client.FamilyID})
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal client rows: %v", err)
	}
	result, err := store.ImportForScope(DefaultScope, payload, ImportOptions{Format: ImportFormatJSON, Entity: ImportEntityClients, Source: "fixtures/golden_migration_dataset.json"}, testActor("migration-bot"))
	if err != nil {
		t.Fatalf("import golden clients: %v", err)
	}
	if result.TotalRows != 3 || result.CreatedRows != 3 || result.SkippedRows != 0 {
		t.Fatalf("unexpected import result: %#v", result)
	}
	report, err := store.ClientImportReconciliationReportForScope(DefaultScope, result, testActor("migration-reviewer"))
	if err != nil {
		t.Fatalf("reconcile golden clients: %v", err)
	}
	if report.SourceCount != 3 || report.ImportedCount != 3 || report.MatchedCount != 3 || len(report.MissingRecords) != 0 || len(report.MismatchedRecords) != 0 {
		t.Fatalf("golden client reconciliation should be clean: %#v", report)
	}
}

func TestGoldenMigrationDatasetDocumentsParityGapsAndMigrationChecks(t *testing.T) {
	dataset := loadGoldenMigrationDataset(t)
	for _, key := range []string{"Client", "Contact", "Sample", "AnalysisRequest", "AnalysisService", "Worksheet", "QC", "Report", "ChainOfCustody"} {
		if strings.TrimSpace(dataset.SENAITEMapping[key]) == "" {
			t.Fatalf("missing SENAITE mapping for %s", key)
		}
	}
	if len(dataset.ExpectedGaps) < 5 {
		t.Fatalf("expected at least five parity gaps, got %d", len(dataset.ExpectedGaps))
	}
	if len(dataset.MigrationChecks) < 4 {
		t.Fatalf("expected executable migration checks, got %d", len(dataset.MigrationChecks))
	}
	for _, check := range dataset.MigrationChecks {
		if len(check.Assertions) == 0 {
			t.Fatalf("migration check %s has no assertions", check.ID)
		}
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "golden-migration-datasets.md"))
	if err != nil {
		t.Fatalf("read golden dataset doc: %v", err)
	}
	if !strings.Contains(string(doc), dataset.DatasetID) || !strings.Contains(string(doc), "Expected parity gaps") {
		t.Fatalf("golden dataset doc does not describe dataset and parity gaps")
	}
}

func loadGoldenMigrationDataset(t *testing.T) goldenMigrationDataset {
	t.Helper()
	var dataset goldenMigrationDataset
	if err := json.Unmarshal(readGoldenMigrationDataset(t), &dataset); err != nil {
		t.Fatalf("parse golden dataset: %v", err)
	}
	return dataset
}

func readGoldenMigrationDataset(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "golden_migration_dataset.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden dataset %s: %v", path, err)
	}
	return raw
}
