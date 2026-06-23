package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResultLifecycleStoresRealisticLabValuesAndAuditsCreateUpdateReview(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActor("chemist")
	client, err := store.CreateClient("Okefenokee Water", "lab@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "DMR June", Matrix: "Wastewater", Tests: []string{"Lead"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected analysis request line for sample test, got %#v", lines)
	}

	result, err := store.CreateResult(ResultInput{
		AnalysisRequestLineID: lines[0].ID,
		Value:                 0.0042,
		RawValue:              "0.0042 mg/L",
		Unit:                  "mg/L",
		Qualifier:             "J",
		MDL:                   0.001,
		RL:                    0.005,
		LOQ:                   0.01,
		Dilution:              2,
		Uncertainty:           0.0004,
		Comments:              "below reporting limit; estimated",
		AnalystID:             "analyst-17",
		InstrumentID:          "ICP-MS-3",
	}, actor)
	if err != nil {
		t.Fatalf("create result: %v", err)
	}
	if result.ID == "" || result.SampleID != sample.ID || result.AnalysisRequestLineID != lines[0].ID {
		t.Fatalf("result was not linked to sample/line: %#v", result)
	}
	if result.Value != 0.0042 || result.RawValue != "0.0042 mg/L" || result.Unit != "mg/L" || result.Qualifier != "J" || result.MDL != 0.001 || result.RL != 0.005 || result.LOQ != 0.01 || result.Dilution != 2 || result.Uncertainty != 0.0004 || result.AnalystID != "analyst-17" || result.InstrumentID != "ICP-MS-3" {
		t.Fatalf("result fields were not preserved: %#v", result)
	}
	if result.Status != ResultStatusEntered {
		t.Fatalf("expected entered status, got %q", result.Status)
	}

	updated, err := store.UpdateResult(result.ID, ResultInput{
		Value:        0.0061,
		RawValue:     "0.0061 mg/L",
		Unit:         "mg/L",
		Qualifier:    "",
		MDL:          0.001,
		RL:           0.005,
		LOQ:          0.01,
		Dilution:     1,
		Uncertainty:  0.0003,
		Comments:     "re-run confirmed above reporting limit",
		AnalystID:    "analyst-17",
		InstrumentID: "ICP-MS-3",
	}, actor)
	if err != nil {
		t.Fatalf("update result: %v", err)
	}
	if updated.Value != 0.0061 || updated.Qualifier != "" || updated.Comments == result.Comments {
		t.Fatalf("update did not persist changed review-relevant fields: %#v", updated)
	}

	accepted, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "approved for report", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer))
	if err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if accepted.Status != ResultStatusAccepted || accepted.ReviewedBy != "reviewer-1" || accepted.ReviewComments != "approved for report" || accepted.ReviewedAt.IsZero() {
		t.Fatalf("review metadata not persisted: %#v", accepted)
	}
	if _, err := store.UpdateResult(result.ID, ResultInput{Value: 0.0072, RawValue: "0.0072 mg/L", Unit: "mg/L", Dilution: 1}, actor); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected post-review result update to be locked, got %v", err)
	}
	amended, err := store.ReopenResult(result.ID, "instrument correction", testActorWithRoles("manager-1", RoleLabManager))
	if err != nil {
		t.Fatalf("reopen result: %v", err)
	}
	if amended.Status != ResultStatusEntered || amended.ReviewedBy != "" || amended.ReopenReason != "instrument correction" {
		t.Fatalf("reopen did not unlock through amend path: %#v", amended)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	assertAuditAction(t, events, "result.created", result.ID)
	assertAuditAction(t, events, "result.updated", result.ID)
	assertAuditAction(t, events, "result.accepted", result.ID)
	assertAuditAction(t, events, "result.update.denied", result.ID)
	assertAuditAction(t, events, "result.reopened", result.ID)
}

func TestReleasedReportBlocksSilentResultMutationAndRequiresReportAmendment(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample: %v", err)
	}
	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "approved for report", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready", manager); err != nil {
		t.Fatalf("QC review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "batch acceptable", manager); err != nil {
		t.Fatalf("QC accept: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}
	originalReport, err := store.GenerateCOAReportArtifact(COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: "coa-standard", Version: "2026.06", Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: "Demo Client"}}, testActorWithRoles("report-releaser-1", RoleReportReleaser))
	if err != nil {
		t.Fatalf("generate original report: %v", err)
	}

	if _, err := store.ReopenResult(result.ID, "silent analyst correction", manager); err == nil || !strings.Contains(err.Error(), "released report amendment") {
		t.Fatalf("expected normal reopen to be blocked after released report, got %v", err)
	}
	if _, err := store.UpdateResult(result.ID, ResultInput{Value: 9.9, RawValue: "9.9 mg/L", Unit: "mg/L", Dilution: 1}, testActorWithRoles("analyst-1", RoleAnalyst)); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected direct post-report update to remain locked, got %v", err)
	}

	amended, err := store.ReopenResultForReportAmendment(result.ID, ReportResultAmendmentInput{Reason: "transcription correction", SupersededSnapshotID: originalReport.Snapshot.ID, SupersededArtifactID: originalReport.Artifact.ID}, testActorWithRoles("report-releaser-2", RoleReportReleaser))
	if err != nil {
		t.Fatalf("open report amendment: %v", err)
	}
	if amended.Status != ResultStatusEntered || amended.ReopenReason != "transcription correction" {
		t.Fatalf("amendment did not reopen result with reason: %#v", amended)
	}
	updated, err := store.UpdateResult(result.ID, ResultInput{Value: 1.4, RawValue: "1.4 mg/L", Unit: "mg/L", Dilution: 1, AnalystID: "analyst-1"}, testActorWithRoles("analyst-1", RoleAnalyst))
	if err != nil {
		t.Fatalf("update amended result: %v", err)
	}
	if updated.Value != 1.4 {
		t.Fatalf("amended result update not persisted: %#v", updated)
	}
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "amendment approved", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-2", RoleReviewer)); err != nil {
		t.Fatalf("review amended result: %v", err)
	}
	newReport, err := store.GenerateCOAReportArtifact(COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: "coa-standard", Version: "2026.06", Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: "Demo Client"}}, testActorWithRoles("report-releaser-2", RoleReportReleaser))
	if err != nil {
		t.Fatalf("generate superseding report: %v", err)
	}
	if newReport.Snapshot.SupersedesSnapshotID != originalReport.Snapshot.ID || newReport.Artifact.SupersedesArtifactID != originalReport.Artifact.ID {
		t.Fatalf("new report did not supersede original: new=%#v original=%#v", newReport, originalReport)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	assertReportAmendmentAudit(t, events, result.ID, originalReport.Snapshot.ID, originalReport.Artifact.ID, "transcription correction")
}

func TestReportAmendmentRequiresReportReleaserReasonAndSupersededLinks(t *testing.T) {
	store, sample, batch := seedReleaseReadinessFixture(t)
	defer store.Close()

	manager := testActorWithRoles("release-manager", RoleLabManager)
	if err := advanceSampleToReview(store, sample.ID, manager); err != nil {
		t.Fatalf("advance sample: %v", err)
	}
	result := store.ResultsForScope(DefaultScope)[0]
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "approved for report", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-1", RoleReviewer)); err != nil {
		t.Fatalf("accept result: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusInReview, "ready", manager); err != nil {
		t.Fatalf("QC review: %v", err)
	}
	if _, err := store.TransitionQCBatch(batch.ID, QCBatchStatusAccepted, "batch acceptable", manager); err != nil {
		t.Fatalf("QC accept: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusReleased, manager); err != nil {
		t.Fatalf("release sample: %v", err)
	}
	originalReport, err := store.GenerateCOAReportArtifact(COAGenerationInput{SampleID: sample.ID, Template: COATemplate{ID: "coa-standard", Version: "2026.06", Style: COAStyleCENLA, LabName: "Clearline Demo Lab", ClientName: "Demo Client"}}, testActorWithRoles("report-releaser-1", RoleReportReleaser))
	if err != nil {
		t.Fatalf("generate original report: %v", err)
	}

	if _, err := store.ReopenResultForReportAmendment(result.ID, ReportResultAmendmentInput{Reason: " ", SupersededSnapshotID: originalReport.Snapshot.ID, SupersededArtifactID: originalReport.Artifact.ID}, testActorWithRoles("report-releaser-2", RoleReportReleaser)); err == nil || !strings.Contains(err.Error(), "amendment reason is required") {
		t.Fatalf("expected reason validation, got %v", err)
	}
	if _, err := store.ReopenResultForReportAmendment(result.ID, ReportResultAmendmentInput{Reason: "transcription correction", SupersededSnapshotID: originalReport.Snapshot.ID, SupersededArtifactID: originalReport.Artifact.ID}, manager); err == nil || !strings.Contains(err.Error(), "report amendment requires report-releaser role") {
		t.Fatalf("expected report-releaser authorization, got %v", err)
	}
	if _, err := store.ReopenResultForReportAmendment(result.ID, ReportResultAmendmentInput{Reason: "transcription correction", SupersededSnapshotID: "RS-404", SupersededArtifactID: originalReport.Artifact.ID}, testActorWithRoles("report-releaser-2", RoleReportReleaser)); err == nil || !strings.Contains(err.Error(), "current released report") {
		t.Fatalf("expected superseded snapshot validation, got %v", err)
	}
}

func assertReportAmendmentAudit(t *testing.T, events []AuditEvent, resultID, snapshotID, artifactID, reason string) {
	t.Helper()
	for _, event := range events {
		if event.Action != "result.report_amendment.opened" || event.Resource.ID != resultID || event.Outcome != AuditOutcomeAllowed {
			continue
		}
		if event.Details["superseded_snapshot_id"] == snapshotID && event.Details["superseded_artifact_id"] == artifactID && event.Details["reason"] == reason {
			return
		}
	}
	t.Fatalf("report amendment audit link missing for result=%s snapshot=%s artifact=%s events=%#v", resultID, snapshotID, artifactID, events)
}

func TestResultValidationRejectsMissingAndInvalidFields(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActor("chemist")
	if _, err := store.CreateResult(ResultInput{Value: 7.1, Unit: "mg/L"}, actor); err == nil || !strings.Contains(err.Error(), "analysis request line id is required") {
		t.Fatalf("expected missing line id validation, got %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: "ARL-404", Value: 7.1, Unit: "mg/L", Dilution: 1}, actor); err == nil || !strings.Contains(err.Error(), "unknown analysis request line") {
		t.Fatalf("expected unknown analysis request line validation, got %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: "ARL-404", Value: 7.1, Unit: " ", Dilution: 1}, actor); err == nil || !strings.Contains(err.Error(), "unit is required") {
		t.Fatalf("expected required unit validation before lookup, got %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: "ARL-404", Value: 7.1, Unit: "mg/L", MDL: -0.1, Dilution: 1}, actor); err == nil || !strings.Contains(err.Error(), "mdl cannot be negative") {
		t.Fatalf("expected negative MDL validation, got %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: "ARL-404", Value: 7.1, Unit: "mg/L", Dilution: 0}, actor); err == nil || !strings.Contains(err.Error(), "dilution must be greater than zero") {
		t.Fatalf("expected dilution validation, got %v", err)
	}
}

func TestResultLookupByAnalysisRequestLineRespectsScope(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActor("chemist")
	client, err := store.CreateClient("Result Lookup", "lookup@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Lookup", Matrix: "Water", Tests: []string{"Nitrate"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one analysis request line, got %#v", lines)
	}
	created, err := store.CreateResult(ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 1.2, RawValue: "1.2 mg/L", Unit: "mg/L", Dilution: 1, AnalystID: "analyst-lookup"}, actor)
	if err != nil {
		t.Fatalf("create result: %v", err)
	}

	loaded, ok := store.GetResultForAnalysisRequestLine(lines[0].ID)
	if !ok || loaded.ID != created.ID || loaded.SampleID != sample.ID {
		t.Fatalf("result lookup by analysis request line failed, ok=%v loaded=%#v", ok, loaded)
	}
	if _, ok := store.GetResultForAnalysisRequestLineForScope(Scope{TenantID: "other-tenant", LabID: DefaultLabID}, lines[0].ID); ok {
		t.Fatalf("cross-tenant result lookup should not return a result")
	}
}

func TestResultEntryRejectsCancelledAnalysisRequestLines(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	actor := testActor("chemist")
	client, err := store.CreateClient("Cancelled Line", "cancelled@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Cancelled", Matrix: "Water", Tests: []string{"Nitrate"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one analysis request line, got %#v", lines)
	}
	if err := store.TransitionAnalysisRequestLine(lines[0].ID, AnalysisRequestLineStatusCancelled, actor); err != nil {
		t.Fatalf("cancel analysis request line: %v", err)
	}
	if _, err := store.CreateResult(ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 1.2, RawValue: "1.2 mg/L", Unit: "mg/L", Dilution: 1}, actor); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancelled analysis request line rejection, got %v", err)
	}
}

func assertAuditAction(t *testing.T, events []AuditEvent, action, id string) {
	t.Helper()
	for _, event := range events {
		if event.Action == action && event.Resource.Type == "result" && event.Resource.ID == id {
			return
		}
	}
	t.Fatalf("missing audit action %s for result %s in %#v", action, id, events)
}
