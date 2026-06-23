package lab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCustomerWorkflowSmokeMatrixCreatesBoundedSyntheticResults(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	outDir := t.TempDir()
	matrix, err := store.GenerateCustomerWorkflowSmokeMatrix(CustomerWorkflowSmokeInput{
		FixturePath:   filepath.Join("..", "..", "fixtures", "golden_migration_dataset.json"),
		GapReportPath: filepath.Join("..", "..", "docs", "customer-workflow-gap-report.md"),
		OutputDir:     outDir,
		CommandOutput: "go test ./... && docker compose up project-scientist && project-scientist smoke --base-url http://127.0.0.1:8097",
	}, testActor("customer-smoke-bot"))
	if err != nil {
		t.Fatalf("generate smoke matrix: %v", err)
	}

	if matrix.DatasetID != "psc-rm-073-golden-migration-v1" || !matrix.SyntheticOnly {
		t.Fatalf("matrix must be bound to the synthetic golden dataset: %#v", matrix)
	}
	if !strings.Contains(strings.ToLower(matrix.Boundary), "not production") || strings.Contains(strings.ToLower(matrix.Boundary), "production-ready") {
		t.Fatalf("matrix boundary must prohibit production/customer-facing claims without production-ready language: %q", matrix.Boundary)
	}
	if matrix.MatrixArtifactPath == "" {
		t.Fatalf("matrix artifact path missing")
	}
	if _, err := os.Stat(matrix.MatrixArtifactPath); err != nil {
		t.Fatalf("matrix artifact not written: %v", err)
	}

	if len(matrix.Lanes) != 3 {
		t.Fatalf("expected three customer workflow lanes, got %d", len(matrix.Lanes))
	}
	wantLabels := map[string]string{
		"Tindall/precast-industrial": "precast-industrial",
		"CENLA/municipal-water":      "municipal-water",
		"RJ Lee/materials-forensics": "materials-forensics",
	}
	seenStatuses := map[SmokeStatus]bool{}
	for _, lane := range matrix.Lanes {
		familyID, ok := wantLabels[lane.Label]
		if !ok {
			t.Fatalf("unexpected lane label %q", lane.Label)
		}
		if lane.FamilyID != familyID {
			t.Fatalf("lane %q bound to family %q, want %q", lane.Label, lane.FamilyID, familyID)
		}
		if len(lane.Checks) == 0 || len(lane.RemainingGaps) == 0 {
			t.Fatalf("lane %q must include checks and remaining gaps: %#v", lane.Label, lane)
		}
		if lane.CommandOutput == "" {
			t.Fatalf("lane %q missing command output evidence", lane.Label)
		}
		if lane.ReportPackage.ArtifactPath == "" {
			t.Fatalf("lane %q missing report package artifact path", lane.Label)
		}
		content, err := os.ReadFile(lane.ReportPackage.ArtifactPath)
		if err != nil {
			t.Fatalf("read package artifact for %q: %v", lane.Label, err)
		}
		for _, must := range []string{lane.FamilyID, lane.ReportPackage.ReportID, "STATIC/SCRIPTED", "not production evidence"} {
			if !strings.Contains(string(content), must) {
				t.Fatalf("package artifact for %q missing %q: %s", lane.Label, must, string(content))
			}
		}
		if lane.ReportPackage.Binding != ReportPackageBindingStaticScripted {
			t.Fatalf("report package for %q must be explicitly static/scripted until migrated sample/result/QC/custody import exists: %#v", lane.Label, lane.ReportPackage)
		}
		for _, check := range lane.Checks {
			seenStatuses[check.Status] = true
			if check.ArtifactPath == "" && check.Status == SmokeStatusGreen {
				t.Fatalf("green check %q for %q needs artifact evidence", check.Name, lane.Label)
			}
		}
	}
	for _, status := range []SmokeStatus{SmokeStatusGreen, SmokeStatusYellow, SmokeStatusRed} {
		if !seenStatuses[status] {
			t.Fatalf("expected matrix to include %s checks, got %#v", status, seenStatuses)
		}
	}
}

func TestGenerateCustomerWorkflowSmokeMatrixRejectsUnsafeCustomerClaims(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.GenerateCustomerWorkflowSmokeMatrix(CustomerWorkflowSmokeInput{
		FixturePath:   filepath.Join("..", "..", "fixtures", "golden_migration_dataset.json"),
		OutputDir:     t.TempDir(),
		CommandOutput: "production-ready customer pilot approved",
	}, testActor("customer-smoke-bot"))
	if err == nil || !strings.Contains(err.Error(), "production-ready") {
		t.Fatalf("expected unsafe production readiness language to be rejected, got %v", err)
	}
}
