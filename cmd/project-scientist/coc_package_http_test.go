package main

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestCOCPackageHTTPGeneratesSyntheticPackageAndExposesWorkflowAction(t *testing.T) {
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}
	actor := lab.MustActorContext(lab.ActorContextInput{UserID: "http-package-releaser", RequestID: "http-package-releaser", CorrelationID: "http-package-releaser", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager), string(lab.RoleReportReleaser)}}}, Roles: []string{string(lab.RoleLabManager), string(lab.RoleReportReleaser)}})
	client, _ := store.CreateClient("COC Package HTTP Client", "coc-package-http@example.test", actor)
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "COC Package HTTP", ClientSampleID: "HTTP-FIELD-1", Matrix: "Water", Tests: []string{"pH"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if _, err := store.RecordCustodyEvent(lab.CustodyEventInput{SampleID: sample.ID, Type: lab.CustodyReceived, Location: "Receiving", Reason: "COC intake", OccurredAt: time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)}, actor); err != nil {
		t.Fatalf("record custody: %v", err)
	}

	indexRR := httptestNewRecorder()
	app.index(indexRR, httptestNewRequest(http.MethodGet, "/", nil))
	if body := indexRR.Body.String(); !strings.Contains(body, "/api/samples/"+sample.ID+"/coc-package") || !strings.Contains(body, "Generate COC package") {
		t.Fatalf("workflow page missing COC package generation action:\n%s", body)
	}

	resp := performForm(t, app.sampleAction, "/api/samples/"+sample.ID+"/coc-package", url.Values{
		"package_format":          {"application/vnd.project-scientist.coc+json"},
		"attachment_name":         {"custody-form.pdf"},
		"attachment_media_type":   {"application/pdf"},
		"attachment_content_text": {"synthetic COC PDF bytes"},
	}, lab.DefaultTenantID, lab.DefaultLabID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("generate COC package expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var pkg lab.COCPackage
	if err := json.Unmarshal(resp.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("decode COC package: %v", err)
	}
	if pkg.SampleID != sample.ID || !strings.HasPrefix(pkg.ContentHash, "sha256:") || len(pkg.CustodyEvents) != 1 || len(pkg.Attachments) != 1 {
		t.Fatalf("unexpected COC package response: %#v", pkg)
	}
	if !strings.Contains(string(pkg.Content), "HTTP-FIELD-1") || !strings.Contains(string(pkg.Content), "custody-form.pdf") {
		t.Fatalf("package content missing synthetic sample/attachment data: %s", pkg.Content)
	}
}

func httptestNewRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }
func httptestNewRequest(method, target string, body io.Reader) *http.Request {
	return httptest.NewRequest(method, target, body)
}
