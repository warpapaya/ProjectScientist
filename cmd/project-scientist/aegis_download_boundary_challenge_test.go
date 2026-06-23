package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestAegisReportArtifactDownloadRejectsCallerSelectedTenantWithoutMembership(t *testing.T) {
	app, victimScope, artifactID, _ := seededDownloadVictimApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/report-artifacts/"+artifactID, nil)
	req.Header.Set("X-PSC-Tenant-ID", victimScope.TenantID)
	req.Header.Set("X-PSC-Lab-ID", victimScope.LabID)
	rr := httptest.NewRecorder()

	app.reportArtifactDownload(rr, req)
	if rr.Code != http.StatusForbidden {
		if rr.Code == http.StatusOK {
			t.Fatalf("report artifact download leaked caller-selected tenant artifact %s: body=%s", artifactID, rr.Body.String())
		}
		t.Fatalf("report artifact download should fail closed with 403 for unauthorized tenant scope, got status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAegisCOCPackageDownloadRejectsCallerSelectedTenantWithoutMembership(t *testing.T) {
	app, victimScope, _, cocID := seededDownloadVictimApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/coc-packages/"+cocID, nil)
	req.Header.Set("X-PSC-Tenant-ID", victimScope.TenantID)
	req.Header.Set("X-PSC-Lab-ID", victimScope.LabID)
	rr := httptest.NewRecorder()

	app.cocPackageDownload(rr, req)
	if rr.Code != http.StatusForbidden {
		if rr.Code == http.StatusOK {
			t.Fatalf("COC package download leaked caller-selected tenant package %s: body=%s", cocID, rr.Body.String())
		}
		t.Fatalf("COC package download should fail closed with 403 for unauthorized tenant scope, got status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func seededDownloadVictimApp(t *testing.T) (*app, lab.Scope, string, string) {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	victimScope := lab.Scope{TenantID: "tenant-victim-download", LabID: lab.DefaultLabID}
	manager := lab.MustActorContext(lab.ActorContextInput{UserID: "victim-manager", RequestID: "victim-manager", CorrelationID: "victim-manager", TenantMemberships: []lab.TenantMembership{{TenantID: victimScope.TenantID, Roles: []string{string(lab.RoleLabManager), string(lab.RoleReportReleaser), string(lab.RoleAnalyst), string(lab.RoleReviewer)}}}, Roles: []string{string(lab.RoleLabManager), string(lab.RoleReportReleaser), string(lab.RoleAnalyst), string(lab.RoleReviewer)}})
	client, err := store.CreateClientForScope(victimScope, "Victim Download Client", "victim-download@example.test", manager)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSampleForScope(victimScope, lab.CreateSampleInput{ClientID: client.ID, Project: "Victim Download", ClientSampleID: "victim-001", LabSampleID: "VD-001", Matrix: "Water", Tests: []string{"Lead"}}, manager)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	for _, status := range []lab.SampleStatus{lab.StatusInPrep, lab.StatusInAnalysis, lab.StatusInReview} {
		if err := store.TransitionSampleForScope(victimScope, sample.ID, status, manager); err != nil {
			t.Fatalf("advance sample to %s: %v", status, err)
		}
	}
	line := store.AnalysisRequestLinesForScope(victimScope)[0]
	result, err := store.CreateResultForScope(victimScope, lab.ResultInput{AnalysisRequestLineID: line.ID, Value: 1.23, RawValue: "1.23 mg/L", Unit: "mg/L", Dilution: 1}, manager)
	if err != nil {
		t.Fatalf("create result: %v", err)
	}
	if _, err := store.ReviewResultForScope(victimScope, result.ID, lab.ResultReviewInput{Decision: lab.ResultDecisionAccept, Comments: "approved", EnforceReviewerSeparation: false}, manager); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if _, err := store.RecordCustodyEventForScope(victimScope, lab.CustodyEventInput{SampleID: sample.ID, Type: lab.CustodyReceived, Location: "Receiving", Reason: "victim fixture"}, manager); err != nil {
		t.Fatalf("record custody: %v", err)
	}
	if err := store.TransitionSampleForScope(victimScope, sample.ID, lab.StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}
	released, err := store.GenerateCOAReportArtifactForScope(victimScope, lab.COAGenerationInput{SampleID: sample.ID, Template: lab.COATemplate{ID: "coa-standard", Version: "2026.06", Style: lab.COAStyleCENLA, LabName: "Victim Lab", ClientName: "Victim Client"}}, manager)
	if err != nil {
		t.Fatalf("generate report artifact: %v", err)
	}
	pkg, err := store.GenerateCOCPackageForScope(victimScope, lab.COCPackageInput{SampleID: sample.ID, PackageFormat: "application/vnd.project-scientist.coc+json", Attachments: []lab.ReportPackageAttachmentInput{{Name: "custody-history.json", MediaType: "application/json", Content: []byte("victim-custody")}}}, manager)
	if err != nil {
		t.Fatalf("generate COC package: %v", err)
	}
	return &app{store: store}, victimScope, released.Artifact.ID, pkg.ID
}
