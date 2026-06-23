package lab

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ReportReadinessBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ReportReleaseReadiness struct {
	SampleID              string                   `json:"sample_id"`
	SampleStatus          SampleStatus             `json:"sample_status"`
	ReadyForRelease       bool                     `json:"ready_for_release"`
	ReleaseAction         string                   `json:"release_action"`
	PreviewLabel          string                   `json:"preview_label"`
	Blockers              []ReportReadinessBlocker `json:"blockers"`
	ResultCount           int                      `json:"result_count"`
	AcceptedResultCount   int                      `json:"accepted_result_count"`
	QCAccepted            bool                     `json:"qc_accepted"`
	CurrentSnapshotID     string                   `json:"current_snapshot_id,omitempty"`
	CurrentArtifactID     string                   `json:"current_artifact_id,omitempty"`
	CurrentArtifactHash   string                   `json:"current_artifact_hash,omitempty"`
	CurrentArtifactFormat string                   `json:"current_artifact_format,omitempty"`
	LatestCOCPackageID    string                   `json:"latest_coc_package_id,omitempty"`
	LatestCOCPackageHash  string                   `json:"latest_coc_package_hash,omitempty"`
}

func (s *Store) ReportReleaseReadinessForScope(scope Scope, sampleID string) (ReportReleaseReadiness, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ReportReleaseReadiness{}, false
	}
	sampleID = strings.TrimSpace(sampleID)
	sample, err := sampleByIDScanner(s.db.QueryRow(sampleSelectSQL+` FROM samples WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, sampleID))
	if err != nil {
		return ReportReleaseReadiness{}, false
	}
	readiness, err := reportReleaseReadinessForSampleQuery(s.db, scope, sample)
	if err != nil {
		return ReportReleaseReadiness{}, false
	}
	return readiness, true
}

func (s *Store) ReportReleaseReadinessForScopeAll(scope Scope) []ReportReleaseReadiness {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return nil
	}
	samples, err := samplesForScopeReadinessQuery(s.db, scope)
	if err != nil {
		return nil
	}
	items := make([]ReportReleaseReadiness, 0, len(samples))
	for _, sample := range samples {
		readiness, err := reportReleaseReadinessForSampleQuery(s.db, scope, sample)
		if err == nil {
			items = append(items, readiness)
		}
	}
	return items
}

func samplesForScopeReadinessQuery(db *sql.DB, scope Scope) ([]Sample, error) {
	rows, err := db.Query(sampleSelectSQL+` FROM samples WHERE tenant_id = ? AND lab_id = ? ORDER BY id`, scope.TenantID, scope.LabID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []Sample{}
	for rows.Next() {
		sample, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func reportReleaseReadinessForSampleQuery(q interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}, scope Scope, sample Sample) (ReportReleaseReadiness, error) {
	results, err := resultsForSampleTx(q, scope, sample.ID)
	if err != nil {
		return ReportReleaseReadiness{}, err
	}
	qcBlockers, err := qcReleaseBlockersForSampleTx(q, scope, sample.ID)
	if err != nil {
		return ReportReleaseReadiness{}, err
	}
	readiness := ReportReleaseReadiness{SampleID: sample.ID, SampleStatus: sample.Status, PreviewLabel: fmt.Sprintf("Preview COA for %s", sample.ID), QCAccepted: len(qcBlockers) == 0, Blockers: []ReportReadinessBlocker{}}
	for _, result := range results {
		readiness.ResultCount++
		if result.Status == ResultStatusAccepted {
			readiness.AcceptedResultCount++
		}
	}
	if sample.Status != StatusReleased {
		readiness.Blockers = append(readiness.Blockers, ReportReadinessBlocker{Code: "sample_status", Message: fmt.Sprintf("sample must be released before report artifact release; current status is %s", sample.Status)})
	}
	if len(results) == 0 {
		readiness.Blockers = append(readiness.Blockers, ReportReadinessBlocker{Code: "no_results", Message: "sample has no result rows available for report snapshot"})
	} else if readiness.AcceptedResultCount != readiness.ResultCount {
		readiness.Blockers = append(readiness.Blockers, ReportReadinessBlocker{Code: "unaccepted_result", Message: "all result rows must be accepted before report release"})
	}
	if len(qcBlockers) > 0 {
		readiness.Blockers = append(readiness.Blockers, ReportReadinessBlocker{Code: "qc_not_accepted", Message: "linked QC batches must be accepted before release: " + strings.Join(qcBlockerIDs(qcBlockers), ", ")})
	}
	current, err := currentReportForSampleQuery(q, scope, sample.ID)
	if err != nil {
		return ReportReleaseReadiness{}, err
	}
	if current != nil {
		readiness.CurrentSnapshotID = current.SnapshotID
		readiness.CurrentArtifactID = current.ArtifactID
		readiness.CurrentArtifactHash = current.ArtifactHash
		readiness.CurrentArtifactFormat = current.ArtifactFormat
	} else if len(readiness.Blockers) > 0 {
		readiness.Blockers = append(readiness.Blockers, ReportReadinessBlocker{Code: "no_current_report", Message: "no released report artifact exists yet; this will be an initial release after blockers clear"})
	}
	if pkg, err := latestCOCPackageForSampleQuery(q, scope, sample.ID); err == nil && pkg != nil {
		readiness.LatestCOCPackageID = pkg.ID
		readiness.LatestCOCPackageHash = pkg.Hash
	}
	readiness.ReadyForRelease = sample.Status == StatusReleased && len(results) > 0 && readiness.AcceptedResultCount == readiness.ResultCount && len(qcBlockers) == 0
	if !readiness.ReadyForRelease {
		readiness.ReleaseAction = "blocked"
	} else if readiness.CurrentArtifactID == "" {
		readiness.ReleaseAction = "initial_release"
	} else {
		readiness.ReleaseAction = "amendment"
	}
	return readiness, nil
}

type reportCurrentDetailRow struct {
	SnapshotID     string
	ArtifactID     string
	ArtifactHash   string
	ArtifactFormat string
}

func currentReportForSampleQuery(q interface{ QueryRow(string, ...any) *sql.Row }, scope Scope, sampleID string) (*reportCurrentDetailRow, error) {
	row := q.QueryRow(`SELECT s.id, a.id, a.content_hash, a.format FROM report_snapshots s JOIN report_artifacts a ON a.snapshot_id = s.id WHERE s.tenant_id = ? AND s.lab_id = ? AND s.sample_id = ? AND NOT EXISTS (SELECT 1 FROM report_supersessions x WHERE x.superseded_snapshot_id = s.id) ORDER BY s.released_at DESC, s.id DESC LIMIT 1`, scope.TenantID, scope.LabID, sampleID)
	var current reportCurrentDetailRow
	if err := row.Scan(&current.SnapshotID, &current.ArtifactID, &current.ArtifactHash, &current.ArtifactFormat); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &current, nil
}

type latestCOCPackageRow struct {
	ID   string
	Hash string
}

func latestCOCPackageForSampleQuery(q interface{ QueryRow(string, ...any) *sql.Row }, scope Scope, sampleID string) (*latestCOCPackageRow, error) {
	row := q.QueryRow(`SELECT id, content_hash FROM coc_packages WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, scope.TenantID, scope.LabID, sampleID)
	var pkg latestCOCPackageRow
	if err := row.Scan(&pkg.ID, &pkg.Hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &pkg, nil
}
