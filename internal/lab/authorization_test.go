package lab

import (
	"errors"
	"path/filepath"
	"testing"
)

func actorWithRoles(userID string, roles ...Role) ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	return MustActorContext(ActorContextInput{
		UserID:            userID,
		DisplayName:       userID,
		AuthProvider:      "test",
		TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID, Roles: roleStrings}},
		Roles:             roleStrings,
		RequestID:         "req-" + userID,
		CorrelationID:     "corr-" + userID,
	})
}

func TestAuthorizeOperationDeniesAndAuditsEveryProtectedOperation(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	unauthorized := actorWithRoles("client-contact", RoleClientContact)
	operations := []Operation{
		OperationClientCreate,
		OperationClientUpdate,
		OperationClientArchive,
		OperationContactCreate,
		OperationContactUpdate,
		OperationContactArchive,
		OperationProjectCreate,
		OperationProjectUpdate,
		OperationProjectArchive,
		OperationSampleIntake,
		OperationSampleUpdate,
		OperationSampleTransition,
		OperationResultEntry,
		OperationResultUpdate,
		OperationResultReview,
		OperationResultRelease,
		OperationReportGenerate,
		OperationReportRelease,
		OperationReportExport,
		OperationReportAmend,
		OperationAuditView,
		OperationAuditExport,
		OperationImportRun,
		OperationExportRun,
		OperationCatalogConfigure,
		OperationAdminConfigure,
	}

	for _, operation := range operations {
		err := store.AuthorizeOperationForScope(defaultScope(), operation, unauthorized, AuditResource{Type: "policy-test", ID: string(operation)}, nil)
		if !errors.Is(err, ErrAuthorizationDenied) {
			t.Fatalf("%s: expected ErrAuthorizationDenied, got %v", operation, err)
		}
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	deniedByOperation := map[string]bool{}
	for _, event := range events {
		if event.Outcome == AuditOutcomeDenied && event.Reason == "authorization_denied" {
			deniedByOperation[event.Action] = true
		}
	}
	for _, operation := range operations {
		if !deniedByOperation[string(operation)] {
			t.Fatalf("missing audited denied event for %s", operation)
		}
	}
}

func TestPolicyAllowsExpectedRolesForProtectedOperations(t *testing.T) {
	cases := []struct {
		operation Operation
		role      Role
	}{
		{OperationClientCreate, RoleLabManager},
		{OperationClientUpdate, RoleLabManager},
		{OperationClientArchive, RoleAdmin},
		{OperationContactCreate, RoleLabManager},
		{OperationContactUpdate, RoleLabManager},
		{OperationContactArchive, RoleAdmin},
		{OperationProjectCreate, RoleLabManager},
		{OperationProjectUpdate, RoleLabManager},
		{OperationProjectArchive, RoleAdmin},
		{OperationSampleIntake, RoleLabManager},
		{OperationSampleUpdate, RoleAnalyst},
		{OperationSampleTransition, RoleAnalyst},
		{OperationResultEntry, RoleAnalyst},
		{OperationResultUpdate, RoleAnalyst},
		{OperationResultReview, RoleReviewer},
		{OperationResultRelease, RoleReportReleaser},
		{OperationReportGenerate, RoleReviewer},
		{OperationReportRelease, RoleReportReleaser},
		{OperationReportExport, RoleReportReleaser},
		{OperationReportAmend, RoleReportReleaser},
		{OperationAuditView, RoleAdmin},
		{OperationAuditExport, RoleAdmin},
		{OperationImportRun, RoleMigrationService},
		{OperationExportRun, RoleLabManager},
		{OperationCatalogConfigure, RoleLabManager},
		{OperationAdminConfigure, RoleAdmin},
	}
	for _, tc := range cases {
		actor := actorWithRoles("allowed-"+string(tc.role), tc.role)
		if err := Authorize(defaultScope(), tc.operation, actor); err != nil {
			t.Fatalf("%s with %s: expected allowed, got %v", tc.operation, tc.role, err)
		}
	}
}

func TestStoreMutationsArePermissionCheckedAndDeniedAuditsAreSafe(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	client, err := store.CreateClient("Allowed Lab", "allowed@example.test", actorWithRoles("manager", RoleLabManager))
	if err != nil {
		t.Fatalf("create client with lab manager: %v", err)
	}
	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Drinking Water", Matrix: "Water", Tests: []string{"pH"}}, actorWithRoles("client", RoleClientContact)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected client/contact sample intake denial, got %v", err)
	}
	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Drinking Water", Matrix: "Water", Tests: []string{"pH"}}, actorWithRoles("analyst", RoleAnalyst)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected analyst sample intake denial, got %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Drinking Water", Matrix: "Water", Tests: []string{"pH"}}, actorWithRoles("manager", RoleLabManager))
	if err != nil {
		t.Fatalf("create sample with lab manager: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusInPrep, actorWithRoles("client", RoleClientContact)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected client/contact transition denial, got %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var denied []AuditEvent
	for _, event := range events {
		if event.Outcome == AuditOutcomeDenied && event.Reason == "authorization_denied" {
			denied = append(denied, event)
		}
	}
	if len(denied) != 3 {
		t.Fatalf("expected 3 authorization denial events, got %d", len(denied))
	}
	for _, event := range denied {
		if err := ValidateAuditEvent(event); err != nil {
			t.Fatalf("denied event is not schema-valid: %v", err)
		}
		if _, ok := event.Details["project"]; ok {
			t.Fatalf("denied event leaked sample project payload: %#v", event.Details)
		}
	}
}
