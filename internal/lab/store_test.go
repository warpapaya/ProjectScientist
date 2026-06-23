package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSampleAssignsIDWorkflowAndAuditEvent(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	client, err := store.CreateClient("Clearline Demo Lab", "qa@example.test", testActor("friday"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	sample, err := store.CreateSample(CreateSampleInput{
		ClientID: client.ID,
		Project:  "Drinking Water Compliance",
		Matrix:   "Water",
		Tests:    []string{"pH", "Turbidity"},
	}, testActor("friday"))
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if sample.ID == "" || !strings.HasPrefix(sample.ID, "S-") {
		t.Fatalf("expected generated S-* sample id, got %q", sample.ID)
	}
	if sample.Status != StatusReceived {
		t.Fatalf("expected status %q, got %q", StatusReceived, sample.Status)
	}
	if len(sample.Analyses) != 2 {
		t.Fatalf("expected 2 analyses, got %d", len(sample.Analyses))
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if got := events[len(events)-1].Action; got != "sample.created" {
		t.Fatalf("expected last audit event sample.created, got %q", got)
	}
	if events[len(events)-1].Actor != "friday" {
		t.Fatalf("expected actor friday, got %q", events[len(events)-1].Actor)
	}
}

func TestWorkflowTransitionRequiresAllowedPathAndAudits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	client, _ := store.CreateClient("CENLA Demo", "cenla@example.test", testActor("ashley"))
	sample, _ := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Metals", Matrix: "Soil", Tests: []string{"Lead"}}, testActor("ashley"))

	if err := store.TransitionSample(sample.ID, StatusReleased, testActor("ashley")); err == nil {
		t.Fatalf("expected direct received -> released transition to fail")
	}
	for _, status := range []SampleStatus{StatusInPrep, StatusInAnalysis, StatusInReview, StatusReleased} {
		if err := store.TransitionSample(sample.ID, status, testActor("ashley")); err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	updated, ok := store.GetSample(sample.ID)
	if !ok || updated.Status != StatusReleased {
		t.Fatalf("expected released sample, got %#v", updated)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var transitions int
	for _, event := range events {
		if event.Action == "sample.transitioned" {
			transitions++
		}
	}
	if transitions != 4 {
		t.Fatalf("expected 4 transition audit events, got %d", transitions)
	}
}

func TestAuditLogIsHashChained(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, _ = store.CreateClient("Tindall Demo", "armando@example.test", testActor("armando"))
	_, _ = store.CreateClient("RJ Lee Demo", "demo@example.test", testActor("friday"))

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events")
	}
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			t.Fatalf("event %d previous hash mismatch: got %q want %q", i, events[i].PreviousHash, events[i-1].Hash)
		}
		if events[i].Hash == "" {
			t.Fatalf("event %d missing hash", i)
		}
	}
}

func TestTenantLabBoundaryScopesReadsWritesAndAudit(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	alpha := Scope{TenantID: "tenant-alpha", LabID: "water-lab"}
	beta := Scope{TenantID: "tenant-beta", LabID: "water-lab"}

	client, err := store.CreateClientForScope(alpha, "Alpha Client", "alpha@example.test", testScopedActor("friday", alpha.TenantID))
	if err != nil {
		t.Fatalf("create alpha client: %v", err)
	}
	if client.TenantID != alpha.TenantID || client.LabID != alpha.LabID {
		t.Fatalf("client missing tenant/lab scope: %#v", client)
	}

	if _, err := store.CreateSampleForScope(beta, CreateSampleInput{ClientID: client.ID, Project: "Cross write", Matrix: "Water", Tests: []string{"pH"}}, testScopedActor("mallory", beta.TenantID)); err == nil {
		t.Fatalf("expected cross-tenant sample write to fail closed")
	}

	sample, err := store.CreateSampleForScope(alpha, CreateSampleInput{ClientID: client.ID, Project: "Alpha Project", Matrix: "Water", Tests: []string{"pH"}}, testScopedActor("friday", alpha.TenantID))
	if err != nil {
		t.Fatalf("create alpha sample: %v", err)
	}
	if sample.TenantID != alpha.TenantID || sample.LabID != alpha.LabID {
		t.Fatalf("sample missing tenant/lab scope: %#v", sample)
	}
	for _, analysis := range sample.Analyses {
		if analysis.TenantID != alpha.TenantID || analysis.LabID != alpha.LabID {
			t.Fatalf("analysis missing tenant/lab scope: %#v", analysis)
		}
	}

	if _, ok := store.GetSampleForScope(beta, sample.ID); ok {
		t.Fatalf("cross-tenant sample read should fail closed")
	}
	if err := store.TransitionSampleForScope(beta, sample.ID, StatusInPrep, testScopedActor("mallory", beta.TenantID)); err == nil {
		t.Fatalf("cross-tenant transition should fail closed")
	}
	if got := store.SamplesForScope(beta); len(got) != 0 {
		t.Fatalf("beta scope should not see alpha samples: %#v", got)
	}

	events, err := store.AuditEventsForScope(alpha, 0)
	if err != nil {
		t.Fatalf("alpha audit events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected alpha client + sample audit events, got %d", len(events))
	}
	for _, event := range events {
		if event.TenantID != alpha.TenantID || event.LabID != alpha.LabID {
			t.Fatalf("audit event missing alpha scope: %#v", event)
		}
	}
	betaEvents, err := store.AuditEventsForScope(beta, 0)
	if err != nil {
		t.Fatalf("beta audit events: %v", err)
	}
	if len(betaEvents) != 1 || betaEvents[0].Outcome != AuditOutcomeDenied || betaEvents[0].Reason != "sample_not_found" {
		t.Fatalf("beta audit should only include opaque denied cross-scope transition proof: %#v", betaEvents)
	}
}
