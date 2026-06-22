package lab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type syntheticFixture struct {
	FixtureID       string                 `json:"fixture_id"`
	Version         string                 `json:"version"`
	Boundary        string                 `json:"boundary"`
	SyntheticOnly   bool                   `json:"synthetic_only"`
	Tenant          namedEntity            `json:"tenant"`
	Client          fixtureClient          `json:"client"`
	Project         namedEntity            `json:"project"`
	Sample          fixtureSample          `json:"sample"`
	AnalysisProfile fixtureAnalysisProfile `json:"analysis_profile"`
	Limits          []fixtureLimit         `json:"limits"`
	Qualifiers      []fixtureQualifier     `json:"qualifiers"`
	QC              fixtureQC              `json:"qc"`
	Report          fixtureReport          `json:"report"`
	Actors          map[string]string      `json:"actors"`
	Scenarios       []fixtureScenario      `json:"scenarios"`
	SENAITEMapping  map[string]string      `json:"senaite_mapping"`
}

type namedEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fixtureClient struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Contact struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"contact"`
}

type fixtureSample struct {
	Matrix       namedEntity `json:"matrix"`
	Container    namedEntity `json:"container"`
	Preservation namedEntity `json:"preservation"`
}

type fixtureAnalysisProfile struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Analytes []fixtureAnalyte `json:"analytes"`
}

type fixtureAnalyte struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Method string `json:"method"`
	Unit   string `json:"unit"`
}

type fixtureLimit struct {
	AnalyteID      string  `json:"analyte_id"`
	ReportingLimit float64 `json:"reporting_limit"`
	Unit           string  `json:"unit"`
}

type fixtureQualifier struct {
	Code    string `json:"code"`
	Meaning string `json:"meaning"`
}

type fixtureQC struct {
	ReviewRule       string   `json:"review_rule"`
	RequiredQCChecks []string `json:"required_qc_checks"`
}

type fixtureReport struct {
	TemplateID string   `json:"template_id"`
	Title      string   `json:"title"`
	Format     string   `json:"format"`
	Fields     []string `json:"fields"`
}

type fixtureScenario struct {
	ID    string   `json:"id"`
	Steps []string `json:"steps"`
}

func TestSyntheticLabFixtureSatisfiesMVPContract(t *testing.T) {
	fixture := loadSyntheticFixture(t)
	if fixture.FixtureID != "psc-mvp-synthetic-lab-v1" {
		t.Fatalf("unexpected fixture id %q", fixture.FixtureID)
	}
	if !fixture.SyntheticOnly || !strings.Contains(strings.ToLower(fixture.Boundary), "lab-test") {
		t.Fatalf("fixture must be synthetic-only and lab-test bounded: %#v", fixture)
	}
	if fixture.Tenant.Name != "Clearline Demo Lab" {
		t.Fatalf("unexpected tenant %q", fixture.Tenant.Name)
	}
	if fixture.Client.Name != "Okefenokee Synthetic Water Authority" {
		t.Fatalf("unexpected client %q", fixture.Client.Name)
	}
	if fixture.Client.Contact.Email != "jordan.demo@example.test" {
		t.Fatalf("unexpected contact email %q", fixture.Client.Contact.Email)
	}
	if fixture.Project.Name != "MVP Drinking Water Compliance Demo" {
		t.Fatalf("unexpected project %q", fixture.Project.Name)
	}
	if fixture.Sample.Matrix.Name != "Drinking Water" {
		t.Fatalf("unexpected matrix %q", fixture.Sample.Matrix.Name)
	}
	if fixture.Sample.Container.Name == "" || fixture.Sample.Preservation.Name == "" {
		t.Fatalf("container and preservation are required: %#v", fixture.Sample)
	}
	if got := len(fixture.AnalysisProfile.Analytes); got < 2 || got > 4 {
		t.Fatalf("expected 2-4 analytes, got %d", got)
	}
}

func TestSyntheticLabFixtureCarriesSENAITEMapping(t *testing.T) {
	fixture := loadSyntheticFixture(t)
	requiredMappings := []string{
		"tenant", "client", "contact", "project", "matrix", "container",
		"preservation", "analysis_profile", "analysis_service", "method",
		"specification_limit", "result_qualifier", "qc_review", "report_template",
	}
	for _, key := range requiredMappings {
		if strings.TrimSpace(fixture.SENAITEMapping[key]) == "" {
			t.Fatalf("missing SENAITE mapping for %s", key)
		}
	}
	seenLimits := map[string]bool{}
	for _, limit := range fixture.Limits {
		seenLimits[limit.AnalyteID] = true
	}
	for _, analyte := range fixture.AnalysisProfile.Analytes {
		if analyte.Method == "" || analyte.Unit == "" {
			t.Fatalf("analyte requires method and unit snapshot: %#v", analyte)
		}
		if !seenLimits[analyte.ID] {
			t.Fatalf("missing reporting limit for analyte %s", analyte.ID)
		}
	}
	if len(fixture.Qualifiers) < 2 {
		t.Fatalf("expected minimal qualifier set, got %d", len(fixture.Qualifiers))
	}
	if len(fixture.QC.RequiredQCChecks) == 0 || fixture.QC.ReviewRule == "" {
		t.Fatalf("expected minimal QC/review expectations: %#v", fixture.QC)
	}
	if fixture.Report.TemplateID == "" || len(fixture.Report.Fields) == 0 {
		t.Fatalf("expected report metadata fields: %#v", fixture.Report)
	}
}

func TestSyntheticLabScenarioPackDefinesE2ESeeds(t *testing.T) {
	fixture := loadSyntheticFixture(t)
	requiredScenarios := map[string]bool{
		"seed-reset":         false,
		"happy-path":         false,
		"denied-controls":    false,
		"audit-verification": false,
	}
	for _, scenario := range fixture.Scenarios {
		if _, ok := requiredScenarios[scenario.ID]; ok {
			requiredScenarios[scenario.ID] = len(scenario.Steps) > 0
		}
	}
	for id, present := range requiredScenarios {
		if !present {
			t.Fatalf("missing scenario %s with executable seed/e2e steps", id)
		}
	}
	for _, role := range []string{"lab-manager", "receiving-tech", "analyst", "reviewer", "report-releaser", "unauthorized-actor"} {
		if strings.TrimSpace(fixture.Actors[role]) == "" {
			t.Fatalf("missing actor role %s", role)
		}
	}
}

func loadSyntheticFixture(t *testing.T) syntheticFixture {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "mvp_synthetic_lab.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var fixture syntheticFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return fixture
}
