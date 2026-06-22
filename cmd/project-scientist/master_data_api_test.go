package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestMasterDataAPICreateAndListState(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	clientResp := performForm(t, app.createClient, "/api/clients", url.Values{"name": {"Alpha Environmental"}, "email": {"lab@example.test"}}, "tenant-alpha", "water-lab")
	if clientResp.Code != http.StatusCreated {
		t.Fatalf("create client status = %d body=%s", clientResp.Code, clientResp.Body.String())
	}
	var client lab.Client
	if err := json.Unmarshal(clientResp.Body.Bytes(), &client); err != nil {
		t.Fatalf("decode client: %v", err)
	}

	defaultsResp := performForm(t, app.upsertClientDefaults, "/api/client-defaults", url.Values{"client_id": {client.ID}, "report_template": {"alpha-coa"}, "invoice_email": {"billing@example.test"}, "default_matrix": {"Water"}, "default_tests": {"pH, Turbidity"}}, "tenant-alpha", "water-lab")
	if defaultsResp.Code != http.StatusCreated {
		t.Fatalf("upsert defaults status = %d body=%s", defaultsResp.Code, defaultsResp.Body.String())
	}
	siteResp := performForm(t, app.createSite, "/api/sites", url.Values{"client_id": {client.ID}, "name": {"North Plant"}, "division": {"Water"}, "address": {"101 Intake Rd"}}, "tenant-alpha", "water-lab")
	if siteResp.Code != http.StatusCreated {
		t.Fatalf("create site status = %d body=%s", siteResp.Code, siteResp.Body.String())
	}
	var site lab.Site
	if err := json.Unmarshal(siteResp.Body.Bytes(), &site); err != nil {
		t.Fatalf("decode site: %v", err)
	}
	contactResp := performForm(t, app.createContact, "/api/contacts", url.Values{"client_id": {client.ID}, "site_id": {site.ID}, "name": {"Avery Chemist"}, "email": {"avery@example.test"}, "phone": {"555-0100"}}, "tenant-alpha", "water-lab")
	if contactResp.Code != http.StatusCreated {
		t.Fatalf("create contact status = %d body=%s", contactResp.Code, contactResp.Body.String())
	}
	var contact lab.Contact
	if err := json.Unmarshal(contactResp.Body.Bytes(), &contact); err != nil {
		t.Fatalf("decode contact: %v", err)
	}
	roleResp := performForm(t, app.assignContactRole, "/api/contact-roles", url.Values{"contact_id": {contact.ID}, "role": {"report_reviewer"}}, "tenant-alpha", "water-lab")
	if roleResp.Code != http.StatusCreated {
		t.Fatalf("assign role status = %d body=%s", roleResp.Code, roleResp.Body.String())
	}
	projectResp := performForm(t, app.createProject, "/api/projects", url.Values{"client_id": {client.ID}, "site_id": {site.ID}, "name": {"Q3 Compliance"}, "work_order": {"WO-2026-001"}, "default_matrix": {"Water"}, "default_tests": {"pH"}}, "tenant-alpha", "water-lab")
	if projectResp.Code != http.StatusCreated {
		t.Fatalf("create project status = %d body=%s", projectResp.Code, projectResp.Body.String())
	}

	alphaState := performGet(t, app.apiState, "/api/state", "tenant-alpha", "water-lab")
	var alpha pageData
	if err := json.Unmarshal(alphaState.Body.Bytes(), &alpha); err != nil {
		t.Fatalf("decode alpha state: %v", err)
	}
	if len(alpha.Clients) != 1 || len(alpha.Sites) != 1 || len(alpha.Contacts) != 1 || len(alpha.ContactRoles) != 1 || len(alpha.Projects) != 1 || len(alpha.ClientDefaults) != 1 {
		t.Fatalf("alpha state missing master data: %#v", alpha)
	}

	betaState := performGet(t, app.apiState, "/api/state", "tenant-beta", "water-lab")
	if strings.Contains(betaState.Body.String(), client.ID) || strings.Contains(betaState.Body.String(), site.ID) || strings.Contains(betaState.Body.String(), contact.ID) {
		t.Fatalf("beta state leaked alpha master data: %s", betaState.Body.String())
	}
}
