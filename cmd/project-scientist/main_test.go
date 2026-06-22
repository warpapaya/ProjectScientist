package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCreateClientIgnoresSpoofedActorHeaderAndFormForAuditIdentity(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	form := url.Values{}
	form.Set("name", "Spoof Test Lab")
	form.Set("email", "spoof@example.test")
	form.Set("actor", "form-attacker")
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Actor", "header-attacker")
	req.Header.Set("X-PSC-Request-ID", "req-spoof")
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

func TestCreateSampleIgnoresSpoofedActorHeaderAndFormForAuditIdentity(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}
	client, err := store.CreateClient("Seed Lab", "seed@example.test", actor(httptest.NewRequest(http.MethodGet, "/", nil)))
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
	app := &app{store: store}
	seedReq := httptest.NewRequest(http.MethodGet, "/", nil)
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
	rr := httptest.NewRecorder()

	app.transitionSample(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertLatestAuditIdentity(t, store, "sample.transitioned", "req-transition-spoof")
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
