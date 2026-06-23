package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestAegisChallengeHTTPDoesNotGrantTenantMembershipFromSpoofedScopeHeader(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	form := url.Values{}
	form.Set("name", "Injected Tenant Client")
	form.Set("email", "attacker@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", "tenant-attacker-controlled")
	req.Header.Set("X-PSC-Lab-ID", lab.DefaultLabID)
	req.Header.Set("X-PSC-Request-ID", "aegis-spoofed-tenant")
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code == http.StatusCreated {
		t.Fatalf("spoofed tenant header created a client in attacker-selected tenant; expected denial, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := store.ClientsForScope(lab.Scope{TenantID: "tenant-attacker-controlled", LabID: lab.DefaultLabID}); len(got) != 0 {
		t.Fatalf("spoofed tenant write persisted objects in attacker-selected tenant: %#v", got)
	}
}

func TestAegisChallengeHTTPDoesNotAcceptSpoofedLabScopeHeader(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	form := url.Values{}
	form.Set("name", "Injected Lab Client")
	form.Set("email", "attacker@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", lab.DefaultTenantID)
	req.Header.Set("X-PSC-Lab-ID", "lab-attacker-controlled")
	req.Header.Set("X-PSC-Request-ID", "aegis-spoofed-lab")
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code == http.StatusCreated {
		t.Fatalf("spoofed lab header created a client in attacker-selected lab; expected denial, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := store.ClientsForScope(lab.Scope{TenantID: lab.DefaultTenantID, LabID: "lab-attacker-controlled"}); len(got) != 0 {
		t.Fatalf("spoofed lab write persisted objects in attacker-selected lab: %#v", got)
	}
}

func TestAegisChallengeAPIStateDoesNotLeakTenantSelectedOnlyByHeader(t *testing.T) {
	app, victimScope := seededAPIStateVictimApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", victimScope.TenantID)
	req.Header.Set("X-PSC-Lab-ID", victimScope.LabID)

	assertAPIStateDoesNotLeakVictimScope(t, app, req)
}

func TestAegisChallengeAPIStateDoesNotLeakTenantSelectedOnlyByQuery(t *testing.T) {
	app, victimScope := seededAPIStateVictimApp(t)

	query := url.Values{}
	query.Set("tenant_id", victimScope.TenantID)
	query.Set("lab_id", victimScope.LabID)
	req := httptest.NewRequest(http.MethodGet, "/api/state?"+query.Encode(), nil)
	req.Header.Set("Accept", "application/json")

	assertAPIStateDoesNotLeakVictimScope(t, app, req)
}

func TestAegisChallengeAPIStateDoesNotLeakTenantSelectedOnlyByForm(t *testing.T) {
	app, victimScope := seededAPIStateVictimApp(t)

	form := url.Values{}
	form.Set("tenant_id", victimScope.TenantID)
	form.Set("lab_id", victimScope.LabID)
	req := httptest.NewRequest(http.MethodPost, "/api/state", strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	assertAPIStateDoesNotLeakVictimScope(t, app, req)
}

func seededAPIStateVictimApp(t *testing.T) (*app, lab.Scope) {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	victimScope := lab.Scope{TenantID: "tenant-victim", LabID: lab.DefaultLabID}
	victimActor := lab.MustActorContext(lab.ActorContextInput{
		UserID:            "victim-admin",
		RequestID:         "victim-seed",
		TenantMemberships: []lab.TenantMembership{{TenantID: victimScope.TenantID, Roles: []string{string(lab.RoleAdmin)}}},
		Roles:             []string{string(lab.RoleAdmin)},
	})
	if _, err := store.CreateClientForScope(victimScope, "Victim Client", "victim@example.test", victimActor); err != nil {
		t.Fatalf("seed victim client: %v", err)
	}
	return &app{store: store}, victimScope
}

func assertAPIStateDoesNotLeakVictimScope(t *testing.T, app *app, req *http.Request) {
	t.Helper()
	rr := httptest.NewRecorder()
	app.apiState(rr, req)
	if rr.Code != http.StatusForbidden {
		var body struct {
			Clients []lab.Client `json:"clients"`
		}
		_ = json.NewDecoder(rr.Body).Decode(&body)
		if len(body.Clients) > 0 {
			t.Fatalf("api/state leaked tenant-selected client data without authenticated tenant binding: code=%d clients=%#v", rr.Code, body.Clients)
		}
		t.Fatalf("api/state should fail closed for caller-selected tenant/lab scope: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
