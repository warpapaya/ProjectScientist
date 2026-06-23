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

func TestSampleContainerHTTPAndHTMLCreateView(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := attachDefaultSession(t, &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))})
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "http-container", RequestID: "http-container", CorrelationID: "http-container", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	client, _ := store.CreateClient("Container HTTP Client", "container-http@example.test", actor)
	containerRef, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	preservative, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferencePreservative, Code: "HNO3", Name: "Nitric Acid", Active: true}, actor)
	condition, _ := store.CreateSampleReferenceItem(lab.SampleReferenceItemInput{Kind: lab.SampleReferenceReceivedCondition, Code: "OK", Name: "Intact", Active: true}, actor)
	dept, _ := store.CreateCatalogDepartment(lab.CatalogDepartmentInput{Name: "Metals", SortOrder: 1}, actor)
	method, _ := store.CreateCatalogMethod(lab.CatalogMethodInput{Name: "EPA 200.8"}, actor)

	indexRR := httptest.NewRecorder()
	app.index(indexRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if body := indexRR.Body.String(); !strings.Contains(body, "name=\"container_volume\"") || !strings.Contains(body, "name=\"aliquot_department_id\"") || !strings.Contains(body, "Containers / aliquots") {
		t.Fatalf("intake page missing container/aliquot create/view affordances:\n%s", body)
	}

	resp := performForm(t, app.createSample, "/api/samples", url.Values{
		"client_id":             {client.ID},
		"project":               {"Container HTTP"},
		"matrix":                {"Water"},
		"tests":                 {"Lead"},
		"container_id":          {containerRef.ID},
		"preservative_id":       {preservative.ID},
		"received_condition_id": {condition.ID},
		"container_volume":      {"500 mL"},
		"container_condition":   {"intact seal"},
		"aliquot_department_id": {dept.ID},
		"aliquot_method_id":     {method.ID},
		"aliquot_volume":        {"125 mL"},
		"aliquot_purpose":       {"dissolved metals"},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create sample expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var sample lab.Sample
	if err := json.Unmarshal(resp.Body.Bytes(), &sample); err != nil {
		t.Fatalf("decode sample: %v", err)
	}
	if len(sample.Containers) != 1 || sample.Containers[0].Volume != "500 mL" || len(sample.Containers[0].Aliquots) != 1 || sample.Containers[0].Aliquots[0].DepartmentName != "Metals" {
		t.Fatalf("HTTP sample missing container/aliquot data: %#v", sample.Containers)
	}

	viewRR := httptest.NewRecorder()
	app.index(viewRR, newDefaultSessionRequest(http.MethodGet, "/", nil))
	if body := viewRR.Body.String(); !strings.Contains(body, "500 mL") || !strings.Contains(body, "dissolved metals") {
		t.Fatalf("workflow view missing container/aliquot details:\n%s", body)
	}
}
