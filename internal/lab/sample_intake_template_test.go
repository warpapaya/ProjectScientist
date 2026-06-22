package lab

import (
	"path/filepath"
	"testing"
)

func TestSampleIntakeTemplateCreatesRepeatableBulkSamples(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	client, err := store.CreateClient("Bulk Intake Client", "bulk@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	project, err := store.CreateProjectForScope(DefaultScope, ProjectInput{ClientID: client.ID, Name: "Monthly Compliance", WorkOrder: "WO-BULK", DefaultMatrix: "Fallback Matrix", DefaultTests: []string{"fallback"}}, actor)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	matrix, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceMatrix, Code: "DW", Name: "Drinking Water", Active: true}, actor)
	container, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	condition, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceReceivedCondition, Code: "ICE", Name: "Received on ice", Active: true}, actor)
	dept, _ := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Wet Chem"}, actor)
	ph, _ := store.CreateAnalysisService(AnalysisServiceInput{Name: "pH", DepartmentID: dept.ID}, actor)
	turbidity, _ := store.CreateAnalysisService(AnalysisServiceInput{Name: "Turbidity", DepartmentID: dept.ID}, actor)
	profile, _ := store.CreateAnalysisProfile(AnalysisProfileInput{Name: "Routine DW", ServiceIDs: []string{ph.ID, turbidity.ID}}, actor)

	template, err := store.CreateSampleIntakeTemplate(SampleIntakeTemplateInput{
		Name:                "Monthly DW 3-pack",
		ClientID:            client.ID,
		ProjectID:           project.ID,
		MatrixReferenceID:   matrix.ID,
		ContainerID:         container.ID,
		ReceivedConditionID: condition.ID,
		Priority:            PriorityRush,
		AnalysisProfileIDs:  []string{profile.ID},
	}, actor)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	created, err := store.CreateSamplesFromTemplate(template.ID, []SampleTemplateRowInput{
		{ClientSampleID: "FIELD-001", LabSampleID: "PSL-001", Comments: "north tap"},
		{ClientSampleID: "FIELD-002", LabSampleID: "PSL-002", Comments: "south tap"},
		{ClientSampleID: "FIELD-003", LabSampleID: "PSL-003", MatrixReferenceID: matrix.ID},
	}, actor)
	if err != nil {
		t.Fatalf("create samples from template: %v", err)
	}
	if got, want := len(created), 3; got != want {
		t.Fatalf("created %d samples, want %d", got, want)
	}
	for i, sample := range created {
		if sample.ClientID != client.ID || sample.ProjectID != project.ID || sample.Project != project.Name {
			t.Fatalf("sample %d lost client/project template fields: %#v", i, sample)
		}
		if sample.Matrix != "Drinking Water" || sample.ContainerID != container.ID || sample.ReceivedConditionID != condition.ID || sample.Priority != PriorityRush {
			t.Fatalf("sample %d lost reference/priority template fields: %#v", i, sample)
		}
		if got, want := analysisNames(sample.Analyses), []string{"pH", "Turbidity"}; !sameStrings(got, want) {
			t.Fatalf("sample %d analyses got %v want %v", i, got, want)
		}
	}
	if created[0].Comments != "north tap" || created[1].ClientSampleID != "FIELD-002" || created[2].LabSampleID != "PSL-003" {
		t.Fatalf("row overrides not preserved: %#v", created)
	}
}
