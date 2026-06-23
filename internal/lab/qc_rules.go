package lab

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type QCEvaluationStatus string

const (
	QCEvaluationPass   QCEvaluationStatus = "pass"
	QCEvaluationWarn   QCEvaluationStatus = "warn"
	QCEvaluationFail   QCEvaluationStatus = "fail"
	QCEvaluationNoRule QCEvaluationStatus = "no_rule"
)

type QCLimitRule struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	LabID     string    `json:"lab_id"`
	MethodID  string    `json:"method_id"`
	Matrix    string    `json:"matrix"`
	Analyte   string    `json:"analyte"`
	Unit      string    `json:"unit,omitempty"`
	Min       *float64  `json:"min,omitempty"`
	Max       *float64  `json:"max,omitempty"`
	WarnLow   *float64  `json:"warn_low,omitempty"`
	WarnHigh  *float64  `json:"warn_high,omitempty"`
	Version   int       `json:"version"`
	Source    string    `json:"source"`
	Notes     string    `json:"notes,omitempty"`
	Active    bool      `json:"active"`
	RuleHash  string    `json:"rule_hash"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateQCLimitRuleInput struct {
	MethodID string   `json:"method_id"`
	Matrix   string   `json:"matrix"`
	Analyte  string   `json:"analyte"`
	Unit     string   `json:"unit"`
	Min      *float64 `json:"min"`
	Max      *float64 `json:"max"`
	WarnLow  *float64 `json:"warn_low"`
	WarnHigh *float64 `json:"warn_high"`
	Source   string   `json:"source"`
	Notes    string   `json:"notes"`
}

type QCEvaluationInput struct {
	MethodID string  `json:"method_id"`
	Matrix   string  `json:"matrix"`
	Analyte  string  `json:"analyte"`
	Unit     string  `json:"unit,omitempty"`
	Value    float64 `json:"value"`
	ResultID string  `json:"result_id,omitempty"`
}

type QCFlag struct {
	Code        string             `json:"code"`
	Severity    QCEvaluationStatus `json:"severity"`
	Message     string             `json:"message"`
	RuleID      string             `json:"rule_id,omitempty"`
	RuleVersion int                `json:"rule_version,omitempty"`
	Value       float64            `json:"value"`
	Limit       *float64           `json:"limit,omitempty"`
}

type QCRuleProvenance struct {
	RuleID   string `json:"rule_id"`
	Version  int    `json:"version"`
	Source   string `json:"source"`
	RuleHash string `json:"rule_hash"`
}

type QCEvaluation struct {
	Status     QCEvaluationStatus `json:"status"`
	Flags      []QCFlag           `json:"flags"`
	Provenance []QCRuleProvenance `json:"provenance"`
}

func (s *Store) CreateQCLimitRule(input CreateQCLimitRuleInput, actor ActorContext) (QCLimitRule, error) {
	return s.CreateQCLimitRuleForScope(defaultScope(), input, actor)
}

func (s *Store) CreateQCLimitRuleForScope(scope Scope, input CreateQCLimitRuleInput, actor ActorContext) (QCLimitRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCLimitRule{}, err
	}
	input = normalizeQCLimitRuleInput(input)
	if err := validateQCLimitRuleInput(input); err != nil {
		return QCLimitRule{}, err
	}

	var rule QCLimitRule
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_limit_rule", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		if err := requireScopedID(tx, "catalog_methods", scope, input.MethodID); err != nil {
			return fmt.Errorf("method: %w", err)
		}
		next, err := nextCounter(tx, "next_qc_limit_rule")
		if err != nil {
			return err
		}
		version, err := nextQCLimitRuleVersionTx(tx, scope, input.MethodID, input.Matrix, input.Analyte)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		rule = QCLimitRule{ID: fmt.Sprintf("QCLR-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, MethodID: input.MethodID, Matrix: input.Matrix, Analyte: input.Analyte, Unit: input.Unit, Min: cloneFloatPtr(input.Min), Max: cloneFloatPtr(input.Max), WarnLow: cloneFloatPtr(input.WarnLow), WarnHigh: cloneFloatPtr(input.WarnHigh), Version: version, Source: input.Source, Notes: input.Notes, Active: true, CreatedAt: now}
		hash, err := qcLimitRuleHash(rule)
		if err != nil {
			return err
		}
		rule.RuleHash = hash
		if _, err := tx.Exec(`INSERT INTO qc_limit_rules(id, tenant_id, lab_id, method_id, matrix_key, analyte_key, matrix, analyte, unit, min_value, max_value, warn_low, warn_high, version, source, notes, active, rule_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rule.ID, rule.TenantID, rule.LabID, rule.MethodID, qcMatchKey(rule.Matrix), qcMatchKey(rule.Analyte), rule.Matrix, rule.Analyte, rule.Unit, nullableFloat(rule.Min), nullableFloat(rule.Max), nullableFloat(rule.WarnLow), nullableFloat(rule.WarnHigh), rule.Version, rule.Source, rule.Notes, boolToInt(rule.Active), rule.RuleHash, formatTime(rule.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_limit_rule.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_limit_rule", ID: rule.ID, Version: strconv.Itoa(rule.Version)}, Details: map[string]any{"method_id": rule.MethodID, "matrix": rule.Matrix, "analyte": rule.Analyte, "version": float64(rule.Version), "source": rule.Source, "rule_hash": rule.RuleHash}})
	})
	if txErr != nil {
		return QCLimitRule{}, txErr
	}
	if deniedErr != nil {
		return QCLimitRule{}, deniedErr
	}
	return rule, nil
}

func (s *Store) EvaluateQCResult(input QCEvaluationInput) (QCEvaluation, error) {
	return s.EvaluateQCResultForScope(defaultScope(), input)
}

func (s *Store) EvaluateQCResultForScope(scope Scope, input QCEvaluationInput) (QCEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCEvaluation{}, err
	}
	input.MethodID = strings.TrimSpace(input.MethodID)
	input.Matrix = strings.TrimSpace(input.Matrix)
	input.Analyte = strings.TrimSpace(input.Analyte)
	input.Unit = strings.TrimSpace(input.Unit)
	input.ResultID = strings.TrimSpace(input.ResultID)
	if input.MethodID == "" || input.Matrix == "" || input.Analyte == "" {
		return QCEvaluation{}, errors.New("QC evaluation requires method, matrix, and analyte")
	}
	if !isFinite(input.Value) {
		return QCEvaluation{}, errors.New("QC evaluation value must be finite")
	}

	rule, ok, err := s.currentQCLimitRuleForInput(scope, input)
	if err != nil {
		return QCEvaluation{}, err
	}
	if !ok {
		return QCEvaluation{Status: QCEvaluationNoRule, Flags: []QCFlag{{Code: "qc_no_rule", Severity: QCEvaluationNoRule, Message: "no QC limit rule matched method, matrix, and analyte", Value: input.Value}}}, nil
	}

	eval := QCEvaluation{Status: QCEvaluationPass, Provenance: []QCRuleProvenance{{RuleID: rule.ID, Version: rule.Version, Source: rule.Source, RuleHash: rule.RuleHash}}}
	if rule.Min != nil && input.Value < *rule.Min {
		eval.Status = QCEvaluationFail
		eval.Flags = append(eval.Flags, qcFlag("qc_fail_low", QCEvaluationFail, "value is below the hard QC minimum", rule, input.Value, rule.Min))
	}
	if rule.Max != nil && input.Value > *rule.Max {
		eval.Status = QCEvaluationFail
		eval.Flags = append(eval.Flags, qcFlag("qc_fail_high", QCEvaluationFail, "value is above the hard QC maximum", rule, input.Value, rule.Max))
	}
	if eval.Status != QCEvaluationFail {
		if rule.WarnLow != nil && input.Value < *rule.WarnLow {
			eval.Status = QCEvaluationWarn
			eval.Flags = append(eval.Flags, qcFlag("qc_warn_low", QCEvaluationWarn, "value is below the QC warning threshold", rule, input.Value, rule.WarnLow))
		}
		if rule.WarnHigh != nil && input.Value > *rule.WarnHigh {
			eval.Status = QCEvaluationWarn
			eval.Flags = append(eval.Flags, qcFlag("qc_warn_high", QCEvaluationWarn, "value is above the QC warning threshold", rule, input.Value, rule.WarnHigh))
		}
	}
	sort.SliceStable(eval.Flags, func(i, j int) bool { return eval.Flags[i].Code < eval.Flags[j].Code })
	return eval, nil
}

func (s *Store) currentQCLimitRuleForInput(scope Scope, input QCEvaluationInput) (QCLimitRule, bool, error) {
	row := s.db.QueryRow(qcLimitRuleSelect+` FROM qc_limit_rules WHERE tenant_id = ? AND lab_id = ? AND method_id = ? AND matrix_key = ? AND analyte_key = ? AND active = 1 ORDER BY version DESC, id DESC LIMIT 1`, scope.TenantID, scope.LabID, input.MethodID, qcMatchKey(input.Matrix), qcMatchKey(input.Analyte))
	rule, err := scanQCLimitRule(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return QCLimitRule{}, false, nil
		}
		return QCLimitRule{}, false, err
	}
	return rule, true, nil
}

func nextQCLimitRuleVersionTx(tx *sql.Tx, scope Scope, methodID, matrix, analyte string) (int, error) {
	var current int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM qc_limit_rules WHERE tenant_id = ? AND lab_id = ? AND method_id = ? AND matrix_key = ? AND analyte_key = ?`, scope.TenantID, scope.LabID, methodID, qcMatchKey(matrix), qcMatchKey(analyte)).Scan(&current); err != nil {
		return 0, err
	}
	return current + 1, nil
}

const qcLimitRuleSelect = `SELECT id, tenant_id, lab_id, method_id, matrix, analyte, unit, min_value, max_value, warn_low, warn_high, version, source, notes, active, rule_hash, created_at`

type qcLimitRuleScanner interface{ Scan(dest ...any) error }

func scanQCLimitRule(row qcLimitRuleScanner) (QCLimitRule, error) {
	var rule QCLimitRule
	var minValue, maxValue, warnLow, warnHigh sql.NullFloat64
	var active int
	var created string
	if err := row.Scan(&rule.ID, &rule.TenantID, &rule.LabID, &rule.MethodID, &rule.Matrix, &rule.Analyte, &rule.Unit, &minValue, &maxValue, &warnLow, &warnHigh, &rule.Version, &rule.Source, &rule.Notes, &active, &rule.RuleHash, &created); err != nil {
		return QCLimitRule{}, err
	}
	rule.Min = ptrFromNullFloat(minValue)
	rule.Max = ptrFromNullFloat(maxValue)
	rule.WarnLow = ptrFromNullFloat(warnLow)
	rule.WarnHigh = ptrFromNullFloat(warnHigh)
	rule.Active = active == 1
	if parsed, err := parseTime(created); err == nil {
		rule.CreatedAt = parsed
	}
	return rule, nil
}

func normalizeQCLimitRuleInput(input CreateQCLimitRuleInput) CreateQCLimitRuleInput {
	input.MethodID = strings.TrimSpace(input.MethodID)
	input.Matrix = strings.TrimSpace(input.Matrix)
	input.Analyte = strings.TrimSpace(input.Analyte)
	input.Unit = strings.TrimSpace(input.Unit)
	input.Source = strings.TrimSpace(input.Source)
	input.Notes = strings.TrimSpace(input.Notes)
	return input
}

func validateQCLimitRuleInput(input CreateQCLimitRuleInput) error {
	if input.MethodID == "" {
		return errors.New("QC limit rule method is required")
	}
	if input.Matrix == "" {
		return errors.New("QC limit rule matrix is required")
	}
	if input.Analyte == "" {
		return errors.New("QC limit rule analyte is required")
	}
	if input.Source == "" {
		return errors.New("QC limit rule source is required")
	}
	if input.Min == nil && input.Max == nil && input.WarnLow == nil && input.WarnHigh == nil {
		return errors.New("QC limit rule needs at least one limit")
	}
	for name, value := range map[string]*float64{"min": input.Min, "max": input.Max, "warn_low": input.WarnLow, "warn_high": input.WarnHigh} {
		if value != nil && !isFinite(*value) {
			return fmt.Errorf("QC limit rule %s must be finite", name)
		}
	}
	if input.Min != nil && input.Max != nil && *input.Min > *input.Max {
		return errors.New("QC limit rule minimum cannot exceed maximum")
	}
	if input.WarnLow != nil && input.Min != nil && *input.WarnLow < *input.Min {
		return errors.New("QC warning low cannot be below the hard minimum")
	}
	if input.WarnHigh != nil && input.Max != nil && *input.WarnHigh > *input.Max {
		return errors.New("QC warning high cannot exceed the hard maximum")
	}
	if input.WarnLow != nil && input.WarnHigh != nil && *input.WarnLow > *input.WarnHigh {
		return errors.New("QC warning low cannot exceed QC warning high")
	}
	return nil
}

func qcFlag(code string, severity QCEvaluationStatus, message string, rule QCLimitRule, value float64, limit *float64) QCFlag {
	return QCFlag{Code: code, Severity: severity, Message: message, RuleID: rule.ID, RuleVersion: rule.Version, Value: value, Limit: cloneFloatPtr(limit)}
}

func qcMatchKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func ptrFromNullFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return cloneFloatPtr(&value.Float64)
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func qcLimitRuleHash(rule QCLimitRule) (string, error) {
	canonical := struct {
		MethodID string   `json:"method_id"`
		Matrix   string   `json:"matrix"`
		Analyte  string   `json:"analyte"`
		Unit     string   `json:"unit,omitempty"`
		Min      *float64 `json:"min,omitempty"`
		Max      *float64 `json:"max,omitempty"`
		WarnLow  *float64 `json:"warn_low,omitempty"`
		WarnHigh *float64 `json:"warn_high,omitempty"`
		Version  int      `json:"version"`
		Source   string   `json:"source"`
	}{MethodID: rule.MethodID, Matrix: rule.Matrix, Analyte: rule.Analyte, Unit: rule.Unit, Min: rule.Min, Max: rule.Max, WarnLow: rule.WarnLow, WarnHigh: rule.WarnHigh, Version: rule.Version, Source: rule.Source}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
