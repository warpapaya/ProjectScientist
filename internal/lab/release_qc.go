package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ReleaseOverrideInput struct {
	OverrideQC     bool   `json:"override_qc"`
	OverrideReason string `json:"override_reason"`
}

type releaseQCBlocker struct {
	BatchID string
	Status  QCBatchStatus
	Reason  string
}

func (s *Store) authorizeSampleReleaseReadinessTx(tx *sql.Tx, scope Scope, sample Sample, next SampleStatus, override ReleaseOverrideInput, actor ActorContext) error {
	if next != StatusReleased {
		return nil
	}
	unacceptedResults, err := unacceptedResultIDsForSampleTx(tx, scope, sample.ID)
	if err != nil {
		return err
	}
	if len(unacceptedResults) > 0 {
		message := fmt.Sprintf("sample %q has unaccepted result(s): %s", sample.ID, strings.Join(unacceptedResults, ", "))
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.release.blocked", Outcome: AuditOutcomeDenied, Reason: "unaccepted_result", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"result_ids": unacceptedResults}}); err != nil {
			return err
		}
		return errors.New(message)
	}
	qcBlockers, err := qcReleaseBlockersForSampleTx(tx, scope, sample.ID)
	if err != nil {
		return err
	}
	if len(qcBlockers) == 0 {
		return nil
	}
	blockerDetails := qcBlockerDetails(qcBlockers)
	if !override.OverrideQC {
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.release.blocked", Outcome: AuditOutcomeDenied, Reason: "qc_not_accepted", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"qc_batches": blockerDetails}}); err != nil {
			return err
		}
		return fmt.Errorf("sample %q has QC batch(es) not accepted: %s", sample.ID, strings.Join(qcBlockerIDs(qcBlockers), ", "))
	}
	override.OverrideReason = strings.TrimSpace(override.OverrideReason)
	if override.OverrideReason == "" {
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.release.qc_override", Outcome: AuditOutcomeDenied, Reason: "override_reason_required", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"qc_batches": blockerDetails}}); err != nil {
			return err
		}
		return fmt.Errorf("QC release override reason is required")
	}
	allowed, authErr := authorizeOperationTx(tx, scope, OperationReportRelease, actor, AuditResource{Type: "sample", ID: sample.ID}, map[string]any{"override_qc": true})
	if authErr != nil {
		return authErr
	}
	if !allowed {
		return fmt.Errorf("report release permission is required for QC release override")
	}
	return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.release.qc_override", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"reason": override.OverrideReason, "qc_batches": blockerDetails}})
}

func unacceptedResultIDsForSampleTx(q rowQueryer, scope Scope, sampleID string) ([]string, error) {
	rows, err := q.Query(`SELECT id FROM results WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? AND status <> ? ORDER BY id`, scope.TenantID, scope.LabID, strings.TrimSpace(sampleID), string(ResultStatusAccepted))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func qcReleaseBlockersForSampleTx(q rowQueryer, scope Scope, sampleID string) ([]releaseQCBlocker, error) {
	rows, err := q.Query(`SELECT DISTINCT b.id, b.status, b.decision_reason FROM qc_batches b JOIN qc_items i ON i.qc_batch_id = b.id AND i.tenant_id = b.tenant_id AND i.lab_id = b.lab_id WHERE b.tenant_id = ? AND b.lab_id = ? AND i.sample_id = ? AND b.status <> ? ORDER BY b.id`, scope.TenantID, scope.LabID, strings.TrimSpace(sampleID), string(QCBatchStatusAccepted))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	blockers := []releaseQCBlocker{}
	for rows.Next() {
		var blocker releaseQCBlocker
		var status string
		if err := rows.Scan(&blocker.BatchID, &status, &blocker.Reason); err != nil {
			return nil, err
		}
		blocker.Status = QCBatchStatus(status)
		blockers = append(blockers, blocker)
	}
	return blockers, rows.Err()
}

func qcBlockerIDs(blockers []releaseQCBlocker) []string {
	ids := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		ids = append(ids, blocker.BatchID)
	}
	return ids
}

func qcBlockerDetails(blockers []releaseQCBlocker) []map[string]any {
	details := make([]map[string]any, 0, len(blockers))
	for _, blocker := range blockers {
		details = append(details, map[string]any{"qc_batch_id": blocker.BatchID, "status": string(blocker.Status), "decision_reason": blocker.Reason})
	}
	return details
}
