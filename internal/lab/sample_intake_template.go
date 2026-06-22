package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SampleIntakeTemplate struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	LabID               string         `json:"lab_id"`
	Name                string         `json:"name"`
	ClientID            string         `json:"client_id"`
	ProjectID           string         `json:"project_id,omitempty"`
	Project             string         `json:"project,omitempty"`
	Matrix              string         `json:"matrix,omitempty"`
	MatrixReferenceID   string         `json:"matrix_reference_id,omitempty"`
	ContainerID         string         `json:"container_id,omitempty"`
	PreservativeID      string         `json:"preservative_id,omitempty"`
	StorageLocationID   string         `json:"storage_location_id,omitempty"`
	ReceivedConditionID string         `json:"received_condition_id,omitempty"`
	Priority            SamplePriority `json:"priority"`
	AnalysisProfileIDs  []string       `json:"analysis_profile_ids,omitempty"`
	AnalysisServiceIDs  []string       `json:"analysis_service_ids,omitempty"`
	Tests               []string       `json:"tests,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type SampleIntakeTemplateInput struct {
	Name                string         `json:"name"`
	ClientID            string         `json:"client_id"`
	ProjectID           string         `json:"project_id"`
	Project             string         `json:"project"`
	Matrix              string         `json:"matrix"`
	MatrixReferenceID   string         `json:"matrix_reference_id"`
	ContainerID         string         `json:"container_id"`
	PreservativeID      string         `json:"preservative_id"`
	StorageLocationID   string         `json:"storage_location_id"`
	ReceivedConditionID string         `json:"received_condition_id"`
	Priority            SamplePriority `json:"priority"`
	AnalysisProfileIDs  []string       `json:"analysis_profile_ids"`
	AnalysisServiceIDs  []string       `json:"analysis_service_ids"`
	Tests               []string       `json:"tests"`
}

type SampleTemplateRowInput struct {
	ClientSampleID      string         `json:"client_sample_id"`
	LabSampleID         string         `json:"lab_sample_id"`
	ProjectID           string         `json:"project_id"`
	Project             string         `json:"project"`
	Matrix              string         `json:"matrix"`
	MatrixReferenceID   string         `json:"matrix_reference_id"`
	ContainerID         string         `json:"container_id"`
	PreservativeID      string         `json:"preservative_id"`
	StorageLocationID   string         `json:"storage_location_id"`
	ReceivedConditionID string         `json:"received_condition_id"`
	SampledAt           time.Time      `json:"sampled_at"`
	ReceivedAt          time.Time      `json:"received_at"`
	Priority            SamplePriority `json:"priority"`
	Comments            string         `json:"comments"`
	AnalysisProfileIDs  []string       `json:"analysis_profile_ids"`
	AnalysisServiceIDs  []string       `json:"analysis_service_ids"`
	Tests               []string       `json:"tests"`
}

func (s *Store) CreateSampleIntakeTemplate(input SampleIntakeTemplateInput, actor ActorContext) (SampleIntakeTemplate, error) {
	return s.CreateSampleIntakeTemplateForScope(defaultScope(), input, actor)
}

func (s *Store) CreateSampleIntakeTemplateForScope(scope Scope, input SampleIntakeTemplateInput, actor ActorContext) (SampleIntakeTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return SampleIntakeTemplate{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return SampleIntakeTemplate{}, errors.New("sample intake template name is required")
	}
	input.ClientID = strings.TrimSpace(input.ClientID)
	if input.ClientID == "" {
		return SampleIntakeTemplate{}, errors.New("client id is required")
	}
	now := time.Now().UTC()
	var tmpl SampleIntakeTemplate
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationSampleIntake, actor, AuditResource{Type: "sample_intake_template", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		var clientTenant, clientLab string
		if err := tx.QueryRow(`SELECT tenant_id, lab_id FROM clients WHERE id = ?`, input.ClientID).Scan(&clientTenant, &clientLab); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown client %q", input.ClientID)
			}
			return err
		}
		if clientTenant != scope.TenantID || clientLab != scope.LabID {
			return fmt.Errorf("client %q is outside requested tenant/lab scope", input.ClientID)
		}
		resolved, err := resolveSampleIntakeTx(tx, scope, CreateSampleInput{ClientID: input.ClientID, ProjectID: input.ProjectID, Project: input.Project, Matrix: input.Matrix, MatrixReferenceID: input.MatrixReferenceID, ContainerID: input.ContainerID, PreservativeID: input.PreservativeID, StorageLocationID: input.StorageLocationID, ReceivedConditionID: input.ReceivedConditionID, AnalysisProfileIDs: input.AnalysisProfileIDs, AnalysisServiceIDs: input.AnalysisServiceIDs, Tests: input.Tests})
		if err != nil {
			return err
		}
		if _, err := buildAnalysesTx(tx, scope, "TEMPLATE-CHECK", CreateSampleInput{AnalysisProfileIDs: input.AnalysisProfileIDs, AnalysisServiceIDs: input.AnalysisServiceIDs}, resolved.Tests, CatalogSnapshot{}, false); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_sample_intake_template")
		if err != nil {
			return err
		}
		profileIDs := cleanStrings(input.AnalysisProfileIDs)
		serviceIDs := cleanStrings(input.AnalysisServiceIDs)
		tests := cleanStrings(resolved.Tests)
		profileJSON, _ := json.Marshal(profileIDs)
		serviceJSON, _ := json.Marshal(serviceIDs)
		testsJSON, _ := json.Marshal(tests)
		tmpl = SampleIntakeTemplate{ID: fmt.Sprintf("SIT-%05d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: name, ClientID: input.ClientID, ProjectID: resolved.ProjectID, Project: resolved.Project, Matrix: resolved.Matrix, MatrixReferenceID: resolved.MatrixReferenceID, ContainerID: resolved.ContainerID, PreservativeID: resolved.PreservativeID, StorageLocationID: resolved.StorageLocationID, ReceivedConditionID: resolved.ReceivedConditionID, Priority: normalizePriority(input.Priority), AnalysisProfileIDs: profileIDs, AnalysisServiceIDs: serviceIDs, Tests: tests, CreatedAt: now, UpdatedAt: now}
		_, err = tx.Exec(`INSERT INTO sample_intake_templates(id, tenant_id, lab_id, name, client_id, project_id, project, matrix, matrix_reference_id, container_id, preservative_id, storage_location_id, received_condition_id, priority, analysis_profile_ids_json, analysis_service_ids_json, tests_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, tmpl.ID, tmpl.TenantID, tmpl.LabID, tmpl.Name, tmpl.ClientID, tmpl.ProjectID, tmpl.Project, tmpl.Matrix, tmpl.MatrixReferenceID, tmpl.ContainerID, tmpl.PreservativeID, tmpl.StorageLocationID, tmpl.ReceivedConditionID, string(tmpl.Priority), string(profileJSON), string(serviceJSON), string(testsJSON), formatTime(tmpl.CreatedAt), formatTime(tmpl.UpdatedAt))
		if err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample_intake_template.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample_intake_template", ID: tmpl.ID}, Details: map[string]any{"client_id": tmpl.ClientID, "project_id": tmpl.ProjectID, "analysis_profile_count": len(tmpl.AnalysisProfileIDs), "analysis_service_count": len(tmpl.AnalysisServiceIDs)}})
	})
	if err != nil {
		return SampleIntakeTemplate{}, err
	}
	if deniedErr != nil {
		return SampleIntakeTemplate{}, deniedErr
	}
	return tmpl, nil
}

func (s *Store) CreateSamplesFromTemplate(templateID string, rows []SampleTemplateRowInput, actor ActorContext) ([]Sample, error) {
	return s.CreateSamplesFromTemplateForScope(defaultScope(), templateID, rows, actor)
}

func (s *Store) CreateSamplesFromTemplateForScope(scope Scope, templateID string, rows []SampleTemplateRowInput, actor ActorContext) ([]Sample, error) {
	tmpl, ok := s.GetSampleIntakeTemplateForScope(scope, templateID)
	if !ok {
		return nil, fmt.Errorf("unknown sample intake template %q", strings.TrimSpace(templateID))
	}
	if len(rows) == 0 {
		return nil, errors.New("at least one sample row is required")
	}
	created := make([]Sample, 0, len(rows))
	for _, row := range rows {
		input := sampleInputFromTemplate(tmpl, row)
		sample, err := s.CreateSampleForScope(scope, input, actor)
		if err != nil {
			return created, err
		}
		created = append(created, sample)
	}
	return created, nil
}

func (s *Store) GetSampleIntakeTemplateForScope(scope Scope, id string) (SampleIntakeTemplate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return SampleIntakeTemplate{}, false
	}
	tmpl, err := sampleIntakeTemplateByID(s.db, scope, strings.TrimSpace(id))
	return tmpl, err == nil
}

func sampleInputFromTemplate(tmpl SampleIntakeTemplate, row SampleTemplateRowInput) CreateSampleInput {
	input := CreateSampleInput{ClientID: tmpl.ClientID, ProjectID: tmpl.ProjectID, Project: tmpl.Project, Matrix: tmpl.Matrix, MatrixReferenceID: tmpl.MatrixReferenceID, ContainerID: tmpl.ContainerID, PreservativeID: tmpl.PreservativeID, StorageLocationID: tmpl.StorageLocationID, ReceivedConditionID: tmpl.ReceivedConditionID, Priority: tmpl.Priority, AnalysisProfileIDs: append([]string(nil), tmpl.AnalysisProfileIDs...), AnalysisServiceIDs: append([]string(nil), tmpl.AnalysisServiceIDs...), Tests: append([]string(nil), tmpl.Tests...)}
	input.ClientSampleID = strings.TrimSpace(row.ClientSampleID)
	input.LabSampleID = strings.TrimSpace(row.LabSampleID)
	input.SampledAt = row.SampledAt
	input.ReceivedAt = row.ReceivedAt
	input.Comments = strings.TrimSpace(row.Comments)
	if strings.TrimSpace(row.ProjectID) != "" {
		input.ProjectID = strings.TrimSpace(row.ProjectID)
	}
	if strings.TrimSpace(row.Project) != "" {
		input.Project = strings.TrimSpace(row.Project)
	}
	if strings.TrimSpace(row.Matrix) != "" {
		input.Matrix = strings.TrimSpace(row.Matrix)
		input.MatrixReferenceID = ""
	}
	if strings.TrimSpace(row.MatrixReferenceID) != "" {
		input.MatrixReferenceID = strings.TrimSpace(row.MatrixReferenceID)
	}
	if strings.TrimSpace(row.ContainerID) != "" {
		input.ContainerID = strings.TrimSpace(row.ContainerID)
	}
	if strings.TrimSpace(row.PreservativeID) != "" {
		input.PreservativeID = strings.TrimSpace(row.PreservativeID)
	}
	if strings.TrimSpace(row.StorageLocationID) != "" {
		input.StorageLocationID = strings.TrimSpace(row.StorageLocationID)
	}
	if strings.TrimSpace(row.ReceivedConditionID) != "" {
		input.ReceivedConditionID = strings.TrimSpace(row.ReceivedConditionID)
	}
	if strings.TrimSpace(string(row.Priority)) != "" {
		input.Priority = row.Priority
	}
	if len(row.AnalysisProfileIDs) > 0 {
		input.AnalysisProfileIDs = cleanStrings(row.AnalysisProfileIDs)
	}
	if len(row.AnalysisServiceIDs) > 0 {
		input.AnalysisServiceIDs = cleanStrings(row.AnalysisServiceIDs)
	}
	if len(row.Tests) > 0 {
		input.Tests = cleanStrings(row.Tests)
		input.AnalysisProfileIDs = nil
		input.AnalysisServiceIDs = nil
	}
	return input
}

func sampleIntakeTemplateByID(db *sql.DB, scope Scope, id string) (SampleIntakeTemplate, error) {
	var tmpl SampleIntakeTemplate
	var priority, profileJSON, serviceJSON, testsJSON, created, updated string
	err := db.QueryRow(`SELECT id, tenant_id, lab_id, name, client_id, project_id, project, matrix, matrix_reference_id, container_id, preservative_id, storage_location_id, received_condition_id, priority, analysis_profile_ids_json, analysis_service_ids_json, tests_json, created_at, updated_at FROM sample_intake_templates WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id).Scan(&tmpl.ID, &tmpl.TenantID, &tmpl.LabID, &tmpl.Name, &tmpl.ClientID, &tmpl.ProjectID, &tmpl.Project, &tmpl.Matrix, &tmpl.MatrixReferenceID, &tmpl.ContainerID, &tmpl.PreservativeID, &tmpl.StorageLocationID, &tmpl.ReceivedConditionID, &priority, &profileJSON, &serviceJSON, &testsJSON, &created, &updated)
	if err != nil {
		return SampleIntakeTemplate{}, err
	}
	tmpl.Priority = normalizePriority(SamplePriority(priority))
	_ = jsonUnmarshalStrings(profileJSON, &tmpl.AnalysisProfileIDs)
	_ = jsonUnmarshalStrings(serviceJSON, &tmpl.AnalysisServiceIDs)
	_ = jsonUnmarshalStrings(testsJSON, &tmpl.Tests)
	tmpl.CreatedAt, _ = parseTime(created)
	tmpl.UpdatedAt, _ = parseTime(updated)
	return tmpl, nil
}
