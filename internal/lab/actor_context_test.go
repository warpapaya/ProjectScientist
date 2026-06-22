package lab

import "testing"

func TestActorContextRequiresStableIdentity(t *testing.T) {
	_, err := NewActorContext(ActorContextInput{
		UserID:            "  ",
		DisplayName:       "Missing ID",
		TenantMemberships: []TenantMembership{{TenantID: "clearline-demo", Roles: []string{"analyst"}}},
		RequestID:         "req-1",
	})
	if err == nil {
		t.Fatalf("expected missing user id to be rejected")
	}
}

func TestActorContextNormalizesAuthenticatedPrincipalMetadata(t *testing.T) {
	actor, err := NewActorContext(ActorContextInput{
		UserID:      "  user-123  ",
		DisplayName: "  Friday Ops  ",
		TenantMemberships: []TenantMembership{
			{TenantID: " clearline-demo ", Roles: []string{" analyst ", "reviewer", "analyst", ""}},
		},
		Roles:          []string{" lab-manager ", "analyst", "lab-manager", ""},
		ServiceAccount: true,
		RequestID:      " req-abc ",
		CorrelationID:  " corr-xyz ",
	})
	if err != nil {
		t.Fatalf("new actor context: %v", err)
	}
	if actor.UserID != "user-123" {
		t.Fatalf("expected trimmed user id, got %q", actor.UserID)
	}
	if actor.DisplayNameSnapshot != "Friday Ops" {
		t.Fatalf("expected trimmed display name snapshot, got %q", actor.DisplayNameSnapshot)
	}
	if !actor.ServiceAccount {
		t.Fatalf("expected service account flag")
	}
	if actor.RequestID != "req-abc" || actor.CorrelationID != "corr-xyz" {
		t.Fatalf("unexpected request/correlation ids: %#v", actor)
	}
	if got := actor.Roles; len(got) != 2 || got[0] != "analyst" || got[1] != "lab-manager" {
		t.Fatalf("expected normalized global roles, got %#v", got)
	}
	if got := actor.TenantMemberships; len(got) != 1 || got[0].TenantID != "clearline-demo" || len(got[0].Roles) != 2 || got[0].Roles[0] != "analyst" || got[0].Roles[1] != "reviewer" {
		t.Fatalf("expected normalized tenant memberships, got %#v", got)
	}
}

func TestAuditEventRecordsActorContext(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir() + "/project-scientist.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := MustActorContext(ActorContextInput{
		UserID:            "user-friday",
		DisplayName:       "Friday Ops",
		TenantMemberships: []TenantMembership{{TenantID: "clearline-demo", Roles: []string{"lab-manager"}}},
		Roles:             []string{"lab-manager"},
		ServiceAccount:    false,
		RequestID:         "req-store",
		CorrelationID:     "corr-store",
	})
	_, err = store.CreateClient("Clearline Demo Lab", "qa@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	event := events[0]
	if event.Actor != "user-friday" {
		t.Fatalf("expected compatibility actor user id, got %q", event.Actor)
	}
	if event.ActorContext.UserID != "user-friday" || event.ActorContext.DisplayNameSnapshot != "Friday Ops" {
		t.Fatalf("missing actor context in audit event: %#v", event.ActorContext)
	}
	if event.ActorContext.RequestID != "req-store" || event.ActorContext.CorrelationID != "corr-store" {
		t.Fatalf("missing request/correlation ids: %#v", event.ActorContext)
	}
}
