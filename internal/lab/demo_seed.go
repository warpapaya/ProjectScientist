package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type SyntheticDemoSeedSummary struct {
	FixtureID            string `json:"fixture_id"`
	ClientID             string `json:"client_id"`
	ClientName           string `json:"client_name"`
	SampleID             string `json:"sample_id"`
	Project              string `json:"project"`
	Matrix               string `json:"matrix"`
	AnalysisCount        int    `json:"analysis_count"`
	SampleReferenceCount int    `json:"sample_reference_count"`
}

type syntheticDemoFixture struct {
	FixtureID     string `json:"fixture_id"`
	SyntheticOnly bool   `json:"synthetic_only"`
	Boundary      string `json:"boundary"`
	Client        struct {
		Name    string `json:"name"`
		Contact struct {
			Email string `json:"email"`
		} `json:"contact"`
	} `json:"client"`
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	Sample struct {
		Matrix struct {
			Name string `json:"name"`
		} `json:"matrix"`
	} `json:"sample"`
	AnalysisProfile struct {
		Analytes []struct {
			Name string `json:"name"`
		} `json:"analytes"`
	} `json:"analysis_profile"`
}

func (s *Store) ResetAndSeedSyntheticDemo(fixturePath string, actor ActorContext) (SyntheticDemoSeedSummary, error) {
	fixture, err := loadSyntheticDemoFixture(fixturePath)
	if err != nil {
		return SyntheticDemoSeedSummary{}, err
	}
	if err := s.AuthorizeOperationForScope(defaultScope(), OperationAdminConfigure, actor, AuditResource{Type: "synthetic-demo", ID: "reset"}, map[string]any{"fixture_id": fixture.FixtureID}); err != nil {
		return SyntheticDemoSeedSummary{}, err
	}
	if err := s.resetLocalDemoStore(); err != nil {
		return SyntheticDemoSeedSummary{}, err
	}

	client, err := s.CreateClient(fixture.Client.Name, fixture.Client.Contact.Email, actor)
	if err != nil {
		return SyntheticDemoSeedSummary{}, fmt.Errorf("seed demo client: %w", err)
	}
	analysisNames := make([]string, 0, len(fixture.AnalysisProfile.Analytes))
	for _, analyte := range fixture.AnalysisProfile.Analytes {
		if name := strings.TrimSpace(analyte.Name); name != "" {
			analysisNames = append(analysisNames, name)
		}
	}
	sample, err := s.CreateSample(CreateSampleInput{ClientID: client.ID, Project: fixture.Project.Name, Matrix: fixture.Sample.Matrix.Name, Tests: analysisNames}, actor)
	if err != nil {
		return SyntheticDemoSeedSummary{}, fmt.Errorf("seed demo sample: %w", err)
	}
	referenceSummary, err := s.SeedDemoSampleReferenceData(actor)
	if err != nil {
		return SyntheticDemoSeedSummary{}, fmt.Errorf("seed demo sample reference data: %w", err)
	}
	return SyntheticDemoSeedSummary{FixtureID: fixture.FixtureID, ClientID: client.ID, ClientName: client.Name, SampleID: sample.ID, Project: sample.Project, Matrix: sample.Matrix, AnalysisCount: len(sample.Analyses), SampleReferenceCount: referenceSummary.TotalCount}, nil
}

func loadSyntheticDemoFixture(path string) (syntheticDemoFixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return syntheticDemoFixture{}, fmt.Errorf("read synthetic fixture %s: %w", path, err)
	}
	var fixture syntheticDemoFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return syntheticDemoFixture{}, fmt.Errorf("parse synthetic fixture %s: %w", path, err)
	}
	if !fixture.SyntheticOnly || !strings.Contains(strings.ToLower(fixture.Boundary), "lab-test") {
		return syntheticDemoFixture{}, errors.New("synthetic fixture must be lab-test only")
	}
	if strings.TrimSpace(fixture.FixtureID) == "" {
		return syntheticDemoFixture{}, errors.New("synthetic fixture id is required")
	}
	if strings.TrimSpace(fixture.Client.Name) == "" || strings.TrimSpace(fixture.Project.Name) == "" || strings.TrimSpace(fixture.Sample.Matrix.Name) == "" {
		return syntheticDemoFixture{}, errors.New("synthetic fixture requires client, project, and matrix names")
	}
	if len(fixture.AnalysisProfile.Analytes) == 0 {
		return syntheticDemoFixture{}, errors.New("synthetic fixture requires at least one analyte")
	}
	return fixture, nil
}

func (s *Store) resetLocalDemoStore() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`DELETE FROM audit_checkpoints`,
			`DELETE FROM audit_events`,
			`DELETE FROM worksheet_lines`,
			`DELETE FROM worksheets`,
			`DELETE FROM results`,
			`DELETE FROM analysis_request_lines`,
			`DELETE FROM samples`,
			`DELETE FROM clients`,
			`DELETE FROM sample_reference_items`,
			`UPDATE store_meta SET value = '1' WHERE key IN ('next_client', 'next_sample', 'next_worksheet', 'next_analysis_request_line', 'next_result', 'next_audit', 'next_sample_reference')`,
			`UPDATE store_meta SET value = '' WHERE key = 'last_hash'`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}
