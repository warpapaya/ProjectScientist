package main

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCreateClientRequiresAuthenticatedSession(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := newSessionTestApp(store)

	form := url.Values{}
	form.Set("name", "No Session Lab")
	form.Set("email", "no-session@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing session, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(store.Clients()); got != 0 {
		t.Fatalf("missing session should create no clients, got %d", got)
	}
}

func TestCreateClientRejectsExpiredSession(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	app, token := newSessionTestAppWithSession(t, store, "expired-user", lab.DefaultTenantID, lab.DefaultLabID, now.Add(-time.Minute))
	app.now = func() time.Time { return now }

	form := url.Values{}
	form.Set("name", "Expired Session Lab")
	form.Set("email", "expired@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired session, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(store.Clients()); got != 0 {
		t.Fatalf("expired session should create no clients, got %d", got)
	}
}

func TestDemoResetRouteRequiresAuthenticatedSessionWhenEnabled(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, fixturePath: filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")})

	req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()

	app.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated demo reset route, got %d body=%s", rr.Code, rr.Body.String())
	}

	authenticatedReq := httptest.NewRequest(http.MethodPost, "/api/demo/reset", nil)
	authenticatedReq.Header.Set("Accept", "application/json")
	authenticatedReq.Header.Set(csrfHeaderName, app.sessions[defaultTestSessionToken].CSRFToken)
	addDefaultSessionCookie(authenticatedReq)
	authenticatedRR := httptest.NewRecorder()
	app.routes().ServeHTTP(authenticatedRR, authenticatedReq)
	if authenticatedRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated demo reset route, got %d body=%s", authenticatedRR.Code, authenticatedRR.Body.String())
	}
}

func TestCreateClientBindsTenantToServerSideSessionNotRequestInputs(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "alpha-manager", "tenant-alpha", lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "Attempted Cross Tenant Lab")
	form.Set("email", "cross-tenant@example.test")
	form.Set("tenant_id", "tenant-beta")
	req := httptest.NewRequest(http.MethodPost, "/api/clients?tenant_id=tenant-beta", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", "tenant-beta")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for caller-selected cross-tenant scope, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(store.Clients()); got != 0 {
		t.Fatalf("cross-tenant request should create no clients, got %d", got)
	}
	if events, err := store.AuditEvents(0); err != nil {
		t.Fatalf("audit events: %v", err)
	} else if len(events) != 0 {
		t.Fatalf("caller-selected scope should be rejected before audited mutation, got %#v", events)
	}
}

func TestCreateClientUsesServerSideSessionScopeWhenRequestHasNoTenant(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "alpha-manager", "tenant-alpha", lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "Alpha Session Lab")
	form.Set("email", "alpha@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for trusted session scope, got %d body=%s", rr.Code, rr.Body.String())
	}
	clients := store.ClientsForScope(lab.Scope{TenantID: "tenant-alpha", LabID: lab.DefaultLabID})
	if len(clients) != 1 || clients[0].TenantID != "tenant-alpha" || clients[0].LabID != lab.DefaultLabID {
		t.Fatalf("expected client scoped to trusted session claims, got %#v", clients)
	}
}

func TestConfiguredInternalSessionCanAuthorizeLocalDemoReset(t *testing.T) {
	t.Setenv("PSC_INTERNAL_SESSION_TOKEN", "configured-demo-reset-token")
	t.Setenv("PSC_INTERNAL_SESSION_TENANT_ID", lab.DefaultTenantID)
	t.Setenv("PSC_INTERNAL_SESSION_LAB_ID", lab.DefaultLabID)
	t.Setenv("PSC_INTERNAL_SESSION_USER", "configured-lab-dev")

	sessions := configuredInternalSessions()
	session, ok := sessions["configured-demo-reset-token"]
	if !ok {
		t.Fatalf("expected configured internal session token")
	}
	for _, role := range session.Actor.Roles {
		if role == string(lab.RoleAdmin) {
			return
		}
	}
	t.Fatalf("configured internal session must include admin role for authorized local demo reset, got %#v", session.Actor.Roles)
}

func TestBrowserMutationRequiresCSRFToken(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "alpha-manager", "tenant-alpha", lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "Missing CSRF Lab")
	form.Set("email", "missing-csrf@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing CSRF token, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(store.ClientsForScope(lab.Scope{TenantID: "tenant-alpha", LabID: lab.DefaultLabID})); got != 0 {
		t.Fatalf("missing CSRF token should create no clients, got %d", got)
	}
}

func TestBrowserMutationAllowsValidSessionAndCSRFToken(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "alpha-manager", "tenant-alpha", lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "CSRF Protected Lab")
	form.Set("email", "csrf@example.test")
	tokenValue := app.sessions[token].CSRFToken
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(csrfHeaderName, tokenValue)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid session+CSRF, got %d body=%s", rr.Code, rr.Body.String())
	}
	clients := store.ClientsForScope(lab.Scope{TenantID: "tenant-alpha", LabID: lab.DefaultLabID})
	if len(clients) != 1 || clients[0].Name != "CSRF Protected Lab" {
		t.Fatalf("expected CSRF-protected mutation to create one client, got %#v", clients)
	}
}

func TestConfiguredInternalSessionsSetCSRFToken(t *testing.T) {
	t.Setenv("PSC_INTERNAL_SESSION_TOKEN", "session-secret")
	t.Setenv("PSC_INTERNAL_CSRF_TOKEN", "csrf-secret")
	sessions := configuredInternalSessions()
	session, ok := sessions["session-secret"]
	if !ok {
		t.Fatalf("configured session missing")
	}
	if session.CSRFToken != "csrf-secret" {
		t.Fatalf("expected explicit CSRF token, got %q", session.CSRFToken)
	}

	t.Setenv("PSC_INTERNAL_CSRF_TOKEN", "")
	sessions = configuredInternalSessions()
	if got := sessions["session-secret"].CSRFToken; got == "" || got == "session-secret" {
		t.Fatalf("expected derived non-empty CSRF token distinct from session token, got %q", got)
	}
}

func TestIndexRendersCSRFTokenForBrowserForms(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})

	req := newDefaultSessionRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	app.index(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected index 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	csrf := app.sessions[defaultTestSessionToken].CSRFToken
	if !strings.Contains(body, `meta name="psc-csrf-token" content="`+csrf+`"`) {
		t.Fatalf("index missing CSRF meta token")
	}
	if !strings.Contains(body, `name="csrf_token" value="`+csrf+`"`) {
		t.Fatalf("index missing hidden CSRF form field")
	}
}

const defaultTestSessionToken = "test-session-default"

func newSessionTestApp(store *lab.Store) *app {
	return &app{store: store, sessions: map[string]authenticatedSession{}, now: time.Now}
}

func attachDefaultSession(t *testing.T, application *app) *app {
	t.Helper()
	if application.sessions == nil {
		application.sessions = map[string]authenticatedSession{}
	}
	if application.now == nil {
		application.now = time.Now
	}
	roles := []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer), string(lab.RoleReportReleaser), string(lab.RoleAdmin)}
	application.sessions[defaultTestSessionToken] = authenticatedSession{
		Actor: lab.MustActorContext(lab.ActorContextInput{
			UserID:            "lab-dev",
			DisplayName:       "lab-dev",
			AuthProvider:      "test-session",
			TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: roles}},
			Roles:             roles,
			RequestID:         "test-session-bootstrap",
			CorrelationID:     "test-session-bootstrap",
		}),
		Scope:     lab.Scope{TenantID: lab.DefaultTenantID, LabID: lab.DefaultLabID},
		ExpiresAt: time.Now().Add(time.Hour),
		CSRFToken: deriveCSRFToken(defaultTestSessionToken),
	}
	return application
}

func addDefaultSessionCookie(req *http.Request) {
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: defaultTestSessionToken})
}

func newDefaultSessionRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	addDefaultSessionCookie(req)
	return req
}

func newSessionTestAppWithSession(t *testing.T, store *lab.Store, userID, tenantID, labID string, expiresAt time.Time) (*app, string) {
	t.Helper()
	application := newSessionTestApp(store)
	token := "test-session-" + userID + "-" + tenantID
	roles := []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer), string(lab.RoleReportReleaser)}
	application.sessions[token] = authenticatedSession{
		Actor: lab.MustActorContext(lab.ActorContextInput{
			UserID:            userID,
			DisplayName:       userID,
			AuthProvider:      "test-session",
			TenantMemberships: []lab.TenantMembership{{TenantID: tenantID, Roles: roles}},
			Roles:             roles,
			RequestID:         "test-session-bootstrap",
			CorrelationID:     "test-session-bootstrap",
		}),
		Scope:     lab.Scope{TenantID: tenantID, LabID: labID},
		ExpiresAt: expiresAt,
		CSRFToken: deriveCSRFToken(token),
	}
	return application, token
}
