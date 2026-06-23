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

func TestProductionNavigationUsesRealRoutes(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := httptest.NewRequest(http.MethodGet, "/samples?tenant_id="+lab.DefaultTenantID+"&lab_id="+lab.DefaultLabID, nil)
	addDefaultSessionCookie(req)
	rec := httptest.NewRecorder()
	application.index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("samples status = %d body=%s", rec.Code, rec.Body.String())
	}
	html := rec.Body.String()
	for _, want := range []string{
		`<nav class="app-nav" aria-label="Primary navigation">`,
		`href="/dashboard"`,
		`href="/samples" aria-current="page"`,
		`href="/results"`,
		`href="/reports"`,
		`href="/admin"`,
		`id="workflow-board"`,
		`id="intake"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("production navigation/samples page missing %q\n%s", want, html)
		}
	}
	for _, avoid := range []string{
		`href="#admin-clients"`,
		`class="saas-dashboard"`,
		`id="result-entry"`,
		`id="admin-clients"`,
	} {
		if strings.Contains(html, avoid) {
			t.Fatalf("samples page still carries single-page/vibe-coded surface %q\n%s", avoid, html)
		}
	}
}

func TestProductionNavigationRejectsUnknownAppRoutes(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := httptest.NewRequest(http.MethodGet, "/not-a-page?tenant_id="+lab.DefaultTenantID+"&lab_id="+lab.DefaultLabID, nil)
	addDefaultSessionCookie(req)
	rec := httptest.NewRecorder()
	application.index(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want 404", rec.Code)
	}
}
