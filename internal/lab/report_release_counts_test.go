package lab

import (
	"path/filepath"
	"testing"
)

func TestReportReleaseReadinessCountsRequestedAnalysesBeforeResults(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := MustActorContext(ActorContextInput{UserID: "readiness-test", RequestID: "readiness-test", CorrelationID: "readiness-test", TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID, Roles: []string{string(RoleAdmin), string(RoleLabManager), string(RoleAnalyst), string(RoleReviewer)}}}, Roles: []string{string(RoleAdmin), string(RoleLabManager), string(RoleAnalyst), string(RoleReviewer)}})
	if _, err := store.ResetAndSeedSyntheticDemo(filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json"), actor); err != nil {
		t.Fatalf("seed demo: %v", err)
	}

	readiness, ok := store.ReportReleaseReadinessForScope(DefaultScope, "S-000001")
	if !ok {
		t.Fatalf("expected readiness row for S-000001")
	}
	if readiness.ResultCount != 4 || readiness.AcceptedResultCount != 0 {
		t.Fatalf("readiness should count requested analyses before result rows exist: %#v", readiness)
	}
}
