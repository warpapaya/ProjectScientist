package lab

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestResultReviewCanRejectAndReviewerSeparationBlocksAnalystSelfReview(t *testing.T) {
	store, result := seedResultForReview(t)
	defer store.Close()

	analyst := testActorWithRoles("analyst-7", RoleAnalyst)
	if _, err := store.UpdateResult(result.ID, ResultInput{Value: 2.4, RawValue: "2.4 mg/L", Unit: "mg/L", Dilution: 1, AnalystID: analyst.UserID}, analyst); err != nil {
		t.Fatalf("analyst update before review: %v", err)
	}
	if _, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "self review", EnforceReviewerSeparation: true}, testActorWithRoles("analyst-7", RoleReviewer)); err == nil || !strings.Contains(err.Error(), "reviewer separation") {
		t.Fatalf("expected reviewer separation denial, got %v", err)
	}

	rejected, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionReject, Comments: "rerun required", EnforceReviewerSeparation: true}, testActorWithRoles("reviewer-2", RoleReviewer))
	if err != nil {
		t.Fatalf("reject result: %v", err)
	}
	if rejected.Status != ResultStatusRejected || rejected.ReviewedBy != "reviewer-2" || rejected.ReviewComments != "rerun required" {
		t.Fatalf("reject review metadata not persisted: %#v", rejected)
	}
	if _, err := store.UpdateResult(result.ID, ResultInput{Value: 2.6, Unit: "mg/L", Dilution: 1}, analyst); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected rejected result to be locked until reopen, got %v", err)
	}

	events, err := store.AuditEvents(0)
	if err != nil {
		t.Fatalf("audit events: %v", err)
	}
	assertAuditAction(t, events, "result.review.denied", result.ID)
	assertAuditAction(t, events, "result.rejected", result.ID)
	assertAuditAction(t, events, "result.update.denied", result.ID)
}

func TestResultReviewRequiresReviewerPermission(t *testing.T) {
	store, result := seedResultForReview(t)
	defer store.Close()

	_, err := store.ReviewResult(result.ID, ResultReviewInput{Decision: ResultDecisionAccept, Comments: "not authorized"}, testActorWithRoles("analyst-only", RoleAnalyst))
	if !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected authorization denied, got %v", err)
	}
}

func seedResultForReview(t *testing.T) (*Store, Result) {
	t.Helper()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	actor := testActorWithRoles("analyst-7", RoleAnalyst)
	manager := testActorWithRoles("manager", RoleLabManager)
	client, err := store.CreateClient("Review Client", "review@example.test", manager)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Review", Matrix: "Water", Tests: []string{"Nitrate"}}, manager)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	lines := store.AnalysisRequestLinesForSample(sample.ID)
	if len(lines) != 1 {
		t.Fatalf("expected one analysis request line, got %#v", lines)
	}
	result, err := store.CreateResult(ResultInput{AnalysisRequestLineID: lines[0].ID, Value: 2.1, RawValue: "2.1 mg/L", Unit: "mg/L", Dilution: 1, AnalystID: actor.UserID}, actor)
	if err != nil {
		t.Fatalf("create result: %v", err)
	}
	return store, result
}

func testActorWithRoles(userID string, roles ...Role) ActorContext {
	roleStrings := make([]string, 0, len(roles))
	for _, role := range roles {
		roleStrings = append(roleStrings, string(role))
	}
	return ActorContext{
		UserID:              userID,
		DisplayNameSnapshot: userID,
		AuthProvider:        "test",
		TenantMemberships:   []TenantMembership{{TenantID: DefaultTenantID, Roles: roleStrings}},
		Roles:               roleStrings,
		RequestID:           "req-" + userID,
		CorrelationID:       "corr-" + userID,
	}
}
