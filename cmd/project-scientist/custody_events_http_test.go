package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCustodyEventsHTTPAndHTMLCreateView(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "http-custodian", RequestID: "http-custodian", CorrelationID: "http-custodian", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	client, _ := store.CreateClient("Custody HTTP Client", "custody-http@example.test", actor)
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "COC HTTP", Matrix: "Water", Tests: []string{"pH"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	indexRR := httptest.NewRecorder()
	app.index(indexRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if body := indexRR.Body.String(); !strings.Contains(body, "Custody history") || !strings.Contains(body, "name=\"custody_type\"") || !strings.Contains(body, "name=\"custody_location\"") {
		t.Fatalf("index missing custody create/view affordances:\n%s", body)
	}

	resp := performForm(t, app.recordCustodyEvent, "/api/samples/"+sample.ID+"/custody-events", url.Values{
		"custody_type":     {"received"},
		"custody_location": {"Receiving fridge A"},
		"custody_reason":   {"COC intake"},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("record custody expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var event lab.CustodyEvent
	if err := json.Unmarshal(resp.Body.Bytes(), &event); err != nil {
		t.Fatalf("decode custody event: %v", err)
	}
	if event.SampleID != sample.ID || event.Type != lab.CustodyReceived || event.Location != "Receiving fridge A" || event.Reason != "COC intake" || event.Sequence != 1 {
		t.Fatalf("custody response missing expected fields: %#v", event)
	}

	state := performGet(t, app.apiState, "/api/state", lab.DefaultTenantID, lab.DefaultLabID)
	if !strings.Contains(state.Body.String(), "custody_events") || !strings.Contains(state.Body.String(), "Receiving fridge A") {
		t.Fatalf("API state missing custody history: %s", state.Body.String())
	}
	viewRR := httptest.NewRecorder()
	app.index(viewRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if body := viewRR.Body.String(); !strings.Contains(body, "Receiving fridge A") || !strings.Contains(body, "COC intake") {
		t.Fatalf("workflow view missing custody history:\n%s", body)
	}
}
