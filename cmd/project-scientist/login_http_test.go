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

func TestLoginPageRendersWithoutExistingSession(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	application.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`<form class="login-card" method="post" action="/login">`, `name="username"`, `name="password"`, `Sign in`} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page missing %q\n%s", want, body)
		}
	}
}

func TestLoginCreatesBrowserSessionCookieAndRedirects(t *testing.T) {
	t.Setenv("PSC_INTERNAL_SESSION_PASSWORD", "dev-password")
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	form := url.Values{}
	form.Set("username", "lab-dev")
	form.Set("password", "dev-password")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	application.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/dashboard" {
		t.Fatalf("login redirect = %q, want /dashboard", got)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("login did not set %s cookie", sessionCookieName)
	}
	if sessionCookie.Value != defaultTestSessionToken || !sessionCookie.HttpOnly || sessionCookie.Path != "/" {
		t.Fatalf("unexpected session cookie: %#v", sessionCookie)
	}
}

func TestLoginRejectsInvalidPasswordWithoutSettingCookie(t *testing.T) {
	t.Setenv("PSC_INTERNAL_SESSION_PASSWORD", "dev-password")
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	form := url.Values{}
	form.Set("username", "lab-dev")
	form.Set("password", "wrong")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	application.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			t.Fatalf("invalid login should not set session cookie: %#v", cookie)
		}
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	application := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles("../../web/templates/index.html"))})

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	addDefaultSessionCookie(req)
	rec := httptest.NewRecorder()
	application.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Fatalf("logout redirect = %q, want /login", got)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.MaxAge >= 0 {
			t.Fatalf("logout did not expire session cookie: %#v", cookie)
		}
	}
}
