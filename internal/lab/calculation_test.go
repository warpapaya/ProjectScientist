package lab

import (
	"math"
	"strings"
	"testing"
)

func TestCalculateDerivedResultIsDeterministicAndTraceable(t *testing.T) {
	formula := CalculationFormula{
		ID:            "calc-total-lead-dilution",
		Version:       3,
		Name:          "Lead with dilution correction",
		Expression:    "{lead_mg_l} * {dilution_factor}",
		OutputUnit:    "mg/L",
		DecimalPlaces: 4,
	}
	input := CalculationRequest{
		Formula: formula,
		Inputs: map[string]CalculationInput{
			"lead_mg_l":       {ResultID: "RES-000123", Analyte: "Lead", Value: 0.00625, Unit: "mg/L"},
			"dilution_factor": {ResultID: "RES-000124", Analyte: "Dilution factor", Value: 2, Unit: "x"},
		},
	}

	first, err := CalculateDerivedResult(input)
	if err != nil {
		t.Fatalf("calculate first: %v", err)
	}
	second, err := CalculateDerivedResult(input)
	if err != nil {
		t.Fatalf("calculate second: %v", err)
	}

	if first.Value != 0.0125 || first.DisplayValue != "0.0125" || first.Unit != "mg/L" {
		t.Fatalf("unexpected derived result: %#v", first)
	}
	if first.FormulaID != formula.ID || first.FormulaVersion != formula.Version || first.FormulaExpression != formula.Expression {
		t.Fatalf("formula provenance not retained: %#v", first)
	}
	if first.FormulaHash == "" || first.FormulaHash != second.FormulaHash || first.Value != second.Value || first.DisplayValue != second.DisplayValue {
		t.Fatalf("calculation is not deterministic/provenanced: first=%#v second=%#v", first, second)
	}
	if len(first.Inputs) != 2 {
		t.Fatalf("expected two trace inputs, got %#v", first.Inputs)
	}
	if first.Inputs[0].Key != "dilution_factor" || first.Inputs[0].ResultID != "RES-000124" || first.Inputs[1].Key != "lead_mg_l" || first.Inputs[1].ResultID != "RES-000123" {
		t.Fatalf("input trace should be stable and result-linked, got %#v", first.Inputs)
	}
	if first.AuditDetails["formula_id"] != formula.ID || first.AuditDetails["formula_version"] != float64(3) || first.AuditDetails["input_count"] != float64(2) {
		t.Fatalf("audit details missing minimal provenance: %#v", first.AuditDetails)
	}
}

func TestCalculateDerivedResultRejectsInvalidInputs(t *testing.T) {
	formula := CalculationFormula{ID: "calc-ratio", Version: 1, Expression: "{mass_mg} / {volume_l}", OutputUnit: "mg/L", DecimalPlaces: 3}

	_, err := CalculateDerivedResult(CalculationRequest{Formula: formula, Inputs: map[string]CalculationInput{
		"mass_mg": {ResultID: "RES-1", Value: 12, Unit: "mg"},
	}})
	if err == nil || !strings.Contains(err.Error(), "missing input") {
		t.Fatalf("expected missing input error, got %v", err)
	}

	_, err = CalculateDerivedResult(CalculationRequest{Formula: formula, Inputs: map[string]CalculationInput{
		"mass_mg":  {ResultID: "RES-1", Value: math.NaN(), Unit: "mg"},
		"volume_l": {ResultID: "RES-2", Value: 2, Unit: "L"},
	}})
	if err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatalf("expected finite value error, got %v", err)
	}

	_, err = CalculateDerivedResult(CalculationRequest{Formula: formula, Inputs: map[string]CalculationInput{
		"mass_mg":  {ResultID: "RES-1", Value: 12, Unit: "mg"},
		"volume_l": {ResultID: "RES-2", Value: 0, Unit: "L"},
	}})
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero error, got %v", err)
	}
}

func TestFormulaVersionProvenanceChangesHash(t *testing.T) {
	inputs := map[string]CalculationInput{"a": {ResultID: "RES-a", Value: 1, Unit: "mg/L"}}
	v1, err := CalculateDerivedResult(CalculationRequest{Formula: CalculationFormula{ID: "calc-a", Version: 1, Expression: "{a}", OutputUnit: "mg/L"}, Inputs: inputs})
	if err != nil {
		t.Fatalf("v1 calculate: %v", err)
	}
	v2, err := CalculateDerivedResult(CalculationRequest{Formula: CalculationFormula{ID: "calc-a", Version: 2, Expression: "{a}", OutputUnit: "mg/L"}, Inputs: inputs})
	if err != nil {
		t.Fatalf("v2 calculate: %v", err)
	}
	if v1.FormulaHash == v2.FormulaHash {
		t.Fatalf("formula hash must include version: v1=%#v v2=%#v", v1, v2)
	}
	if v1.FormulaVersion != 1 || v2.FormulaVersion != 2 {
		t.Fatalf("formula versions not retained: v1=%#v v2=%#v", v1, v2)
	}
}
