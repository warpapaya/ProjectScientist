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

func TestResultEntryGridAndReviewDeskExposeKeyboardFriendlyWorkflow(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "ux-seed", RequestID: "ux-seed", CorrelationID: "ux-seed", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	client, err := store.CreateClient("UX Lab", "ux@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "MVP UX", Matrix: "Water", Tests: []string{"Lead", "Copper"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 2 {
		t.Fatalf("expected two analysis request lines, got %#v", lines)
	}
	if _, err := store.CreateResult(lab.ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 7.1, RawValue: "7.1 ug/L", Unit: "ug/L", Dilution: 1, Comments: "ready for review", AnalystID: "analyst-ux", InstrumentID: "ICP-1"}, actor); err != nil {
		t.Fatalf("create result: %v", err)
	}

	rr := httptest.NewRecorder()
	app.index(rr, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"id=\"result-entry\"",
		"data-result-entry-grid",
		"Result entry grid",
		"Alt + 5: result entry",
		"Ctrl/⌘ + Enter: save active result",
		"action=\"/api/results\"",
		"name=\"analysis_request_line_id\" value=\"" + lines[1].ID + "\"",
		"autofocus",
		"id=\"result-review\"",
		"Review queue",
		"action=\"/api/results/R-000001/review\"",
		"name=\"decision\" value=\"accept\"",
		"href=\"#report-release\"",
		"id=\"audit\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("workflow page missing %q:\n%s", want, body)
		}
	}
}
