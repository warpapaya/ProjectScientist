package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type QCBatchStatus string

type QCItemRole string

const (
	QCBatchStatusOpen     QCBatchStatus = "open"
	QCBatchStatusInReview QCBatchStatus = "in_review"
	QCBatchStatusAccepted QCBatchStatus = "accepted"
	QCBatchStatusRejected QCBatchStatus = "rejected"

	QCItemRoleClientSample QCItemRole = "client_sample"
	QCItemRoleQCSample     QCItemRole = "qc_sample"
)

type QCBatch struct {
	ID             string           `json:"id"`
	TenantID       string           `json:"tenant_id"`
	LabID          string           `json:"lab_id"`
	Name           string           `json:"name"`
	MethodID       string           `json:"method_id,omitempty"`
	Matrix         string           `json:"matrix,omitempty"`
	Status         QCBatchStatus    `json:"status"`
	DecisionReason string           `json:"decision_reason,omitempty"`
	Checks         []string         `json:"checks,omitempty"`
	Notes          string           `json:"notes,omitempty"`
	Items          []QCItem         `json:"items,omitempty"`
	Relationships  []QCRelationship `json:"relationships,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type QCItem struct {
	ID           string       `json:"id"`
	TenantID     string       `json:"tenant_id"`
	LabID        string       `json:"lab_id"`
	QCBatchID    string       `json:"qc_batch_id"`
	SampleID     string       `json:"sample_id"`
	Role         QCItemRole   `json:"role"`
	QCSampleKind QCSampleKind `json:"qc_sample_kind,omitempty"`
	Notes        string       `json:"notes,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type QCRelationship struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	LabID                 string             `json:"lab_id"`
	QCBatchID             string             `json:"qc_batch_id"`
	QCItemID              string             `json:"qc_item_id"`
	RelationshipType      QCRelationshipType `json:"relationship_type"`
	RelatedSampleID       string             `json:"related_sample_id,omitempty"`
	AnalysisRequestLineID string             `json:"analysis_request_line_id,omitempty"`
	Notes                 string             `json:"notes,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
}

type CreateQCBatchInput struct {
	Name     string   `json:"name"`
	MethodID string   `json:"method_id"`
	Matrix   string   `json:"matrix"`
	Checks   []string `json:"checks"`
	Notes    string   `json:"notes"`
}

type CreateQCItemInput struct {
	SampleID     string       `json:"sample_id"`
	Role         QCItemRole   `json:"role"`
	QCSampleKind QCSampleKind `json:"qc_sample_kind"`
	Notes        string       `json:"notes"`
}

type CreateQCRelationshipInput struct {
	QCBatchID             string             `json:"qc_batch_id"`
	QCItemID              string             `json:"qc_item_id"`
	RelationshipType      QCRelationshipType `json:"relationship_type"`
	RelatedSampleID       string             `json:"related_sample_id"`
	AnalysisRequestLineID string             `json:"analysis_request_line_id"`
	Notes                 string             `json:"notes"`
}

func (s *Store) CreateQCBatch(input CreateQCBatchInput, actor ActorContext) (QCBatch, error) {
	return s.CreateQCBatchForScope(defaultScope(), input, actor)
}

func (s *Store) CreateQCBatchForScope(scope Scope, input CreateQCBatchInput, actor ActorContext) (QCBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCBatch{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.MethodID = strings.TrimSpace(input.MethodID)
	input.Matrix = strings.TrimSpace(input.Matrix)
	input.Checks = normalizeStrings(input.Checks)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.Name == "" {
		return QCBatch{}, errors.New("QC batch name is required")
	}
	var batch QCBatch
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_batch", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		if input.MethodID != "" {
			if err := requireScopedID(tx, "catalog_methods", scope, input.MethodID); err != nil {
				return fmt.Errorf("method: %w", err)
			}
		}
		next, err := nextCounter(tx, "next_qc_batch")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		batch = QCBatch{ID: fmt.Sprintf("QCB-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, Name: input.Name, MethodID: input.MethodID, Matrix: input.Matrix, Status: QCBatchStatusOpen, Checks: input.Checks, Notes: input.Notes, CreatedAt: now, UpdatedAt: now}
		checksJSON, err := json.Marshal(batch.Checks)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO qc_batches(id, tenant_id, lab_id, name, method_id, matrix, status, decision_reason, checks_json, notes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, batch.ID, batch.TenantID, batch.LabID, batch.Name, batch.MethodID, batch.Matrix, string(batch.Status), batch.DecisionReason, string(checksJSON), batch.Notes, formatTime(batch.CreatedAt), formatTime(batch.UpdatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_batch.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_batch", ID: batch.ID}, Details: map[string]any{"method_id": batch.MethodID, "matrix": batch.Matrix, "status": string(batch.Status), "checks": batch.Checks}})
	})
	if txErr != nil {
		return QCBatch{}, txErr
	}
	if deniedErr != nil {
		return QCBatch{}, deniedErr
	}
	return batch, nil
}

func (s *Store) AddQCItemToBatch(batchID string, input CreateQCItemInput, actor ActorContext) (QCItem, error) {
	return s.AddQCItemToBatchForScope(defaultScope(), batchID, input, actor)
}

func (s *Store) AddQCItemToBatchForScope(scope Scope, batchID string, input CreateQCItemInput, actor ActorContext) (QCItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCItem{}, err
	}
	batchID = strings.TrimSpace(batchID)
	input = normalizeQCItemInput(input)
	if input.Role != QCItemRoleClientSample && input.Role != QCItemRoleQCSample {
		return QCItem{}, fmt.Errorf("unknown QC item role %q", input.Role)
	}
	if input.Role == QCItemRoleQCSample {
		if _, ok := QCDefinitionForKind(input.QCSampleKind); !ok {
			return QCItem{}, fmt.Errorf("unknown QC sample kind %q", input.QCSampleKind)
		}
	} else if input.QCSampleKind != "" {
		return QCItem{}, errors.New("client sample QC item cannot carry a QC sample kind")
	}
	var item QCItem
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_item", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		batch, err := qcBatchByIDTx(tx, scope, batchID)
		if err != nil {
			return err
		}
		if batch.Status != QCBatchStatusOpen {
			return fmt.Errorf("QC items can only be added to open batches, got %q", batch.Status)
		}
		if err := requireSampleInScopeTx(tx, scope, input.SampleID); err != nil {
			return fmt.Errorf("sample %q is outside requested tenant/lab scope", input.SampleID)
		}
		next, err := nextCounter(tx, "next_qc_item")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item = QCItem{ID: fmt.Sprintf("QCI-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, QCBatchID: batchID, SampleID: input.SampleID, Role: input.Role, QCSampleKind: input.QCSampleKind, Notes: input.Notes, CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO qc_items(id, tenant_id, lab_id, qc_batch_id, sample_id, role, qc_sample_kind, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.TenantID, item.LabID, item.QCBatchID, item.SampleID, string(item.Role), string(item.QCSampleKind), item.Notes, formatTime(item.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_batch.item.added", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_item", ID: item.ID}, Details: map[string]any{"qc_batch_id": item.QCBatchID, "sample_id": item.SampleID, "role": string(item.Role), "qc_sample_kind": string(item.QCSampleKind)}})
	})
	if txErr != nil {
		return QCItem{}, txErr
	}
	if deniedErr != nil {
		return QCItem{}, deniedErr
	}
	return item, nil
}

func (s *Store) CreateQCRelationship(input CreateQCRelationshipInput, actor ActorContext) (QCRelationship, error) {
	return s.CreateQCRelationshipForScope(defaultScope(), input, actor)
}

func (s *Store) CreateQCRelationshipForScope(scope Scope, input CreateQCRelationshipInput, actor ActorContext) (QCRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCRelationship{}, err
	}
	input = normalizeQCRelationshipInput(input)
	var rel QCRelationship
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_relationship", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		batch, err := qcBatchByIDTx(tx, scope, input.QCBatchID)
		if err != nil {
			return err
		}
		if batch.Status != QCBatchStatusOpen {
			return fmt.Errorf("QC relationships can only be added to open batches, got %q", batch.Status)
		}
		item, err := qcItemByIDTx(tx, scope, input.QCItemID)
		if err != nil {
			return err
		}
		if item.QCBatchID != input.QCBatchID {
			return fmt.Errorf("QC item %q belongs to batch %q, not %q", input.QCItemID, item.QCBatchID, input.QCBatchID)
		}
		definition, ok := QCDefinitionForKind(item.QCSampleKind)
		if item.Role != QCItemRoleQCSample || !ok {
			return fmt.Errorf("QC relationship requires a QC sample item, got role %q", item.Role)
		}
		if !relationshipTypeAllowed(definition, input.RelationshipType) {
			return fmt.Errorf("relationship type %q is not allowed for QC sample kind %q", input.RelationshipType, item.QCSampleKind)
		}
		if input.RelatedSampleID == "" && input.AnalysisRequestLineID == "" {
			return errors.New("QC relationship requires a related sample or analysis request line")
		}
		if err := requireQCRelationshipTargetsInBatchTx(tx, scope, input.QCBatchID, input.RelatedSampleID, input.AnalysisRequestLineID); err != nil {
			return err
		}
		next, err := nextCounter(tx, "next_qc_relationship")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		rel = QCRelationship{ID: fmt.Sprintf("QCRL-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, QCBatchID: input.QCBatchID, QCItemID: input.QCItemID, RelationshipType: input.RelationshipType, RelatedSampleID: input.RelatedSampleID, AnalysisRequestLineID: input.AnalysisRequestLineID, Notes: input.Notes, CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO qc_relationships(id, tenant_id, lab_id, qc_batch_id, qc_item_id, relationship_type, related_sample_id, analysis_request_line_id, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rel.ID, rel.TenantID, rel.LabID, rel.QCBatchID, rel.QCItemID, string(rel.RelationshipType), rel.RelatedSampleID, rel.AnalysisRequestLineID, rel.Notes, formatTime(rel.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_batch.relationship.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_relationship", ID: rel.ID}, Details: map[string]any{"qc_batch_id": rel.QCBatchID, "qc_item_id": rel.QCItemID, "relationship_type": string(rel.RelationshipType), "related_sample_id": rel.RelatedSampleID, "analysis_request_line_id": rel.AnalysisRequestLineID}})
	})
	if txErr != nil {
		return QCRelationship{}, txErr
	}
	if deniedErr != nil {
		return QCRelationship{}, deniedErr
	}
	return rel, nil
}

func (s *Store) TransitionQCBatch(batchID string, next QCBatchStatus, reason string, actor ActorContext) (QCBatch, error) {
	return s.TransitionQCBatchForScope(defaultScope(), batchID, next, reason, actor)
}

func (s *Store) TransitionQCBatchForScope(scope Scope, batchID string, next QCBatchStatus, reason string, actor ActorContext) (QCBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCBatch{}, err
	}
	batchID = strings.TrimSpace(batchID)
	next = normalizeQCBatchStatus(next)
	reason = strings.TrimSpace(reason)
	var batch QCBatch
	var previous QCBatchStatus
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_batch", ID: batchID}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		current, err := qcBatchByIDTx(tx, scope, batchID)
		if err != nil {
			return err
		}
		previous = current.Status
		if !allowedQCBatchTransition(current.Status, next) {
			return fmt.Errorf("illegal QC batch transition from %q to %q", current.Status, next)
		}
		if (next == QCBatchStatusAccepted || next == QCBatchStatusRejected) && reason == "" {
			return errors.New("QC batch acceptance/rejection reason is required")
		}
		if next == QCBatchStatusInReview || next == QCBatchStatusAccepted {
			qcItems, relationships, err := qcBatchCompositionTx(tx, scope, batchID)
			if err != nil {
				return err
			}
			if ok, err := batchHasValidQCItemAndRelationshipTx(tx, scope, qcItems, relationships); err != nil {
				return err
			} else if !ok {
				return errors.New("QC batch requires at least one QC item with a valid in-batch relationship before review or acceptance")
			}
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(`UPDATE qc_batches SET status = ?, decision_reason = ?, updated_at = ? WHERE id = ? AND tenant_id = ? AND lab_id = ?`, string(next), reason, formatTime(now), batchID, scope.TenantID, scope.LabID); err != nil {
			return err
		}
		batch, err = qcBatchByIDTx(tx, scope, batchID)
		if err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_batch.status.changed", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_batch", ID: batchID}, Details: map[string]any{"from_status": string(previous), "to_status": string(next), "reason": reason}})
	})
	if txErr != nil {
		return QCBatch{}, txErr
	}
	if deniedErr != nil {
		return QCBatch{}, deniedErr
	}
	return batch, nil
}

func (s *Store) QCBatchByID(id string) (QCBatch, bool) {
	return s.QCBatchByIDForScope(defaultScope(), id)
}

func (s *Store) QCBatchByIDForScope(scope Scope, id string) (QCBatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCBatch{}, false
	}
	batch, err := qcBatchByIDTx(s.db, scope, strings.TrimSpace(id))
	if err != nil {
		return QCBatch{}, false
	}
	items, relationships, err := qcBatchCompositionTx(s.db, scope, batch.ID)
	if err != nil {
		return QCBatch{}, false
	}
	batch.Items = items
	batch.Relationships = relationships
	return batch, true
}

func normalizeQCItemInput(input CreateQCItemInput) CreateQCItemInput {
	input.SampleID = strings.TrimSpace(input.SampleID)
	input.Role = QCItemRole(strings.ToLower(strings.TrimSpace(string(input.Role))))
	input.QCSampleKind = normalizeQCSampleKind(input.QCSampleKind)
	input.Notes = strings.TrimSpace(input.Notes)
	return input
}

func normalizeQCRelationshipInput(input CreateQCRelationshipInput) CreateQCRelationshipInput {
	input.QCBatchID = strings.TrimSpace(input.QCBatchID)
	input.QCItemID = strings.TrimSpace(input.QCItemID)
	input.RelationshipType = QCRelationshipType(strings.ToLower(strings.TrimSpace(string(input.RelationshipType))))
	input.RelatedSampleID = strings.TrimSpace(input.RelatedSampleID)
	input.AnalysisRequestLineID = strings.TrimSpace(input.AnalysisRequestLineID)
	input.Notes = strings.TrimSpace(input.Notes)
	return input
}

func normalizeQCBatchStatus(status QCBatchStatus) QCBatchStatus {
	return QCBatchStatus(strings.ToLower(strings.TrimSpace(string(status))))
}

func allowedQCBatchTransition(current, next QCBatchStatus) bool {
	if current == next {
		return true
	}
	switch current {
	case QCBatchStatusOpen:
		return next == QCBatchStatusInReview
	case QCBatchStatusInReview:
		return next == QCBatchStatusAccepted || next == QCBatchStatusRejected
	case QCBatchStatusRejected:
		return next == QCBatchStatusOpen
	default:
		return false
	}
}

func batchHasValidQCItemAndRelationshipTx(q rowQueryer, scope Scope, items []QCItem, relationships []QCRelationship) (bool, error) {
	qcItems := map[string]QCItem{}
	for _, item := range items {
		if item.Role == QCItemRoleQCSample {
			qcItems[item.ID] = item
		}
	}
	for _, rel := range relationships {
		item, ok := qcItems[rel.QCItemID]
		if !ok {
			continue
		}
		definition, ok := QCDefinitionForKind(item.QCSampleKind)
		if !ok || !relationshipTypeAllowed(definition, rel.RelationshipType) {
			continue
		}
		if rel.RelatedSampleID == "" && rel.AnalysisRequestLineID == "" {
			continue
		}
		if err := requireQCRelationshipTargetsInBatchTx(q, scope, rel.QCBatchID, rel.RelatedSampleID, rel.AnalysisRequestLineID); err == nil {
			return true, nil
		} else if isUnknownQCRelationshipReferenceErr(err) {
			return false, err
		}
	}
	return false, nil
}

func requireQCRelationshipTargetsInBatchTx(q rowQueryer, scope Scope, batchID, relatedSampleID, analysisRequestLineID string) error {
	batchID = strings.TrimSpace(batchID)
	relatedSampleID = strings.TrimSpace(relatedSampleID)
	analysisRequestLineID = strings.TrimSpace(analysisRequestLineID)
	batchSamples, err := qcBatchSampleIDsTx(q, scope, batchID)
	if err != nil {
		return err
	}
	if relatedSampleID != "" {
		if err := requireSampleInScopeQueryer(q, scope, relatedSampleID); err != nil {
			return fmt.Errorf("related sample %q is outside requested tenant/lab scope", relatedSampleID)
		}
		if !batchSamples[relatedSampleID] {
			return fmt.Errorf("related sample %q is not part of QC batch %q composition", relatedSampleID, batchID)
		}
	}
	if analysisRequestLineID != "" {
		line, err := analysisRequestLineByIDTx(q, analysisRequestLineID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown analysis request line %q", analysisRequestLineID)
			}
			return err
		}
		if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
			return fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", analysisRequestLineID)
		}
		if relatedSampleID != "" && line.SampleID != relatedSampleID {
			return fmt.Errorf("analysis request line %q belongs to sample %q, not related sample %q", analysisRequestLineID, line.SampleID, relatedSampleID)
		}
		if !batchSamples[line.SampleID] {
			return fmt.Errorf("analysis request line %q sample %q is not part of QC batch %q composition", analysisRequestLineID, line.SampleID, batchID)
		}
	}
	return nil
}

func requireSampleInScopeQueryer(q rowQueryer, scope Scope, sampleID string) error {
	if strings.TrimSpace(sampleID) == "" {
		return errors.New("sample id is required")
	}
	var tenantID, labID string
	if err := q.QueryRow(`SELECT tenant_id, lab_id FROM samples WHERE id = ?`, strings.TrimSpace(sampleID)).Scan(&tenantID, &labID); err != nil {
		return err
	}
	if tenantID != scope.TenantID || labID != scope.LabID {
		return fmt.Errorf("sample %q is outside requested tenant/lab scope", sampleID)
	}
	return nil
}

func qcBatchSampleIDsTx(q rowQueryer, scope Scope, batchID string) (map[string]bool, error) {
	rows, err := q.Query(`SELECT sample_id FROM qc_items WHERE tenant_id = ? AND lab_id = ? AND qc_batch_id = ?`, scope.TenantID, scope.LabID, strings.TrimSpace(batchID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sampleIDs := map[string]bool{}
	for rows.Next() {
		var sampleID string
		if err := rows.Scan(&sampleID); err != nil {
			return nil, err
		}
		sampleIDs[sampleID] = true
	}
	return sampleIDs, rows.Err()
}

func isUnknownQCRelationshipReferenceErr(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "unknown analysis request line") || strings.Contains(message, "outside requested tenant/lab scope")
}

type rowQueryer interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

func qcBatchByIDTx(q rowQueryer, scope Scope, id string) (QCBatch, error) {
	var batch QCBatch
	var status, checksJSON, created, updated string
	if err := q.QueryRow(`SELECT id, tenant_id, lab_id, name, method_id, matrix, status, decision_reason, checks_json, notes, created_at, updated_at FROM qc_batches WHERE id = ? AND tenant_id = ? AND lab_id = ?`, strings.TrimSpace(id), scope.TenantID, scope.LabID).Scan(&batch.ID, &batch.TenantID, &batch.LabID, &batch.Name, &batch.MethodID, &batch.Matrix, &status, &batch.DecisionReason, &checksJSON, &batch.Notes, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return QCBatch{}, fmt.Errorf("unknown QC batch %q", id)
		}
		return QCBatch{}, err
	}
	batch.Status = QCBatchStatus(status)
	batch.Checks = decodeQCChecks(checksJSON)
	batch.CreatedAt, _ = parseTime(created)
	batch.UpdatedAt, _ = parseTime(updated)
	return batch, nil
}

func decodeQCChecks(raw string) []string {
	var checks []string
	if err := json.Unmarshal([]byte(raw), &checks); err != nil {
		return nil
	}
	return normalizeStrings(checks)
}

func qcItemByIDTx(q rowQueryer, scope Scope, id string) (QCItem, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, qc_batch_id, sample_id, role, qc_sample_kind, notes, created_at FROM qc_items WHERE id = ? AND tenant_id = ? AND lab_id = ?`, strings.TrimSpace(id), scope.TenantID, scope.LabID)
	if err != nil {
		return QCItem{}, err
	}
	defer rows.Close()
	items, err := scanQCItems(rows)
	if err != nil {
		return QCItem{}, err
	}
	if len(items) != 1 {
		return QCItem{}, fmt.Errorf("unknown QC item %q", id)
	}
	return items[0], nil
}

func qcBatchCompositionTx(q rowQueryer, scope Scope, batchID string) ([]QCItem, []QCRelationship, error) {
	itemRows, err := q.Query(`SELECT id, tenant_id, lab_id, qc_batch_id, sample_id, role, qc_sample_kind, notes, created_at FROM qc_items WHERE tenant_id = ? AND lab_id = ? AND qc_batch_id = ? ORDER BY id`, scope.TenantID, scope.LabID, batchID)
	if err != nil {
		return nil, nil, err
	}
	defer itemRows.Close()
	items, err := scanQCItems(itemRows)
	if err != nil {
		return nil, nil, err
	}
	relRows, err := q.Query(`SELECT id, tenant_id, lab_id, qc_batch_id, qc_item_id, relationship_type, related_sample_id, analysis_request_line_id, notes, created_at FROM qc_relationships WHERE tenant_id = ? AND lab_id = ? AND qc_batch_id = ? ORDER BY id`, scope.TenantID, scope.LabID, batchID)
	if err != nil {
		return nil, nil, err
	}
	defer relRows.Close()
	rels, err := scanQCRelationships(relRows)
	if err != nil {
		return nil, nil, err
	}
	return items, rels, nil
}

func scanQCItems(rows *sql.Rows) ([]QCItem, error) {
	items := []QCItem{}
	for rows.Next() {
		var item QCItem
		var role, kind, created string
		if err := rows.Scan(&item.ID, &item.TenantID, &item.LabID, &item.QCBatchID, &item.SampleID, &role, &kind, &item.Notes, &created); err != nil {
			return nil, err
		}
		item.Role = QCItemRole(role)
		item.QCSampleKind = QCSampleKind(kind)
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanQCRelationships(rows *sql.Rows) ([]QCRelationship, error) {
	rels := []QCRelationship{}
	for rows.Next() {
		var rel QCRelationship
		var relType, created string
		if err := rows.Scan(&rel.ID, &rel.TenantID, &rel.LabID, &rel.QCBatchID, &rel.QCItemID, &relType, &rel.RelatedSampleID, &rel.AnalysisRequestLineID, &rel.Notes, &created); err != nil {
			return nil, err
		}
		rel.RelationshipType = QCRelationshipType(relType)
		rel.CreatedAt, _ = parseTime(created)
		rels = append(rels, rel)
	}
	return rels, rows.Err()
}
