package lab

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func insertImportedSampleTx(tx *sql.Tx, scope Scope, row ImportRow, actor ActorContext, source string, rowNum int) (Sample, error) {
	matrixID, matrixName, err := ensureSampleReferenceTx(tx, scope, SampleReferenceMatrix, row["matrix"])
	if err != nil {
		return Sample{}, err
	}
	preservativeID := ""
	if strings.TrimSpace(row["preservation"]) != "" {
		preservativeID, _, err = ensureSampleReferenceTx(tx, scope, SampleReferencePreservative, row["preservation"])
		if err != nil {
			return Sample{}, err
		}
	}
	receivedConditionID := ""
	if strings.TrimSpace(row["received_condition"]) != "" {
		receivedConditionID, _, err = ensureSampleReferenceTx(tx, scope, SampleReferenceReceivedCondition, row["received_condition"])
		if err != nil {
			return Sample{}, err
		}
	}

	containers, err := importedContainerInputsTx(tx, scope, row, preservativeID, receivedConditionID)
	if err != nil {
		return Sample{}, err
	}
	analyses := stringSliceFromJSON(row["analyses_json"])
	if len(analyses) == 0 {
		analyses = cleanStrings(strings.Split(row["analyses"], ";"))
	}
	input := CreateSampleInput{
		ClientID:            strings.TrimSpace(row["client_id"]),
		Project:             importedProjectName(row),
		ClientSampleID:      strings.TrimSpace(row["client_sample_id"]),
		LabSampleID:         strings.TrimSpace(row["lab_sample_id"]),
		Matrix:              matrixName,
		MatrixReferenceID:   matrixID,
		PreservativeID:      preservativeID,
		ReceivedConditionID: receivedConditionID,
		Priority:            PriorityRoutine,
		Tests:               analyses,
		Containers:          containers,
	}
	if strings.TrimSpace(input.LabSampleID) == "" {
		input.LabSampleID = strings.TrimSpace(row["legacy_id"])
	}
	resolved, err := resolveSampleIntakeTx(tx, scope, input)
	if err != nil {
		return Sample{}, err
	}
	next, err := nextCounter(tx, "next_sample")
	if err != nil {
		return Sample{}, err
	}
	sampleID := fmt.Sprintf("S-%06d", next)
	snapshot, hasSnapshot, err := currentCatalogSnapshotTx(tx, scope)
	if err != nil {
		return Sample{}, err
	}
	builtAnalyses, err := buildAnalysesTx(tx, scope, sampleID, input, resolved.Tests, snapshot, hasSnapshot)
	if err != nil {
		return Sample{}, err
	}
	builtContainers, err := buildSampleContainersTx(tx, scope, sampleID, input.Containers)
	if err != nil {
		return Sample{}, err
	}
	now := time.Now().UTC()
	sample := Sample{ID: sampleID, TenantID: scope.TenantID, LabID: scope.LabID, ClientID: input.ClientID, ProjectID: resolved.ProjectID, Project: resolved.Project, ClientSampleID: input.ClientSampleID, LabSampleID: input.LabSampleID, Matrix: resolved.Matrix, MatrixReferenceID: resolved.MatrixReferenceID, ContainerID: resolved.ContainerID, PreservativeID: resolved.PreservativeID, ReceivedConditionID: resolved.ReceivedConditionID, Priority: PriorityRoutine, Status: StatusReceived, Analyses: builtAnalyses, Containers: builtContainers, CreatedAt: now, UpdatedAt: now}
	encodedAnalyses, err := json.Marshal(sample.Analyses)
	if err != nil {
		return Sample{}, err
	}
	encodedContainers, err := json.Marshal(sample.Containers)
	if err != nil {
		return Sample{}, err
	}
	if _, err := tx.Exec(`INSERT INTO samples(id, tenant_id, lab_id, client_id, project_id, project, client_sample_id, lab_sample_id, matrix, matrix_reference_id, container_id, preservative_id, storage_location_id, received_condition_id, containers_json, sampled_at, received_at, priority, comments, status, analyses_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sample.ID, sample.TenantID, sample.LabID, sample.ClientID, sample.ProjectID, sample.Project, sample.ClientSampleID, sample.LabSampleID, sample.Matrix, sample.MatrixReferenceID, sample.ContainerID, sample.PreservativeID, sample.StorageLocationID, sample.ReceivedConditionID, string(encodedContainers), "", "", string(sample.Priority), sample.Comments, string(sample.Status), string(encodedAnalyses), formatTime(sample.CreatedAt), formatTime(sample.UpdatedAt)); err != nil {
		return Sample{}, err
	}
	if err := createAnalysisRequestLinesForSampleTx(tx, scope, sample); err != nil {
		return Sample{}, err
	}
	if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.imported", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"source": source, "row": rowNum, "legacy_id": row["legacy_id"], "client_sample_id": sample.ClientSampleID, "lab_sample_id": sample.LabSampleID, "container_count": len(sample.Containers), "custody_event_count": len(stringSliceFromJSON(row["custody_events_json"]))}}); err != nil {
		return Sample{}, err
	}
	custodyEvents, err := insertImportedCustodyEventsTx(tx, scope, sample, stringSliceFromJSON(row["custody_events_json"]), actor)
	if err != nil {
		return Sample{}, err
	}
	sample.CustodyEvents = custodyEvents
	return sample, nil
}

func importedContainerInputsTx(tx *sql.Tx, scope Scope, row ImportRow, preservativeID, receivedConditionID string) ([]SampleContainerInput, error) {
	values := stringSliceFromJSON(row["containers_json"])
	out := make([]SampleContainerInput, 0, len(values))
	for _, value := range values {
		containerID, _, err := ensureSampleReferenceTx(tx, scope, SampleReferenceContainer, value)
		if err != nil {
			return nil, err
		}
		out = append(out, SampleContainerInput{ContainerReferenceID: containerID, PreservativeID: preservativeID, ReceivedConditionID: receivedConditionID, Condition: strings.TrimSpace(row["received_condition"]), AliquotInstructions: strings.TrimSpace(value)})
	}
	return out, nil
}

func insertImportedCustodyEventsTx(tx *sql.Tx, scope Scope, sample Sample, rawEvents []string, actor ActorContext) ([]CustodyEvent, error) {
	events := make([]CustodyEvent, 0, len(rawEvents))
	for i, raw := range rawEvents {
		nextID, err := nextCounter(tx, "next_custody_event")
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		event := CustodyEvent{ID: fmt.Sprintf("CE-%06d", nextID), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, Type: custodyTypeFromGoldenEvent(raw), Actor: normalizeActorContext(actor, fmt.Sprintf("custody-%06d", nextID)), OccurredAt: now.Add(time.Duration(i) * time.Second), Location: "synthetic migration import", Reason: strings.TrimSpace(raw), Sequence: int64(i + 1), CreatedAt: now}
		actorJSON, err := json.Marshal(event.Actor)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO custody_events(id, tenant_id, lab_id, sample_id, type, actor_json, occurred_at, location, reason, sequence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.TenantID, event.LabID, event.SampleID, string(event.Type), actorJSON, formatTime(event.OccurredAt), event.Location, event.Reason, event.Sequence, formatTime(event.CreatedAt)); err != nil {
			return nil, err
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.custody.recorded", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"custody_event_id": event.ID, "custody_type": string(event.Type), "reason": event.Reason, "custody_sequence": event.Sequence}}); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func ensureSampleReferenceTx(tx *sql.Tx, scope Scope, kind SampleReferenceKind, name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("%s reference name is required", kind)
	}
	code := sampleReferenceCode(name)
	var id, storedName string
	err := tx.QueryRow(`SELECT id, name FROM sample_reference_items WHERE tenant_id = ? AND lab_id = ? AND kind = ? AND code = ?`, scope.TenantID, scope.LabID, string(kind), code).Scan(&id, &storedName)
	if err == nil {
		return id, storedName, nil
	}
	if err != sql.ErrNoRows {
		return "", "", err
	}
	next, err := nextCounter(tx, "next_sample_reference")
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	id = fmt.Sprintf("SR-%05d", next)
	if _, err := tx.Exec(`INSERT INTO sample_reference_items(id, tenant_id, lab_id, kind, name, code, description, sort_order, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, scope.TenantID, scope.LabID, string(kind), name, code, "created from synthetic golden migration import", 1000+int(next), 1, formatTime(now), formatTime(now)); err != nil {
		return "", "", err
	}
	return id, name, nil
}

func stringSliceFromJSON(raw string) []string {
	var values []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	return cleanStrings(values)
}

func custodyTypeFromGoldenEvent(raw string) CustodyEventType {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(value, "received"):
		return CustodyReceived
	case strings.Contains(value, "split"), strings.Contains(value, "subsample"):
		return CustodySplit
	case strings.Contains(value, "stored"), strings.Contains(value, "hold"):
		return CustodyStored
	default:
		return CustodyTransferred
	}
}

func importedProjectName(row ImportRow) string {
	if family := strings.TrimSpace(row["family_id"]); family != "" {
		return "Golden migration " + family
	}
	return "Golden migration sample import"
}

func sampleExportRow(sample Sample) ImportRow {
	return ImportRow{"id": sample.ID, "client_id": sample.ClientID, "client_sample_id": sample.ClientSampleID, "lab_sample_id": sample.LabSampleID, "matrix": sample.Matrix, "container_count": fmt.Sprint(len(sample.Containers)), "custody_event_count": fmt.Sprint(len(sample.CustodyEvents)), "tenant_id": sample.TenantID, "lab_id": sample.LabID}
}
