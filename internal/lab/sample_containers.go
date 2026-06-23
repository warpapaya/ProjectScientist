package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func buildSampleContainersTx(tx *sql.Tx, scope Scope, sampleID string, inputs []SampleContainerInput) ([]SampleContainer, error) {
	containers := make([]SampleContainer, 0, len(inputs))
	for i, input := range inputs {
		container, err := buildSampleContainerTx(tx, scope, sampleID, i, input)
		if err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func buildSampleContainerTx(tx *sql.Tx, scope Scope, sampleID string, index int, input SampleContainerInput) (SampleContainer, error) {
	input.ContainerReferenceID = strings.TrimSpace(input.ContainerReferenceID)
	input.PreservativeID = strings.TrimSpace(input.PreservativeID)
	input.ReceivedConditionID = strings.TrimSpace(input.ReceivedConditionID)
	if input.ContainerReferenceID == "" {
		return SampleContainer{}, errors.New("container reference id is required")
	}
	if _, err := requireSampleReferenceTx(tx, scope, input.ContainerReferenceID, SampleReferenceContainer); err != nil {
		return SampleContainer{}, err
	}
	if input.PreservativeID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, input.PreservativeID, SampleReferencePreservative); err != nil {
			return SampleContainer{}, err
		}
	}
	if input.ReceivedConditionID != "" {
		if _, err := requireSampleReferenceTx(tx, scope, input.ReceivedConditionID, SampleReferenceReceivedCondition); err != nil {
			return SampleContainer{}, err
		}
	}

	containerID := fmt.Sprintf("%s-C%02d", sampleID, index+1)
	container := SampleContainer{
		ID:                   containerID,
		SampleID:             sampleID,
		ContainerReferenceID: input.ContainerReferenceID,
		PreservativeID:       input.PreservativeID,
		ReceivedConditionID:  input.ReceivedConditionID,
		Volume:               strings.TrimSpace(input.Volume),
		Condition:            strings.TrimSpace(input.Condition),
		AliquotInstructions:  strings.TrimSpace(input.AliquotInstructions),
		Aliquots:             make([]SampleAliquot, 0, len(input.Aliquots)),
	}
	for aliquotIndex, aliquotInput := range input.Aliquots {
		aliquot, err := buildSampleAliquotTx(tx, scope, containerID, aliquotIndex, aliquotInput)
		if err != nil {
			return SampleContainer{}, err
		}
		container.Aliquots = append(container.Aliquots, aliquot)
	}
	return container, nil
}

func buildSampleAliquotTx(tx *sql.Tx, scope Scope, containerID string, index int, input SampleAliquotInput) (SampleAliquot, error) {
	aliquot := SampleAliquot{
		ID:           fmt.Sprintf("%s-Q%02d", containerID, index+1),
		ContainerID:  containerID,
		DepartmentID: strings.TrimSpace(input.DepartmentID),
		MethodID:     strings.TrimSpace(input.MethodID),
		Volume:       strings.TrimSpace(input.Volume),
		Purpose:      strings.TrimSpace(input.Purpose),
	}
	if aliquot.DepartmentID != "" {
		name, err := catalogDepartmentNameTx(tx, scope, aliquot.DepartmentID)
		if err != nil {
			return SampleAliquot{}, err
		}
		aliquot.DepartmentName = name
	}
	if aliquot.MethodID != "" {
		name, err := catalogMethodNameTx(tx, scope, aliquot.MethodID)
		if err != nil {
			return SampleAliquot{}, err
		}
		aliquot.MethodName = name
	}
	return aliquot, nil
}

func catalogDepartmentNameTx(tx *sql.Tx, scope Scope, id string) (string, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM catalog_departments WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("unknown catalog department %q", id)
	}
	return name, err
}

func catalogMethodNameTx(tx *sql.Tx, scope Scope, id string) (string, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM catalog_methods WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("unknown catalog method %q", id)
	}
	return name, err
}

func (s *Store) AddSampleContainer(sampleID string, input SampleContainerInput, actor ActorContext) (Sample, error) {
	return s.AddSampleContainerForScope(defaultScope(), sampleID, input, actor)
}

func (s *Store) AddSampleContainerForScope(scope Scope, sampleID string, input SampleContainerInput, actor ActorContext) (Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return Sample{}, err
	}
	var sample Sample
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationSampleIntake, actor, AuditResource{Type: "sample", ID: sampleID}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		sample, err = sampleByIDForScopeTx(tx, scope, sampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				deniedErr = fmt.Errorf("unknown sample %q", sampleID)
				return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.container.add.requested", Outcome: AuditOutcomeDenied, Reason: "sample_not_found", Resource: AuditResource{Type: "sample", ID: sampleID}})
			}
			return err
		}
		if sample.Status != StatusReceived {
			deniedErr = errors.New("containers can only be added to received samples before custody advances")
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.container.add.requested", Outcome: AuditOutcomeDenied, Reason: "custody_advanced", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"status": string(sample.Status)}})
		}
		container, err := buildSampleContainerTx(tx, scope, sample.ID, len(sample.Containers), input)
		if err != nil {
			return err
		}
		sample.Containers = append(sample.Containers, container)
		sample.UpdatedAt = time.Now().UTC()
		encodedContainers, err := json.Marshal(sample.Containers)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE samples SET containers_json = ?, updated_at = ? WHERE id = ?`, string(encodedContainers), formatTime(sample.UpdatedAt), sample.ID); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.container.added", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"container_id": container.ID, "aliquot_count": len(container.Aliquots)}})
	})
	if err != nil {
		return Sample{}, err
	}
	if deniedErr != nil {
		return Sample{}, deniedErr
	}
	return sample, nil
}
