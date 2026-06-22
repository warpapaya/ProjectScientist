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
