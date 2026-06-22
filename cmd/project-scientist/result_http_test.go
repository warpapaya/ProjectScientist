package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestResultEntryHTTPPersistsReviewRelevantFieldsAsJSON(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}
	seedActor := actor(httptest.NewRequest(http.MethodGet, "/", nil))
	line := createHTTPResultLineFixture(t, store, seedActor)

	resp := performForm(t, app.upsertResult, "/api/results", url.Values{
		"analysis_request_line_id": {line.ID},
		"value":                    {"12.4"},
		"raw_value":                {"12.38"},
		"unit":                     {"mg/L"},
		"qualifier":                {"j"},
		"mdl":                      {"0.01"},
		"rl":                       {"0.05"},
		"loq":                      {"0.10"},
		"dilution":                 {"5"},
		"uncertainty":              {"+/- 0.2"},
		"comments":                 {"instrument import reviewed"},
		"analyst_id":               {"analyst-http"},
		"instrument_id":            {"ICP-MS-7"},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var result lab.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.AnalysisRequestLineID != line.ID || result.Value != "12.4" || result.RawValue != "12.38" || result.Unit != "mg/L" || result.Qualifier != "J" || result.MDL != "0.01" || result.RL != "0.05" || result.LOQ != "0.10" || result.Dilution != "5" || result.Uncertainty != "+/- 0.2" || result.Comments != "instrument import reviewed" || result.AnalystID != "analyst-http" || result.InstrumentID != "ICP-MS-7" {
		t.Fatalf("result response missing persisted fields: %#v", result)
	}
}

func TestResultEntryHTTPValidationErrors(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}
	resp := performForm(t, app.upsertResult, "/api/results", url.Values{"analysis_request_line_id": {"ARL-missing"}, "value": {"1.0"}, "dilution": {"0"}}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid dilution, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func createHTTPResultLineFixture(t *testing.T, store *lab.Store, actor lab.ActorContext) lab.AnalysisRequestLine {
	t.Helper()
	client, err := store.CreateClient("HTTP Result Client", "result-http@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "HTTP Result", Matrix: "Water", Tests: []string{"Nitrate"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	return lines[0]
}
