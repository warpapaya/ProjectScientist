package lab

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAuditEventsForScopeAsActorRequiresAuditViewPermission(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, _ = store.CreateClient("Alpha Environmental", "alpha@example.test", actorWithRoles("seed-manager", RoleLabManager))

	if _, err := store.AuditEventsForScopeAsActor(DefaultScope, 0, actorWithRoles("analyst", RoleAnalyst)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected analyst audit view denial, got %v", err)
	}

	events, err := store.AuditEventsForScopeAsActor(DefaultScope, 0, actorWithRoles("manager", RoleLabManager))
	if err != nil {
		t.Fatalf("expected lab manager audit view allowed: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected authorized audit view to return seeded audit events")
	}
	if !auditDeniedEventExists(events, string(OperationAuditView), "audit", "events") {
		t.Fatalf("missing denied audit view event: %#v", events)
	}
}

func TestAuditExportEventsForScopeAsActorRequiresAuditExportPermission(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, _ = store.CreateClient("Alpha Environmental", "alpha@example.test", actorWithRoles("seed-manager", RoleLabManager))

	if _, err := store.AuditExportEventsForScopeAsActor(DefaultScope, 0, actorWithRoles("manager", RoleLabManager)); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("expected lab manager audit export denial, got %v", err)
	}

	events, err := store.AuditExportEventsForScopeAsActor(DefaultScope, 0, actorWithRoles("admin", RoleAdmin))
	if err != nil {
		t.Fatalf("expected admin audit export allowed: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected authorized audit export to return seeded audit events")
	}
	if !auditDeniedEventExists(events, string(OperationAuditExport), "audit_export", "events") {
		t.Fatalf("missing denied audit export event: %#v", events)
	}
}
