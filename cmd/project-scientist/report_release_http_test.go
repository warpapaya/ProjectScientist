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

func TestReportReleaseDeskShowsBlockedReleaseAndAmendmentActions(t *testing.T) {
	store, sample := labSeedReportReleaseHTTPFixture(t)
	defer store.Close()
	app := &app{store: store, tmpl: template.Must(template.ParseFiles(filepath.Join("..", "..", "web", "templates", "index.html")))}

	indexRR := httptest.NewRecorder()
	app.index(indexRR, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRR.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", indexRR.Code, indexRR.Body.String())
	}
	blockedBody := indexRR.Body.String()
	for _, want := range []string{"Report release desk", "Blocked", "sample_status", "unaccepted_result", "Preview COA", "Release report", "/api/samples/" + sample.ID + "/report-preview", "/api/samples/" + sample.ID + "/report-release"} {
		if !strings.Contains(blockedBody, want) {
			t.Fatalf("release desk missing %q before readiness:\n%s", want, blockedBody)
		}
	}

	previewRR := httptest.NewRecorder()
	app.routes().ServeHTTP(previewRR, httptest.NewRequest(http.MethodGet, "/api/samples/"+sample.ID+"/report-preview?tenant_id="+lab.DefaultTenantID+"&lab_id="+lab.DefaultLabID, nil))
	if previewRR.Code != http.StatusOK || !strings.Contains(previewRR.Body.String(), "CERTIFICATE OF ANALYSIS") || !strings.Contains(previewRR.Body.String(), sample.ID) {
		t.Fatalf("preview should render COA content without releasing, status=%d body=%s", previewRR.Code, previewRR.Body.String())
	}
	if readiness, ok := store.ReportReleaseReadinessForScope(lab.DefaultScope, sample.ID); !ok || readiness.CurrentArtifactID != "" {
		t.Fatalf("preview must not create a current report artifact: ok=%v readiness=%#v", ok, readiness)
	}

	blockedRelease := performForm(t, app.sampleAction, "/api/samples/"+sample.ID+"/report-release", url.Values{"template_id": {"coa-standard"}, "template_version": {"2026.06"}}, lab.DefaultTenantID, lab.DefaultLabID)
	if blockedRelease.Code != http.StatusBadRequest || !strings.Contains(blockedRelease.Body.String(), "released sample") {
		t.Fatalf("blocked release should explain backend blocker, status=%d body=%s", blockedRelease.Code, blockedRelease.Body.String())
	}

	manager := lab.MustActorContext(lab.ActorContextInput{UserID: "release-manager", RequestID: "release-manager", CorrelationID: "release-manager", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	result := store.ResultsForScope(lab.DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, lab.ResultReviewInput{Decision: lab.ResultDecisionAccept, Comments: "approved for COA", EnforceReviewerSeparation: true}, lab.MustActorContext(lab.ActorContextInput{UserID: "reviewer-1", RequestID: "reviewer-1", CorrelationID: "reviewer-1", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleReviewer)}}}, Roles: []string{string(lab.RoleReviewer)}})); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if err := store.TransitionSample(sample.ID, lab.StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}

	firstRelease := performForm(t, app.sampleAction, "/api/samples/"+sample.ID+"/report-release", url.Values{"template_id": {"coa-standard"}, "template_version": {"2026.06"}, "lab_name": {"Clearline Demo Lab"}, "client_name": {"Demo Client"}}, lab.DefaultTenantID, lab.DefaultLabID)
	if firstRelease.Code != http.StatusCreated {
		t.Fatalf("first release status=%d body=%s", firstRelease.Code, firstRelease.Body.String())
	}

	readyRR := httptest.NewRecorder()
	app.index(readyRR, httptest.NewRequest(http.MethodGet, "/", nil))
	readyBody := readyRR.Body.String()
	for _, want := range []string{"Ready", "Amend report", "Download current report", "Download COC package"} {
		if !strings.Contains(readyBody, want) {
			t.Fatalf("release desk missing %q after release/package:\n%s", want, readyBody)
		}
	}

	stateRR := performGet(t, app.apiState, "/api/state", lab.DefaultTenantID, lab.DefaultLabID)
	var state pageData
	if err := json.Unmarshal(stateRR.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if len(state.ReportReadiness) != 1 || state.ReportReadiness[0].ReleaseAction != "amendment" || state.ReportReadiness[0].CurrentArtifactID == "" {
		t.Fatalf("api state should expose report readiness/current artifact: %#v", state.ReportReadiness)
	}
}

func labSeedReportReleaseHTTPFixture(t *testing.T) (*lab.Store, lab.Sample) {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	manager := lab.MustActorContext(lab.ActorContextInput{UserID: "release-manager", RequestID: "release-manager", CorrelationID: "release-manager", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleLabManager)}}}, Roles: []string{string(lab.RoleLabManager)}})
	client, err := store.CreateClient("Report Release HTTP Client", "report-http@example.test", manager)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Report HTTP", ClientSampleID: "client-01", LabSampleID: "PS-RPT-0001", Matrix: "Water", Tests: []string{"Lead"}}, manager)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	for _, status := range []lab.SampleStatus{lab.StatusInPrep, lab.StatusInAnalysis, lab.StatusInReview} {
		if err := store.TransitionSample(sample.ID, status, manager); err != nil {
			t.Fatalf("advance sample to %s: %v", status, err)
		}
	}
	line := store.AnalysisRequestLinesForScope(lab.DefaultScope)[0]
	if _, err := store.CreateResult(lab.ResultInput{AnalysisRequestLineID: line.ID, Value: 1.23, RawValue: "1.23 mg/L", Unit: "mg/L", Dilution: 1}, lab.MustActorContext(lab.ActorContextInput{UserID: "analyst-1", RequestID: "analyst-1", CorrelationID: "analyst-1", TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID, Roles: []string{string(lab.RoleAnalyst)}}}, Roles: []string{string(lab.RoleAnalyst)}})); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if _, err := store.RecordCustodyEvent(lab.CustodyEventInput{SampleID: sample.ID, Type: lab.CustodyReceived, Location: "Receiving", Reason: "HTTP fixture"}, manager); err != nil {
		t.Fatalf("record custody: %v", err)
	}
	if _, err := store.GenerateCOCPackage(lab.COCPackageInput{SampleID: sample.ID, PackageFormat: "application/vnd.project-scientist.coc+json", Attachments: []lab.ReportPackageAttachmentInput{{Name: "custody-history.json", MediaType: "application/json", Content: []byte("{}")}}}, manager); err != nil {
		t.Fatalf("generate COC package: %v", err)
	}
	return store, sample
}
