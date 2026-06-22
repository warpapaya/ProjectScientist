package lab

import (
	"path/filepath"
	"testing"
)

func TestMasterDataCreatesClientSitesContactsRolesProjectsDefaultsScopedAndAudited(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	alpha := Scope{TenantID: "tenant-alpha", LabID: "water-lab"}
	beta := Scope{TenantID: "tenant-beta", LabID: "water-lab"}
	manager := testScopedActor("manager", alpha.TenantID)
	mallory := testScopedActor("mallory", beta.TenantID)

	client, err := store.CreateClientForScope(alpha, "Alpha Environmental", "lab@example.test", manager)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defaults, err := store.UpsertClientDefaultsForScope(alpha, ClientDefaultsInput{ClientID: client.ID, ReportTemplate: "alpha-coa", InvoiceEmail: "billing@example.test", DefaultMatrix: "Water", DefaultTests: []string{"pH", "Turbidity"}}, manager)
	if err != nil {
		t.Fatalf("upsert defaults: %v", err)
	}
	site, err := store.CreateSiteForScope(alpha, SiteInput{ClientID: client.ID, Name: "North Plant", Division: "Water", Address: "101 Intake Rd"}, manager)
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	contact, err := store.CreateContactForScope(alpha, ContactInput{ClientID: client.ID, SiteID: site.ID, Name: "Avery Chemist", Email: "avery@example.test", Phone: "555-0100"}, manager)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	role, err := store.AssignContactRoleForScope(alpha, ContactRoleInput{ContactID: contact.ID, Role: "report_reviewer"}, manager)
	if err != nil {
		t.Fatalf("assign contact role: %v", err)
	}
	project, err := store.CreateProjectForScope(alpha, ProjectInput{ClientID: client.ID, SiteID: site.ID, Name: "Q3 Compliance", WorkOrder: "WO-2026-001", DefaultMatrix: "Water", DefaultTests: []string{"pH"}}, manager)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	if defaults.ClientID != client.ID || defaults.DefaultMatrix != "Water" || len(defaults.DefaultTests) != 2 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	if site.ClientID != client.ID || site.TenantID != alpha.TenantID || site.LabID != alpha.LabID {
		t.Fatalf("site missing scope/client: %#v", site)
	}
	if contact.SiteID != site.ID || role.ContactID != contact.ID || project.WorkOrder != "WO-2026-001" {
		t.Fatalf("master data relationships not retained: contact=%#v role=%#v project=%#v", contact, role, project)
	}

	if got := store.SitesForScope(beta); len(got) != 0 {
		t.Fatalf("beta scope leaked alpha sites: %#v", got)
	}
	if got := store.ContactsForScope(beta); len(got) != 0 {
		t.Fatalf("beta scope leaked alpha contacts: %#v", got)
	}
	if got := store.ContactRolesForScope(beta); len(got) != 0 {
		t.Fatalf("beta scope leaked alpha contact roles: %#v", got)
	}
	if got := store.ProjectsForScope(beta); len(got) != 0 {
		t.Fatalf("beta scope leaked alpha projects: %#v", got)
	}
	if _, ok := store.ClientDefaultsForScope(beta, client.ID); ok {
		t.Fatalf("beta scope read alpha client defaults")
	}

	if _, err := store.CreateSiteForScope(beta, SiteInput{ClientID: client.ID, Name: "Cross Tenant Site"}, mallory); err == nil {
		t.Fatalf("expected cross-tenant site create to fail")
	}
	if _, err := store.CreateContactForScope(beta, ContactInput{ClientID: client.ID, SiteID: site.ID, Name: "Cross Tenant Contact"}, mallory); err == nil {
		t.Fatalf("expected cross-tenant contact create to fail")
	}
	if _, err := store.AssignContactRoleForScope(beta, ContactRoleInput{ContactID: contact.ID, Role: "report_reviewer"}, mallory); err == nil {
		t.Fatalf("expected cross-tenant role assignment to fail")
	}
	if _, err := store.CreateProjectForScope(beta, ProjectInput{ClientID: client.ID, SiteID: site.ID, Name: "Cross Tenant Project"}, mallory); err == nil {
		t.Fatalf("expected cross-tenant project create to fail")
	}

	events, err := store.AuditEventsForScope(alpha, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	wantActions := []string{"client.created", "client.defaults.upserted", "site.created", "contact.created", "contact.role.assigned", "project.created"}
	if len(events) != len(wantActions) {
		t.Fatalf("expected %d audit events, got %d: %#v", len(wantActions), len(events), events)
	}
	for i, action := range wantActions {
		if events[i].Action != action {
			t.Fatalf("event %d action = %q, want %q", i, events[i].Action, action)
		}
	}
}

func testScopedActor(userID, tenantID string) ActorContext {
	return MustActorContext(ActorContextInput{
		UserID:            userID,
		DisplayName:       userID,
		AuthProvider:      "test",
		TenantMemberships: []TenantMembership{{TenantID: tenantID, Roles: []string{string(RoleAdmin), string(RoleLabManager)}}},
		Roles:             []string{string(RoleAdmin), string(RoleLabManager)},
		RequestID:         "req-" + userID,
		CorrelationID:     "corr-" + userID,
	})
}
