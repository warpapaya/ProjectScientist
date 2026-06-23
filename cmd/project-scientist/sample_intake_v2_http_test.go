package main

import (
	"bytes"
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

func selectHTML(t *testing.T, body, name string) string {
	t.Helper()
	needle := "<select name=\"" + name + "\""
	start := strings.Index(body, needle)
	if start < 0 {
		t.Fatalf("select %q not found", name)
	}
	end := strings.Index(body[start:], "</select>")
	if end < 0 {
		t.Fatalf("select %q not closed", name)
	}
	return body[start : start+end+len("</select>")]
}

func TestSampleIntakeV2HTTPAndHTMLLowClickFlow(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
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
	app.index(indexRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if indexRR.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", indexRR.Code, indexRR.Body.String())
	}
	body := indexRR.Body.String()
	for _, want := range []string{"name=\"project_id\"", "name=\"client_sample_id\"", "name=\"lab_sample_id\"", "name=\"analysis_profile_ids\"", "name=\"matrix_reference_id\"", "Routine Water", "Drinking Water"} {
		if !strings.Contains(body, want) {
			t.Fatalf("intake page missing %q in body:\n%s", want, body)
		}
	}
	containerSelect := selectHTML(t, body, "container_id")
	if strings.Contains(containerSelect, "matrix • Drinking Water") || !strings.Contains(containerSelect, "container • HDPE Bottle") {
		t.Fatalf("container select should only expose container reference items, got: %s", containerSelect)
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

func TestSampleIntakeTemplateHTTPBulkCreatesSamples(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "bulk-http", RequestID: "bulk-http", CorrelationID: "bulk-http", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})

	client, _ := store.CreateClient("HTTP Bulk Client", "bulk-http@example.test", actor)
	project, _ := store.CreateProjectForScope(lab.DefaultScope, lab.ProjectInput{ClientID: client.ID, Name: "Bulk Project", WorkOrder: "WO-BULK", DefaultMatrix: "Fallback"}, actor)
	matrix, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceMatrix, Code: "DW", Name: "Drinking Water", Active: true}, actor)
	dept, _ := store.CreateCatalogDepartment(lab.CatalogDepartmentInput{Name: "Wet Chem"}, actor)
	ph, _ := store.CreateAnalysisService(lab.AnalysisServiceInput{Name: "pH", DepartmentID: dept.ID}, actor)
	profile, _ := store.CreateAnalysisProfile(lab.AnalysisProfileInput{Name: "Bulk Routine", ServiceIDs: []string{ph.ID}}, actor)

	createTemplate := performForm(t, app.createSampleIntakeTemplate, "/api/sample-intake-templates", url.Values{
		"name":                 {"Bulk HTTP template"},
		"client_id":            {client.ID},
		"project_id":           {project.ID},
		"matrix_reference_id":  {matrix.ID},
		"priority":             {"rush"},
		"analysis_profile_ids": {profile.ID},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if createTemplate.Code != http.StatusCreated {
		t.Fatalf("create template status=%d body=%s", createTemplate.Code, createTemplate.Body.String())
	}
	var tmpl lab.SampleIntakeTemplate
	if err := json.Unmarshal(createTemplate.Body.Bytes(), &tmpl); err != nil {
		t.Fatalf("decode template: %v", err)
	}

	payload := []lab.SampleTemplateRowInput{{ClientSampleID: "FIELD-A", LabSampleID: "LAB-A"}, {ClientSampleID: "FIELD-B", LabSampleID: "LAB-B", Comments: "second row"}}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/sample-intake-templates/"+tmpl.ID+"/samples", bytes.NewReader(bodyBytes))
	addDefaultSessionCookie(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", lab.DefaultTenantID)
	req.Header.Set("X-PSC-Lab-ID", lab.DefaultLabID)
	bulk := httptest.NewRecorder()
	app.createSamplesFromTemplate(bulk, req)
	if bulk.Code != http.StatusCreated {
		t.Fatalf("bulk create status=%d body=%s", bulk.Code, bulk.Body.String())
	}
	var samples []lab.Sample
	if err := json.Unmarshal(bulk.Body.Bytes(), &samples); err != nil {
		t.Fatalf("decode bulk samples: %v", err)
	}
	if len(samples) != 2 || samples[0].ProjectID != project.ID || samples[0].Priority != lab.PriorityRush || samples[1].Comments != "second row" {
		t.Fatalf("unexpected bulk samples: %#v", samples)
	}
}
