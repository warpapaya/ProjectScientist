package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenant_id"`
	LabID                 string    `json:"lab_id"`
	AnalysisRequestLineID string    `json:"analysis_request_line_id"`
	SampleID              string    `json:"sample_id"`
	Value                 string    `json:"value,omitempty"`
	RawValue              string    `json:"raw_value,omitempty"`
	Unit                  string    `json:"unit,omitempty"`
	Qualifier             string    `json:"qualifier,omitempty"`
	MDL                   string    `json:"mdl,omitempty"`
	RL                    string    `json:"rl,omitempty"`
	LOQ                   string    `json:"loq,omitempty"`
	Dilution              string    `json:"dilution,omitempty"`
	Uncertainty           string    `json:"uncertainty,omitempty"`
	Comments              string    `json:"comments,omitempty"`
	AnalystID             string    `json:"analyst_id,omitempty"`
	InstrumentID          string    `json:"instrument_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ResultInput struct {
	AnalysisRequestLineID string `json:"analysis_request_line_id"`
	Value                 string `json:"value"`
	RawValue              string `json:"raw_value"`
	Unit                  string `json:"unit"`
	Qualifier             string `json:"qualifier"`
	MDL                   string `json:"mdl"`
	RL                    string `json:"rl"`
	LOQ                   string `json:"loq"`
	Dilution              string `json:"dilution"`
	Uncertainty           string `json:"uncertainty"`
	Comments              string `json:"comments"`
	AnalystID             string `json:"analyst_id"`
	InstrumentID          string `json:"instrument_id"`
}

func (s *Store) UpsertResult(input ResultInput, actor ActorContext) (Result, error) {
	return s.UpsertResultForScope(defaultScope(), input, actor)
}

func (s *Store) UpsertResultForScope(scope Scope, input ResultInput, actor ActorContext) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Result{}, err
	}
	input = normalizeResultInput(input)
	if err := validateResultInput(input); err != nil {
		return Result{}, err
	}

	var result Result
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationResultEntry, actor, AuditResource{Type: "result", ID: "new"}, map[string]any{"analysis_request_line_id": input.AnalysisRequestLineID})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}

		line, err := analysisRequestLineByIDForScopeTx(tx, scope, input.AnalysisRequestLineID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown analysis request line %q", input.AnalysisRequestLineID)
			}
			return err
		}
		if line.Status == AnalysisRequestLineStatusCancelled {
			deniedErr = fmt.Errorf("cannot enter result for cancelled analysis request line %q", input.AnalysisRequestLineID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.upsert.requested", Outcome: AuditOutcomeDenied, Reason: "line_cancelled", Resource: AuditResource{Type: "analysis_request_line", ID: input.AnalysisRequestLineID}, Details: map[string]any{"analysis_request_line_id": input.AnalysisRequestLineID, "sample_id": line.SampleID}})
		}

		existing, err := resultByAnalysisRequestLineTx(tx, input.AnalysisRequestLineID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := time.Now().UTC()
		if errors.Is(err, sql.ErrNoRows) {
			next, err := nextCounter(tx, "next_result")
			if err != nil {
				return err
			}
			result = resultFromInput(input)
			result.ID = fmt.Sprintf("R-%06d", next)
			result.TenantID = scope.TenantID
			result.LabID = scope.LabID
			result.SampleID = line.SampleID
			result.CreatedAt = now
			result.UpdatedAt = now
			if err := insertResultTx(tx, result); err != nil {
				return err
			}
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: result.ID}, Details: resultAuditDetails(result, nil)})
		}

		result = resultFromInput(input)
		result.ID = existing.ID
		result.TenantID = existing.TenantID
		result.LabID = existing.LabID
		result.SampleID = existing.SampleID
		result.CreatedAt = existing.CreatedAt
		result.UpdatedAt = now
		changed := changedResultFields(existing, result)
		if err := updateResultTx(tx, result); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "result.updated", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "result", ID: result.ID}, Details: resultAuditDetails(result, changed)})
	})
	if err != nil {
		return Result{}, err
	}
	if deniedErr != nil {
		return Result{}, deniedErr
	}
	return result, nil
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
	result, err := resultByAnalysisRequestLineTx(s.db, strings.TrimSpace(lineID))
	if err != nil || result.TenantID != scope.TenantID || result.LabID != scope.LabID {
		return Result{}, false
	}
	return result, true
}

func normalizeResultInput(input ResultInput) ResultInput {
	input.AnalysisRequestLineID = strings.TrimSpace(input.AnalysisRequestLineID)
	input.Value = strings.TrimSpace(input.Value)
	input.RawValue = strings.TrimSpace(input.RawValue)
	input.Unit = strings.TrimSpace(input.Unit)
	input.Qualifier = strings.ToUpper(strings.TrimSpace(input.Qualifier))
	input.MDL = strings.TrimSpace(input.MDL)
	input.RL = strings.TrimSpace(input.RL)
	input.LOQ = strings.TrimSpace(input.LOQ)
	input.Dilution = strings.TrimSpace(input.Dilution)
	input.Uncertainty = strings.TrimSpace(input.Uncertainty)
	input.Comments = strings.TrimSpace(input.Comments)
	input.AnalystID = strings.TrimSpace(input.AnalystID)
	input.InstrumentID = strings.TrimSpace(input.InstrumentID)
	return input
}

func validateResultInput(input ResultInput) error {
	if input.AnalysisRequestLineID == "" {
		return errors.New("analysis request line id is required")
	}
	if input.Value == "" && input.RawValue == "" {
		return errors.New("result value or raw value is required")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"MDL", input.MDL},
		{"RL", input.RL},
		{"LOQ", input.LOQ},
	} {
		if field.value == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(field.value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
			return fmt.Errorf("%s must be a finite non-negative number", field.name)
		}
	}
	if input.Dilution != "" {
		parsed, err := strconv.ParseFloat(input.Dilution, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 {
			return errors.New("dilution must be a finite positive number")
		}
	}
	return nil
}

func resultFromInput(input ResultInput) Result {
	return Result{AnalysisRequestLineID: input.AnalysisRequestLineID, Value: input.Value, RawValue: input.RawValue, Unit: input.Unit, Qualifier: input.Qualifier, MDL: input.MDL, RL: input.RL, LOQ: input.LOQ, Dilution: input.Dilution, Uncertainty: input.Uncertainty, Comments: input.Comments, AnalystID: input.AnalystID, InstrumentID: input.InstrumentID}
}

func resultAuditDetails(result Result, changed []string) map[string]any {
	details := map[string]any{
		"analysis_request_line_id": result.AnalysisRequestLineID,
		"sample_id":                result.SampleID,
		"value":                    result.Value,
		"raw_value":                result.RawValue,
		"unit":                     result.Unit,
		"qualifier":                result.Qualifier,
		"mdl":                      result.MDL,
		"rl":                       result.RL,
		"loq":                      result.LOQ,
		"dilution":                 result.Dilution,
		"uncertainty":              result.Uncertainty,
		"comments":                 result.Comments,
		"analyst_id":               result.AnalystID,
		"instrument_id":            result.InstrumentID,
	}
	if changed != nil {
		details["changed_fields"] = changed
	}
	return details
}

func changedResultFields(old, next Result) []string {
	changed := []string{}
	checks := []struct {
		name string
		old  string
		next string
	}{
		{"value", old.Value, next.Value},
		{"raw_value", old.RawValue, next.RawValue},
		{"unit", old.Unit, next.Unit},
		{"qualifier", old.Qualifier, next.Qualifier},
		{"mdl", old.MDL, next.MDL},
		{"rl", old.RL, next.RL},
		{"loq", old.LOQ, next.LOQ},
		{"dilution", old.Dilution, next.Dilution},
		{"uncertainty", old.Uncertainty, next.Uncertainty},
		{"comments", old.Comments, next.Comments},
		{"analyst_id", old.AnalystID, next.AnalystID},
		{"instrument_id", old.InstrumentID, next.InstrumentID},
	}
	for _, check := range checks {
		if check.old != check.next {
			changed = append(changed, check.name)
		}
	}
	return changed
}

type resultQueryer interface{ QueryRow(string, ...any) *sql.Row }

const resultSelectSQL = `SELECT id, tenant_id, lab_id, analysis_request_line_id, sample_id, value, raw_value, unit, qualifier, mdl, rl, loq, dilution, uncertainty, comments, analyst_id, instrument_id, created_at, updated_at`

func resultByAnalysisRequestLineTx(q resultQueryer, lineID string) (Result, error) {
	return scanResult(q.QueryRow(resultSelectSQL+` FROM results WHERE analysis_request_line_id = ?`, strings.TrimSpace(lineID)))
}

type resultScanner interface{ Scan(dest ...any) error }

func scanResult(row resultScanner) (Result, error) {
	var result Result
	var created, updated string
	if err := row.Scan(&result.ID, &result.TenantID, &result.LabID, &result.AnalysisRequestLineID, &result.SampleID, &result.Value, &result.RawValue, &result.Unit, &result.Qualifier, &result.MDL, &result.RL, &result.LOQ, &result.Dilution, &result.Uncertainty, &result.Comments, &result.AnalystID, &result.InstrumentID, &created, &updated); err != nil {
		return Result{}, err
	}
	result.CreatedAt, _ = parseTime(created)
	result.UpdatedAt, _ = parseTime(updated)
	return result, nil
}

func insertResultTx(tx *sql.Tx, result Result) error {
	_, err := tx.Exec(`INSERT INTO results(id, tenant_id, lab_id, analysis_request_line_id, sample_id, value, raw_value, unit, qualifier, mdl, rl, loq, dilution, uncertainty, comments, analyst_id, instrument_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, result.ID, result.TenantID, result.LabID, result.AnalysisRequestLineID, result.SampleID, result.Value, result.RawValue, result.Unit, result.Qualifier, result.MDL, result.RL, result.LOQ, result.Dilution, result.Uncertainty, result.Comments, result.AnalystID, result.InstrumentID, formatTime(result.CreatedAt), formatTime(result.UpdatedAt))
	return err
}

func updateResultTx(tx *sql.Tx, result Result) error {
	_, err := tx.Exec(`UPDATE results SET value = ?, raw_value = ?, unit = ?, qualifier = ?, mdl = ?, rl = ?, loq = ?, dilution = ?, uncertainty = ?, comments = ?, analyst_id = ?, instrument_id = ?, updated_at = ? WHERE id = ?`, result.Value, result.RawValue, result.Unit, result.Qualifier, result.MDL, result.RL, result.LOQ, result.Dilution, result.Uncertainty, result.Comments, result.AnalystID, result.InstrumentID, formatTime(result.UpdatedAt), result.ID)
	return err
}
