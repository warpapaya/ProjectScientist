package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCreateResultHTTPPersistsResultAndReturnsJSON(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store})

	seedActor := actor(newDefaultSessionRequest(http.MethodGet, "/", nil))
	client, err := store.CreateClient("Seed Lab", "seed@example.test", seedActor)
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Compliance", Matrix: "Water", Tests: []string{"Nitrate"}}, seedActor)
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one analysis request line, got %#v", lines)
	}

	form := url.Values{}
	form.Set("analysis_request_line_id", lines[0].ID)
	form.Set("value", "9.8")
	form.Set("raw_value", "9.8 mg/L")
	form.Set("unit", "mg/L")
	form.Set("qualifier", "")
	form.Set("mdl", "0.2")
	form.Set("rl", "0.5")
	form.Set("loq", "1.0")
	form.Set("dilution", "1")
	form.Set("uncertainty", "0.3")
	form.Set("comments", "near MCL")
	form.Set("analyst_id", "analyst-http")
	form.Set("instrument_id", "IC-2")
	res := performForm(t, app.createResult, "/api/results", form, lab.DefaultTenantID, lab.DefaultLabID)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", res.Code, res.Body.String())
	}
	var result lab.Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.ID == "" || result.SampleID != sample.ID || result.Value != 9.8 || result.Unit != "mg/L" || result.AnalystID != "analyst-http" || result.InstrumentID != "IC-2" {
		t.Fatalf("unexpected result payload: %#v", result)
	}
}
