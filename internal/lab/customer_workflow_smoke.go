package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SmokeStatus string

const (
	SmokeStatusGreen  SmokeStatus = "GREEN"
	SmokeStatusYellow SmokeStatus = "YELLOW"
	SmokeStatusRed    SmokeStatus = "RED"
)

type ReportPackageBinding string

const (
	ReportPackageBindingDataBound      ReportPackageBinding = "data-bound-to-migrated-fixture-data"
	ReportPackageBindingStaticScripted ReportPackageBinding = "static-scripted"
)

type CustomerWorkflowSmokeInput struct {
	FixturePath   string
	GapReportPath string
	OutputDir     string
	CommandOutput string
}

type CustomerWorkflowSmokeMatrix struct {
	DatasetID           string                      `json:"dataset_id"`
	DatasetVersion      string                      `json:"dataset_version"`
	SyntheticOnly       bool                        `json:"synthetic_only"`
	Boundary            string                      `json:"boundary"`
	SourceFixturePath   string                      `json:"source_fixture_path"`
	SourceGapReportPath string                      `json:"source_gap_report_path,omitempty"`
	MatrixArtifactPath  string                      `json:"matrix_artifact_path"`
	Lanes               []CustomerWorkflowSmokeLane `json:"lanes"`
	StatusCounts        map[SmokeStatus]int         `json:"status_counts"`
}

type CustomerWorkflowSmokeLane struct {
	Label          string                     `json:"label"`
	FamilyID       string                     `json:"family_id"`
	ClientLegacyID string                     `json:"client_legacy_id"`
	ClientName     string                     `json:"client_name"`
	OverallStatus  SmokeStatus                `json:"overall_status"`
	CommandOutput  string                     `json:"command_output"`
	Checks         []CustomerWorkflowCheck    `json:"checks"`
	ReportPackage  CustomerReportPackageProof `json:"report_package"`
	RemainingGaps  []string                   `json:"remaining_gaps"`
}

type CustomerWorkflowCheck struct {
	Name         string      `json:"name"`
	Status       SmokeStatus `json:"status"`
	Evidence     string      `json:"evidence"`
	ArtifactPath string      `json:"artifact_path,omitempty"`
}

type CustomerReportPackageProof struct {
	ReportID      string               `json:"report_id"`
	Outputs       []string             `json:"outputs"`
	Includes      []string             `json:"includes"`
	Binding       ReportPackageBinding `json:"binding"`
	BindingNote   string               `json:"binding_note"`
	ArtifactPath  string               `json:"artifact_path"`
	ArtifactHash  string               `json:"artifact_hash"`
	SourceFamily  string               `json:"source_family"`
	SourceFixture string               `json:"source_fixture"`
}

type customerWorkflowFixture struct {
	DatasetID       string                    `json:"dataset_id"`
	Version         string                    `json:"version"`
	SyntheticOnly   bool                      `json:"synthetic_only"`
	Boundary        string                    `json:"boundary"`
	FixtureFamilies []customerFixtureFamily   `json:"fixture_families"`
	Clients         []customerFixtureClient   `json:"clients"`
	Samples         []customerFixtureSample   `json:"samples"`
	Analyses        []customerFixtureAnalysis `json:"analyses"`
	QCBatches       []customerFixtureQCBatch  `json:"qc_batches"`
	Reports         []customerFixtureReport   `json:"reports"`
	ExpectedGaps    []customerFixtureGap      `json:"expected_parity_gaps"`
}

type customerFixtureFamily struct {
	ID       string   `json:"id"`
	Style    string   `json:"style"`
	Workflow []string `json:"workflow"`
}

type customerFixtureClient struct {
	LegacyID string `json:"legacy_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	FamilyID string `json:"family_id"`
}

type customerFixtureSample struct {
	LegacyID       string   `json:"legacy_id"`
	ClientLegacyID string   `json:"client_legacy_id"`
	FamilyID       string   `json:"family_id"`
	ClientSampleID string   `json:"client_sample_id"`
	Matrix         string   `json:"matrix"`
	Containers     []string `json:"containers"`
	Analyses       []string `json:"analyses"`
	CustodyEvents  []string `json:"custody_events"`
}

type customerFixtureAnalysis struct {
	SampleLegacyID string `json:"sample_legacy_id"`
	Service        string `json:"service"`
	Method         string `json:"method"`
	Unit           string `json:"unit"`
	Result         string `json:"result"`
	Qualifier      string `json:"qualifier"`
	QCRole         string `json:"qc_role"`
}

type customerFixtureQCBatch struct {
	ID       string   `json:"id"`
	FamilyID string   `json:"family_id"`
	Method   string   `json:"method"`
	Samples  []string `json:"samples"`
	Checks   []string `json:"checks"`
}

type customerFixtureReport struct {
	ID       string   `json:"id"`
	FamilyID string   `json:"family_id"`
	Outputs  []string `json:"outputs"`
	Includes []string `json:"includes"`
}

type customerFixtureGap struct {
	ID              string   `json:"id"`
	Severity        string   `json:"severity"`
	CurrentBehavior string   `json:"current_behavior"`
	RequiredFor     []string `json:"required_for"`
}

func (s *Store) GenerateCustomerWorkflowSmokeMatrix(input CustomerWorkflowSmokeInput, actor ActorContext) (CustomerWorkflowSmokeMatrix, error) {
	input = normalizeCustomerWorkflowSmokeInput(input)
	if err := rejectUnsafeCustomerWorkflowSmokeText(input.CommandOutput); err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	if strings.TrimSpace(input.FixturePath) == "" {
		return CustomerWorkflowSmokeMatrix{}, errors.New("fixture path is required")
	}
	if strings.TrimSpace(input.OutputDir) == "" {
		return CustomerWorkflowSmokeMatrix{}, errors.New("output directory is required")
	}
	fixture, raw, err := loadCustomerWorkflowFixture(input.FixturePath)
	if err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	if !fixture.SyntheticOnly {
		return CustomerWorkflowSmokeMatrix{}, errors.New("customer workflow smoke requires synthetic-only fixture")
	}
	if input.GapReportPath != "" {
		if _, err := os.Stat(input.GapReportPath); err != nil {
			return CustomerWorkflowSmokeMatrix{}, fmt.Errorf("gap report: %w", err)
		}
	}
	if err := os.MkdirAll(input.OutputDir, 0o755); err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}

	clientRows := make([]ImportRow, 0, len(fixture.Clients))
	for _, client := range fixture.Clients {
		clientRows = append(clientRows, ImportRow{"legacy_id": client.LegacyID, "name": client.Name, "email": client.Email, "family_id": client.FamilyID})
	}
	clientPayload, err := json.Marshal(clientRows)
	if err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	importResult, err := s.ImportForScope(DefaultScope, clientPayload, ImportOptions{Format: ImportFormatJSON, Entity: ImportEntityClients, Source: input.FixturePath}, actor)
	if err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	reconciliation, err := s.ClientImportReconciliationReportForScope(DefaultScope, importResult, actor)
	if err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	reconPath := filepath.Join(input.OutputDir, "client-import-reconciliation.json")
	if err := writeJSONArtifact(reconPath, reconciliation); err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}

	matrix := CustomerWorkflowSmokeMatrix{
		DatasetID:           fixture.DatasetID,
		DatasetVersion:      fixture.Version,
		SyntheticOnly:       fixture.SyntheticOnly,
		Boundary:            "Lab-test synthetic smoke only; not production evidence, not customer migration approval, and not customer-facing evidence.",
		SourceFixturePath:   input.FixturePath,
		SourceGapReportPath: input.GapReportPath,
		StatusCounts:        map[SmokeStatus]int{},
	}
	families := mapFamilies(fixture.FixtureFamilies)
	clientsByFamily := mapClientsByFamily(fixture.Clients)
	reportsByFamily := mapReportsByFamily(fixture.Reports)
	for _, laneSpec := range []struct{ label, familyID string }{{"Tindall/precast-industrial", "precast-industrial"}, {"CENLA/municipal-water", "municipal-water"}, {"RJ Lee/materials-forensics", "materials-forensics"}} {
		client := clientsByFamily[laneSpec.familyID]
		family := families[laneSpec.familyID]
		report := reportsByFamily[laneSpec.familyID]
		remainingGaps := remainingGapsForFamily(fixture.ExpectedGaps, laneSpec.familyID)
		packageProof, err := writeCustomerWorkflowPackageArtifact(input.OutputDir, input.FixturePath, fixture, family, client, report)
		if err != nil {
			return CustomerWorkflowSmokeMatrix{}, err
		}
		lane := CustomerWorkflowSmokeLane{
			Label:          laneSpec.label,
			FamilyID:       laneSpec.familyID,
			ClientLegacyID: client.LegacyID,
			ClientName:     client.Name,
			OverallStatus:  laneOverallStatus(laneSpec.familyID),
			CommandOutput:  input.CommandOutput,
			RemainingGaps:  remainingGaps,
			ReportPackage:  packageProof,
		}
		lane.Checks = []CustomerWorkflowCheck{
			{Name: "synthetic client migration reconciliation", Status: SmokeStatusGreen, Evidence: fmt.Sprintf("imported %d/%d synthetic clients; matched=%d missing=%d mismatched=%d", importResult.CreatedRows, importResult.TotalRows, reconciliation.MatchedCount, len(reconciliation.MissingRecords), len(reconciliation.MismatchedRecords)), ArtifactPath: reconPath},
			{Name: "fixture workflow shape", Status: SmokeStatusGreen, Evidence: fmt.Sprintf("family %s has %d workflow steps, %d samples, %d analyses", laneSpec.familyID, len(family.Workflow), countSamples(fixture.Samples, laneSpec.familyID), countAnalysesForFamily(fixture.Samples, fixture.Analyses, laneSpec.familyID)), ArtifactPath: input.FixturePath},
			{Name: "report package proof", Status: SmokeStatusYellow, Evidence: "package artifact is explicitly STATIC/SCRIPTED because sample/result/QC/custody imports are still documented gaps", ArtifactPath: packageProof.ArtifactPath},
			{Name: "remaining customer workflow blockers", Status: SmokeStatusRed, Evidence: strings.Join(remainingGaps, "; "), ArtifactPath: input.GapReportPath},
		}
		for _, check := range lane.Checks {
			matrix.StatusCounts[check.Status]++
		}
		matrix.Lanes = append(matrix.Lanes, lane)
	}
	matrixPath := filepath.Join(input.OutputDir, "customer-workflow-smoke-matrix.json")
	matrix.MatrixArtifactPath = matrixPath
	if err := writeJSONArtifact(matrixPath, matrix); err != nil {
		return CustomerWorkflowSmokeMatrix{}, err
	}
	_ = raw
	return matrix, nil
}

func normalizeCustomerWorkflowSmokeInput(input CustomerWorkflowSmokeInput) CustomerWorkflowSmokeInput {
	input.FixturePath = strings.TrimSpace(input.FixturePath)
	input.GapReportPath = strings.TrimSpace(input.GapReportPath)
	input.OutputDir = strings.TrimSpace(input.OutputDir)
	input.CommandOutput = strings.TrimSpace(input.CommandOutput)
	if input.CommandOutput == "" {
		input.CommandOutput = "not run; caller did not provide command output"
	}
	return input
}

func rejectUnsafeCustomerWorkflowSmokeText(text string) error {
	lower := strings.ToLower(text)
	for _, phrase := range []string{"production-ready", "customer pilot approved", "migration approved", "customer-facing claim"} {
		if strings.Contains(lower, phrase) {
			return fmt.Errorf("customer workflow smoke output contains unsafe readiness language %q", phrase)
		}
	}
	return nil
}

func loadCustomerWorkflowFixture(path string) (customerWorkflowFixture, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return customerWorkflowFixture{}, nil, err
	}
	var fixture customerWorkflowFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return customerWorkflowFixture{}, nil, err
	}
	return fixture, raw, nil
}

func writeJSONArtifact(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func writeCustomerWorkflowPackageArtifact(outDir, fixturePath string, fixture customerWorkflowFixture, family customerFixtureFamily, client customerFixtureClient, report customerFixtureReport) (CustomerReportPackageProof, error) {
	filename := sanitizeArtifactName(family.ID + "-report-package-proof.txt")
	path := filepath.Join(outDir, filename)
	samples := filterSamples(fixture.Samples, family.ID)
	analyses := filterAnalysesForSamples(samples, fixture.Analyses)
	qc := filterQCBatches(fixture.QCBatches, family.ID)
	var b strings.Builder
	b.WriteString("STATIC/SCRIPTED report package proof\n")
	b.WriteString("Boundary: lab-test synthetic only; not production evidence; not customer-facing evidence.\n")
	b.WriteString("Source fixture: " + fixturePath + "\n")
	b.WriteString("Dataset: " + fixture.DatasetID + " version " + fixture.Version + "\n")
	b.WriteString("Family: " + family.ID + "\n")
	b.WriteString("Client: " + client.Name + " (" + client.LegacyID + ")\n")
	b.WriteString("Report: " + report.ID + "\n")
	b.WriteString("Outputs: " + strings.Join(report.Outputs, ", ") + "\n")
	b.WriteString("Includes: " + strings.Join(report.Includes, ", ") + "\n")
	b.WriteString("Samples:\n")
	for _, sample := range samples {
		b.WriteString("- " + sample.LegacyID + " / " + sample.ClientSampleID + " / " + sample.Matrix + " / containers=" + strings.Join(sample.Containers, " | ") + "\n")
	}
	b.WriteString("Analyses:\n")
	for _, analysis := range analyses {
		b.WriteString("- " + analysis.SampleLegacyID + " / " + analysis.Service + " / " + analysis.Method + " / " + analysis.Result + " " + analysis.Unit + " / qualifier=" + analysis.Qualifier + "\n")
	}
	b.WriteString("QC checks:\n")
	for _, batch := range qc {
		b.WriteString("- " + batch.ID + " / " + strings.Join(batch.Checks, " | ") + "\n")
	}
	content := []byte(b.String())
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return CustomerReportPackageProof{}, err
	}
	return CustomerReportPackageProof{ReportID: report.ID, Outputs: append([]string(nil), report.Outputs...), Includes: append([]string(nil), report.Includes...), Binding: ReportPackageBindingStaticScripted, BindingNote: "Report package text is scripted from the synthetic golden fixture and is not a migrated sample/result/QC/custody report artifact.", ArtifactPath: path, ArtifactHash: hashBytes(content), SourceFamily: family.ID, SourceFixture: fixturePath}, nil
}

func mapFamilies(families []customerFixtureFamily) map[string]customerFixtureFamily {
	out := map[string]customerFixtureFamily{}
	for _, family := range families {
		out[family.ID] = family
	}
	return out
}

func mapClientsByFamily(clients []customerFixtureClient) map[string]customerFixtureClient {
	out := map[string]customerFixtureClient{}
	for _, client := range clients {
		out[client.FamilyID] = client
	}
	return out
}

func mapReportsByFamily(reports []customerFixtureReport) map[string]customerFixtureReport {
	out := map[string]customerFixtureReport{}
	for _, report := range reports {
		out[report.FamilyID] = report
	}
	return out
}

func remainingGapsForFamily(gaps []customerFixtureGap, familyID string) []string {
	out := []string{}
	for _, gap := range gaps {
		for _, required := range gap.RequiredFor {
			if required == familyID {
				out = append(out, gap.ID+" ("+gap.Severity+"): "+gap.CurrentBehavior)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func laneOverallStatus(familyID string) SmokeStatus {
	if familyID == "materials-forensics" {
		return SmokeStatusYellow
	}
	return SmokeStatusRed
}

func countSamples(samples []customerFixtureSample, familyID string) int {
	return len(filterSamples(samples, familyID))
}

func countAnalysesForFamily(samples []customerFixtureSample, analyses []customerFixtureAnalysis, familyID string) int {
	return len(filterAnalysesForSamples(filterSamples(samples, familyID), analyses))
}

func filterSamples(samples []customerFixtureSample, familyID string) []customerFixtureSample {
	out := []customerFixtureSample{}
	for _, sample := range samples {
		if sample.FamilyID == familyID {
			out = append(out, sample)
		}
	}
	return out
}

func filterAnalysesForSamples(samples []customerFixtureSample, analyses []customerFixtureAnalysis) []customerFixtureAnalysis {
	ids := map[string]bool{}
	for _, sample := range samples {
		ids[sample.LegacyID] = true
	}
	out := []customerFixtureAnalysis{}
	for _, analysis := range analyses {
		if ids[analysis.SampleLegacyID] {
			out = append(out, analysis)
		}
	}
	return out
}

func filterQCBatches(batches []customerFixtureQCBatch, familyID string) []customerFixtureQCBatch {
	out := []customerFixtureQCBatch{}
	for _, batch := range batches {
		if batch.FamilyID == familyID {
			out = append(out, batch)
		}
	}
	return out
}

func sanitizeArtifactName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
