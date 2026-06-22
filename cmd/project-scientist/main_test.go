package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestScopedFormMutationsRedirectBackToSameTenantLab(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	form := url.Values{
		"tenant_id": {"tenant-alpha"},
		"lab_id":    {"water-lab"},
		"name":      {"Alpha Client"},
		"email":     {"alpha@example.test"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()

	app.createClient(res, req)

	if res.Code != http.StatusSeeOther {
		t.Fatalf("redirect status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	location := res.Header().Get("Location")
	redirectReq := httptest.NewRequest(http.MethodGet, location, nil)
	got := scopeFromRequest(redirectReq)
	if got.TenantID != "tenant-alpha" || got.LabID != "water-lab" {
		t.Fatalf("redirected request scope = %#v", got)
	}
}

func TestAPIStateAndMutationsAreTenantLabScoped(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store}

	clientResp := performForm(t, app.createClient, "/api/clients", url.Values{"name": {"Alpha Client"}, "email": {"alpha@example.test"}}, "tenant-alpha", "water-lab")
	if clientResp.Code != http.StatusCreated {
		t.Fatalf("create alpha client status = %d body=%s", clientResp.Code, clientResp.Body.String())
	}
	var client lab.Client
	if err := json.Unmarshal(clientResp.Body.Bytes(), &client); err != nil {
		t.Fatalf("decode client: %v", err)
	}
	if client.TenantID != "tenant-alpha" || client.LabID != "water-lab" {
		t.Fatalf("client response missing scope: %#v", client)
	}

	crossResp := performForm(t, app.createSample, "/api/samples", url.Values{"client_id": {client.ID}, "project": {"Cross Tenant"}, "matrix": {"Water"}, "tests": {"pH"}}, "tenant-beta", "water-lab")
	if crossResp.Code != http.StatusBadRequest {
		t.Fatalf("cross-tenant sample status = %d, want %d", crossResp.Code, http.StatusBadRequest)
	}

	sampleResp := performForm(t, app.createSample, "/api/samples", url.Values{"client_id": {client.ID}, "project": {"Alpha Project"}, "matrix": {"Water"}, "tests": {"pH"}}, "tenant-alpha", "water-lab")
	if sampleResp.Code != http.StatusCreated {
		t.Fatalf("create alpha sample status = %d body=%s", sampleResp.Code, sampleResp.Body.String())
	}

	betaState := performGet(t, app.apiState, "/api/state", "tenant-beta", "water-lab")
	var beta pageData
	if err := json.Unmarshal(betaState.Body.Bytes(), &beta); err != nil {
		t.Fatalf("decode beta state: %v", err)
	}
	if len(beta.Clients) != 0 || len(beta.Samples) != 0 || len(beta.Audit) != 0 {
		t.Fatalf("beta state leaked alpha data: %#v", beta)
	}

	alphaState := performGet(t, app.apiState, "/api/state", "tenant-alpha", "water-lab")
	var alpha pageData
	if err := json.Unmarshal(alphaState.Body.Bytes(), &alpha); err != nil {
		t.Fatalf("decode alpha state: %v", err)
	}
	if len(alpha.Clients) != 1 || len(alpha.Samples) != 1 || len(alpha.Audit) != 2 {
		t.Fatalf("alpha state missing scoped data: %#v", alpha)
	}
}

func performForm(t *testing.T, handler http.HandlerFunc, path string, form url.Values, tenantID, labID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", tenantID)
	req.Header.Set("X-PSC-Lab-ID", labID)
	res := httptest.NewRecorder()
	handler(res, req)
	return res
}

func performGet(t *testing.T, handler http.HandlerFunc, path, tenantID, labID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PSC-Tenant-ID", tenantID)
	req.Header.Set("X-PSC-Lab-ID", labID)
	res := httptest.NewRecorder()
	handler(res, req)
	return res
}
