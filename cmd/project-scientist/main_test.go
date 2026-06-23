package main

import (
	"encoding/json"
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

func TestCreateClientIgnoresSpoofedActorHeaderAndFormForAuditIdentity(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "lab-dev", lab.DefaultTenantID, lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "Spoof Test Lab")
	form.Set("email", "spoof@example.test")
	form.Set("actor", "form-attacker")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Actor", "header-attacker")
	req.Header.Set("X-PSC-Request-ID", "req-spoof")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(events))
	}
	if events[0].Actor == "header-attacker" || events[0].Actor == "form-attacker" {
		t.Fatalf("spoofed actor set audit identity: %#v", events[0])
	}
	if events[0].Actor != "lab-dev" {
		t.Fatalf("expected local authenticated dev identity lab-dev, got %q", events[0].Actor)
	}
	if events[0].ActorContext.UserID != "lab-dev" || events[0].ActorContext.RequestID != "req-spoof" {
		t.Fatalf("expected dev actor context with request id, got %#v", events[0].ActorContext)
	}
}

func TestCreateClientRejectsArbitraryTenantSelectionWithoutTrustedMembership(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		tenantID string
		mutate   func(*http.Request, url.Values)
	}{
		{
			name:     "header tenant",
			path:     "/api/clients",
			tenantID: "tenant-header-attacker",
			mutate: func(req *http.Request, form url.Values) {
				req.Header.Set("X-PSC-Tenant-ID", "tenant-header-attacker")
			},
		},
		{
			name:     "form tenant",
			path:     "/api/clients",
			tenantID: "tenant-form-attacker",
			mutate: func(req *http.Request, form url.Values) {
				form.Set("tenant_id", "tenant-form-attacker")
			},
		},
		{
			name:     "query tenant",
			path:     "/api/clients?tenant_id=tenant-query-attacker",
			tenantID: "tenant-query-attacker",
			mutate:   func(req *http.Request, form url.Values) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer store.Close()
			app, token := newSessionTestAppWithSession(t, store, "lab-dev", lab.DefaultTenantID, lab.DefaultLabID, time.Now().Add(time.Hour))

			form := url.Values{}
			form.Set("name", "Boundary Test Lab")
			form.Set("email", "boundary@example.test")
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(form.Encode()))
			tc.mutate(req, form)
			req.Body = io.NopCloser(strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Accept", "application/json")
			req.Header.Set("X-PSC-Request-ID", "req-tenant-boundary")
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
			rr := httptest.NewRecorder()

			app.createClient(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for arbitrary %s, got %d body=%s", tc.name, rr.Code, rr.Body.String())
			}
			if got := len(store.Clients()); got != 0 {
				t.Fatalf("expected denied mutation to create no clients, got %d", got)
			}
			events, err := store.AuditEvents(0)
			if err != nil {
				t.Fatalf("audit events: %v", err)
			}
			if len(events) != 0 {
				t.Fatalf("caller-selected scope must be rejected before audit/mutation, got %#v", events)
			}
		})
	}
}

func TestCreateClientAllowsTrustedLocalLabTestTenant(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "lab-dev", "lab-test", lab.DefaultLabID, time.Now().Add(time.Hour))

	form := url.Values{}
	form.Set("name", "Lab Test Client")
	form.Set("email", "lab-test@example.test")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createClient(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected lab-test local tenant to remain usable, got %d body=%s", rr.Code, rr.Body.String())
	}
	clients := store.ClientsForScope(lab.Scope{TenantID: "lab-test", LabID: lab.DefaultLabID})
	if len(clients) != 1 || clients[0].TenantID != "lab-test" {
		t.Fatalf("expected client in lab-test tenant, got %#v", clients)
	}
}

func TestCreateSampleIgnoresSpoofedActorHeaderAndFormForAuditIdentity(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "lab-dev", lab.DefaultTenantID, lab.DefaultLabID, time.Now().Add(time.Hour))
	client, err := store.CreateClient("Seed Lab", "seed@example.test", actor(newDefaultSessionRequest(http.MethodGet, "/", nil)))
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}

	form := url.Values{}
	form.Set("client_id", client.ID)
	form.Set("project", "Spoofed Sample")
	form.Set("matrix", "Water")
	form.Set("tests", "pH,Turbidity")
	form.Set("actor", "form-attacker")
	req := httptest.NewRequest(http.MethodPost, "/api/samples", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Actor", "header-attacker")
	req.Header.Set("X-PSC-Request-ID", "req-sample-spoof")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.createSample(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertLatestAuditIdentity(t, store, "sample.created", "req-sample-spoof")
}

func TestTransitionSampleIgnoresSpoofedActorHeaderAndFormForAuditIdentity(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app, token := newSessionTestAppWithSession(t, store, "lab-dev", lab.DefaultTenantID, lab.DefaultLabID, time.Now().Add(time.Hour))
	seedReq := newDefaultSessionRequest(http.MethodGet, "/", nil)
	seedActor := actor(seedReq)
	client, err := store.CreateClient("Seed Lab", "seed@example.test", seedActor)
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Metals", Matrix: "Soil", Tests: []string{"Lead"}}, seedActor)
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}

	form := url.Values{}
	form.Set("status", string(lab.StatusInPrep))
	form.Set("actor", "form-attacker")
	req := httptest.NewRequest(http.MethodPost, "/api/samples/"+sample.ID+"/transition", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Actor", "header-attacker")
	req.Header.Set("X-PSC-Request-ID", "req-transition-spoof")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.transitionSample(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertLatestAuditIdentity(t, store, "sample.transitioned", "req-transition-spoof")
}

func TestDemoResetEndpointSeedsFixtureAndIsRerunnable(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, fixturePath: filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-PSC-Request-ID", "demo-reset-test")
		req.Header.Set(csrfHeaderName, app.sessions[defaultTestSessionToken].CSRFToken)
		addDefaultSessionCookie(req)
		rr := httptest.NewRecorder()

		app.routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("run %d expected 200, got %d body=%s", i+1, rr.Code, rr.Body.String())
		}
		var summary lab.SyntheticDemoSeedSummary
		if err := json.NewDecoder(rr.Body).Decode(&summary); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		if summary.ClientID != "C-00001" || summary.SampleID != "S-000001" || summary.ClientName != "Okefenokee Synthetic Water Authority" || summary.AnalysisCount != 4 {
			t.Fatalf("unexpected summary after run %d: %#v", i+1, summary)
		}
	}
	if got := len(store.Clients()); got != 1 {
		t.Fatalf("expected rerun to leave one client, got %d", got)
	}
	if got := len(store.Samples()); got != 1 {
		t.Fatalf("expected rerun to leave one sample, got %d", got)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].ActorContext.UserID != "lab-dev" {
		t.Fatalf("expected demo reset to use authenticated session actor, got %#v", events)
	}
}

func TestDemoResetRouteRejectsUnauthenticatedResetEvenWhenEnabled(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, demoResetEnabled: true, fixturePath: filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")})

	req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Request-ID", "unauth-demo-reset-test")
	rr := httptest.NewRecorder()

	app.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated demo reset, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := len(store.Clients()); got != 0 {
		t.Fatalf("unauthenticated demo reset should create no clients, got %d", got)
	}
	if got := len(store.Samples()); got != 0 {
		t.Fatalf("unauthenticated demo reset should create no samples, got %d", got)
	}
}

func TestDemoResetEndpointIsDisabledUnlessExplicitlyEnabled(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store, fixturePath: filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")}

	req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", nil)
	rr := httptest.NewRecorder()
	app.demoReset(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected disabled endpoint to 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func assertLatestAuditIdentity(t *testing.T, store *lab.Store, action, requestID string) {
	t.Helper()
	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected audit events")
	}
	event := events[len(events)-1]
	if event.Action != action {
		t.Fatalf("expected latest action %q, got %q", action, event.Action)
	}
	if event.Actor == "header-attacker" || event.Actor == "form-attacker" {
		t.Fatalf("spoofed actor set audit identity: %#v", event)
	}
	if event.Actor != "lab-dev" {
		t.Fatalf("expected local authenticated dev identity lab-dev, got %q", event.Actor)
	}
	if event.ActorContext.UserID != "lab-dev" || event.ActorContext.RequestID != requestID {
		t.Fatalf("expected dev actor context with request id %q, got %#v", requestID, event.ActorContext)
	}
}
