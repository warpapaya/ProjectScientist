package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Role string

const (
	RoleAdmin            Role = "admin"
	RoleLabManager       Role = "lab-manager"
	RoleAnalyst          Role = "analyst"
	RoleReviewer         Role = "reviewer"
	RoleReportReleaser   Role = "report-releaser"
	RoleClientContact    Role = "client-contact"
	RoleMigrationService Role = "migration-service"
)

type Operation string

const (
	OperationClientCreate   Operation = "client.create"
	OperationClientUpdate   Operation = "client.update"
	OperationClientArchive  Operation = "client.archive"
	OperationContactCreate  Operation = "contact.create"
	OperationContactUpdate  Operation = "contact.update"
	OperationContactArchive Operation = "contact.archive"
	OperationProjectCreate  Operation = "project.create"
	OperationProjectUpdate  Operation = "project.update"
	OperationProjectArchive Operation = "project.archive"

	OperationSampleIntake     Operation = "sample.intake"
	OperationSampleUpdate     Operation = "sample.update"
	OperationSampleTransition Operation = "sample.transition"

	OperationResultEntry   Operation = "result.entry"
	OperationResultUpdate  Operation = "result.update"
	OperationResultReview  Operation = "result.review"
	OperationResultRelease Operation = "result.release"

	OperationReportGenerate Operation = "report.generate"
	OperationReportRelease  Operation = "report.release"
	OperationReportExport   Operation = "report.export"
	OperationReportAmend    Operation = "report.amend"

	OperationAuditView        Operation = "audit.view"
	OperationAuditExport      Operation = "audit.export"
	OperationImportRun        Operation = "import.run"
	OperationExportRun        Operation = "export.run"
	OperationCatalogConfigure Operation = "catalog.configure"
	OperationAdminConfigure   Operation = "admin.configure"
)

var ErrAuthorizationDenied = errors.New("authorization denied")

var operationAllowedRoles = map[Operation][]Role{
	OperationClientCreate:   {RoleAdmin, RoleLabManager},
	OperationClientUpdate:   {RoleAdmin, RoleLabManager},
	OperationClientArchive:  {RoleAdmin},
	OperationContactCreate:  {RoleAdmin, RoleLabManager},
	OperationContactUpdate:  {RoleAdmin, RoleLabManager},
	OperationContactArchive: {RoleAdmin},
	OperationProjectCreate:  {RoleAdmin, RoleLabManager},
	OperationProjectUpdate:  {RoleAdmin, RoleLabManager},
	OperationProjectArchive: {RoleAdmin},

	OperationSampleIntake:     {RoleAdmin, RoleLabManager},
	OperationSampleUpdate:     {RoleAdmin, RoleLabManager, RoleAnalyst},
	OperationSampleTransition: {RoleAdmin, RoleLabManager, RoleAnalyst},

	OperationResultEntry:   {RoleAdmin, RoleLabManager, RoleAnalyst},
	OperationResultUpdate:  {RoleAdmin, RoleLabManager, RoleAnalyst},
	OperationResultReview:  {RoleAdmin, RoleLabManager, RoleReviewer},
	OperationResultRelease: {RoleAdmin, RoleReportReleaser},

	OperationReportGenerate: {RoleAdmin, RoleLabManager, RoleReviewer, RoleReportReleaser},
	OperationReportRelease:  {RoleAdmin, RoleReportReleaser},
	OperationReportExport:   {RoleAdmin, RoleLabManager, RoleReportReleaser},
	OperationReportAmend:    {RoleAdmin, RoleReportReleaser},

	OperationAuditView:        {RoleAdmin, RoleLabManager},
	OperationAuditExport:      {RoleAdmin},
	OperationImportRun:        {RoleAdmin, RoleMigrationService},
	OperationExportRun:        {RoleAdmin, RoleLabManager},
	OperationCatalogConfigure: {RoleAdmin, RoleLabManager},
	OperationAdminConfigure:   {RoleAdmin},
}

func Authorize(scope Scope, operation Operation, actor ActorContext) error {
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	operation = Operation(strings.TrimSpace(string(operation)))
	allowed, ok := operationAllowedRoles[operation]
	if !ok {
		return fmt.Errorf("%w: unknown protected operation %q", ErrAuthorizationDenied, operation)
	}
	if !actorHasTenantMembership(actor, scope.TenantID) {
		return fmt.Errorf("%w: actor is not a member of tenant %q", ErrAuthorizationDenied, scope.TenantID)
	}
	actorRoles := actorRoleSetForTenant(actor, scope.TenantID)
	for _, role := range allowed {
		if actorRoles[role] {
			return nil
		}
	}
	return fmt.Errorf("%w: %s requires one of %s", ErrAuthorizationDenied, operation, roleList(allowed))
}

func (s *Store) AuthorizeOperationForScope(scope Scope, operation Operation, actor ActorContext, resource AuditResource, details map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	var authErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, err := authorizeOperationTx(tx, scope, operation, actor, resource, details)
		if err != nil {
			return err
		}
		if !allowed {
			authErr = ErrAuthorizationDenied
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	return authErr
}

func authorizeOperationTx(tx *sql.Tx, scope Scope, operation Operation, actor ActorContext, resource AuditResource, details map[string]any) (bool, error) {
	if err := Authorize(scope, operation, actor); err != nil {
		if errors.Is(err, ErrAuthorizationDenied) {
			if writeErr := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: string(operation), Outcome: AuditOutcomeDenied, Reason: "authorization_denied", Resource: normalizeDeniedResource(resource, operation), Details: safeDeniedDetails(operation, actor)}); writeErr != nil {
				return false, writeErr
			}
		}
		return false, nil
	}
	return true, nil
}

func normalizeDeniedResource(resource AuditResource, operation Operation) AuditResource {
	resource.Type = strings.TrimSpace(resource.Type)
	resource.ID = strings.TrimSpace(resource.ID)
	if resource.Type == "" {
		resource.Type = "operation"
	}
	if resource.ID == "" {
		resource.ID = string(operation)
	}
	return resource
}

func safeDeniedDetails(operation Operation, actor ActorContext) map[string]any {
	return map[string]any{
		"operation":   string(operation),
		"actor_roles": normalizeStrings(actor.Roles),
	}
}

func actorHasTenantMembership(actor ActorContext, tenantID string) bool {
	for _, membership := range actor.TenantMemberships {
		if membership.TenantID == tenantID {
			return true
		}
	}
	return false
}

func actorRoleSetForTenant(actor ActorContext, tenantID string) map[Role]bool {
	roles := map[Role]bool{}
	for _, raw := range actor.Roles {
		if role := Role(strings.TrimSpace(raw)); role != "" {
			roles[role] = true
		}
	}
	for _, membership := range actor.TenantMemberships {
		if membership.TenantID != tenantID {
			continue
		}
		for _, raw := range membership.Roles {
			if role := Role(strings.TrimSpace(raw)); role != "" {
				roles[role] = true
			}
		}
	}
	return roles
}

func roleList(roles []Role) string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return strings.Join(out, ",")
}
