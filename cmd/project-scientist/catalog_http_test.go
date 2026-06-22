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

func TestCatalogServiceAPIAndHTMLExposeBasicConfig(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}

	deptReq := formRequest("/api/catalog/departments", url.Values{"name": {"Metals"}, "sort_order": {"10"}})
	deptRR := httptest.NewRecorder()
	app.createCatalogDepartment(deptRR, deptReq)
	if deptRR.Code != http.StatusCreated {
		t.Fatalf("department expected 201, got %d body=%s", deptRR.Code, deptRR.Body.String())
	}
	var dept lab.CatalogDepartment
	if err := json.NewDecoder(deptRR.Body).Decode(&dept); err != nil {
		t.Fatalf("decode department: %v", err)
	}

	serviceReq := formRequest("/api/catalog/services", url.Values{"name": {"Lead dissolved"}, "department_id": {dept.ID}, "sort_order": {"20"}})
	serviceRR := httptest.NewRecorder()
	app.createAnalysisService(serviceRR, serviceReq)
	if serviceRR.Code != http.StatusCreated {
		t.Fatalf("service expected 201, got %d body=%s", serviceRR.Code, serviceRR.Body.String())
	}
	var service lab.AnalysisService
	if err := json.NewDecoder(serviceRR.Body).Decode(&service); err != nil {
		t.Fatalf("decode service: %v", err)
	}
	if service.Name != "Lead dissolved" || service.DepartmentName != "Metals" {
		t.Fatalf("unexpected service response: %#v", service)
	}

	profileReq := formRequest("/api/catalog/profiles", url.Values{"name": {"Metals Panel"}, "service_ids": {service.ID}})
	profileRR := httptest.NewRecorder()
	app.createAnalysisProfile(profileRR, profileReq)
	if profileRR.Code != http.StatusCreated {
		t.Fatalf("profile expected 201, got %d body=%s", profileRR.Code, profileRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	app.index(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("index expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Analysis catalog", "Lead dissolved", "Metals Panel"} {
		if !strings.Contains(body, want) {
			t.Fatalf("index missing %q in body:\n%s", want, body)
		}
	}
}

func formRequest(path string, values url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Request-ID", "catalog-http-test")
	return req
}
