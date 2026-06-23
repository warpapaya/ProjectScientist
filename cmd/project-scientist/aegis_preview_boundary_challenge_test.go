package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

func TestAegisReportPreviewRejectsCallerSelectedTenantWithoutMembership(t *testing.T) {
	app, victimScope, sampleID := seededPreviewVictimApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/samples/"+sampleID+"/report-preview", nil)
	req.Header.Set("X-PSC-Tenant-ID", victimScope.TenantID)
	req.Header.Set("X-PSC-Lab-ID", victimScope.LabID)
	rr := httptest.NewRecorder()

	app.previewReportArtifact(rr, req)
	if rr.Code != http.StatusForbidden {
		if rr.Code == http.StatusOK && strings.Contains(rr.Body.String(), "Victim Preview Client") {
			t.Fatalf("report preview leaked caller-selected tenant report content for sample %s: body=%s", sampleID, rr.Body.String())
		}
		t.Fatalf("report preview should fail closed with 403 for unauthorized tenant scope, got status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func seededPreviewVictimApp(t *testing.T) (*app, lab.Scope, string) {
	t.Helper()
	store, err := lab.OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	victimScope := lab.Scope{TenantID: "tenant-victim-preview", LabID: lab.DefaultLabID}
	manager := lab.MustActorContext(lab.ActorContextInput{UserID: "victim-preview-manager", RequestID: "victim-preview-manager", CorrelationID: "victim-preview-manager", TenantMemberships: []lab.TenantMembership{{TenantID: victimScope.TenantID, Roles: []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer)}}}, Roles: []string{string(lab.RoleLabManager), string(lab.RoleAnalyst), string(lab.RoleReviewer)}})
	client, err := store.CreateClientForScope(victimScope, "Victim Preview Client", "victim-preview@example.test", manager)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSampleForScope(victimScope, lab.CreateSampleInput{ClientID: client.ID, Project: "Victim Preview", ClientSampleID: "victim-preview-001", LabSampleID: "VP-001", Matrix: "Water", Tests: []string{"Lead"}}, manager)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	return &app{store: store}, victimScope, sample.ID
}
