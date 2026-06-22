package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestResultEntryPersistsQualifierLimitsUnitsUncertaintyAndAuditsCreate(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	line := createAnalysisRequestLineFixture(t, store, actor)

	result, err := store.UpsertResult(ResultInput{
		AnalysisRequestLineID: line.ID,
		Value:                 "0.004",
		RawValue:              "0.0037",
		Unit:                  "mg/L",
		Qualifier:             "J",
		MDL:                   "0.001",
		RL:                    "0.005",
		LOQ:                   "0.010",
		Dilution:              "2",
		Uncertainty:           "+/- 0.0004",
		Comments:              "estimated below reporting limit",
		AnalystID:             "analyst-1",
		InstrumentID:          "ICP-01",
	}, actor)
	if err != nil {
		t.Fatalf("upsert result: %v", err)
	}
	if result.ID == "" || result.AnalysisRequestLineID != line.ID || result.SampleID != line.SampleID {
		t.Fatalf("result identity not linked to line/sample: %#v line=%#v", result, line)
	}
	assertResultFields(t, result)

	loaded, ok := store.GetResultForAnalysisRequestLine(line.ID)
	if !ok {
		t.Fatalf("load result for line %q", line.ID)
	}
	assertResultFields(t, loaded)

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var created bool
	for _, event := range events {
		if event.Action == "result.created" && event.Outcome == AuditOutcomeAllowed && event.Resource.Type == "result" && event.Resource.ID == result.ID {
			created = true
			if event.Details["analysis_request_line_id"] != line.ID || event.Details["sample_id"] != line.SampleID || event.Details["qualifier"] != "J" || event.Details["unit"] != "mg/L" {
				t.Fatalf("result create audit missing review-relevant details: %#v", event.Details)
			}
		}
	}
	if !created {
		t.Fatalf("missing result.created audit event for %#v", result)
	}
}

func TestResultUpdateValidatesReviewRelevantChangesAndDeniedAudits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	line := createAnalysisRequestLineFixture(t, store, actor)
	created, err := store.UpsertResult(ResultInput{AnalysisRequestLineID: line.ID, Value: "7.1", Unit: "pH", AnalystID: "analyst-a"}, actor)
	if err != nil {
		t.Fatalf("create result: %v", err)
	}

	updated, err := store.UpsertResult(ResultInput{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", Qualifier: "R", RL: "0.1", Comments: "rerun confirmed", AnalystID: "analyst-b", InstrumentID: "meter-2"}, actor)
	if err != nil {
		t.Fatalf("update result: %v", err)
	}
	if updated.ID != created.ID || updated.Value != "7.3" || updated.Qualifier != "R" || updated.Comments != "rerun confirmed" || updated.AnalystID != "analyst-b" || updated.InstrumentID != "meter-2" {
		t.Fatalf("unexpected updated result: %#v", updated)
	}
	if _, err := store.UpsertResult(ResultInput{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", Dilution: "0"}, actor); err == nil || !strings.Contains(err.Error(), "dilution") {
		t.Fatalf("expected invalid dilution validation error, got %v", err)
	}
	for _, invalid := range []ResultInput{
		{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", MDL: "NaN"},
		{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", RL: "+Inf"},
		{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", LOQ: "-Inf"},
		{AnalysisRequestLineID: line.ID, Value: "7.3", Unit: "pH", Dilution: "+Inf"},
	} {
		if _, err := store.UpsertResult(invalid, actor); err == nil || !strings.Contains(strings.ToLower(err.Error()), "finite") {
			t.Fatalf("expected finite numeric validation error for %#v, got %v", invalid, err)
		}
	}

	otherScope := Scope{TenantID: "other-tenant", LabID: DefaultLabID}
	otherActor := MustActorContext(ActorContextInput{UserID: "other-analyst", DisplayName: "Other Analyst", AuthProvider: "test", RequestID: "other", CorrelationID: "other", TenantMemberships: []TenantMembership{{TenantID: otherScope.TenantID, Roles: []string{string(RoleAnalyst)}}}, Roles: []string{string(RoleAnalyst)}})
	if _, err := store.UpsertResultForScope(otherScope, ResultInput{AnalysisRequestLineID: line.ID, Value: "8.0", Unit: "pH"}, otherActor); err == nil || !strings.Contains(err.Error(), "unknown analysis request line") {
		t.Fatalf("expected non-revealing unknown line error, got %v", err)
	}
	if _, ok := store.GetResultForAnalysisRequestLineForScope(otherScope, line.ID); ok {
		t.Fatalf("cross-tenant read should not return line result")
	}

	clientActor := actorWithRoles("client-contact", RoleClientContact)
	if _, err := store.UpsertResult(ResultInput{AnalysisRequestLineID: line.ID, Value: "8.0", Unit: "pH"}, clientActor); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected authorization denial, got %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	var updateAudited, authDenied bool
	for _, event := range events {
		if event.Action == "result.updated" && event.Resource.ID == created.ID && event.Outcome == AuditOutcomeAllowed {
			updateAudited = true
			fields, _ := event.Details["changed_fields"].([]any)
			if len(fields) == 0 {
				t.Fatalf("result update audit missing changed_fields: %#v", event.Details)
			}
		}
		if event.Action == string(OperationResultEntry) && event.Outcome == AuditOutcomeDenied && event.Reason == "authorization_denied" {
			authDenied = true
		}
	}
	if !updateAudited || !authDenied {
		t.Fatalf("expected update/auth audit events, update=%v auth=%v", updateAudited, authDenied)
	}
}

func TestResultEntryRejectsCancelledAnalysisRequestLines(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	line := createAnalysisRequestLineFixture(t, store, actor)
	if err := store.TransitionAnalysisRequestLine(line.ID, AnalysisRequestLineStatusCancelled, actor); err != nil {
		t.Fatalf("cancel line: %v", err)
	}
	if _, err := store.UpsertResult(ResultInput{AnalysisRequestLineID: line.ID, Value: "1.2", Unit: "mg/L"}, actor); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancelled line result rejection, got %v", err)
	}
}

func assertResultFields(t *testing.T, result Result) {
	t.Helper()
	if result.Value != "0.004" || result.RawValue != "0.0037" || result.Unit != "mg/L" || result.Qualifier != "J" || result.MDL != "0.001" || result.RL != "0.005" || result.LOQ != "0.010" || result.Dilution != "2" || result.Uncertainty != "+/- 0.0004" || result.Comments != "estimated below reporting limit" || result.AnalystID != "analyst-1" || result.InstrumentID != "ICP-01" {
		t.Fatalf("result fields not persisted: %#v", result)
	}
}
