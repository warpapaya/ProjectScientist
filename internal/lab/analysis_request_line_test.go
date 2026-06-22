package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisRequestLinesExpandProfilesWithCatalogAssignmentAndSnapshot(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()

	client, err := store.CreateClient("AR Client", "ar@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	dept, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Wet Chem", SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}
	method, err := store.CreateCatalogMethod(CatalogMethodInput{Name: "SM 2540 C"}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	service, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Total Dissolved Solids", DepartmentID: dept.ID, MethodID: method.ID, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	profile, err := store.CreateAnalysisProfile(AnalysisProfileInput{Name: "Routine solids", ServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	catalogVersion, ok := store.CurrentCatalogSnapshot()
	if !ok {
		t.Fatal("expected current catalog snapshot")
	}

	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "AR study", Matrix: "Water", AnalysisProfileIDs: []string{profile.ID}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected 1 analysis request line, got %d: %#v", len(lines), lines)
	}
	line := lines[0]
	if line.SampleID != sample.ID || line.ServiceID != service.ID || line.ProfileID != profile.ID || line.Name != service.Name {
		t.Fatalf("line did not preserve sample/profile/service identity: %#v", line)
	}
	if line.Status != AnalysisRequestLineStatusRequested {
		t.Fatalf("new line status got %q want %q", line.Status, AnalysisRequestLineStatusRequested)
	}
	if line.DepartmentID != dept.ID || line.DepartmentName != "Wet Chem" || line.MethodID != method.ID || line.MethodName != "SM 2540 C" {
		t.Fatalf("line missing assigned department/method labels: %#v", line)
	}
	if line.CatalogSnapshotID != catalogVersion.ID || line.CatalogSnapshotVersion != catalogVersion.Version {
		t.Fatalf("line snapshot got %s/%d want %s/%d", line.CatalogSnapshotID, line.CatalogSnapshotVersion, catalogVersion.ID, catalogVersion.Version)
	}
}

func TestAnalysisRequestLineStatusLifecycleIsAuditedAndEnforced(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	line := createAnalysisRequestLineFixture(t, store, actor)

	if err := store.TransitionAnalysisRequestLine(line.ID, AnalysisRequestLineStatusInProgress, actor); err != nil {
		t.Fatalf("requested -> in_progress: %v", err)
	}
	if err := store.TransitionAnalysisRequestLine(line.ID, AnalysisRequestLineStatusCompleted, actor); err != nil {
		t.Fatalf("in_progress -> completed: %v", err)
	}
	loaded, ok := store.GetAnalysisRequestLine(line.ID)
	if !ok {
		t.Fatalf("load line %q", line.ID)
	}
	if loaded.Status != AnalysisRequestLineStatusCompleted {
		t.Fatalf("line status got %q want %q", loaded.Status, AnalysisRequestLineStatusCompleted)
	}
	if err := store.TransitionAnalysisRequestLine(line.ID, AnalysisRequestLineStatusInProgress, actor); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected completed -> in_progress denial, got %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var allowed, denied bool
	for _, event := range events {
		if event.Resource.Type != "analysis_request_line" || event.Resource.ID != line.ID {
			continue
		}
		if event.Action == "analysis_request_line.transitioned" && event.Outcome == AuditOutcomeAllowed {
			allowed = true
		}
		if event.Action == "analysis_request_line.transition.requested" && event.Outcome == AuditOutcomeDenied && event.Reason == "transition_not_allowed" {
			denied = true
		}
	}
	if !allowed || !denied {
		t.Fatalf("expected allowed and denied line transition audit events, allowed=%v denied=%v", allowed, denied)
	}
}

func TestAnalysisRequestLineTenantAndAuthorizationEnforcement(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	line := createAnalysisRequestLineFixture(t, store, actor)

	otherScope := Scope{TenantID: "other-tenant", LabID: DefaultLabID}
	otherActor := MustActorContext(ActorContextInput{UserID: "other-manager", DisplayName: "Other Manager", AuthProvider: "test", RequestID: "other", CorrelationID: "other", TenantMemberships: []TenantMembership{{TenantID: otherScope.TenantID, Roles: []string{string(RoleLabManager)}}}, Roles: []string{string(RoleLabManager)}})
	if err := store.TransitionAnalysisRequestLineForScope(otherScope, line.ID, AnalysisRequestLineStatusInProgress, otherActor); err == nil || !strings.Contains(err.Error(), "outside requested tenant/lab scope") {
		t.Fatalf("expected cross-tenant line transition denial, got %v", err)
	}
	if _, ok := store.GetAnalysisRequestLineForScope(otherScope, line.ID); ok {
		t.Fatalf("cross-tenant read should not return line %q", line.ID)
	}

	clientActor := actorWithRoles("client-contact", RoleClientContact)
	if err := store.TransitionAnalysisRequestLine(line.ID, AnalysisRequestLineStatusInProgress, clientActor); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected authorization denial, got %v", err)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var scopeDenied, authDenied bool
	for _, event := range events {
		if event.Outcome != AuditOutcomeDenied || event.Resource.ID != line.ID {
			continue
		}
		if event.Reason == "scope_mismatch" {
			scopeDenied = true
		}
		if event.Action == string(OperationResultUpdate) && event.Reason == "authorization_denied" {
			authDenied = true
		}
	}
	if !scopeDenied || !authDenied {
		t.Fatalf("expected scope and auth denied audit events, scope=%v auth=%v", scopeDenied, authDenied)
	}
}

func createAnalysisRequestLineFixture(t *testing.T, store *Store, actor ActorContext) AnalysisRequestLine {
	t.Helper()
	client, err := store.CreateClient("Line Client", "line@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	dept, err := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Metals", SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}
	method, err := store.CreateCatalogMethod(CatalogMethodInput{Name: "EPA 200.8"}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	service, err := store.CreateAnalysisService(AnalysisServiceInput{Name: "Lead", DepartmentID: dept.ID, MethodID: method.ID, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Line study", Matrix: "Water", AnalysisServiceIDs: []string{service.ID}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	return lines[0]
}
