package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AnalysisRequestLineStatus string

const (
	AnalysisRequestLineStatusRequested  AnalysisRequestLineStatus = "requested"
	AnalysisRequestLineStatusInProgress AnalysisRequestLineStatus = "in_progress"
	AnalysisRequestLineStatusCompleted  AnalysisRequestLineStatus = "completed"
	AnalysisRequestLineStatusCancelled  AnalysisRequestLineStatus = "cancelled"
)

type AnalysisRequestLine struct {
	ID                     string                    `json:"id"`
	TenantID               string                    `json:"tenant_id"`
	LabID                  string                    `json:"lab_id"`
	SampleID               string                    `json:"sample_id"`
	ServiceID              string                    `json:"service_id,omitempty"`
	ProfileID              string                    `json:"profile_id,omitempty"`
	Name                   string                    `json:"name"`
	Status                 AnalysisRequestLineStatus `json:"status"`
	DepartmentID           string                    `json:"department_id,omitempty"`
	DepartmentName         string                    `json:"department_name,omitempty"`
	MethodID               string                    `json:"method_id,omitempty"`
	MethodName             string                    `json:"method_name,omitempty"`
	CatalogSnapshotID      string                    `json:"catalog_snapshot_id,omitempty"`
	CatalogSnapshotVersion int                       `json:"catalog_snapshot_version,omitempty"`
	CreatedAt              time.Time                 `json:"created_at"`
	UpdatedAt              time.Time                 `json:"updated_at"`
}

func createAnalysisRequestLinesForSampleTx(tx *sql.Tx, scope Scope, sample Sample) error {
	now := time.Now().UTC()
	for _, analysis := range sample.Analyses {
		name := strings.TrimSpace(analysis.Name)
		if name == "" {
			continue
		}
		next, err := nextCounter(tx, "next_analysis_request_line")
		if err != nil {
			return err
		}
		line := AnalysisRequestLine{ID: fmt.Sprintf("ARL-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, ServiceID: strings.TrimSpace(analysis.ServiceID), ProfileID: strings.TrimSpace(analysis.ProfileID), Name: name, Status: AnalysisRequestLineStatusRequested, DepartmentID: strings.TrimSpace(analysis.DepartmentID), DepartmentName: strings.TrimSpace(analysis.DepartmentName), MethodID: strings.TrimSpace(analysis.MethodID), MethodName: strings.TrimSpace(analysis.MethodName), CatalogSnapshotID: strings.TrimSpace(analysis.CatalogSnapshotID), CatalogSnapshotVersion: analysis.CatalogSnapshotVersion, CreatedAt: now, UpdatedAt: now}
		if _, err := tx.Exec(`INSERT INTO analysis_request_lines(id, tenant_id, lab_id, sample_id, service_id, profile_id, name, status, department_id, department_name, method_id, method_name, catalog_snapshot_id, catalog_snapshot_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, line.ID, line.TenantID, line.LabID, line.SampleID, line.ServiceID, line.ProfileID, line.Name, string(line.Status), line.DepartmentID, line.DepartmentName, line.MethodID, line.MethodName, line.CatalogSnapshotID, line.CatalogSnapshotVersion, formatTime(line.CreatedAt), formatTime(line.UpdatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AnalysisRequestLinesForSample(sampleID string) []AnalysisRequestLine {
	return s.AnalysisRequestLinesForSampleForScope(defaultScope(), sampleID)
}

func (s *Store) AnalysisRequestLinesForSampleForScope(scope Scope, sampleID string) []AnalysisRequestLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(analysisRequestLineSelect+` FROM analysis_request_lines WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? ORDER BY id`, scope.TenantID, scope.LabID, strings.TrimSpace(sampleID))
	if err != nil {
		return nil
	}
	defer rows.Close()
	lines, err := scanAnalysisRequestLines(rows)
	if err != nil {
		return nil
	}
	return lines
}

func (s *Store) GetAnalysisRequestLine(id string) (AnalysisRequestLine, bool) {
	return s.GetAnalysisRequestLineForScope(defaultScope(), id)
}

func (s *Store) GetAnalysisRequestLineForScope(scope Scope, id string) (AnalysisRequestLine, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return AnalysisRequestLine{}, false
	}
	line, err := analysisRequestLineByIDTx(s.db, id)
	if err != nil || line.TenantID != scope.TenantID || line.LabID != scope.LabID {
		return AnalysisRequestLine{}, false
	}
	return line, true
}

func (s *Store) TransitionAnalysisRequestLine(id string, next AnalysisRequestLineStatus, actor ActorContext) error {
	return s.TransitionAnalysisRequestLineForScope(defaultScope(), id, next, actor)
}

func (s *Store) TransitionAnalysisRequestLineForScope(scope Scope, id string, next AnalysisRequestLineStatus, actor ActorContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	next = normalizeAnalysisRequestLineStatus(next)
	if next == "" {
		return errors.New("analysis request line status is required")
	}
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultUpdate, actor, AuditResource{Type: "analysis_request_line", ID: id}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		line, err := analysisRequestLineByIDTx(tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown analysis request line %q", id)
			}
			return err
		}
		if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
			deniedErr = fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", id)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "analysis_request_line.transition.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "analysis_request_line", ID: id}, Details: map[string]any{"requested_status": string(next)}})
		}
		if !allowedAnalysisRequestLineTransition(line.Status, next) {
			deniedErr = fmt.Errorf("analysis request line transition %s -> %s is not allowed", line.Status, next)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "analysis_request_line.transition.requested", Outcome: AuditOutcomeDenied, Reason: "transition_not_allowed", Resource: AuditResource{Type: "analysis_request_line", ID: line.ID}, Details: map[string]any{"from": string(line.Status), "to": string(next)}})
		}
		previous := line.Status
		line.Status = next
		line.UpdatedAt = time.Now().UTC()
		if _, err := tx.Exec(`UPDATE analysis_request_lines SET status = ?, updated_at = ? WHERE id = ?`, string(line.Status), formatTime(line.UpdatedAt), line.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "analysis_request_line.transitioned", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "analysis_request_line", ID: line.ID}, Details: map[string]any{"from": string(previous), "to": string(next), "sample_id": line.SampleID}})
	})
	if txErr != nil {
		return txErr
	}
	return deniedErr
}

const analysisRequestLineSelect = `SELECT id, tenant_id, lab_id, sample_id, service_id, profile_id, name, status, department_id, department_name, method_id, method_name, catalog_snapshot_id, catalog_snapshot_version, created_at, updated_at`

type analysisRequestLineQueryer interface{ QueryRow(string, ...any) *sql.Row }

func analysisRequestLineByIDTx(q analysisRequestLineQueryer, id string) (AnalysisRequestLine, error) {
	return scanAnalysisRequestLine(q.QueryRow(analysisRequestLineSelect+` FROM analysis_request_lines WHERE id = ?`, strings.TrimSpace(id)))
}

type analysisRequestLineScanner interface{ Scan(dest ...any) error }

func scanAnalysisRequestLine(row analysisRequestLineScanner) (AnalysisRequestLine, error) {
	var line AnalysisRequestLine
	var status, created, updated string
	if err := row.Scan(&line.ID, &line.TenantID, &line.LabID, &line.SampleID, &line.ServiceID, &line.ProfileID, &line.Name, &status, &line.DepartmentID, &line.DepartmentName, &line.MethodID, &line.MethodName, &line.CatalogSnapshotID, &line.CatalogSnapshotVersion, &created, &updated); err != nil {
		return AnalysisRequestLine{}, err
	}
	line.Status = AnalysisRequestLineStatus(status)
	line.CreatedAt, _ = parseTime(created)
	line.UpdatedAt, _ = parseTime(updated)
	return line, nil
}

func scanAnalysisRequestLines(rows *sql.Rows) ([]AnalysisRequestLine, error) {
	lines := []AnalysisRequestLine{}
	for rows.Next() {
		line, err := scanAnalysisRequestLine(rows)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func normalizeAnalysisRequestLineStatus(status AnalysisRequestLineStatus) AnalysisRequestLineStatus {
	switch AnalysisRequestLineStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case AnalysisRequestLineStatusRequested:
		return AnalysisRequestLineStatusRequested
	case AnalysisRequestLineStatusInProgress:
		return AnalysisRequestLineStatusInProgress
	case AnalysisRequestLineStatusCompleted:
		return AnalysisRequestLineStatusCompleted
	case AnalysisRequestLineStatusCancelled:
		return AnalysisRequestLineStatusCancelled
	default:
		return ""
	}
}

func allowedAnalysisRequestLineTransition(current, next AnalysisRequestLineStatus) bool {
	if current == next {
		return false
	}
	switch current {
	case AnalysisRequestLineStatusRequested:
		return next == AnalysisRequestLineStatusInProgress || next == AnalysisRequestLineStatusCancelled
	case AnalysisRequestLineStatusInProgress:
		return next == AnalysisRequestLineStatusCompleted || next == AnalysisRequestLineStatusCancelled
	default:
		return false
	}
}
