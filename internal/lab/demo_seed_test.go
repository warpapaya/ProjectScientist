package lab

import (
	"path/filepath"
	"testing"
)

func TestResetAndSeedSyntheticDemoIsDeterministicAndFixtureBacked(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	fixturePath := filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")
	first, err := store.ResetAndSeedSyntheticDemo(fixturePath, demoSeedActorForTest())
	if err != nil {
		t.Fatalf("first reset/seed: %v", err)
	}
	second, err := store.ResetAndSeedSyntheticDemo(fixturePath, demoSeedActorForTest())
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
	return MustActorContext(ActorContextInput{
		UserID:            "demo-seed-test",
		DisplayName:       "Demo Seed Test",
		AuthProvider:      "local-dev",
		RequestID:         "demo-seed-test",
		CorrelationID:     "demo-seed-test",
		TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID}},
	})
}
