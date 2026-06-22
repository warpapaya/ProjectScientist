package lab

import (
	"path/filepath"
	"testing"
)

func TestSampleReferenceItemsAreTenantScopedAndAuditedCRUD(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	scopeA := Scope{TenantID: DefaultTenantID, LabID: DefaultLabID}
	scopeB := Scope{TenantID: "tenant-b", LabID: "lab-b"}
	actorB := sampleReferenceActorForScope(scopeB)

	matrix, err := store.CreateSampleReferenceItemForScope(scopeA, SampleReferenceItemInput{Kind: SampleReferenceMatrix, Name: " Drinking Water ", Code: " DW ", SortOrder: 20, Active: true}, actor)
	if err != nil {
		t.Fatalf("create matrix: %v", err)
	}
	if matrix.ID != "SR-00001" || matrix.TenantID != scopeA.TenantID || matrix.LabID != scopeA.LabID || matrix.Name != "Drinking Water" || matrix.Code != "DW" || !matrix.Active {
		t.Fatalf("created item not normalized/scoped: %#v", matrix)
	}
	if _, err := store.CreateSampleReferenceItemForScope(scopeB, SampleReferenceItemInput{Kind: SampleReferenceMatrix, Name: "Wastewater", Code: "WW", Active: true}, actorB); err != nil {
		t.Fatalf("create tenant-b matrix: %v", err)
	}

	updated, err := store.UpdateSampleReferenceItemForScope(scopeA, matrix.ID, SampleReferenceItemInput{Kind: SampleReferenceMatrix, Name: "Potable Water", Code: "PW", Description: "Routine drinking-water matrix", SortOrder: 5, Active: true}, actor)
	if err != nil {
		t.Fatalf("update matrix: %v", err)
	}
	if updated.Name != "Potable Water" || updated.Code != "PW" || updated.Description != "Routine drinking-water matrix" || updated.SortOrder != 5 {
		t.Fatalf("updated item mismatch: %#v", updated)
	}
	if err := store.DeleteSampleReferenceItemForScope(scopeA, matrix.ID, actor); err != nil {
		t.Fatalf("delete matrix: %v", err)
	}

	if got := store.SampleReferenceItemsForScope(scopeA, SampleReferenceMatrix); len(got) != 0 {
		t.Fatalf("expected deleted tenant-a matrix hidden from active list, got %#v", got)
	}
	if got := store.SampleReferenceItemsForScope(scopeB, SampleReferenceMatrix); len(got) != 1 || got[0].Name != "Wastewater" {
		t.Fatalf("tenant-b list should be isolated from tenant-a delete, got %#v", got)
	}

	events, err := store.AuditEventsForScope(scopeA, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if got, want := sampleReferenceAuditActions(events), []string{"sample_reference.created", "sample_reference.updated", "sample_reference.deleted"}; !sameStrings(got, want) {
		t.Fatalf("sample reference CRUD audit actions got %v want %v", got, want)
	}
}

func TestSeedDemoSampleReferenceDataCreatesExpectedVocabulary(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	summary, err := store.SeedDemoSampleReferenceData(actor)
	if err != nil {
		t.Fatalf("seed sample reference data: %v", err)
	}
	if summary.MatrixCount < 3 || summary.ContainerCount < 3 || summary.PreservativeCount < 3 || summary.StorageLocationCount < 2 || summary.ReceivedConditionCount < 4 {
		t.Fatalf("seed summary missing expected vocabulary counts: %#v", summary)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferenceMatrix)); !containsAll(names, []string{"Drinking Water", "Wastewater", "Soil"}) {
		t.Fatalf("seeded matrices missing expected names: %v", names)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferenceContainer)); !containsAll(names, []string{"500 mL HDPE Bottle", "40 mL VOA Vial", "Glass Jar"}) {
		t.Fatalf("seeded containers missing expected names: %v", names)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferencePreservative)); !containsAll(names, []string{"None", "HNO3", "HCl"}) {
		t.Fatalf("seeded preservatives missing expected names: %v", names)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferenceStorageLocation)); !containsAll(names, []string{"Walk-in Cooler", "Ambient Receiving Shelf"}) {
		t.Fatalf("seeded storage locations missing expected names: %v", names)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferenceReceivedCondition)); !containsAll(names, []string{"Acceptable", "Broken Container", "Insufficient Volume", "Temperature Out of Range"}) {
		t.Fatalf("seeded received conditions missing expected names: %v", names)
	}

	second, err := store.SeedDemoSampleReferenceData(actor)
	if err != nil {
		t.Fatalf("seed sample reference data second run: %v", err)
	}
	if summary.TotalCount != second.TotalCount || len(store.SampleReferenceItems(SampleReferenceMatrix)) != summary.MatrixCount {
		t.Fatalf("seed should be idempotent, first=%#v second=%#v matrices=%d", summary, second, len(store.SampleReferenceItems(SampleReferenceMatrix)))
	}
}

func sampleReferenceNames(items []SampleReferenceItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

func containsAll(got, want []string) bool {
	seen := map[string]bool{}
	for _, name := range got {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			return false
		}
	}
	return true
}

func sampleReferenceAuditActions(events []AuditEvent) []string {
	actions := []string{}
	for _, event := range events {
		if event.Resource.Type == "sample_reference" {
			actions = append(actions, event.Action)
		}
	}
	return actions
}

func sampleReferenceActorForScope(scope Scope) ActorContext {
	return MustActorContext(ActorContextInput{
		UserID:            "sample-reference-manager",
		DisplayName:       "Sample Reference Manager",
		AuthProvider:      "test",
		RequestID:         "sample-reference-test",
		CorrelationID:     "sample-reference-test",
		TenantMemberships: []TenantMembership{{TenantID: scope.TenantID, Roles: []string{string(RoleLabManager)}}},
		Roles:             []string{string(RoleLabManager)},
	})
}
