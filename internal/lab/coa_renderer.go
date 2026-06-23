package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const COAArtifactFormatText = "text/psc-coa-v1"

type COAStyle string

const (
	COAStyleTindall COAStyle = "tindall"
	COAStyleCENLA   COAStyle = "cenla"
)

type COATemplate struct {
	ID         string   `json:"id"`
	Version    string   `json:"version"`
	Style      COAStyle `json:"style"`
	LabName    string   `json:"lab_name"`
	ClientName string   `json:"client_name"`
}

type COARenderInput struct {
	Template COATemplate        `json:"template"`
	Snapshot ReportDataSnapshot `json:"snapshot"`
}

type COAArtifact struct {
	Format      string `json:"format"`
	ContentHash string `json:"content_hash"`
	Content     []byte `json:"content"`
}

type COAGenerationInput struct {
	SampleID  string      `json:"sample_id"`
	Template  COATemplate `json:"template"`
	Locale    string      `json:"locale"`
	Narrative string      `json:"narrative"`
}

func RenderCOAArtifact(input COARenderInput) (COAArtifact, error) {
	input.Template = normalizeCOATemplate(input.Template)
	if err := validateCOATemplate(input.Template); err != nil {
		return COAArtifact{}, err
	}
	if strings.TrimSpace(input.Snapshot.Sample.ID) == "" {
		return COAArtifact{}, errors.New("COA sample id is required")
	}
	results := append([]Result(nil), input.Snapshot.Results...)
	sort.SliceStable(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	var b strings.Builder
	fmt.Fprintf(&b, "CERTIFICATE OF ANALYSIS\n")
	fmt.Fprintf(&b, "Template: %s version %s\n", input.Template.ID, input.Template.Version)
	fmt.Fprintf(&b, "Style: %s\n", input.Template.Style)
	fmt.Fprintf(&b, "Laboratory: %s\n", input.Template.LabName)
	fmt.Fprintf(&b, "Client: %s\n", input.Template.ClientName)
	fmt.Fprintf(&b, "Project: %s\n", emptyCOAValue(input.Snapshot.Sample.Project))
	fmt.Fprintf(&b, "Sample: %s\n", input.Snapshot.Sample.ID)
	fmt.Fprintf(&b, "Lab sample: %s\n", emptyCOAValue(input.Snapshot.Sample.LabSampleID))
	fmt.Fprintf(&b, "Client sample: %s\n", emptyCOAValue(input.Snapshot.Sample.ClientSampleID))
	fmt.Fprintf(&b, "Matrix: %s\n", emptyCOAValue(input.Snapshot.Sample.Matrix))
	fmt.Fprintf(&b, "Status: %s\n", input.Snapshot.Sample.Status)
	b.WriteString("\nRESULTS\n")
	b.WriteString("Result ID | Value | Unit | Qualifier | MDL | RL | Analyst | Reviewer\n")
	for _, result := range results {
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s | %s | %s | %s\n",
			result.ID,
			coaResultValue(result),
			emptyCOAValue(result.Unit),
			emptyCOAValue(result.Qualifier),
			coaFloat(result.MDL),
			coaFloat(result.RL),
			emptyCOAValue(result.AnalystID),
			emptyCOAValue(result.ReviewedBy),
		)
	}
	b.WriteString("\nSynthetic lab-test artifact. Not customer-facing.\n")
	content := []byte(b.String())
	return COAArtifact{Format: COAArtifactFormatText, ContentHash: hashBytes(content), Content: content}, nil
}

func (s *Store) GenerateCOAReportArtifact(input COAGenerationInput, actor ActorContext) (ReleasedReportArtifact, error) {
	return s.GenerateCOAReportArtifactForScope(defaultScope(), input, actor)
}

func (s *Store) GenerateCOAReportArtifactForScope(scope Scope, input COAGenerationInput, actor ActorContext) (ReleasedReportArtifact, error) {
	scope, err := normalizeScope(scope)
	if err != nil {
		return ReleasedReportArtifact{}, err
	}
	input.SampleID = strings.TrimSpace(input.SampleID)
	if input.SampleID == "" {
		return ReleasedReportArtifact{}, errors.New("sample id is required")
	}
	input.Template = normalizeCOATemplate(input.Template)
	if err := validateCOATemplate(input.Template); err != nil {
		return ReleasedReportArtifact{}, err
	}
	snapshot, err := s.reportDataSnapshotForCOA(scope, input.SampleID)
	if err != nil {
		return ReleasedReportArtifact{}, err
	}
	artifact, err := RenderCOAArtifact(COARenderInput{Template: input.Template, Snapshot: snapshot})
	if err != nil {
		return ReleasedReportArtifact{}, err
	}
	generationInputs := map[string]string{"renderer": "psc-coa-text-v1", "style": string(input.Template.Style), "artifact_hash": artifact.ContentHash}
	if locale := strings.TrimSpace(input.Locale); locale != "" {
		generationInputs["locale"] = locale
	}
	if narrative := strings.TrimSpace(input.Narrative); narrative != "" {
		generationInputs["narrative"] = narrative
	}
	return s.ReleaseReportArtifactForScope(scope, ReportReleaseInput{SampleID: input.SampleID, TemplateID: input.Template.ID, TemplateVersion: input.Template.Version, GenerationInputs: generationInputs, ArtifactFormat: artifact.Format, ArtifactContent: artifact.Content, ReleaseSignature: reportReleaseSignature(actor), SupersessionReason: "COA generated from template"}, actor)
}

func (s *Store) reportDataSnapshotForCOA(scope Scope, sampleID string) (ReportDataSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshot ReportDataSnapshot
	err := s.withTx(func(tx *sql.Tx) error {
		sample, err := sampleByIDForScopeTx(tx, scope, sampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown sample %q", sampleID)
			}
			return err
		}
		results, err := resultsForSampleTx(tx, scope, sample.ID)
		if err != nil {
			return err
		}
		snapshot = ReportDataSnapshot{Sample: sample, Results: results}
		return nil
	})
	if err != nil {
		return ReportDataSnapshot{}, err
	}
	return snapshot, nil
}

func normalizeCOATemplate(template COATemplate) COATemplate {
	template.ID = strings.TrimSpace(template.ID)
	template.Version = strings.TrimSpace(template.Version)
	template.Style = COAStyle(strings.ToLower(strings.TrimSpace(string(template.Style))))
	template.LabName = strings.TrimSpace(template.LabName)
	template.ClientName = strings.TrimSpace(template.ClientName)
	return template
}

func validateCOATemplate(template COATemplate) error {
	if template.ID == "" {
		return errors.New("COA template id is required")
	}
	if template.Version == "" {
		return errors.New("COA template version is required")
	}
	switch template.Style {
	case COAStyleTindall, COAStyleCENLA:
	default:
		return fmt.Errorf("unsupported COA template style %q", template.Style)
	}
	if template.LabName == "" {
		return errors.New("COA lab name is required")
	}
	if template.ClientName == "" {
		return errors.New("COA client name is required")
	}
	return nil
}

func emptyCOAValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func coaResultValue(result Result) string {
	if raw := strings.TrimSpace(result.RawValue); raw != "" {
		return raw
	}
	return coaFloat(result.Value)
}

func coaFloat(value float64) string {
	if value == 0 {
		return "-"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}
