package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type resolvedSampleIntake struct {
	ProjectID           string
	Project             string
	Matrix              string
	MatrixReferenceID   string
	ContainerID         string
	PreservativeID      string
	StorageLocationID   string
	ReceivedConditionID string
	Tests               []string
}

func resolveSampleIntakeTx(tx *sql.Tx, scope Scope, input CreateSampleInput) (resolvedSampleIntake, error) {
	input.ClientID = strings.TrimSpace(input.ClientID)
	resolved := resolvedSampleIntake{
		ProjectID:           strings.TrimSpace(input.ProjectID),
		Project:             strings.TrimSpace(input.Project),
		Matrix:              strings.TrimSpace(input.Matrix),
		MatrixReferenceID:   strings.TrimSpace(input.MatrixReferenceID),
		ContainerID:         strings.TrimSpace(input.ContainerID),
		PreservativeID:      strings.TrimSpace(input.PreservativeID),
		StorageLocationID:   strings.TrimSpace(input.StorageLocationID),
		ReceivedConditionID: strings.TrimSpace(input.ReceivedConditionID),
		Tests:               cleanStrings(input.Tests),
	}

	if resolved.ProjectID != "" {
		var project Project
		var testsJSON string
		err := tx.QueryRow(`SELECT id, name, default_matrix, default_tests_json FROM projects WHERE tenant_id = ? AND lab_id = ? AND id = ? AND client_id = ?`, scope.TenantID, scope.LabID, resolved.ProjectID, input.ClientID).Scan(&project.ID, &project.Name, &project.DefaultMatrix, &testsJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return resolvedSampleIntake{}, fmt.Errorf("unknown project %q", resolved.ProjectID)
		}
		if err != nil {
			return resolvedSampleIntake{}, err
		}
		if resolved.Project == "" {
			resolved.Project = project.Name
		}
		if resolved.Matrix == "" && resolved.MatrixReferenceID == "" {
			resolved.Matrix = project.DefaultMatrix
		}
		if len(resolved.Tests) == 0 && len(input.AnalysisProfileIDs) == 0 && len(input.AnalysisServiceIDs) == 0 {
			_ = jsonUnmarshalStrings(testsJSON, &resolved.Tests)
		}
	}

	if defaults, ok, err := clientDefaultsTx(tx, scope, input.ClientID); err != nil {
		return resolvedSampleIntake{}, err
	} else if ok {
		if resolved.Matrix == "" && resolved.MatrixReferenceID == "" {
			resolved.Matrix = defaults.DefaultMatrix
		}
		if len(resolved.Tests) == 0 && len(input.AnalysisProfileIDs) == 0 && len(input.AnalysisServiceIDs) == 0 {
			resolved.Tests = defaults.DefaultTests
		}
	}

	var err error
	if resolved.MatrixReferenceID != "" {
		resolved.Matrix, err = requireSampleReferenceTx(tx, scope, resolved.MatrixReferenceID, SampleReferenceMatrix)
		if err != nil {
			return resolvedSampleIntake{}, err
		}
	}
	if resolved.ContainerID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, resolved.ContainerID, SampleReferenceContainer); err != nil {
			return resolvedSampleIntake{}, err
		}
	}
	if resolved.PreservativeID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, resolved.PreservativeID, SampleReferencePreservative); err != nil {
			return resolvedSampleIntake{}, err
		}
	}
	if resolved.StorageLocationID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, resolved.StorageLocationID, SampleReferenceStorageLocation); err != nil {
			return resolvedSampleIntake{}, err
		}
	}
	if resolved.ReceivedConditionID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, resolved.ReceivedConditionID, SampleReferenceReceivedCondition); err != nil {
			return resolvedSampleIntake{}, err
		}
	}
	if resolved.Project == "" {
		return resolvedSampleIntake{}, errors.New("project is required")
	}
	return resolved, nil
}

func clientDefaultsTx(tx *sql.Tx, scope Scope, clientID string) (ClientDefaults, bool, error) {
	var defaults ClientDefaults
	var testsJSON, created, updated string
	err := tx.QueryRow(`SELECT tenant_id, lab_id, client_id, report_template, invoice_email, default_matrix, default_tests_json, created_at, updated_at FROM client_defaults WHERE tenant_id = ? AND lab_id = ? AND client_id = ?`, scope.TenantID, scope.LabID, clientID).Scan(&defaults.TenantID, &defaults.LabID, &defaults.ClientID, &defaults.ReportTemplate, &defaults.InvoiceEmail, &defaults.DefaultMatrix, &testsJSON, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ClientDefaults{}, false, nil
	}
	if err != nil {
		return ClientDefaults{}, false, err
	}
	_ = jsonUnmarshalStrings(testsJSON, &defaults.DefaultTests)
	defaults.CreatedAt, _ = parseTime(created)
	defaults.UpdatedAt, _ = parseTime(updated)
	return defaults, true, nil
}

func requireSampleReferenceTx(tx *sql.Tx, scope Scope, id string, kind SampleReferenceKind) (string, error) {
	var name string
	var active int
	err := tx.QueryRow(`SELECT name, active FROM sample_reference_items WHERE tenant_id = ? AND lab_id = ? AND id = ? AND kind = ?`, scope.TenantID, scope.LabID, id, string(kind)).Scan(&name, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("unknown %s reference %q", kind, id)
	}
	if err != nil {
		return "", err
	}
	if active != 1 {
		return "", fmt.Errorf("inactive %s reference %q", kind, id)
	}
	return name, nil
}

func buildAnalysesTx(tx *sql.Tx, scope Scope, sampleID string, input CreateSampleInput, fallbackTests []string, snapshot CatalogSnapshot, hasSnapshot bool) ([]Analysis, error) {
	analyses := []Analysis{}
	seenServices := map[string]int{}
	appendService := func(service AnalysisService, profileID string) {
		if idx, ok := seenServices[service.ID]; ok {
			if analyses[idx].ProfileID == "" && profileID != "" {
				analyses[idx].ProfileID = profileID
			}
			return
		}
		analysis := Analysis{ID: fmt.Sprintf("%s-A%02d", sampleID, len(analyses)+1), TenantID: scope.TenantID, LabID: scope.LabID, Name: service.Name, ServiceID: service.ID, ProfileID: profileID, DepartmentID: service.DepartmentID, DepartmentName: service.DepartmentName, MethodID: service.MethodID, MethodName: service.MethodName, Method: service.MethodName, Units: service.UnitSymbol}
		if hasSnapshot {
			analysis.CatalogSnapshotID = snapshot.ID
			analysis.CatalogSnapshotVersion = snapshot.Version
		}
		seenServices[service.ID] = len(analyses)
		analyses = append(analyses, analysis)
	}

	for _, profileID := range cleanStrings(input.AnalysisProfileIDs) {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM analysis_profiles WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, profileID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown analysis profile %q", profileID)
		} else if err != nil {
			return nil, err
		}
		services, err := profileServicesTx(tx, scope, profileID)
		if err != nil {
			return nil, err
		}
		if len(services) == 0 {
			return nil, fmt.Errorf("analysis profile %q has no services", profileID)
		}
		for _, service := range services {
			appendService(service, profileID)
		}
	}
	for _, serviceID := range cleanStrings(input.AnalysisServiceIDs) {
		service, err := analysisServiceByIDTx(tx, scope, serviceID)
		if err != nil {
			return nil, err
		}
		appendService(service, "")
	}
	if len(analyses) == 0 {
		for _, test := range cleanStrings(fallbackTests) {
			analysis := Analysis{ID: fmt.Sprintf("%s-A%02d", sampleID, len(analyses)+1), TenantID: scope.TenantID, LabID: scope.LabID, Name: test}
			if hasSnapshot {
				analysis.CatalogSnapshotID = snapshot.ID
				analysis.CatalogSnapshotVersion = snapshot.Version
			}
			analyses = append(analyses, analysis)
		}
	}
	if len(analyses) == 0 {
		return nil, errors.New("at least one analysis is required")
	}
	return analyses, nil
}

func normalizePriority(priority SamplePriority) SamplePriority {
	switch SamplePriority(strings.ToLower(strings.TrimSpace(string(priority)))) {
	case PriorityRush:
		return PriorityRush
	default:
		return PriorityRoutine
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return formatTime(value.UTC())
}

func parseOptionalTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, _ := parseTime(raw)
	return parsed
}

func jsonUnmarshalStrings(raw string, dest *[]string) error {
	if strings.TrimSpace(raw) == "" {
		*dest = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), dest)
}
