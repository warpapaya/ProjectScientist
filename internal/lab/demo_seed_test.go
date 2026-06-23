package lab

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResetAndSeedSyntheticDemoDeniesUnauthorizedActorBeforeMutationAndAudits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	seedClient, err := store.CreateClient("Do Not Delete Lab", "keep@example.test", demoSeedManagerActorForTest())
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	fixturePath := filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")
	_, err = store.ResetAndSeedSyntheticDemo(fixturePath, demoSeedManagerActorForTest())
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected ErrAuthorizationDenied, got %v", err)
	}
	clients := store.Clients()
	if len(clients) != 1 || clients[0].ID != seedClient.ID || clients[0].Name != seedClient.Name {
		t.Fatalf("unauthorized reset mutated clients: %#v", clients)
	}
	if samples := store.Samples(); len(samples) != 0 {
		t.Fatalf("unauthorized reset seeded samples: %#v", samples)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected seed client audit plus denied reset audit, got %d: %#v", len(events), events)
	}
	denied := events[len(events)-1]
	if denied.Action != string(OperationAdminConfigure) || denied.Outcome != AuditOutcomeDenied || denied.Resource.Type != "synthetic-demo" || denied.Resource.ID != "reset" {
		t.Fatalf("expected denied admin.configure audit for synthetic-demo reset, got %#v", denied)
	}
}

func TestResetAndSeedSyntheticDemoIsDeterministicAndFixtureBacked(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	fixturePath := filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")
	first, err := store.ResetAndSeedSyntheticDemo(fixturePath, demoSeedAdminActorForTest())
	if err != nil {
		t.Fatalf("first reset/seed: %v", err)
	}
	second, err := store.ResetAndSeedSyntheticDemo(fixturePath, demoSeedAdminActorForTest())
	if err != nil {
		t.Fatalf("second reset/seed: %v", err)
	}

	assertDemoSeedSummary(t, first)
	assertDemoSeedSummary(t, second)
	if first.ClientID != second.ClientID || first.SampleID != second.SampleID {
		t.Fatalf("expected rerun to produce stable ids, first=%#v second=%#v", first, second)
	}

	clients := store.Clients()
	if len(clients) != 1 {
		t.Fatalf("expected exactly one client after rerun, got %d: %#v", len(clients), clients)
	}
	samples := store.Samples()
	if len(samples) != 1 {
		t.Fatalf("expected exactly one sample after rerun, got %d: %#v", len(samples), samples)
	}
	if samples[0].ClientID != clients[0].ID {
		t.Fatalf("sample client id mismatch: sample=%#v client=%#v", samples[0], clients[0])
	}
	if len(samples[0].Analyses) != 4 {
		t.Fatalf("expected fixture analytes to expand to four analyses, got %#v", samples[0].Analyses)
	}
	if first.SampleReferenceCount == 0 || second.SampleReferenceCount != first.SampleReferenceCount {
		t.Fatalf("demo reset should seed stable sample reference vocabulary counts, first=%#v second=%#v", first, second)
	}
	if names := sampleReferenceNames(store.SampleReferenceItems(SampleReferenceMatrix)); !containsAll(names, []string{"Drinking Water", "Wastewater", "Soil"}) {
		t.Fatalf("demo reset missing seeded matrix reference data: %v", names)
	}
}

func TestResetAndSeedSyntheticDemoClearsExistingWorksheets(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := demoSeedAdminActorForTest()
	lines := createWorksheetLineFixtures(t, store, actor, "EPA 200.8", "Metals", "Lead")
	if _, err := store.CreateWorksheet(CreateWorksheetInput{AnalysisRequestLineIDs: []string{lines[0].ID}, BatchID: "BATCH-RESET", AnalystID: "analyst-reset"}, actor); err != nil {
		t.Fatalf("create worksheet: %v", err)
	}

	fixturePath := filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")
	if _, err := store.ResetAndSeedSyntheticDemo(fixturePath, actor); err != nil {
		t.Fatalf("reset/seed with existing worksheet: %v", err)
	}
	if worksheets := store.Worksheets(); len(worksheets) != 0 {
		t.Fatalf("expected worksheets cleared by demo reset, got %#v", worksheets)
	}
}

func assertDemoSeedSummary(t *testing.T, summary SyntheticDemoSeedSummary) {
	t.Helper()
	if summary.FixtureID != "psc-mvp-synthetic-lab-v1" {
		t.Fatalf("unexpected fixture id %q", summary.FixtureID)
	}
	if summary.ClientID != "C-00001" || summary.SampleID != "S-000001" {
		t.Fatalf("expected deterministic generated ids, got %#v", summary)
	}
	if summary.ClientName != "Okefenokee Synthetic Water Authority" {
		t.Fatalf("expected fixture client name, got %#v", summary)
	}
	if summary.Project != "MVP Drinking Water Compliance Demo" || summary.Matrix != "Drinking Water" {
		t.Fatalf("expected fixture project/matrix, got %#v", summary)
	}
	if summary.AnalysisCount != 4 {
		t.Fatalf("expected four fixture analyses, got %#v", summary)
	}
}

func demoSeedActorForTest() ActorContext {
	return demoSeedManagerActorForTest()
}

func demoSeedManagerActorForTest() ActorContext {
	return demoSeedActorWithRolesForTest("demo-seed-manager-test", RoleLabManager)
}

func demoSeedAdminActorForTest() ActorContext {
	return demoSeedActorWithRolesForTest("demo-seed-admin-test", RoleAdmin)
}

func demoSeedActorWithRolesForTest(userID string, roles ...Role) ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	return MustActorContext(ActorContextInput{
		UserID:            userID,
		DisplayName:       userID,
		AuthProvider:      "local-dev",
		RequestID:         userID,
		CorrelationID:     userID,
		TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID, Roles: roleStrings}},
		Roles:             roleStrings,
	})
}
