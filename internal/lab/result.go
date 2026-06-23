package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ResultStatus string

const (
	ResultStatusEntered  ResultStatus = "entered"
	ResultStatusAccepted ResultStatus = "accepted"
	ResultStatusRejected ResultStatus = "rejected"
)

type ResultDecision string

const (
	ResultDecisionAccept ResultDecision = "accept"
	ResultDecisionReject ResultDecision = "reject"
)

type Result struct {
	ID                    string       `json:"id"`
	TenantID              string       `json:"tenant_id"`
	LabID                 string       `json:"lab_id"`
	SampleID              string       `json:"sample_id"`
	AnalysisRequestLineID string       `json:"analysis_request_line_id"`
	Value                 float64      `json:"value"`
	RawValue              string       `json:"raw_value"`
	Unit                  string       `json:"unit"`
	Qualifier             string       `json:"qualifier,omitempty"`
	MDL                   float64      `json:"mdl,omitempty"`
	RL                    float64      `json:"rl,omitempty"`
	LOQ                   float64      `json:"loq,omitempty"`
	Dilution              float64      `json:"dilution"`
	Uncertainty           float64      `json:"uncertainty,omitempty"`
	Comments              string       `json:"comments,omitempty"`
	AnalystID             string       `json:"analyst_id,omitempty"`
	InstrumentID          string       `json:"instrument_id,omitempty"`
	Status                ResultStatus `json:"status"`
	ReviewedBy            string       `json:"reviewed_by,omitempty"`
	ReviewComments        string       `json:"review_comments,omitempty"`
	ReviewedAt            time.Time    `json:"reviewed_at,omitempty"`
	ReopenReason          string       `json:"reopen_reason,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
	UpdatedAt             time.Time    `json:"updated_at"`
}

type ResultInput struct {
	AnalysisRequestLineID string  `json:"analysis_request_line_id"`
	Value                 float64 `json:"value"`
	RawValue              string  `json:"raw_value"`
	Unit                  string  `json:"unit"`
	Qualifier             string  `json:"qualifier"`
	MDL                   float64 `json:"mdl"`
	RL                    float64 `json:"rl"`
	LOQ                   float64 `json:"loq"`
	Dilution              float64 `json:"dilution"`
	Uncertainty           float64 `json:"uncertainty"`
	Comments              string  `json:"comments"`
	AnalystID             string  `json:"analyst_id"`
	InstrumentID          string  `json:"instrument_id"`
}

type ResultReviewInput struct {
	Decision                  ResultDecision `json:"decision"`
	Comments                  string         `json:"comments"`
	EnforceReviewerSeparation bool           `json:"enforce_reviewer_separation"`
}

func (s *Store) CreateResult(input ResultInput, actor ActorContext) (Result, error) {
	return s.CreateResultForScope(defaultScope(), input, actor)
}

func (s *Store) CreateResultForScope(scope Scope, input ResultInput, actor ActorContext) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, err
	}
	input = normalizeResultInput(input)
	if err := validateResultInput(input, true); err != nil {
		return Result{}, err
	}
	var result Result
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		line, err := analysisRequestLineByIDTx(tx, input.AnalysisRequestLineID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown analysis request line %q", input.AnalysisRequestLineID)
			}
			return err
		}
		if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
			deniedErr = fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", line.ID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.create.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "analysis_request_line", ID: line.ID}})
		}
		if line.Status == AnalysisRequestLineStatusCancelled {
			deniedErr = fmt.Errorf("cannot enter result for cancelled analysis request line %q", input.AnalysisRequestLineID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.create.requested", Outcome: AuditOutcomeDenied, Reason: "line_cancelled", Resource: AuditResource{Type: "analysis_request_line", ID: line.ID}, Details: map[string]any{"sample_id": line.SampleID}})
		}
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultEntry, actor, AuditResource{Type: "analysis_request_line", ID: line.ID}, map[string]any{"sample_id": line.SampleID})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		next, err := nextCounter(tx, "next_result")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		result = resultFromInput(scope, fmt.Sprintf("R-%06d", next), line.SampleID, input, now)
		if _, err := tx.Exec(`INSERT INTO results(id, tenant_id, lab_id, sample_id, analysis_request_line_id, value, raw_value, unit, qualifier, mdl, rl, loq, dilution, uncertainty, comments, analyst_id, instrument_id, status, reviewed_by, review_comments, reviewed_at, reopen_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, result.ID, result.TenantID, result.LabID, result.SampleID, result.AnalysisRequestLineID, result.Value, result.RawValue, result.Unit, result.Qualifier, result.MDL, result.RL, result.LOQ, result.Dilution, result.Uncertainty, result.Comments, result.AnalystID, result.InstrumentID, string(result.Status), result.ReviewedBy, result.ReviewComments, formatOptionalTime(result.ReviewedAt), result.ReopenReason, formatTime(result.CreatedAt), formatTime(result.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: result.ID}, Details: resultAuditDetails(result)})
	})
	if err != nil {
		return Result{}, err
	}
	if deniedErr != nil {
		return Result{}, deniedErr
	}
	return result, nil
}

func (s *Store) UpdateResult(id string, input ResultInput, actor ActorContext) (Result, error) {
	return s.UpdateResultForScope(defaultScope(), id, input, actor)
}

func (s *Store) UpdateResultForScope(scope Scope, id string, input ResultInput, actor ActorContext) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, err
	}
	id = strings.TrimSpace(id)
	input = normalizeResultInput(input)
	if err := validateResultInput(input, false); err != nil {
		return Result{}, err
	}
	var updated Result
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		current, err := resultByIDTx(tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown result %q", id)
			}
			return err
		}
		if current.TenantID != scope.TenantID || current.LabID != scope.LabID {
			deniedErr = fmt.Errorf("result %q is outside requested tenant/lab scope", id)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.update.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "result", ID: id}})
		}
		line, err := analysisRequestLineByIDTx(tx, current.AnalysisRequestLineID)
		if err != nil {
			return err
		}
		if line.Status == AnalysisRequestLineStatusCancelled {
			deniedErr = fmt.Errorf("cannot update result for cancelled analysis request line %q", current.AnalysisRequestLineID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.update.requested", Outcome: AuditOutcomeDenied, Reason: "line_cancelled", Resource: AuditResource{Type: "result", ID: current.ID}, Details: map[string]any{"analysis_request_line_id": current.AnalysisRequestLineID, "sample_id": current.SampleID}})
		}
		if current.Status != ResultStatusEntered {
			deniedErr = fmt.Errorf("result %q is locked after review; reopen before amending", id)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.update.denied", Outcome: AuditOutcomeDenied, Reason: "result_locked", Resource: AuditResource{Type: "result", ID: current.ID}, Details: map[string]any{"status": string(current.Status)}})
		}
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultUpdate, actor, AuditResource{Type: "result", ID: current.ID}, map[string]any{"sample_id": current.SampleID, "status": string(current.Status)})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		input.AnalysisRequestLineID = current.AnalysisRequestLineID
		updated = resultFromInput(scope, current.ID, current.SampleID, input, current.CreatedAt)
		updated.Status = ResultStatusEntered
		updated.UpdatedAt = time.Now().UTC()
		updated.ReopenReason = current.ReopenReason
		if _, err := tx.Exec(`UPDATE results SET value = ?, raw_value = ?, unit = ?, qualifier = ?, mdl = ?, rl = ?, loq = ?, dilution = ?, uncertainty = ?, comments = ?, analyst_id = ?, instrument_id = ?, status = ?, reviewed_by = '', review_comments = '', reviewed_at = '', updated_at = ? WHERE id = ?`, updated.Value, updated.RawValue, updated.Unit, updated.Qualifier, updated.MDL, updated.RL, updated.LOQ, updated.Dilution, updated.Uncertainty, updated.Comments, updated.AnalystID, updated.InstrumentID, string(updated.Status), formatTime(updated.UpdatedAt), updated.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.updated", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: updated.ID}, Details: map[string]any{"sample_id": updated.SampleID, "analysis_request_line_id": updated.AnalysisRequestLineID, "changed_fields": changedResultFields(current, updated)}})
	})
	if err != nil {
		return Result{}, err
	}
	if deniedErr != nil {
		return Result{}, deniedErr
	}
	return updated, nil
}

func (s *Store) ReviewResult(id string, input ResultReviewInput, actor ActorContext) (Result, error) {
	return s.ReviewResultForScope(defaultScope(), id, input, actor)
}

func (s *Store) ReviewResultForScope(scope Scope, id string, input ResultReviewInput, actor ActorContext) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, err
	}
	id = strings.TrimSpace(id)
	input.Decision = ResultDecision(strings.TrimSpace(string(input.Decision)))
	input.Comments = strings.TrimSpace(input.Comments)
	if input.Decision != ResultDecisionAccept && input.Decision != ResultDecisionReject {
		return Result{}, errors.New("review decision must be accept or reject")
	}
	var reviewed Result
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		current, err := resultByIDTx(tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown result %q", id)
			}
			return err
		}
		if current.TenantID != scope.TenantID || current.LabID != scope.LabID {
			deniedErr = fmt.Errorf("result %q is outside requested tenant/lab scope", id)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.review.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "result", ID: id}})
		}
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultReview, actor, AuditResource{Type: "result", ID: current.ID}, map[string]any{"sample_id": current.SampleID})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		reviewer := normalizeActorContext(actor, "result-review").UserID
		if input.EnforceReviewerSeparation && reviewer != "" && reviewer == current.AnalystID {
			deniedErr = fmt.Errorf("reviewer separation required: reviewer %q entered result %q", reviewer, current.ID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.review.denied", Outcome: AuditOutcomeDenied, Reason: "reviewer_separation", Resource: AuditResource{Type: "result", ID: current.ID}, Details: map[string]any{"analyst_id": current.AnalystID, "reviewer_id": reviewer}})
		}
		reviewed = current
		if input.Decision == ResultDecisionAccept {
			reviewed.Status = ResultStatusAccepted
		} else {
			reviewed.Status = ResultStatusRejected
		}
		reviewed.ReviewedBy = reviewer
		reviewed.ReviewComments = input.Comments
		reviewed.ReviewedAt = time.Now().UTC()
		reviewed.UpdatedAt = reviewed.ReviewedAt
		if _, err := tx.Exec(`UPDATE results SET status = ?, reviewed_by = ?, review_comments = ?, reviewed_at = ?, updated_at = ? WHERE id = ?`, string(reviewed.Status), reviewed.ReviewedBy, reviewed.ReviewComments, formatTime(reviewed.ReviewedAt), formatTime(reviewed.UpdatedAt), reviewed.ID); err != nil {
			return err
		}
		action := "result.accepted"
		if reviewed.Status == ResultStatusRejected {
			action = "result.rejected"
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: action, Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: reviewed.ID}, Details: map[string]any{"sample_id": reviewed.SampleID, "analysis_request_line_id": reviewed.AnalysisRequestLineID, "reviewed_by": reviewed.ReviewedBy, "decision": string(input.Decision)}})
	})
	if err != nil {
		return Result{}, err
	}
	if deniedErr != nil {
		return Result{}, deniedErr
	}
	return reviewed, nil
}

func (s *Store) ReopenResult(id, reason string, actor ActorContext) (Result, error) {
	return s.ReopenResultForScope(defaultScope(), id, reason, actor)
}

func (s *Store) ReopenResultForScope(scope Scope, id, reason string, actor ActorContext) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, err
	}
	id = strings.TrimSpace(id)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Result{}, errors.New("reopen reason is required")
	}
	var reopened Result
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		current, err := resultByIDTx(tx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown result %q", id)
			}
			return err
		}
		if current.TenantID != scope.TenantID || current.LabID != scope.LabID {
			deniedErr = fmt.Errorf("result %q is outside requested tenant/lab scope", id)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.reopen.requested", Outcome: AuditOutcomeDenied, Reason: "scope_mismatch", Resource: AuditResource{Type: "result", ID: id}})
		}
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultUpdate, actor, AuditResource{Type: "result", ID: current.ID}, map[string]any{"sample_id": current.SampleID, "status": string(current.Status), "amend": true})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		reopened = current
		reopened.Status = ResultStatusEntered
		reopened.ReviewedBy = ""
		reopened.ReviewComments = ""
		reopened.ReviewedAt = time.Time{}
		reopened.ReopenReason = reason
		reopened.UpdatedAt = time.Now().UTC()
		if _, err := tx.Exec(`UPDATE results SET status = ?, reviewed_by = '', review_comments = '', reviewed_at = '', reopen_reason = ?, updated_at = ? WHERE id = ?`, string(reopened.Status), reopened.ReopenReason, formatTime(reopened.UpdatedAt), reopened.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.reopened", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: reopened.ID}, Details: map[string]any{"sample_id": reopened.SampleID, "analysis_request_line_id": reopened.AnalysisRequestLineID, "reason": reason}})
	})
	if err != nil {
		return Result{}, err
	}
	if deniedErr != nil {
		return Result{}, deniedErr
	}
	return reopened, nil
}

func (s *Store) ResultsForScope(scope Scope) []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	rows, err := s.db.Query(resultSelectSQL+` FROM results WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	results, err := scanResults(rows)
	if err != nil {
		return nil
	}
	return results
}

func (s *Store) GetResultForAnalysisRequestLine(lineID string) (Result, bool) {
	return s.GetResultForAnalysisRequestLineForScope(defaultScope(), lineID)
}

func (s *Store) GetResultForAnalysisRequestLineForScope(scope Scope, lineID string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, false
	}
	result, err := resultByAnalysisRequestLineTx(s.db, lineID)
	if err != nil || result.TenantID != scope.TenantID || result.LabID != scope.LabID {
		return Result{}, false
	}
	return result, true
}

func normalizeResultInput(input ResultInput) ResultInput {
	input.AnalysisRequestLineID = strings.TrimSpace(input.AnalysisRequestLineID)
	input.RawValue = strings.TrimSpace(input.RawValue)
	input.Unit = strings.TrimSpace(input.Unit)
	input.Qualifier = strings.TrimSpace(input.Qualifier)
	input.Comments = strings.TrimSpace(input.Comments)
	input.AnalystID = strings.TrimSpace(input.AnalystID)
	input.InstrumentID = strings.TrimSpace(input.InstrumentID)
	return input
}

func validateResultInput(input ResultInput, requireLine bool) error {
	if requireLine && input.AnalysisRequestLineID == "" {
		return errors.New("analysis request line id is required")
	}
	if input.Unit == "" {
		return errors.New("unit is required")
	}
	if input.MDL < 0 {
		return errors.New("mdl cannot be negative")
	}
	if input.RL < 0 {
		return errors.New("rl cannot be negative")
	}
	if input.LOQ < 0 {
		return errors.New("loq cannot be negative")
	}
	if input.Dilution <= 0 {
		return errors.New("dilution must be greater than zero")
	}
	if input.Uncertainty < 0 {
		return errors.New("uncertainty cannot be negative")
	}
	return nil
}

func resultFromInput(scope Scope, id, sampleID string, input ResultInput, createdAt time.Time) Result {
	now := time.Now().UTC()
	return Result{ID: id, TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sampleID, AnalysisRequestLineID: input.AnalysisRequestLineID, Value: input.Value, RawValue: input.RawValue, Unit: input.Unit, Qualifier: input.Qualifier, MDL: input.MDL, RL: input.RL, LOQ: input.LOQ, Dilution: input.Dilution, Uncertainty: input.Uncertainty, Comments: input.Comments, AnalystID: input.AnalystID, InstrumentID: input.InstrumentID, Status: ResultStatusEntered, CreatedAt: createdAt, UpdatedAt: now}
}

func resultAuditDetails(result Result) map[string]any {
	return map[string]any{"sample_id": result.SampleID, "analysis_request_line_id": result.AnalysisRequestLineID, "value": result.Value, "unit": result.Unit, "qualifier": result.Qualifier, "analyst_id": result.AnalystID, "instrument_id": result.InstrumentID}
}

func changedResultFields(before, after Result) []string {
	fields := []string{}
	if before.Value != after.Value {
		fields = append(fields, "value")
	}
	if before.RawValue != after.RawValue {
		fields = append(fields, "raw_value")
	}
	if before.Unit != after.Unit {
		fields = append(fields, "unit")
	}
	if before.Qualifier != after.Qualifier {
		fields = append(fields, "qualifier")
	}
	if before.MDL != after.MDL {
		fields = append(fields, "mdl")
	}
	if before.RL != after.RL {
		fields = append(fields, "rl")
	}
	if before.LOQ != after.LOQ {
		fields = append(fields, "loq")
	}
	if before.Dilution != after.Dilution {
		fields = append(fields, "dilution")
	}
	if before.Uncertainty != after.Uncertainty {
		fields = append(fields, "uncertainty")
	}
	if before.Comments != after.Comments {
		fields = append(fields, "comments")
	}
	if before.AnalystID != after.AnalystID {
		fields = append(fields, "analyst_id")
	}
	if before.InstrumentID != after.InstrumentID {
		fields = append(fields, "instrument_id")
	}
	return fields
}

const resultSelectSQL = `SELECT id, tenant_id, lab_id, sample_id, analysis_request_line_id, value, raw_value, unit, qualifier, mdl, rl, loq, dilution, uncertainty, comments, analyst_id, instrument_id, status, reviewed_by, review_comments, reviewed_at, reopen_reason, created_at, updated_at`

type resultQueryer interface{ QueryRow(string, ...any) *sql.Row }

func resultByIDTx(q resultQueryer, id string) (Result, error) {
	return scanResult(q.QueryRow(resultSelectSQL+` FROM results WHERE id = ?`, strings.TrimSpace(id)))
}

func resultByAnalysisRequestLineTx(q resultQueryer, lineID string) (Result, error) {
	return scanResult(q.QueryRow(resultSelectSQL+` FROM results WHERE analysis_request_line_id = ?`, strings.TrimSpace(lineID)))
}

type resultScanner interface{ Scan(dest ...any) error }

func scanResult(row resultScanner) (Result, error) {
	var result Result
	var status, reviewedAt, createdAt, updatedAt string
	if err := row.Scan(&result.ID, &result.TenantID, &result.LabID, &result.SampleID, &result.AnalysisRequestLineID, &result.Value, &result.RawValue, &result.Unit, &result.Qualifier, &result.MDL, &result.RL, &result.LOQ, &result.Dilution, &result.Uncertainty, &result.Comments, &result.AnalystID, &result.InstrumentID, &status, &result.ReviewedBy, &result.ReviewComments, &reviewedAt, &result.ReopenReason, &createdAt, &updatedAt); err != nil {
		return Result{}, err
	}
	result.Status = ResultStatus(status)
	result.ReviewedAt = parseOptionalTime(reviewedAt)
	result.CreatedAt, _ = parseTime(createdAt)
	result.UpdatedAt, _ = parseTime(updatedAt)
	return result, nil
}

func scanResults(rows *sql.Rows) ([]Result, error) {
	results := []Result{}
	for rows.Next() {
		result, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}
