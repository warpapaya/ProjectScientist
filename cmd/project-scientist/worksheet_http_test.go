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

func TestWorksheetHTTPAPIAndHTMLSmoke(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "worksheet-http", RequestID: "worksheet-http", CorrelationID: "worksheet-http", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	lines := createHTTPWorksheetLines(t, store, actor)

	indexRR := httptest.NewRecorder()
	app.index(indexRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if indexRR.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", indexRR.Code, indexRR.Body.String())
	}
	body := indexRR.Body.String()
	for _, want := range []string{"id=\"worksheets\"", "action=\"/api/worksheets\"", "name=\"analysis_request_line_ids\"", lines[0].ID, lines[1].ID} {
		if !strings.Contains(body, want) {
			t.Fatalf("worksheet HTML missing %q in body:\n%s", want, body)
		}
	}

	create := performForm(t, app.createWorksheet, "/api/worksheets", url.Values{
		"analysis_request_line_ids": {strings.Join([]string{lines[0].ID, lines[1].ID}, "\n")},
		"batch_id":                  {"BATCH-HTTP-1"},
		"analyst_id":                {"analyst-http"},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if create.Code != http.StatusCreated {
		t.Fatalf("create worksheet expected 201, got %d body=%s", create.Code, create.Body.String())
	}
	var worksheet lab.Worksheet
	if err := json.Unmarshal(create.Body.Bytes(), &worksheet); err != nil {
		t.Fatalf("decode worksheet: %v", err)
	}
	if worksheet.BatchID != "BATCH-HTTP-1" || len(worksheet.Lines) != 2 {
		t.Fatalf("unexpected created worksheet: %#v", worksheet)
	}

	assign := performForm(t, app.routeWorksheetMutation, "/api/worksheets/"+worksheet.ID+"/assign", url.Values{"analyst_id": {"analyst-two"}}, lab.DefaultTenantID, lab.DefaultLabID)
	if assign.Code != http.StatusOK {
		t.Fatalf("assign analyst expected 200, got %d body=%s", assign.Code, assign.Body.String())
	}
	remove := performForm(t, app.routeWorksheetMutation, "/api/worksheets/"+worksheet.ID+"/lines/"+lines[1].ID+"/remove", url.Values{}, lab.DefaultTenantID, lab.DefaultLabID)
	if remove.Code != http.StatusOK {
		t.Fatalf("remove line expected 200, got %d body=%s", remove.Code, remove.Body.String())
	}
	transition := performForm(t, app.routeWorksheetMutation, "/api/worksheets/"+worksheet.ID+"/transition", url.Values{"status": {string(lab.WorksheetStatusInProgress)}}, lab.DefaultTenantID, lab.DefaultLabID)
	if transition.Code != http.StatusOK {
		t.Fatalf("transition expected 200, got %d body=%s", transition.Code, transition.Body.String())
	}

	state := performGet(t, app.apiState, "/api/state", lab.DefaultTenantID, lab.DefaultLabID)
	if state.Code != http.StatusOK {
		t.Fatalf("state expected 200, got %d", state.Code)
	}
	if !strings.Contains(state.Body.String(), "BATCH-HTTP-1") || !strings.Contains(state.Body.String(), "analyst-two") {
		t.Fatalf("state missing worksheet batch/analyst: %s", state.Body.String())
	}
}

func createHTTPWorksheetLines(t *testing.T, store *lab.Store, actor lab.ActorContext) []lab.AnalysisRequestLine {
	t.Helper()
	client, err := store.CreateClient("Worksheet HTTP Client", "worksheet@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	dept, err := store.CreateCatalogDepartment(lab.CatalogDepartmentInput{Name: "Metals", SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}
	method, err := store.CreateCatalogMethod(lab.CatalogMethodInput{Name: "EPA 200.8"}, actor)
	if err != nil {
		t.Fatalf("create method: %v", err)
	}
	lead, err := store.CreateAnalysisService(lab.AnalysisServiceInput{Name: "Lead", DepartmentID: dept.ID, MethodID: method.ID, SortOrder: 1}, actor)
	if err != nil {
		t.Fatalf("create lead service: %v", err)
	}
	copper, err := store.CreateAnalysisService(lab.AnalysisServiceInput{Name: "Copper", DepartmentID: dept.ID, MethodID: method.ID, SortOrder: 2}, actor)
	if err != nil {
		t.Fatalf("create copper service: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Worksheet HTTP", Matrix: "Water", AnalysisServiceIDs: []string{lead.ID, copper.ID}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	return lines
}
