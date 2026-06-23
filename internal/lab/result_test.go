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

func assertAuditAction(t *testing.T, events []AuditEvent, action, id string) {
	t.Helper()
	for _, event := range events {
		if event.Action == action && event.Resource.Type == "result" && event.Resource.ID == id {
			return
		}
	}
	t.Fatalf("missing audit action %s for result %s in %#v", action, id, events)
}
