package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type QCSampleKind string

type QCRelationshipRequirement string

type QCRelationshipType string

const (
	RelationshipRequired QCRelationshipRequirement = "required"
	RelationshipOptional QCRelationshipRequirement = "optional"

	QCSampleKindMethodBlank                       QCSampleKind = "method_blank"
	QCSampleKindTripBlank                         QCSampleKind = "trip_blank"
	QCSampleKindEquipmentBlank                    QCSampleKind = "equipment_blank"
	QCSampleKindFieldDuplicate                    QCSampleKind = "field_duplicate"
	QCSampleKindLabDuplicate                      QCSampleKind = "lab_duplicate"
	QCSampleKindMatrixSpike                       QCSampleKind = "matrix_spike"
	QCSampleKindMatrixSpikeDuplicate              QCSampleKind = "matrix_spike_duplicate"
	QCSampleKindLaboratoryControlSample           QCSampleKind = "laboratory_control_sample"
	QCSampleKindControlSample                     QCSampleKind = "control_sample"
	QCSampleKindInitialCalibrationVerification    QCSampleKind = "initial_calibration_verification"
	QCSampleKindContinuingCalibrationVerification QCSampleKind = "continuing_calibration_verification"

	QCRelationshipTypeBatchControl         QCRelationshipType = "batch_control"
	QCRelationshipTypeDuplicateOf          QCRelationshipType = "duplicate_of"
	QCRelationshipTypeSpikeOf              QCRelationshipType = "spike_of"
	QCRelationshipTypeControlForMethod     QCRelationshipType = "control_for_method"
	QCRelationshipTypeCalibrationForMethod QCRelationshipType = "calibration_for_method"
)

type QCSampleDefinition struct {
	Kind                     QCSampleKind              `json:"kind"`
	Label                    string                    `json:"label"`
	Purpose                  string                    `json:"purpose"`
	RelationshipRequired     QCRelationshipRequirement `json:"relationship_required"`
	AllowedRelationshipTypes []QCRelationshipType      `json:"allowed_relationship_types"`
}

type QCSampleRelationship struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	LabID               string             `json:"lab_id"`
	QCSampleID          string             `json:"qc_sample_id"`
	QCSampleKind        QCSampleKind       `json:"qc_sample_kind"`
	RelationshipType    QCRelationshipType `json:"relationship_type"`
	RelatedSampleID     string             `json:"related_sample_id,omitempty"`
	MethodID            string             `json:"method_id,omitempty"`
	AnalysisRequestLine string             `json:"analysis_request_line_id,omitempty"`
	BatchID             string             `json:"batch_id,omitempty"`
	Notes               string             `json:"notes,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
}

type CreateQCSampleRelationshipInput struct {
	QCSampleID          string             `json:"qc_sample_id"`
	QCSampleKind        QCSampleKind       `json:"qc_sample_kind"`
	RelationshipType    QCRelationshipType `json:"relationship_type"`
	RelatedSampleID     string             `json:"related_sample_id"`
	MethodID            string             `json:"method_id"`
	AnalysisRequestLine string             `json:"analysis_request_line_id"`
	BatchID             string             `json:"batch_id"`
	Notes               string             `json:"notes"`
}

var qcSampleTaxonomy = []QCSampleDefinition{
	{Kind: QCSampleKindMethodBlank, Label: "Method blank", Purpose: "Reagent/process blank carried through a method batch to prove contamination is not introduced by preparation or analysis.", RelationshipRequired: RelationshipOptional, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeBatchControl, QCRelationshipTypeControlForMethod}},
	{Kind: QCSampleKindTripBlank, Label: "Trip blank", Purpose: "Blank transported with field samples to assess shipment/field transport contamination.", RelationshipRequired: RelationshipOptional, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeBatchControl}},
	{Kind: QCSampleKindEquipmentBlank, Label: "Equipment blank", Purpose: "Blank passed over or through sampling equipment to assess field/equipment contamination.", RelationshipRequired: RelationshipOptional, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeBatchControl}},
	{Kind: QCSampleKindFieldDuplicate, Label: "Field duplicate", Purpose: "Independently collected duplicate of a client sample used to assess field and analytical precision.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeDuplicateOf}},
	{Kind: QCSampleKindLabDuplicate, Label: "Laboratory duplicate", Purpose: "Duplicate aliquot/preparation of a client sample used to assess laboratory precision.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeDuplicateOf}},
	{Kind: QCSampleKindMatrixSpike, Label: "Matrix spike", Purpose: "Client matrix fortified with target analytes to assess matrix-specific recovery.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeSpikeOf}},
	{Kind: QCSampleKindMatrixSpikeDuplicate, Label: "Matrix spike duplicate", Purpose: "Second fortified aliquot used with the matrix spike for recovery and precision assessment.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeSpikeOf, QCRelationshipTypeDuplicateOf}},
	{Kind: QCSampleKindLaboratoryControlSample, Label: "Laboratory control sample", Purpose: "Known clean/reference matrix fortified independently of client matrix to assess method performance.", RelationshipRequired: RelationshipOptional, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeControlForMethod, QCRelationshipTypeBatchControl}},
	{Kind: QCSampleKindControlSample, Label: "Control sample", Purpose: "Generic positive/negative control or certified/reference sample associated with a method or batch.", RelationshipRequired: RelationshipOptional, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeControlForMethod, QCRelationshipTypeBatchControl}},
	{Kind: QCSampleKindInitialCalibrationVerification, Label: "Initial calibration verification", Purpose: "Independent standard confirming the initial calibration before sample analysis.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeCalibrationForMethod}},
	{Kind: QCSampleKindContinuingCalibrationVerification, Label: "Continuing calibration verification", Purpose: "Continuing standard confirming instrument calibration remains acceptable during a run.", RelationshipRequired: RelationshipRequired, AllowedRelationshipTypes: []QCRelationshipType{QCRelationshipTypeCalibrationForMethod}},
}

func QCSampleTaxonomy() []QCSampleDefinition {
	out := make([]QCSampleDefinition, len(qcSampleTaxonomy))
	copy(out, qcSampleTaxonomy)
	return out
}

func QCDefinitionForKind(kind QCSampleKind) (QCSampleDefinition, bool) {
	kind = normalizeQCSampleKind(kind)
	for _, def := range qcSampleTaxonomy {
		if def.Kind == kind {
			return def, true
		}
	}
	return QCSampleDefinition{}, false
}

func (s *Store) CreateQCSampleRelationship(input CreateQCSampleRelationshipInput, actor ActorContext) (QCSampleRelationship, error) {
	return s.CreateQCSampleRelationshipForScope(defaultScope(), input, actor)
}

func (s *Store) CreateQCSampleRelationshipForScope(scope Scope, input CreateQCSampleRelationshipInput, actor ActorContext) (QCSampleRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return QCSampleRelationship{}, err
	}
	input = normalizeQCSampleRelationshipInput(input)
	definition, ok := QCDefinitionForKind(input.QCSampleKind)
	if !ok {
		return QCSampleRelationship{}, fmt.Errorf("unknown QC sample kind %q", input.QCSampleKind)
	}
	if !relationshipTypeAllowed(definition, input.RelationshipType) {
		return QCSampleRelationship{}, fmt.Errorf("relationship type %q is not allowed for QC sample kind %q", input.RelationshipType, input.QCSampleKind)
	}
	if definition.RelationshipRequired == RelationshipRequired && input.RelatedSampleID == "" && input.MethodID == "" && input.AnalysisRequestLine == "" && input.BatchID == "" {
		return QCSampleRelationship{}, fmt.Errorf("QC sample kind %q requires at least one sample, method, line, or batch relationship", input.QCSampleKind)
	}
	var rel QCSampleRelationship
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationQCRelate, actor, AuditResource{Type: "qc_sample_relationship", ID: "new"}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		if err := requireSampleInScopeTx(tx, scope, input.QCSampleID); err != nil {
			return fmt.Errorf("qc sample %q is outside requested tenant/lab scope", input.QCSampleID)
		}
		if input.RelatedSampleID != "" {
			if err := requireSampleInScopeTx(tx, scope, input.RelatedSampleID); err != nil {
				return fmt.Errorf("related sample %q is outside requested tenant/lab scope", input.RelatedSampleID)
			}
		}
		if input.MethodID != "" {
			if err := requireScopedID(tx, "catalog_methods", scope, input.MethodID); err != nil {
				return fmt.Errorf("method: %w", err)
			}
		}
		if input.AnalysisRequestLine != "" {
			line, err := analysisRequestLineByIDTx(tx, input.AnalysisRequestLine)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("unknown analysis request line %q", input.AnalysisRequestLine)
				}
				return err
			}
			if line.TenantID != scope.TenantID || line.LabID != scope.LabID {
				return fmt.Errorf("analysis request line %q is outside requested tenant/lab scope", input.AnalysisRequestLine)
			}
			if input.RelatedSampleID != "" && line.SampleID != input.RelatedSampleID {
				return fmt.Errorf("analysis request line %q belongs to sample %q, not related sample %q", input.AnalysisRequestLine, line.SampleID, input.RelatedSampleID)
			}
		}
		next, err := nextCounter(tx, "next_qc_sample_relationship")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		rel = QCSampleRelationship{ID: fmt.Sprintf("QCR-%06d", next), TenantID: scope.TenantID, LabID: scope.LabID, QCSampleID: input.QCSampleID, QCSampleKind: input.QCSampleKind, RelationshipType: input.RelationshipType, RelatedSampleID: input.RelatedSampleID, MethodID: input.MethodID, AnalysisRequestLine: input.AnalysisRequestLine, BatchID: input.BatchID, Notes: input.Notes, CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO qc_sample_relationships(id, tenant_id, lab_id, qc_sample_id, qc_sample_kind, relationship_type, related_sample_id, method_id, analysis_request_line_id, batch_id, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rel.ID, rel.TenantID, rel.LabID, rel.QCSampleID, string(rel.QCSampleKind), string(rel.RelationshipType), rel.RelatedSampleID, rel.MethodID, rel.AnalysisRequestLine, rel.BatchID, rel.Notes, formatTime(rel.CreatedAt)); err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "qc_sample.relationship.created", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "qc_sample_relationship", ID: rel.ID}, Details: map[string]any{"qc_sample_id": rel.QCSampleID, "qc_sample_kind": string(rel.QCSampleKind), "relationship_type": string(rel.RelationshipType), "related_sample_id": rel.RelatedSampleID, "method_id": rel.MethodID, "analysis_request_line_id": rel.AnalysisRequestLine, "batch_id": rel.BatchID}})
	})
	if txErr != nil {
		return QCSampleRelationship{}, txErr
	}
	if deniedErr != nil {
		return QCSampleRelationship{}, deniedErr
	}
	return rel, nil
}

func (s *Store) QCSampleRelationshipsForSample(sampleID string) []QCSampleRelationship {
	return s.QCSampleRelationshipsForSampleForScope(defaultScope(), sampleID)
}

func (s *Store) QCSampleRelationshipsForSampleForScope(scope Scope, sampleID string) []QCSampleRelationship {
	return s.qcSampleRelationshipsForScope(`related_sample_id = ?`, scope, strings.TrimSpace(sampleID))
}

func (s *Store) QCSampleRelationshipsForQCSample(qcSampleID string) []QCSampleRelationship {
	return s.QCSampleRelationshipsForQCSampleForScope(defaultScope(), qcSampleID)
}

func (s *Store) QCSampleRelationshipsForQCSampleForScope(scope Scope, qcSampleID string) []QCSampleRelationship {
	return s.qcSampleRelationshipsForScope(`qc_sample_id = ?`, scope, strings.TrimSpace(qcSampleID))
}

func (s *Store) qcSampleRelationshipsForScope(where string, scope Scope, id string) []QCSampleRelationship {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil || id == "" {
		return nil
	}
	rows, err := s.db.Query(qcSampleRelationshipSelect+` FROM qc_sample_relationships WHERE tenant_id = ? AND lab_id = ? AND `+where+` ORDER BY id`, scope.TenantID, scope.LabID, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	rels, err := scanQCSampleRelationships(rows)
	if err != nil {
		return nil
	}
	return rels
}

const qcSampleRelationshipSelect = `SELECT id, tenant_id, lab_id, qc_sample_id, qc_sample_kind, relationship_type, related_sample_id, method_id, analysis_request_line_id, batch_id, notes, created_at`

type qcSampleRelationshipScanner interface{ Scan(dest ...any) error }

func scanQCSampleRelationship(row qcSampleRelationshipScanner) (QCSampleRelationship, error) {
	var rel QCSampleRelationship
	var kind, relType, created string
	if err := row.Scan(&rel.ID, &rel.TenantID, &rel.LabID, &rel.QCSampleID, &kind, &relType, &rel.RelatedSampleID, &rel.MethodID, &rel.AnalysisRequestLine, &rel.BatchID, &rel.Notes, &created); err != nil {
		return QCSampleRelationship{}, err
	}
	rel.QCSampleKind = QCSampleKind(kind)
	rel.RelationshipType = QCRelationshipType(relType)
	rel.CreatedAt, _ = parseTime(created)
	return rel, nil
}

func scanQCSampleRelationships(rows *sql.Rows) ([]QCSampleRelationship, error) {
	rels := []QCSampleRelationship{}
	for rows.Next() {
		rel, err := scanQCSampleRelationship(rows)
		if err != nil {
			return nil, err
		}
		rels = append(rels, rel)
	}
	return rels, rows.Err()
}

func normalizeQCSampleRelationshipInput(input CreateQCSampleRelationshipInput) CreateQCSampleRelationshipInput {
	input.QCSampleID = strings.TrimSpace(input.QCSampleID)
	input.QCSampleKind = normalizeQCSampleKind(input.QCSampleKind)
	input.RelationshipType = QCRelationshipType(strings.ToLower(strings.TrimSpace(string(input.RelationshipType))))
	input.RelatedSampleID = strings.TrimSpace(input.RelatedSampleID)
	input.MethodID = strings.TrimSpace(input.MethodID)
	input.AnalysisRequestLine = strings.TrimSpace(input.AnalysisRequestLine)
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.Notes = strings.TrimSpace(input.Notes)
	return input
}

func normalizeQCSampleKind(kind QCSampleKind) QCSampleKind {
	return QCSampleKind(strings.ToLower(strings.TrimSpace(string(kind))))
}

func relationshipTypeAllowed(def QCSampleDefinition, relType QCRelationshipType) bool {
	relType = QCRelationshipType(strings.ToLower(strings.TrimSpace(string(relType))))
	if relType == "" {
		return false
	}
	for _, allowed := range def.AllowedRelationshipTypes {
		if allowed == relType {
			return true
		}
	}
	return false
}

func requireSampleInScopeTx(tx *sql.Tx, scope Scope, sampleID string) error {
	if strings.TrimSpace(sampleID) == "" {
		return errors.New("sample id is required")
	}
	var tenantID, labID string
	if err := tx.QueryRow(`SELECT tenant_id, lab_id FROM samples WHERE id = ?`, strings.TrimSpace(sampleID)).Scan(&tenantID, &labID); err != nil {
		return err
	}
	if tenantID != scope.TenantID || labID != scope.LabID {
		return fmt.Errorf("sample %q is outside requested tenant/lab scope", sampleID)
	}
	return nil
}
