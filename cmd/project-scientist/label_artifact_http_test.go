package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestSampleLabelArtifactHTTPAndUIAction(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "http-label", RequestID: "http-label", CorrelationID: "http-label", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	client, _ := store.CreateClient("Label HTTP Client", "label-http@example.test", actor)
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Label HTTP", ClientSampleID: "client-01", LabSampleID: "PS-2026-0002", Matrix: "Water", Tests: []string{"Lead"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}

	indexRR := httptest.NewRecorder()
	app.index(indexRR, httptest.NewRequest(http.MethodGet, "/", nil))
	if body := indexRR.Body.String(); !strings.Contains(body, "/api/samples/"+sample.ID+"/label-artifact") || !strings.Contains(body, "Print label") {
		t.Fatalf("workflow page missing label print action:\n%s", body)
	}

	rr := performGet(t, app.sampleLabelArtifact, "/api/samples/"+sample.ID+"/label-artifact", lab.DefaultTenantID, lab.DefaultLabID)
	if rr.Code != http.StatusOK {
		t.Fatalf("label artifact expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON content type, got %q", ct)
	}
	var artifact lab.LabelArtifact
	if err := json.Unmarshal(rr.Body.Bytes(), &artifact); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	if artifact.SampleID != sample.ID || artifact.BarcodeValue != "PS-2026-0002" || !strings.Contains(artifact.PrintableText, "PROJECT SCIENTIST SAMPLE LABEL") {
		t.Fatalf("unexpected label artifact response: %#v", artifact)
	}
}
