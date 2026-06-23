package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type WorksheetStatus string

const (
	WorksheetStatusOpen       WorksheetStatus = "open"
	WorksheetStatusInProgress WorksheetStatus = "in_progress"
	WorksheetStatusCompleted  WorksheetStatus = "completed"
	WorksheetStatusCancelled  WorksheetStatus = "cancelled"
)

type Worksheet struct {
	ID             string                `json:"id"`
	TenantID       string                `json:"tenant_id"`
	LabID          string                `json:"lab_id"`
	BatchID        string                `json:"batch_id"`
	DepartmentID   string                `json:"department_id,omitempty"`
	DepartmentName string                `json:"department_name,omitempty"`
	MethodID       string                `json:"method_id,omitempty"`
	MethodName     string                `json:"method_name,omitempty"`
	AnalystID      string                `json:"analyst_id,omitempty"`
	Status         WorksheetStatus       `json:"status"`
	Lines          []AnalysisRequestLine `json:"lines"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

type CreateWorksheetInput struct {
	AnalysisRequestLineIDs []string `json:"analysis_request_line_ids"`
	BatchID                string   `json:"batch_id"`
	AnalystID              string   `json:"analyst_id"`
}

func (s *Store) CreateWorksheet(input CreateWorksheetInput, actor ActorContext) (Worksheet, error) {
	return s.CreateWorksheetForScope(defaultScope(), input, actor)
}

func (s *Store) CreateWorksheetForScope(scope Scope, input CreateWorksheetInput, actor ActorContext) (Worksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Worksheet{}, err
	}
	lineIDs := normalizeUniqueIDs(input.AnalysisRequestLineIDs)
	if len(lineIDs) == 0 {
		return Worksheet{}, errors.New("at least one analysis request line is required")
	}
	batchID := strings.TrimSpace(input.BatchID)
	analystID := strings.TrimSpace(input.AnalystID)
	var created Worksheet
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultUpdate, actor, AuditResource{Type: "worksheet", ID: "new"}, map[string]any{"batch_id": batchID})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		lines := make([]AnalysisRequestLine, 0, len(lineIDs))
		for _, id := range lineIDs {
			line, err := analysisRequestLineByIDTx(tx, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("unknown analysis request line %q", id)
				}
				return err
			}
			if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
				return fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", id)
			}
			if line.Status != AnalysisRequestLineStatusRequested {
				return fmt.Errorf("analysis request line %q must be requested before worksheet assignment", id)
			}
			lines = append(lines, line)
		}
		if err := ensureSameWorksheetGroup(lines); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_worksheet")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		created = Worksheet{ID: fmt.Sprintf("WS-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, BatchID: batchID, DepartmentID: lines[0].DepartmentID, DepartmentName: lines[0].DepartmentName, MethodID: lines[0].MethodID, MethodName: lines[0].MethodName, AnalystID: analystID, Status: WorksheetStatusOpen, Lines: lines, CreatedAt: now, UpdatedAt: now}
		if _, err := tx.Exec(`INSERT INTO worksheets(id, tenant_id, lab_id, batch_id, department_id, department_name, method_id, method_name, analyst_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, created.ID, created.TenantID, created.LabID, created.BatchID, created.DepartmentID, created.DepartmentName, created.MethodID, created.MethodName, created.AnalystID, string(created.Status), formatTime(created.CreatedAt), formatTime(created.UpdatedAt)); err != nil {
			return err
		}
		for _, line := range lines {
			if _, err := tx.Exec(`INSERT INTO worksheet_lines(worksheet_id, analysis_request_line_id, created_at) VALUES (?, ?, ?)`, created.ID, line.ID, formatTime(now)); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE analysis_request_lines SET status = ?, updated_at = ? WHERE id = ?`, string(AnalysisRequestLineStatusInProgress), formatTime(now), line.ID); err != nil {
				return err
			}
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "worksheet.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "worksheet", ID: created.ID}, Details: map[string]any{"batch_id": created.BatchID, "analyst_id": created.AnalystID, "line_ids": lineIDs, "department_id": created.DepartmentID, "method_id": created.MethodID}})
	})
	if txErr != nil {
		return Worksheet{}, txErr
	}
	if deniedErr != nil {
		return Worksheet{}, deniedErr
	}
	return created, nil
}

func (s *Store) Worksheets() []Worksheet { return s.WorksheetsForScope(defaultScope()) }

func (s *Store) WorksheetsForScope(scope Scope) []Worksheet {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(worksheetSelect+` FROM worksheets WHERE tenant_id = ? AND lab_id = ? ORDER BY batch_id, department_name, method_name, id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	worksheets, err := scanWorksheets(rows)
	if err != nil {
		return nil
	}
	for i := range worksheets {
		worksheets[i].Lines, _ = worksheetLinesForID(s.db, worksheets[i].ID)
	}
	return worksheets
}

func (s *Store) GetWorksheet(id string) (Worksheet, bool) {
	return s.GetWorksheetForScope(defaultScope(), id)
}

func (s *Store) GetWorksheetForScope(scope Scope, id string) (Worksheet, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Worksheet{}, false
	}
	worksheet, err := worksheetByIDTx(s.db, id)
	if err != nil || worksheet.TenantID != scope.TenantID || worksheet.LabID != scope.LabID {
		return Worksheet{}, false
	}
	worksheet.Lines, _ = worksheetLinesForID(s.db, worksheet.ID)
	return worksheet, true
}

func (s *Store) AssignWorksheetAnalyst(id, analystID string, actor ActorContext) error {
	return s.AssignWorksheetAnalystForScope(defaultScope(), id, analystID, actor)
}

func (s *Store) AssignWorksheetAnalystForScope(scope Scope, id, analystID string, actor ActorContext) error {
	return s.updateWorksheetForScope(scope, id, actor, func(tx *sql.Tx, worksheet Worksheet) (string, map[string]any, error) {
		analystID = strings.TrimSpace(analystID)
		if analystID == "" {
			return "", nil, errors.New("analyst id is required")
		}
		if worksheet.Status == WorksheetStatusCompleted || worksheet.Status == WorksheetStatusCancelled {
			return "", nil, fmt.Errorf("cannot assign analyst on %s worksheet", worksheet.Status)
		}
		if _, err := tx.Exec(`UPDATE worksheets SET analyst_id = ?, updated_at = ? WHERE id = ?`, analystID, formatTime(time.Now().UTC()), worksheet.ID); err != nil {
			return "", nil, err
		}
		return "worksheet.analyst_assigned", map[string]any{"from": worksheet.AnalystID, "to": analystID}, nil
	})
}

func (s *Store) RemoveWorksheetLine(id, lineID string, actor ActorContext) error {
	return s.RemoveWorksheetLineForScope(defaultScope(), id, lineID, actor)
}

func (s *Store) RemoveWorksheetLineForScope(scope Scope, id, lineID string, actor ActorContext) error {
	lineID = strings.TrimSpace(lineID)
	return s.updateWorksheetForScope(scope, id, actor, func(tx *sql.Tx, worksheet Worksheet) (string, map[string]any, error) {
		if worksheet.Status == WorksheetStatusCompleted {
			return "", nil, errors.New("cannot remove a line from a completed worksheet")
		}
		if worksheet.Status == WorksheetStatusCancelled {
			return "", nil, errors.New("cannot remove a line from a cancelled worksheet")
		}
		res, err := tx.Exec(`DELETE FROM worksheet_lines WHERE worksheet_id = ? AND analysis_request_line_id = ?`, worksheet.ID, lineID)
		if err != nil {
			return "", nil, err
		}
		removed, _ := res.RowsAffected()
		if removed == 0 {
			return "", nil, fmt.Errorf("analysis request line %q is not on worksheet %q", lineID, worksheet.ID)
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(`UPDATE analysis_request_lines SET status = ?, updated_at = ? WHERE id = ?`, string(AnalysisRequestLineStatusRequested), formatTime(now), lineID); err != nil {
			return "", nil, err
		}
		return "worksheet.line_removed", map[string]any{"line_id": lineID}, nil
	})
}

func (s *Store) TransitionWorksheet(id string, next WorksheetStatus, actor ActorContext) error {
	return s.TransitionWorksheetForScope(defaultScope(), id, next, actor)
}

func (s *Store) TransitionWorksheetForScope(scope Scope, id string, next WorksheetStatus, actor ActorContext) error {
	next = normalizeWorksheetStatus(next)
	if next == "" {
		return errors.New("worksheet status is required")
	}
	return s.updateWorksheetForScope(scope, id, actor, func(tx *sql.Tx, worksheet Worksheet) (string, map[string]any, error) {
		if !allowedWorksheetTransition(worksheet.Status, next) {
			return "", nil, fmt.Errorf("worksheet transition %s -> %s is not allowed", worksheet.Status, next)
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(`UPDATE worksheets SET status = ?, updated_at = ? WHERE id = ?`, string(next), formatTime(now), worksheet.ID); err != nil {
			return "", nil, err
		}
		if next == WorksheetStatusCompleted || next == WorksheetStatusCancelled {
			lineStatus := AnalysisRequestLineStatusCompleted
			if next == WorksheetStatusCancelled {
				lineStatus = AnalysisRequestLineStatusCancelled
			}
			if _, err := tx.Exec(`UPDATE analysis_request_lines SET status = ?, updated_at = ? WHERE id IN (SELECT analysis_request_line_id FROM worksheet_lines WHERE worksheet_id = ?)`, string(lineStatus), formatTime(now), worksheet.ID); err != nil {
				return "", nil, err
			}
		}
		return "worksheet.transitioned", map[string]any{"from": string(worksheet.Status), "to": string(next)}, nil
	})
}

func (s *Store) updateWorksheetForScope(scope Scope, id string, actor ActorContext, mutate func(*sql.Tx, Worksheet) (string, map[string]any, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultUpdate, actor, AuditResource{Type: "worksheet", ID: id}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		worksheet, err := worksheetByIDTx(tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown worksheet %q", id)
			}
			return err
		}
		if worksheet.TenantID != scope.TenantID || worksheet.LabID != scope.LabID {
			return fmt.Errorf("worksheet %q is outside requested tenant/lab scope", id)
		}
		action, details, err := mutate(tx, worksheet)
		if err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: action, Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "worksheet", ID: worksheet.ID}, Details: details})
	})
	if txErr != nil {
		return txErr
	}
	return deniedErr
}

const worksheetSelect = `SELECT id, tenant_id, lab_id, batch_id, department_id, department_name, method_id, method_name, analyst_id, status, created_at, updated_at`

type worksheetQueryer interface{ QueryRow(string, ...any) *sql.Row }
type worksheetLineQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func worksheetByIDTx(q worksheetQueryer, id string) (Worksheet, error) {
	return scanWorksheet(q.QueryRow(worksheetSelect+` FROM worksheets WHERE id = ?`, strings.TrimSpace(id)))
}

type worksheetScanner interface{ Scan(dest ...any) error }

func scanWorksheet(row worksheetScanner) (Worksheet, error) {
	var worksheet Worksheet
	var status, created, updated string
	if err := row.Scan(&worksheet.ID, &worksheet.TenantID, &worksheet.LabID, &worksheet.BatchID, &worksheet.DepartmentID, &worksheet.DepartmentName, &worksheet.MethodID, &worksheet.MethodName, &worksheet.AnalystID, &status, &created, &updated); err != nil {
		return Worksheet{}, err
	}
	worksheet.Status = WorksheetStatus(status)
	worksheet.CreatedAt, _ = parseTime(created)
	worksheet.UpdatedAt, _ = parseTime(updated)
	return worksheet, nil
}

func scanWorksheets(rows *sql.Rows) ([]Worksheet, error) {
	worksheets := []Worksheet{}
	for rows.Next() {
		worksheet, err := scanWorksheet(rows)
		if err != nil {
			return nil, err
		}
		worksheets = append(worksheets, worksheet)
	}
	return worksheets, rows.Err()
}

func worksheetLinesForID(q worksheetLineQueryer, worksheetID string) ([]AnalysisRequestLine, error) {
	rows, err := q.Query(`SELECT arl.id, arl.tenant_id, arl.lab_id, arl.sample_id, arl.service_id, arl.profile_id, arl.name, arl.status, arl.department_id, arl.department_name, arl.method_id, arl.method_name, arl.catalog_snapshot_id, arl.catalog_snapshot_version, arl.created_at, arl.updated_at FROM analysis_request_lines arl JOIN worksheet_lines wl ON wl.analysis_request_line_id = arl.id WHERE wl.worksheet_id = ? ORDER BY arl.id`, strings.TrimSpace(worksheetID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnalysisRequestLines(rows)
}

func normalizeUniqueIDs(raw []string) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, item := range raw {
		for _, part := range strings.FieldsFunc(item, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' }) {
			id := strings.TrimSpace(part)
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func ensureSameWorksheetGroup(lines []AnalysisRequestLine) error {
	if len(lines) == 0 {
		return errors.New("at least one analysis request line is required")
	}
	first := lines[0]
	for _, line := range lines[1:] {
		if line.DepartmentID != first.DepartmentID || line.MethodID != first.MethodID {
			return errors.New("worksheet lines must share the same method and department")
		}
	}
	return nil
}

func normalizeWorksheetStatus(status WorksheetStatus) WorksheetStatus {
	switch WorksheetStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case WorksheetStatusOpen:
		return WorksheetStatusOpen
	case WorksheetStatusInProgress:
		return WorksheetStatusInProgress
	case WorksheetStatusCompleted:
		return WorksheetStatusCompleted
	case WorksheetStatusCancelled:
		return WorksheetStatusCancelled
	default:
		return ""
	}
}

func allowedWorksheetTransition(current, next WorksheetStatus) bool {
	if current == next {
		return false
	}
	switch current {
	case WorksheetStatusOpen:
		return next == WorksheetStatusInProgress || next == WorksheetStatusCancelled
	case WorksheetStatusInProgress:
		return next == WorksheetStatusCompleted || next == WorksheetStatusCancelled
	default:
		return false
	}
}
