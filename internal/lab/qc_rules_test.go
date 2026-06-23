package lab

import (
	"path/filepath"
	"testing"
)

func TestQCLimitRulesEvaluatePassWarnAndFailDeterministically(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	_, method, _ := createQCClientMethodAndService(t, store, actor)

	oldRule, err := store.CreateQCLimitRule(CreateQCLimitRuleInput{
		MethodID: method.ID,
		Matrix:   "Water",
		Analyte:  "Lead",
		Unit:     "mg/L",
		Min:      floatPtr(0),
		Max:      floatPtr(10),
		WarnHigh: floatPtr(8),
		Source:   "synthetic-v1",
		Notes:    "old rule superseded by version 2",
	}, actor)
	if err != nil {
		t.Fatalf("create old rule: %v", err)
	}
	currentRule, err := store.CreateQCLimitRule(CreateQCLimitRuleInput{
		MethodID: method.ID,
		Matrix:   " water ",
		Analyte:  " lead ",
		Unit:     "mg/L",
		Min:      floatPtr(0),
		Max:      floatPtr(5),
		WarnHigh: floatPtr(4),
		Source:   "synthetic-v2",
	}, actor)
	if err != nil {
		t.Fatalf("create current rule: %v", err)
	}
	if oldRule.Version != 1 || currentRule.Version != 2 {
		t.Fatalf("rule versions got old=%d current=%d want 1/2", oldRule.Version, currentRule.Version)
	}

	cases := []struct {
		name       string
		value      float64
		wantStatus QCEvaluationStatus
		wantCode   string
	}{
		{name: "pass below warn band", value: 3.9, wantStatus: QCEvaluationPass},
		{name: "warn inside limits but outside warning band", value: 4.2, wantStatus: QCEvaluationWarn, wantCode: "qc_warn_high"},
		{name: "fail above hard limit", value: 5.1, wantStatus: QCEvaluationFail, wantCode: "qc_fail_high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eval, err := store.EvaluateQCResult(QCEvaluationInput{MethodID: method.ID, Matrix: "WATER", Analyte: "Lead", Unit: "mg/L", Value: tc.value})
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if eval.Status != tc.wantStatus {
				t.Fatalf("status got %q want %q: %#v", eval.Status, tc.wantStatus, eval)
			}
			if len(eval.Provenance) != 1 || eval.Provenance[0].RuleID != currentRule.ID || eval.Provenance[0].Version != 2 || eval.Provenance[0].Source != "synthetic-v2" {
				t.Fatalf("expected current rule provenance, got %#v", eval.Provenance)
			}
			if tc.wantCode == "" && len(eval.Flags) != 0 {
				t.Fatalf("expected no flags, got %#v", eval.Flags)
			}
			if tc.wantCode != "" {
				if len(eval.Flags) != 1 || eval.Flags[0].Code != tc.wantCode || eval.Flags[0].RuleID != currentRule.ID || eval.Flags[0].RuleVersion != 2 {
					t.Fatalf("expected %s flag from current rule, got %#v", tc.wantCode, eval.Flags)
				}
			}
		})
	}
}

func TestQCLimitRulesRequireExactMethodMatrixAnalyteScope(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	_, method, _ := createQCClientMethodAndService(t, store, actor)
	if _, err := store.CreateQCLimitRule(CreateQCLimitRuleInput{MethodID: method.ID, Matrix: "Water", Analyte: "Lead", Max: floatPtr(5), Source: "synthetic"}, actor); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	eval, err := store.EvaluateQCResult(QCEvaluationInput{MethodID: method.ID, Matrix: "Soil", Analyte: "Lead", Value: 100})
	if err != nil {
		t.Fatalf("evaluate unmatched: %v", err)
	}
	if eval.Status != QCEvaluationNoRule || len(eval.Flags) != 1 || eval.Flags[0].Code != "qc_no_rule" {
		t.Fatalf("expected no-rule flag for unmatched matrix, got %#v", eval)
	}
}

func TestQCLimitRulesRejectInvalidLimits(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := catalogTestActor()
	_, method, _ := createQCClientMethodAndService(t, store, actor)
	_, err = store.CreateQCLimitRule(CreateQCLimitRuleInput{MethodID: method.ID, Matrix: "Water", Analyte: "Lead", Min: floatPtr(10), Max: floatPtr(5), Source: "bad"}, actor)
	if err == nil {
		t.Fatalf("expected invalid min/max rule to fail")
	}
}

func floatPtr(v float64) *float64 { return &v }
