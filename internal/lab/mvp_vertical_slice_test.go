package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestMVPVerticalSliceRunsSyntheticLIMSHappyPathAndNegativeControls(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	summary, err := store.RunMVPVerticalSlice(MVPVerticalSliceInput{
		ClientName:      "Clearline Demo Client",
		ContactName:     "Sam Sample Submitter",
		ProjectName:     "MVP synthetic compliance project",
		WorkOrder:       "WO-MVP-005",
		ClientSampleID:  "MVP-CLIENT-001",
		LabSampleID:     "MVP-LAB-001",
		AnalysisProfile: "MVP Metals Profile",
		TemplateID:      "coa-mvp-standard",
		TemplateVersion: "2026.06-mvp",
	}, testActorWithRoles("mvp-lab-manager", RoleLabManager, RoleAdmin))
	if err != nil {
		t.Fatalf("run mvp vertical slice: %v", err)
	}

	if summary.Client.ID == "" || summary.Project.ID == "" || summary.Sample.ID == "" {
		t.Fatalf("master data and sample were not created: %#v", summary)
	}
	if got := len(summary.Sample.Containers); got != 1 {
		t.Fatalf("expected one received container on the sample, got %d", got)
	}
	if summary.Label.ArtifactID == "" || !strings.HasPrefix(summary.Label.ContentHash, "sha256:") {
		t.Fatalf("label artifact was not generated: %#v", summary.Label)
	}
	if got := len(summary.AnalysisRequestLines); got != 2 {
		t.Fatalf("expected two profile-expanded analysis request lines, got %d: %#v", got, summary.AnalysisRequestLines)
	}
	if summary.Worksheet.ID == "" || summary.Worksheet.Status != WorksheetStatusCompleted {
		t.Fatalf("worksheet should be completed, got %#v", summary.Worksheet)
	}
	if got := len(summary.Results); got != 2 {
		t.Fatalf("expected two accepted results, got %d: %#v", got, summary.Results)
	}
	for _, result := range summary.Results {
		if result.Status != ResultStatusAccepted || result.ReviewedBy == "" {
			t.Fatalf("result was not reviewed and locked: %#v", result)
		}
	}
	if summary.QCBatch.ID == "" || summary.QCBatch.Status != QCBatchStatusAccepted {
		t.Fatalf("QC batch should be accepted before release: %#v", summary.QCBatch)
	}
	if summary.Report.Artifact.ID == "" || summary.Report.Artifact.ContentHash == "" || !strings.Contains(string(summary.Report.Artifact.Content), "CERTIFICATE OF ANALYSIS") {
		t.Fatalf("COA/report artifact was not released: %#v", summary.Report.Artifact)
	}
	if summary.Sample.Status != StatusReleased {
		t.Fatalf("sample should be released after review/QC/report preconditions, got %#v", summary.Sample)
	}
	if len(summary.DeniedControls) < 3 {
		t.Fatalf("expected denied control evidence, got %#v", summary.DeniedControls)
	}

	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: summary.AnalysisRequestLines[0].ID, Value: 9.9, RawValue: "9.9 mg/L", Unit: "mg/L", Dilution: 1}, testActorWithRoles("client-contact", RoleClientContact)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected unauthorized protected mutation to still be denied after vertical slice, got %v", err)
	}
	events, err := store.AuditEventsForScope(DefaultScope, 0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	for _, action := range []string{"client.created", "sample.created", "sample.label_artifact.generated", "worksheet.created", "result.accepted", "sample.release.blocked", "report.artifact.released"} {
		if !auditContainsAnyResource(events, action) {
			t.Fatalf("missing audit action %q in %#v", action, events)
		}
	}
}

func auditContainsAnyResource(events []AuditEvent, action string) bool {
	for _, event := range events {
		if event.Action == action {
			return true
		}
	}
	return false
}
