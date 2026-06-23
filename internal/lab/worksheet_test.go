package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorksheetGroupsLinesByMethodDepartmentBatchAndAnalyst(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	lines := createWorksheetLineFixtures(t, store, actor, "EPA 200.8", "Metals", "Lead", "Copper")

	worksheet, err := store.CreateWorksheet(CreateWorksheetInput{AnalysisRequestLineIDs: []string{lines[0].ID, lines[1].ID}, BatchID: "BATCH-2026-001", AnalystID: "analyst-jane"}, actor)
	if err != nil {
		t.Fatalf("create worksheet: %v", err)
	}
	if worksheet.BatchID != "BATCH-2026-001" || worksheet.AnalystID != "analyst-jane" {
		t.Fatalf("worksheet missing batch/analyst assignment: %#v", worksheet)
	}
	if worksheet.DepartmentName != "Metals" || worksheet.MethodName != "EPA 200.8" {
		t.Fatalf("worksheet did not inherit method/department grouping: %#v", worksheet)
	}
	if worksheet.Status != WorksheetStatusOpen {
		t.Fatalf("new worksheet status got %q want %q", worksheet.Status, WorksheetStatusOpen)
	}
	if len(worksheet.Lines) != 2 {
		t.Fatalf("expected 2 worksheet lines, got %d: %#v", len(worksheet.Lines), worksheet.Lines)
	}
	for _, line := range lines {
		loaded, ok := store.GetAnalysisRequestLine(line.ID)
		if !ok {
			t.Fatalf("load line %s", line.ID)
		}
		if loaded.Status != AnalysisRequestLineStatusInProgress {
			t.Fatalf("assigned line %s status got %q want %q", loaded.ID, loaded.Status, AnalysisRequestLineStatusInProgress)
		}
	}

	if err := store.AssignWorksheetAnalyst(worksheet.ID, "analyst-mike", actor); err != nil {
		t.Fatalf("assign analyst: %v", err)
	}
	loaded, ok := store.GetWorksheet(worksheet.ID)
	if !ok {
		t.Fatalf("load worksheet %s", worksheet.ID)
	}
	if loaded.AnalystID != "analyst-mike" {
		t.Fatalf("analyst assignment got %q want analyst-mike", loaded.AnalystID)
	}
}

func TestWorksheetRejectsMixedMethodDepartmentGroups(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	lead := createWorksheetLineFixtures(t, store, actor, "EPA 200.8", "Metals", "Lead")[0]
	tds := createWorksheetLineFixtures(t, store, actor, "SM 2540 C", "Wet Chem", "TDS")[0]

	_, err = store.CreateWorksheet(CreateWorksheetInput{AnalysisRequestLineIDs: []string{lead.ID, tds.ID}, BatchID: "BATCH-MIXED", AnalystID: "analyst-jane"}, actor)
	if err == nil || !strings.Contains(err.Error(), "same method and department") {
		t.Fatalf("expected mixed method/department rejection, got %v", err)
	}
}

func TestWorksheetLineRemovalAndStatusTransitions(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	lines := createWorksheetLineFixtures(t, store, actor, "EPA 200.8", "Metals", "Lead", "Copper")
	worksheet, err := store.CreateWorksheet(CreateWorksheetInput{AnalysisRequestLineIDs: []string{lines[0].ID, lines[1].ID}, BatchID: "BATCH-REMOVE", AnalystID: "analyst-jane"}, actor)
	if err != nil {
		t.Fatalf("create worksheet: %v", err)
	}

	if err := store.RemoveWorksheetLine(worksheet.ID, lines[1].ID, actor); err != nil {
		t.Fatalf("remove worksheet line: %v", err)
	}
	loaded, ok := store.GetWorksheet(worksheet.ID)
	if !ok {
		t.Fatalf("load worksheet %s", worksheet.ID)
	}
	if len(loaded.Lines) != 1 || loaded.Lines[0].ID != lines[0].ID {
		t.Fatalf("worksheet lines after removal: %#v", loaded.Lines)
	}
	removed, ok := store.GetAnalysisRequestLine(lines[1].ID)
	if !ok {
		t.Fatalf("load removed line")
	}
	if removed.Status != AnalysisRequestLineStatusRequested {
		t.Fatalf("removed line status got %q want requested", removed.Status)
	}

	if err := store.TransitionWorksheet(worksheet.ID, WorksheetStatusInProgress, actor); err != nil {
		t.Fatalf("open -> in_progress: %v", err)
	}
	if err := store.TransitionWorksheet(worksheet.ID, WorksheetStatusCompleted, actor); err != nil {
		t.Fatalf("in_progress -> completed: %v", err)
	}
	completed, ok := store.GetWorksheet(worksheet.ID)
	if !ok || completed.Status != WorksheetStatusCompleted {
		t.Fatalf("completed worksheet got ok=%v %#v", ok, completed)
	}
	remaining, _ := store.GetAnalysisRequestLine(lines[0].ID)
	if remaining.Status != AnalysisRequestLineStatusCompleted {
		t.Fatalf("completed worksheet line status got %q want completed", remaining.Status)
	}
	if err := store.RemoveWorksheetLine(worksheet.ID, lines[0].ID, actor); err == nil || !strings.Contains(err.Error(), "completed worksheet") {
		t.Fatalf("expected completed worksheet removal denial, got %v", err)
	}
}

func createWorksheetLineFixtures(t *testing.T, store *Store, actor ActorContext, methodName, departmentName string, serviceNames ...string) []AnalysisRequestLine {
	t.Helper()
	client, err := store.CreateClient(departmentName+" Client", strings.ToLower(strings.ReplaceAll(departmentName, " ", "-"))+"@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	dept, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: departmentName, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}
	method, err := store.CreateCatalogMethod(CatalogMethodInput{Name: methodName}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	serviceIDs := make([]string, 0, len(serviceNames))
	for i, name := range serviceNames {
		service, err := store.CreateAnalysisService(AnalysisServiceInput{Name: name, DepartmentID: dept.ID, MethodID: method.ID, SortOrder: i + 1}, actor)
		if err != nil {
			t.Fatalf("create service %s: %v", name, err)
		}
		serviceIDs = append(serviceIDs, service.ID)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: departmentName + " Batch", Matrix: "Water", AnalysisServiceIDs: serviceIDs}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != len(serviceNames) {
		t.Fatalf("expected %d lines, got %d", len(serviceNames), len(lines))
	}
	return lines
}
