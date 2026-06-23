package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// CalculationFormula is the immutable formula/version snapshot used for a derived result.
// Expression supports numbers, +, -, *, /, parentheses, and input references as {input_key}.
type CalculationFormula struct {
	ID            string `json:"id"`
	Version       int    `json:"version"`
	Name          string `json:"name,omitempty"`
	Expression    string `json:"expression"`
	OutputUnit    string `json:"output_unit,omitempty"`
	DecimalPlaces int    `json:"decimal_places,omitempty"`
}

type CalculationInput struct {
	ResultID string  `json:"result_id"`
	Analyte  string  `json:"analyte,omitempty"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit,omitempty"`
}

type CalculationRequest struct {
	Formula CalculationFormula          `json:"formula"`
	Inputs  map[string]CalculationInput `json:"inputs"`
}

type CalculationInputTrace struct {
	Key      string  `json:"key"`
	ResultID string  `json:"result_id"`
	Analyte  string  `json:"analyte,omitempty"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit,omitempty"`
}

type DerivedCalculationResult struct {
	Value             float64                 `json:"value"`
	DisplayValue      string                  `json:"display_value"`
	Unit              string                  `json:"unit,omitempty"`
	FormulaID         string                  `json:"formula_id"`
	FormulaVersion    int                     `json:"formula_version"`
	FormulaExpression string                  `json:"formula_expression"`
	FormulaHash       string                  `json:"formula_hash"`
	Inputs            []CalculationInputTrace `json:"inputs"`
	AuditDetails      map[string]any          `json:"audit_details"`
}

func CalculateDerivedResult(request CalculationRequest) (DerivedCalculationResult, error) {
	formula, err := normalizeCalculationFormula(request.Formula)
	if err != nil {
		return DerivedCalculationResult{}, err
	}
	if request.Inputs == nil {
		return DerivedCalculationResult{}, errors.New("calculation inputs are required")
	}

	parser, err := newCalculationParser(formula.Expression, request.Inputs)
	if err != nil {
		return DerivedCalculationResult{}, err
	}
	value, err := parser.parse()
	if err != nil {
		return DerivedCalculationResult{}, err
	}
	if !isFinite(value) {
		return DerivedCalculationResult{}, errors.New("derived calculation result must be finite")
	}
	value = roundToPlaces(value, formula.DecimalPlaces)
	hash, err := formulaHash(formula)
	if err != nil {
		return DerivedCalculationResult{}, err
	}
	trace := calculationTrace(parser.usedInputs, request.Inputs)

	return DerivedCalculationResult{
		Value:             value,
		DisplayValue:      formatCalculationValue(value, formula.DecimalPlaces),
		Unit:              formula.OutputUnit,
		FormulaID:         formula.ID,
		FormulaVersion:    formula.Version,
		FormulaExpression: formula.Expression,
		FormulaHash:       hash,
		Inputs:            trace,
		AuditDetails: map[string]any{
			"formula_id":      formula.ID,
			"formula_version": float64(formula.Version),
			"formula_hash":    hash,
			"expression":      formula.Expression,
			"input_count":     float64(len(trace)),
		},
	}, nil
}

func normalizeCalculationFormula(formula CalculationFormula) (CalculationFormula, error) {
	formula.ID = strings.TrimSpace(formula.ID)
	formula.Name = strings.TrimSpace(formula.Name)
	formula.Expression = strings.TrimSpace(formula.Expression)
	formula.OutputUnit = strings.TrimSpace(formula.OutputUnit)
	if formula.ID == "" {
		return CalculationFormula{}, errors.New("calculation formula id is required")
	}
	if formula.Version <= 0 {
		return CalculationFormula{}, errors.New("calculation formula version is required")
	}
	if formula.Expression == "" {
		return CalculationFormula{}, errors.New("calculation formula expression is required")
	}
	if formula.DecimalPlaces < 0 || formula.DecimalPlaces > 12 {
		return CalculationFormula{}, errors.New("calculation decimal places must be between 0 and 12")
	}
	return formula, nil
}

func formulaHash(formula CalculationFormula) (string, error) {
	canonical := struct {
		ID            string `json:"id"`
		Version       int    `json:"version"`
		Name          string `json:"name,omitempty"`
		Expression    string `json:"expression"`
		OutputUnit    string `json:"output_unit,omitempty"`
		DecimalPlaces int    `json:"decimal_places,omitempty"`
	}{ID: formula.ID, Version: formula.Version, Name: formula.Name, Expression: formula.Expression, OutputUnit: formula.OutputUnit, DecimalPlaces: formula.DecimalPlaces}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func calculationTrace(used map[string]struct{}, inputs map[string]CalculationInput) []CalculationInputTrace {
	keys := make([]string, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	trace := make([]CalculationInputTrace, 0, len(keys))
	for _, key := range keys {
		input := inputs[key]
		trace = append(trace, CalculationInputTrace{Key: key, ResultID: strings.TrimSpace(input.ResultID), Analyte: strings.TrimSpace(input.Analyte), Value: input.Value, Unit: strings.TrimSpace(input.Unit)})
	}
	return trace
}

func roundToPlaces(value float64, places int) float64 {
	if places <= 0 {
		return math.Round(value)
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func formatCalculationValue(value float64, places int) string {
	if places > 0 {
		return strconv.FormatFloat(value, 'f', places, 64)
	}
	return strconv.FormatFloat(value, 'f', 0, 64)
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

type calculationTokenKind int

const (
	calculationTokenEOF calculationTokenKind = iota
	calculationTokenNumber
	calculationTokenVariable
	calculationTokenOperator
	calculationTokenLeftParen
	calculationTokenRightParen
)

type calculationToken struct {
	kind  calculationTokenKind
	text  string
	value float64
}

type calculationParser struct {
	tokens     []calculationToken
	position   int
	inputs     map[string]CalculationInput
	usedInputs map[string]struct{}
}

func newCalculationParser(expression string, inputs map[string]CalculationInput) (*calculationParser, error) {
	tokens, err := tokenizeCalculationExpression(expression)
	if err != nil {
		return nil, err
	}
	return &calculationParser{tokens: tokens, inputs: inputs, usedInputs: map[string]struct{}{}}, nil
}

func (p *calculationParser) parse() (float64, error) {
	value, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	if p.peek().kind != calculationTokenEOF {
		return 0, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return value, nil
}

func (p *calculationParser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.peek().kind == calculationTokenOperator && (p.peek().text == "+" || p.peek().text == "-") {
		op := p.next().text
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == "+" {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *calculationParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.peek().kind == calculationTokenOperator && (p.peek().text == "*" || p.peek().text == "/") {
		op := p.next().text
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == "*" {
			left *= right
			continue
		}
		if right == 0 {
			return 0, errors.New("division by zero in calculation formula")
		}
		left /= right
	}
	return left, nil
}

func (p *calculationParser) parseFactor() (float64, error) {
	token := p.next()
	switch token.kind {
	case calculationTokenNumber:
		return token.value, nil
	case calculationTokenVariable:
		input, ok := p.inputs[token.text]
		if !ok {
			return 0, fmt.Errorf("missing input %q", token.text)
		}
		if strings.TrimSpace(input.ResultID) == "" {
			return 0, fmt.Errorf("input %q result id is required", token.text)
		}
		if !isFinite(input.Value) {
			return 0, fmt.Errorf("input %q value must be finite", token.text)
		}
		p.usedInputs[token.text] = struct{}{}
		return input.Value, nil
	case calculationTokenOperator:
		if token.text == "+" {
			return p.parseFactor()
		}
		if token.text == "-" {
			value, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			return -value, nil
		}
	case calculationTokenLeftParen:
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if p.next().kind != calculationTokenRightParen {
			return 0, errors.New("missing closing parenthesis in calculation formula")
		}
		return value, nil
	}
	return 0, fmt.Errorf("unexpected token %q", token.text)
}

func (p *calculationParser) peek() calculationToken {
	if p.position >= len(p.tokens) {
		return calculationToken{kind: calculationTokenEOF}
	}
	return p.tokens[p.position]
}

func (p *calculationParser) next() calculationToken {
	token := p.peek()
	if p.position < len(p.tokens) {
		p.position++
	}
	return token
}

func tokenizeCalculationExpression(expression string) ([]calculationToken, error) {
	tokens := []calculationToken{}
	for i := 0; i < len(expression); {
		r := rune(expression[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		switch r {
		case '+', '-', '*', '/':
			tokens = append(tokens, calculationToken{kind: calculationTokenOperator, text: string(r)})
			i++
			continue
		case '(':
			tokens = append(tokens, calculationToken{kind: calculationTokenLeftParen, text: string(r)})
			i++
			continue
		case ')':
			tokens = append(tokens, calculationToken{kind: calculationTokenRightParen, text: string(r)})
			i++
			continue
		case '{':
			end := strings.IndexByte(expression[i+1:], '}')
			if end < 0 {
				return nil, errors.New("unterminated input reference in calculation formula")
			}
			key := strings.TrimSpace(expression[i+1 : i+1+end])
			if key == "" {
				return nil, errors.New("empty input reference in calculation formula")
			}
			tokens = append(tokens, calculationToken{kind: calculationTokenVariable, text: key})
			i += end + 2
			continue
		}
		if unicode.IsDigit(r) || r == '.' {
			start := i
			for i < len(expression) {
				current := rune(expression[i])
				if !unicode.IsDigit(current) && current != '.' {
					break
				}
				i++
			}
			raw := expression[start:i]
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil || !isFinite(value) {
				return nil, fmt.Errorf("invalid number %q in calculation formula", raw)
			}
			tokens = append(tokens, calculationToken{kind: calculationTokenNumber, text: raw, value: value})
			continue
		}
		return nil, fmt.Errorf("unsupported token %q in calculation formula", string(r))
	}
	tokens = append(tokens, calculationToken{kind: calculationTokenEOF})
	return tokens, nil
}
