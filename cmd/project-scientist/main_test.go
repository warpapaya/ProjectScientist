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
