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

func TestSampleIntakeV2HTTPAndHTMLLowClickFlow(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "http-fixture", RequestID: "http-fixture", CorrelationID: "http-fixture", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})

	client, _ := store.CreateClient("HTTP Intake Client", "http@example.test", actor)
	project, _ := store.CreateProjectForScope(lab.DefaultScope, lab.ProjectInput{ClientID: client.ID, Name: "HTTP Project", WorkOrder: "WO-HTTP", DefaultMatrix: "Project Matrix"}, actor)
	matrix, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceMatrix, Code: "DW", Name: "Drinking Water", Active: true}, actor)
	container, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	preservative, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferencePreservative, Code: "HNO3", Name: "Nitric Acid", Active: true}, actor)
	storage, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceStorageLocation, Code: "FR1", Name: "Receiving Fridge", Active: true}, actor)
	condition, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceReceivedCondition, Code: "OK", Name: "Received on ice", Active: true}, actor)
	dept, _ := store.CreateCatalogDepartment(lab.CatalogDepartmentInput{Name: "Wet Chem", SortOrder: 1}, actor)
	ph, _ := store.CreateAnalysisService(lab.AnalysisServiceInput{Name: "pH", DepartmentID: dept.ID, SortOrder: 1}, actor)
	alk, _ := store.CreateAnalysisService(lab.AnalysisServiceInput{Name: "Alkalinity", DepartmentID: dept.ID, SortOrder: 2}, actor)
	profile, _ := store.CreateAnalysisProfile(lab.AnalysisProfileInput{Name: "Routine Water", ServiceIDs: []string{ph.ID, alk.ID}}, actor)

	indexRR := httptest.NewRecorder()
	app.index(indexRR, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRR.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", indexRR.Code, indexRR.Body.String())
	}
	for _, want := range []string{"name=\"project_id\"", "name=\"client_sample_id\"", "name=\"lab_sample_id\"", "name=\"analysis_profile_ids\"", "name=\"matrix_reference_id\"", "Routine Water", "Drinking Water"} {
		if !strings.Contains(indexRR.Body.String(), want) {
			t.Fatalf("intake form missing %q in body:\n%s", want, indexRR.Body.String())
		}
	}

	resp := performForm(t, app.createSample, "/api/samples", url.Values{
		"client_id":             {client.ID},
		"project_id":            {project.ID},
		"client_sample_id":      {"HTTP-FIELD-1"},
		"lab_sample_id":         {"HTTP-LAB-1"},
		"matrix_reference_id":   {matrix.ID},
		"container_id":          {container.ID},
		"preservative_id":       {preservative.ID},
		"storage_location_id":   {storage.ID},
		"received_condition_id": {condition.ID},
		"sampled_at":            {"2026-06-20T14:30:00Z"},
		"received_at":           {"2026-06-21T08:15:00Z"},
		"priority":              {"rush"},
		"comments":              {"HTTP low-click intake"},
		"analysis_profile_ids":  {profile.ID},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create sample expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var sample lab.Sample
	if err := json.Unmarshal(resp.Body.Bytes(), &sample); err != nil {
		t.Fatalf("decode sample: %v", err)
	}
	if sample.ProjectID != project.ID || sample.Matrix != "Drinking Water" || sample.Priority != lab.PriorityRush || len(sample.Analyses) != 2 {
		t.Fatalf("unexpected HTTP sample: %#v", sample)
	}
	if sample.Analyses[0].ProfileID != profile.ID || sample.Analyses[0].ServiceID != ph.ID {
		t.Fatalf("HTTP sample missing catalog-driven profile/service analysis: %#v", sample.Analyses)
	}
}
