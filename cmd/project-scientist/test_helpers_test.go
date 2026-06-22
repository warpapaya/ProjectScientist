package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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
