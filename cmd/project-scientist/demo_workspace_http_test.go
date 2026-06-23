package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestDashboardOffersBrowserDemoWorkspaceWhenEnabled(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := newDefaultSessionRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	application.index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="demo-workspace"`,
		`method="post" action="/api/demo/reset"`,
		`Load demo workspace`,
		`Tindall/CENLA-style sample data`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing demo workspace affordance %q\n%s", want, body)
		}
	}
}

func TestBrowserDemoResetRedirectsToDashboardAndSeedsWorkspace(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, fixturePath: filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")})

	req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", strings.NewReader("csrf_token="+application.sessions[defaultTestSessionToken].CSRFToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addDefaultSessionCookie(req)
	rec := httptest.NewRecorder()
	application.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("browser demo reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/dashboard" {
		t.Fatalf("browser demo reset redirect = %q, want /dashboard", got)
	}
	if got := len(store.ClientsForScope(lab.Scope{TenantID: lab.DefaultTenantID, LabID: lab.DefaultLabID})); got == 0 {
		t.Fatalf("browser demo reset did not seed clients")
	}
	if got := len(store.SamplesForScope(lab.Scope{TenantID: lab.DefaultTenantID, LabID: lab.DefaultLabID})); got == 0 {
		t.Fatalf("browser demo reset did not seed samples")
	}
}
