package lab

import (
	"path/filepath"
	"testing"
)

func TestCatalogServicesListByDepartmentAndServiceOrder(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	water, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Water", SortOrder: 20}, actor)
	if err != nil {
		t.Fatalf("create water department: %v", err)
	}
	metals, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Metals", SortOrder: 10}, actor)
	if err != nil {
		t.Fatalf("create metals department: %v", err)
	}
	mgL, err := store.CreateCatalogUnit(CatalogUnitInput{Name: "Milligrams per liter", Symbol: "mg/L"}, actor)
	if err != nil {
		t.Fatalf("create unit: %v", err)
	}
	epa2008, err := store.CreateCatalogMethod(CatalogMethodInput{Name: "EPA 200.8"}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	lead, err := store.CreateCatalogAnalyte(CatalogAnalyteInput{Name: "Lead", DefaultUnitID: mgL.ID}, actor)
	if err != nil {
		t.Fatalf("create analyte: %v", err)
	}

	if _, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "pH", DepartmentID: water.ID, SortOrder: 1}, actor); err != nil {
		t.Fatalf("create pH service: %v", err)
	}
	if _, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Lead dissolved", DepartmentID: metals.ID, MethodID: epa2008.ID, AnalyteID: lead.ID, UnitID: mgL.ID, SortOrder: 20}, actor); err != nil {
		t.Fatalf("create lead service: %v", err)
	}
	if _, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Arsenic", DepartmentID: metals.ID, MethodID: epa2008.ID, SortOrder: 10}, actor); err != nil {
		t.Fatalf("create arsenic service: %v", err)
	}

	services := store.AnalysisServices()
	if got, want := serviceNames(services), []string{"Arsenic", "Lead dissolved", "pH"}; !sameStrings(got, want) {
		t.Fatalf("services ordered by department sort then service sort/name: got %v want %v", got, want)
	}
	if services[1].MethodName != "EPA 200.8" || services[1].AnalyteName != "Lead" || services[1].UnitSymbol != "mg/L" || services[1].DepartmentName != "Metals" {
		t.Fatalf("expected expanded catalog labels on service, got %#v", services[1])
	}
}

func TestAnalysisProfilePreservesExplicitServiceOrder(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	dept, _ := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Wet Chem", SortOrder: 1}, actor)
	first, _ := store.CreateAnalysisService(AnalysisServiceInput{Name: "Alkalinity", DepartmentID: dept.ID, SortOrder: 1}, actor)
	second, _ := store.CreateAnalysisService(AnalysisServiceInput{Name: "Hardness", DepartmentID: dept.ID, SortOrder: 2}, actor)
	third, _ := store.CreateAnalysisService(AnalysisServiceInput{Name: "Conductivity", DepartmentID: dept.ID, SortOrder: 3}, actor)

	profile, err := store.CreateAnalysisProfile(AnalysisProfileInput{Name: "Routine Water", ServiceIDs: []string{third.ID, first.ID, second.ID}}, actor)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if got, want := serviceNames(profile.Services), []string{"Conductivity", "Alkalinity", "Hardness"}; !sameStrings(got, want) {
		t.Fatalf("profile service order follows requested panel order: got %v want %v", got, want)
	}

	profiles := store.AnalysisProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	if got, want := serviceNames(profiles[0].Services), []string{"Conductivity", "Alkalinity", "Hardness"}; !sameStrings(got, want) {
		t.Fatalf("listed profile service order follows panel order: got %v want %v", got, want)
	}
}

func TestCatalogSnapshotsRetainImmutableVersionsAfterUpdate(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	unit, err := store.CreateCatalogUnit(CatalogUnitInput{Name: "Milligrams per liter", Symbol: "mg/L"}, actor)
	if err != nil {
		t.Fatalf("create unit: %v", err)
	}
	initial, ok := store.CurrentCatalogSnapshot()
	if !ok {
		t.Fatal("expected initial catalog snapshot")
	}
	if got, want := initial.Units[0].Symbol, "mg/L"; got != want {
		t.Fatalf("initial snapshot unit symbol got %q want %q", got, want)
	}

	if _, err := store.UpdateCatalogUnit(CatalogUnitUpdateInput{ID: unit.ID, Name: "Milligrams per kilogram", Symbol: "mg/kg"}, actor); err != nil {
		t.Fatalf("update unit: %v", err)
	}
	current, ok := store.CurrentCatalogSnapshot()
	if !ok {
		t.Fatal("expected current catalog snapshot")
	}
	if current.Version <= initial.Version || current.ID == initial.ID {
		t.Fatalf("expected update to create a later snapshot, initial=%#v current=%#v", initial, current)
	}
	if got, want := current.Units[0].Symbol, "mg/kg"; got != want {
		t.Fatalf("current snapshot unit symbol got %q want %q", got, want)
	}

	snapshots := store.CatalogSnapshots()
	if len(snapshots) < 2 {
		t.Fatalf("expected retained snapshot history, got %d", len(snapshots))
	}
	retained, ok := store.CatalogSnapshot(initial.ID)
	if !ok {
		t.Fatalf("expected retained initial snapshot %q", initial.ID)
	}
	if got, want := retained.Units[0].Symbol, "mg/L"; got != want {
		t.Fatalf("retained snapshot mutated: got %q want %q", got, want)
	}
}

func TestCreateSampleReferencesImmutableCatalogSnapshot(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	client, err := store.CreateClient("Snapshot Client", "snapshot@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	unit, err := store.CreateCatalogUnit(CatalogUnitInput{Name: "Milligrams per liter", Symbol: "mg/L"}, actor)
	if err != nil {
		t.Fatalf("create unit: %v", err)
	}
	dept, _ := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Wet Chem", SortOrder: 1}, actor)
	if _, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Nitrate", DepartmentID: dept.ID, UnitID: unit.ID, SortOrder: 1}, actor); err != nil {
		t.Fatalf("create service: %v", err)
	}
	atIntake, ok := store.CurrentCatalogSnapshot()
	if !ok {
		t.Fatal("expected catalog snapshot before sample intake")
	}

	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Snapshot Study", Matrix: "Water", Tests: []string{"Nitrate"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if got, want := sample.Analyses[0].CatalogSnapshotID, atIntake.ID; got != want {
		t.Fatalf("analysis snapshot id got %q want %q", got, want)
	}
	if got, want := sample.Analyses[0].CatalogSnapshotVersion, atIntake.Version; got != want {
		t.Fatalf("analysis snapshot version got %d want %d", got, want)
	}

	if _, err := store.UpdateCatalogUnit(CatalogUnitUpdateInput{ID: unit.ID, Name: "Milligrams per kilogram", Symbol: "mg/kg"}, actor); err != nil {
		t.Fatalf("update unit: %v", err)
	}
	loaded, ok := store.GetSample(sample.ID)
	if !ok {
		t.Fatalf("get sample %q", sample.ID)
	}
	if got, want := loaded.Analyses[0].CatalogSnapshotID, atIntake.ID; got != want {
		t.Fatalf("loaded analysis snapshot id changed got %q want %q", got, want)
	}
}

func serviceNames(services []AnalysisService) []string {
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func catalogTestActor() ActorContext {
	return MustActorContext(ActorContextInput{
		UserID:            "catalog-manager",
		DisplayName:       "Catalog Manager",
		AuthProvider:      "test",
		RequestID:         "catalog-test",
		CorrelationID:     "catalog-test",
		TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID, Roles: []string{string(RoleLabManager)}}},
		Roles:             []string{string(RoleLabManager)},
	})
}
