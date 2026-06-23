package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestAdminConfigIndexRendersLowClickWorkbench(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := httptest.NewRequest(http.MethodGet, "/?tenant_id="+lab.DefaultTenantID+"&lab_id="+lab.DefaultLabID, nil)
	addDefaultSessionCookie(req)
	rec := httptest.NewRecorder()
	application.index(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d body=%s", rec.Code, rec.Body.String())
	}
	html := rec.Body.String()
	for _, want := range []string{
		`<nav class="admin-nav" aria-label="Admin configuration">`,
		`href="#admin-clients" accesskey="1"`,
		`href="#admin-contacts" accesskey="2"`,
		`href="#admin-catalog" accesskey="3"`,
		`href="#admin-reference" accesskey="4"`,
		`id="admin-command-search"`,
		`data-command-target="#admin-client-name"`,
		`id="master-data-summary"`,
		`<section class="panel admin-workbench" id="admin-clients">`,
		`<section class="panel admin-workbench" id="admin-contacts">`,
		`<section class="panel admin-workbench" id="admin-catalog">`,
		`<section class="panel admin-workbench" id="admin-reference">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered admin/config UI missing %q\n%s", want, html)
		}
	}
	if strings.Contains(html, "prettier disaster") {
		t.Fatalf("admin UI should stay operational, not novelty-copy-driven: %s", html)
	}
}

func TestAdminConfigStaticKeyboardShortcutsAreDiscoverable(t *testing.T) {
	content, err := os.ReadFile("../../web/static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	for _, want := range []string{"admin-command-search", "data-command-target", "altKey", "#admin-client-name", "#admin-contact-name", "#admin-service-name", "#admin-reference-name"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("app.js missing keyboard affordance %q\n%s", want, string(content))
		}
	}
}
