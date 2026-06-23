package lab

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SampleQCReadinessProof struct {
	SampleID           string                     `json:"sample_id"`
	TenantID           string                     `json:"tenant_id"`
	LabID              string                     `json:"lab_id"`
	GeneratedAt        time.Time                  `json:"generated_at"`
	GeneratedBy        string                     `json:"generated_by"`
	ReleaseReady       bool                       `json:"release_ready"`
	AcceptedBatchIDs   []string                   `json:"accepted_batch_ids,omitempty"`
	Blockers           []SampleQCReadinessBlocker `json:"blockers,omitempty"`
	ReconciliationHash string                     `json:"reconciliation_hash"`
}

type SampleQCReadinessBlocker struct {
	BatchID        string        `json:"batch_id"`
	Status         QCBatchStatus `json:"status"`
	DecisionReason string        `json:"decision_reason,omitempty"`
	Checks         []string      `json:"checks,omitempty"`
}

func (s *Store) SampleQCReadinessProof(sampleID string, actor ActorContext) (SampleQCReadinessProof, error) {
	return s.SampleQCReadinessProofForScope(defaultScope(), sampleID, actor)
}

func (s *Store) SampleQCReadinessProofForScope(scope Scope, sampleID string, actor ActorContext) (SampleQCReadinessProof, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return SampleQCReadinessProof{}, err
	}
	sampleID = strings.TrimSpace(sampleID)
	var proof SampleQCReadinessProof
	var deniedErr error
	txErr := s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationAuditView, actor, AuditResource{Type: "sample", ID: sampleID}, map[string]any{"proof": "sample_qc_readiness"})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		if err := requireSampleInScopeTx(tx, scope, sampleID); err != nil {
			return fmt.Errorf("sample %q is outside requested tenant/lab scope", sampleID)
		}
		rows, err := tx.Query(`SELECT DISTINCT b.id, b.status, b.decision_reason, b.checks_json FROM qc_batches b JOIN qc_items i ON i.qc_batch_id = b.id AND i.tenant_id = b.tenant_id AND i.lab_id = b.lab_id WHERE b.tenant_id = ? AND b.lab_id = ? AND i.sample_id = ? ORDER BY b.id`, scope.TenantID, scope.LabID, sampleID)
		if err != nil {
			return err
		}
		defer rows.Close()
		proof = SampleQCReadinessProof{SampleID: sampleID, TenantID: scope.TenantID, LabID: scope.LabID, GeneratedAt: time.Now().UTC(), GeneratedBy: actor.UserID, ReleaseReady: true}
		for rows.Next() {
			var batchID, status, reason, checksJSON string
			if err := rows.Scan(&batchID, &status, &reason, &checksJSON); err != nil {
				return err
			}
			checks := decodeQCChecks(checksJSON)
			if QCBatchStatus(status) == QCBatchStatusAccepted {
				proof.AcceptedBatchIDs = append(proof.AcceptedBatchIDs, batchID)
				continue
			}
			proof.ReleaseReady = false
			proof.Blockers = append(proof.Blockers, SampleQCReadinessBlocker{BatchID: batchID, Status: QCBatchStatus(status), DecisionReason: reason, Checks: checks})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		proof.ReconciliationHash = hashAny(struct {
			SampleID         string                     `json:"sample_id"`
			TenantID         string                     `json:"tenant_id"`
			LabID            string                     `json:"lab_id"`
			ReleaseReady     bool                       `json:"release_ready"`
			AcceptedBatchIDs []string                   `json:"accepted_batch_ids,omitempty"`
			Blockers         []SampleQCReadinessBlocker `json:"blockers,omitempty"`
		}{SampleID: proof.SampleID, TenantID: proof.TenantID, LabID: proof.LabID, ReleaseReady: proof.ReleaseReady, AcceptedBatchIDs: proof.AcceptedBatchIDs, Blockers: proof.Blockers})
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.qc_readiness_reported", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sampleID}, Details: map[string]any{"release_ready": proof.ReleaseReady, "accepted_batch_ids": proof.AcceptedBatchIDs, "blocker_count": len(proof.Blockers), "reconciliation_hash": proof.ReconciliationHash}})
	})
	if txErr != nil {
		return SampleQCReadinessProof{}, txErr
	}
	if deniedErr != nil {
		return SampleQCReadinessProof{}, deniedErr
	}
	return proof, nil
}
