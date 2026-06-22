package lab

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSampleIntakeV2AppliesConfiguredClientProjectReferenceAndCatalog(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	client, err := store.CreateClient("Alpha Environmental", "alpha@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := store.UpsertClientDefaultsForScope(DefaultScope, ClientDefaultsInput{ClientID: client.ID, DefaultMatrix: "Drinking Water", DefaultTests: []string{"client-default-ignored-by-project"}}, actor); err != nil {
		t.Fatalf("client defaults: %v", err)
	}
	project, err := store.CreateProjectForScope(DefaultScope, ProjectInput{ClientID: client.ID, Name: "Q3 Compliance", WorkOrder: "WO-2026-030", DefaultMatrix: "Wastewater", DefaultTests: []string{"project-default-ignored-by-profile"}}, actor)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	matrix, err := store.CreateSampleReferenceItemForScope(DefaultScope, SampleReferenceItemInput{Kind: SampleReferenceMatrix, Code: "DW", Name: "Drinking Water", Active: true}, actor)
	if err != nil {
		t.Fatalf("matrix reference: %v", err)
	}
	container, err := store.CreateSampleReferenceItemForScope(DefaultScope, SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	if err != nil {
		t.Fatalf("container reference: %v", err)
	}
	preservative, err := store.CreateSampleReferenceItemForScope(DefaultScope, SampleReferenceItemInput{Kind: SampleReferencePreservative, Code: "HNO3", Name: "Nitric Acid", Active: true}, actor)
	if err != nil {
		t.Fatalf("preservative reference: %v", err)
	}
	storage, err := store.CreateSampleReferenceItemForScope(DefaultScope, SampleReferenceItemInput{Kind: SampleReferenceStorageLocation, Code: "FR1", Name: "Receiving Fridge", Active: true}, actor)
	if err != nil {
		t.Fatalf("storage reference: %v", err)
	}
	receivedCondition, err := store.CreateSampleReferenceItemForScope(DefaultScope, SampleReferenceItemInput{Kind: SampleReferenceReceivedCondition, Code: "OK", Name: "Received on ice", Active: true}, actor)
	if err != nil {
		t.Fatalf("received condition reference: %v", err)
	}

	dept, _ := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Wet Chem", SortOrder: 1}, actor)
	unit, _ := store.CreateCatalogUnit(CatalogUnitInput{Name: "Standard units", Symbol: "SU"}, actor)
	method, _ := store.CreateCatalogMethod(CatalogMethodInput{Name: "SM 4500-H+"}, actor)
	phAnalyte, _ := store.CreateCatalogAnalyte(CatalogAnalyteInput{Name: "pH", DefaultUnitID: unit.ID}, actor)
	ph, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "pH", DepartmentID: dept.ID, MethodID: method.ID, AnalyteID: phAnalyte.ID, UnitID: unit.ID, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create pH service: %v", err)
	}
	turbidity, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Turbidity", DepartmentID: dept.ID, SortOrder: 2}, actor)
	if err != nil {
		t.Fatalf("create turbidity service: %v", err)
	}
	profile, err := store.CreateAnalysisProfile(AnalysisProfileInput{Name: "Routine Water", ServiceIDs: []string{ph.ID, turbidity.ID}}, actor)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	sampledAt := time.Date(2026, 6, 20, 14, 30, 0, 0, time.UTC)
	receivedAt := time.Date(2026, 6, 21, 8, 15, 0, 0, time.UTC)
	sample, err := store.CreateSample(CreateSampleInput{
		ClientID:            client.ID,
		ProjectID:           project.ID,
		ClientSampleID:      "ALPHA-001",
		LabSampleID:         "PSL-2026-0001",
		MatrixReferenceID:   matrix.ID,
		ContainerID:         container.ID,
		PreservativeID:      preservative.ID,
		StorageLocationID:   storage.ID,
		ReceivedConditionID: receivedCondition.ID,
		SampledAt:           sampledAt,
		ReceivedAt:          receivedAt,
		Priority:            PriorityRush,
		Comments:            "low-click v2 intake",
		AnalysisProfileIDs:  []string{profile.ID},
		AnalysisServiceIDs:  []string{turbidity.ID},
	}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	if sample.ProjectID != project.ID || sample.Project != project.Name {
		t.Fatalf("sample should link project and expose project name: %#v", sample)
	}
	if sample.ClientSampleID != "ALPHA-001" || sample.LabSampleID != "PSL-2026-0001" || sample.Priority != PriorityRush || sample.Comments != "low-click v2 intake" {
		t.Fatalf("sample missing intake identifiers/priority/comments: %#v", sample)
	}
	if !sample.SampledAt.Equal(sampledAt) || !sample.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("sample dates not persisted: sampled=%s received=%s", sample.SampledAt, sample.ReceivedAt)
	}
	if sample.Matrix != "Drinking Water" || sample.MatrixReferenceID != matrix.ID || sample.ContainerID != container.ID || sample.PreservativeID != preservative.ID || sample.StorageLocationID != storage.ID || sample.ReceivedConditionID != receivedCondition.ID {
		t.Fatalf("sample missing reference-driven fields: %#v", sample)
	}
	if got, want := analysisNames(sample.Analyses), []string{"pH", "Turbidity"}; !sameStrings(got, want) {
		t.Fatalf("catalog-driven analyses got %v want %v", got, want)
	}
	if sample.Analyses[0].ServiceID != ph.ID || sample.Analyses[0].ProfileID != profile.ID || sample.Analyses[0].Method != "SM 4500-H+" || sample.Analyses[0].Units != "SU" {
		t.Fatalf("analysis missing catalog metadata/profile: %#v", sample.Analyses[0])
	}
	if sample.Analyses[1].ServiceID != turbidity.ID || sample.Analyses[1].ProfileID != profile.ID {
		t.Fatalf("duplicate direct service should be deduped into profile-backed analysis, got %#v", sample.Analyses[1])
	}

	loaded, ok := store.GetSample(sample.ID)
	if !ok {
		t.Fatalf("load sample %q", sample.ID)
	}
	if loaded.ClientSampleID != sample.ClientSampleID || loaded.LabSampleID != sample.LabSampleID || !loaded.SampledAt.Equal(sampledAt) || len(loaded.Analyses) != 2 {
		t.Fatalf("loaded sample lost v2 fields: %#v", loaded)
	}
}

func TestSampleIntakeV2EnforcesLabAndClientIdentifierUniqueness(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("accessioner")
	client, _ := store.CreateClient("Unique Client", "unique@example.test", actor)

	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Uniqueness", Matrix: "Water", ClientSampleID: "FIELD-1", LabSampleID: "LAB-1", Tests: []string{"pH"}}, actor); err != nil {
		t.Fatalf("create initial sample: %v", err)
	}
	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Uniqueness", Matrix: "Water", ClientSampleID: "FIELD-1", LabSampleID: "LAB-2", Tests: []string{"pH"}}, actor); err == nil || !strings.Contains(err.Error(), "client sample id") {
		t.Fatalf("expected duplicate client sample id error, got %v", err)
	}
	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Uniqueness", Matrix: "Water", ClientSampleID: "FIELD-2", LabSampleID: "LAB-1", Tests: []string{"pH"}}, actor); err == nil || !strings.Contains(err.Error(), "lab sample id") {
		t.Fatalf("expected duplicate lab sample id error, got %v", err)
	}
}

func analysisNames(analyses []Analysis) []string {
	names := make([]string, 0, len(analyses))
	for _, analysis := range analyses {
		names = append(names, analysis.Name)
	}
	return names
}
