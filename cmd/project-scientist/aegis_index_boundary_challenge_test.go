package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestAegisChallengeIndexDoesNotLeakTenantSelectedOnlyByHeader(t *testing.T) {
	app, victimScope := seededIndexVictimApp(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-PSC-Tenant-ID", victimScope.TenantID)
	req.Header.Set("X-PSC-Lab-ID", victimScope.LabID)

	assertIndexDoesNotLeakVictimScope(t, app, req)
}

func TestAegisChallengeIndexDoesNotLeakTenantSelectedOnlyByQuery(t *testing.T) {
	app, victimScope := seededIndexVictimApp(t)

	query := url.Values{}
	query.Set("tenant_id", victimScope.TenantID)
	query.Set("lab_id", victimScope.LabID)
	req := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)

	assertIndexDoesNotLeakVictimScope(t, app, req)
}

func TestDefaultIndexReturnsOK(t *testing.T) {
	app, _ := seededIndexVictimApp(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	app.index(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("default index status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func seededIndexVictimApp(t *testing.T) (*app, lab.Scope) {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	victimScope := lab.Scope{TenantID: "tenant-victim-index", LabID: lab.DefaultLabID}
	victimActor := lab.MustActorContext(lab.ActorContextInput{
		UserID:            "victim-index-admin",
		RequestID:         "victim-index-seed",
		CorrelationID:     "victim-index-seed",
		TenantMemberships: []lab.TenantMembership{{TenantID: victimScope.TenantID, Roles: []string{string(lab.RoleAdmin)}}},
		Roles:             []string{string(lab.RoleAdmin)},
	})
	if _, err := store.CreateClientForScope(victimScope, "Victim Index Client", "victim-index@example.test", victimActor); err != nil {
		t.Fatalf("seed victim client: %v", err)
	}

	return &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}, victimScope
}

func assertIndexDoesNotLeakVictimScope(t *testing.T, app *app, req *http.Request) {
	t.Helper()
	rr := httptest.NewRecorder()

	app.index(rr, req)
	body := rr.Body.String()
	if strings.Contains(body, "Victim Index Client") || strings.Contains(body, "victim-index@example.test") {
		t.Fatalf("index leaked caller-selected tenant client data: status=%d body=%s", rr.Code, body)
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("index should fail closed with 403 for caller-selected tenant/lab scope, got status=%d body=%s", rr.Code, body)
	}
}
