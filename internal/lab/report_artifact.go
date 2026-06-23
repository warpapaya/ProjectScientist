package lab

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ReportReleaseInput struct {
	SampleID           string            `json:"sample_id"`
	TemplateID         string            `json:"template_id"`
	TemplateVersion    string            `json:"template_version"`
	GenerationInputs   map[string]string `json:"generation_inputs"`
	ArtifactFormat     string            `json:"artifact_format"`
	ArtifactContent    []byte            `json:"artifact_content"`
	ReleaseSignature   string            `json:"release_signature"`
	SupersessionReason string            `json:"supersession_reason"`
}

type ReportDataSnapshot struct {
	Sample  Sample   `json:"sample"`
	Results []Result `json:"results"`
}

type ReportSnapshot struct {
	ID                     string             `json:"id"`
	TenantID               string             `json:"tenant_id"`
	LabID                  string             `json:"lab_id"`
	SampleID               string             `json:"sample_id"`
	TemplateID             string             `json:"template_id"`
	TemplateVersion        string             `json:"template_version"`
	GenerationInputs       map[string]string  `json:"generation_inputs"`
	DataSnapshot           ReportDataSnapshot `json:"data_snapshot"`
	ReviewedBy             string             `json:"reviewed_by"`
	ReleasedBy             string             `json:"released_by"`
	ReleaseSignature       string             `json:"release_signature"`
	ReleasedAt             time.Time          `json:"released_at"`
	ContentHash            string             `json:"content_hash"`
	SupersedesSnapshotID   string             `json:"supersedes_snapshot_id,omitempty"`
	SupersededBySnapshotID string             `json:"superseded_by_snapshot_id,omitempty"`
	CreatedAt              time.Time          `json:"created_at"`
}

type ReportArtifact struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	LabID                string    `json:"lab_id"`
	SampleID             string    `json:"sample_id"`
	SnapshotID           string    `json:"snapshot_id"`
	Format               string    `json:"format"`
	ContentHash          string    `json:"content_hash"`
	Content              []byte    `json:"content"`
	SupersedesArtifactID string    `json:"supersedes_artifact_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type ReleasedReportArtifact struct {
	Snapshot ReportSnapshot `json:"snapshot"`
	Artifact ReportArtifact `json:"artifact"`
}

type reportSnapshotPayload struct {
	CanonicalJSON []byte
	ContentHash   string
	DataSnapshot  ReportDataSnapshot
	ReviewedBy    string
}

func (s *Store) ReleaseReportArtifact(input ReportReleaseInput, actor ActorContext) (ReleasedReportArtifact, error) {
	return s.ReleaseReportArtifactForScope(defaultScope(), input, actor)
}

func (s *Store) ReleaseReportArtifactForScope(scope Scope, input ReportReleaseInput, actor ActorContext) (ReleasedReportArtifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ReleasedReportArtifact{}, err
	}
	input = normalizeReportReleaseInput(input)
	if err := validateReportReleaseInput(input); err != nil {
		return ReleasedReportArtifact{}, err
	}
	var released ReleasedReportArtifact
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationReportRelease, actor, AuditResource{Type: "sample", ID: input.SampleID}, map[string]any{"template_id": input.TemplateID, "template_version": input.TemplateVersion})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = fmt.Errorf("%w: report release requires report-releaser role", ErrAuthorizationDenied)
			return nil
		}
		sample, err := sampleByIDForScopeTx(tx, scope, input.SampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unknown sample %q", input.SampleID)
			}
			return err
		}
		if sample.Status != StatusReleased {
			deniedErr = fmt.Errorf("report artifact requires released sample %q", sample.ID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "report.release.denied", Outcome: AuditOutcomeDenied, Reason: "sample_not_released", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"status": string(sample.Status)}})
		}
		results, err := resultsForSampleTx(tx, scope, sample.ID)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			deniedErr = fmt.Errorf("report artifact requires accepted results for released sample %q", sample.ID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "report.release.denied", Outcome: AuditOutcomeDenied, Reason: "no_results", Resource: AuditResource{Type: "sample", ID: sample.ID}})
		}
		for _, result := range results {
			if result.Status != ResultStatusAccepted {
				deniedErr = fmt.Errorf("report artifact requires accepted results for released sample %q", sample.ID)
				return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "report.release.denied", Outcome: AuditOutcomeDenied, Reason: "unaccepted_result", Resource: AuditResource{Type: "result", ID: result.ID}, Details: map[string]any{"sample_id": sample.ID, "status": string(result.Status)}})
			}
		}
		qcBlockers, err := qcReleaseBlockersForSampleTx(tx, scope, sample.ID)
		if err != nil {
			return err
		}
		if len(qcBlockers) > 0 {
			deniedErr = fmt.Errorf("report artifact requires accepted QC batch(es) for released sample %q: %s", sample.ID, strings.Join(qcBlockerIDs(qcBlockers), ", "))
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "report.release.denied", Outcome: AuditOutcomeDenied, Reason: "qc_not_accepted", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"qc_batches": qcBlockerDetails(qcBlockers)}})
		}

		now := time.Now().UTC()
		releasedBy := normalizeActorContext(actor, "report-release").UserID
		payload, err := buildReportSnapshotPayload(sample, results, input.TemplateID, input.TemplateVersion, input.GenerationInputs, releasedBy, input.ReleaseSignature, now)
		if err != nil {
			return err
		}
		previous, err := currentReportForSampleTx(tx, scope, sample.ID)
		if err != nil {
			return err
		}
		nextSnapshot, err := nextCounter(tx, "next_report_snapshot")
		if err != nil {
			return err
		}
		nextArtifact, err := nextCounter(tx, "next_report_artifact")
		if err != nil {
			return err
		}
		snapshot := ReportSnapshot{ID: fmt.Sprintf("RS-%06d", nextSnapshot), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, TemplateID: input.TemplateID, TemplateVersion: input.TemplateVersion, GenerationInputs: input.GenerationInputs, DataSnapshot: payload.DataSnapshot, ReviewedBy: payload.ReviewedBy, ReleasedBy: releasedBy, ReleaseSignature: input.ReleaseSignature, ReleasedAt: now, ContentHash: payload.ContentHash, CreatedAt: now}
		artifactHash := hashBytes(input.ArtifactContent)
		artifact := ReportArtifact{ID: fmt.Sprintf("RA-%06d", nextArtifact), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, SnapshotID: snapshot.ID, Format: input.ArtifactFormat, ContentHash: artifactHash, Content: append([]byte(nil), input.ArtifactContent...), CreatedAt: now}
		if previous != nil {
			snapshot.SupersedesSnapshotID = previous.SnapshotID
			artifact.SupersedesArtifactID = previous.ArtifactID
		}
		generationJSON, err := json.Marshal(snapshot.GenerationInputs)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO report_snapshots(id, tenant_id, lab_id, sample_id, template_id, template_version, generation_inputs_json, data_snapshot_json, reviewed_by, released_by, release_signature, released_at, content_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, snapshot.TenantID, snapshot.LabID, snapshot.SampleID, snapshot.TemplateID, snapshot.TemplateVersion, string(generationJSON), string(payload.CanonicalJSON), snapshot.ReviewedBy, snapshot.ReleasedBy, snapshot.ReleaseSignature, formatTime(snapshot.ReleasedAt), snapshot.ContentHash, formatTime(snapshot.CreatedAt)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO report_artifacts(id, tenant_id, lab_id, sample_id, snapshot_id, format, content_hash, content_blob, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, artifact.ID, artifact.TenantID, artifact.LabID, artifact.SampleID, artifact.SnapshotID, artifact.Format, artifact.ContentHash, artifact.Content, formatTime(artifact.CreatedAt)); err != nil {
			return err
		}
		if previous != nil {
			if _, err := tx.Exec(`INSERT INTO report_supersessions(tenant_id, lab_id, superseded_snapshot_id, superseding_snapshot_id, superseded_artifact_id, superseding_artifact_id, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, scope.TenantID, scope.LabID, previous.SnapshotID, snapshot.ID, previous.ArtifactID, artifact.ID, input.SupersessionReason, formatTime(now)); err != nil {
				return err
			}
		}
		if err := appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "report.artifact.released", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "report_artifact", ID: artifact.ID}, Details: map[string]any{"sample_id": sample.ID, "snapshot_id": snapshot.ID, "snapshot_hash": snapshot.ContentHash, "artifact_hash": artifact.ContentHash, "template_id": snapshot.TemplateID, "template_version": snapshot.TemplateVersion, "supersedes_snapshot_id": snapshot.SupersedesSnapshotID}}); err != nil {
			return err
		}
		released = ReleasedReportArtifact{Snapshot: snapshot, Artifact: artifact}
		return nil
	})
	if err != nil {
		return ReleasedReportArtifact{}, err
	}
	if deniedErr != nil {
		return ReleasedReportArtifact{}, deniedErr
	}
	return released, nil
}

func (s *Store) ReportArtifactForScope(scope Scope, id string) (ReportArtifact, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ReportArtifact{}, false
	}
	artifact, err := reportArtifactByIDQuery(s.db, scope, strings.TrimSpace(id))
	if err != nil {
		return ReportArtifact{}, false
	}
	return artifact, true
}

func reportArtifactByIDQuery(q interface{ QueryRow(string, ...any) *sql.Row }, scope Scope, id string) (ReportArtifact, error) {
	row := q.QueryRow(`SELECT id, tenant_id, lab_id, sample_id, snapshot_id, format, content_hash, content_blob, created_at FROM report_artifacts WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id)
	var artifact ReportArtifact
	var createdAt string
	if err := row.Scan(&artifact.ID, &artifact.TenantID, &artifact.LabID, &artifact.SampleID, &artifact.SnapshotID, &artifact.Format, &artifact.ContentHash, &artifact.Content, &createdAt); err != nil {
		return ReportArtifact{}, err
	}
	artifact.CreatedAt, _ = parseTime(createdAt)
	return artifact, nil
}

func (s *Store) ReportSnapshot(id string) (ReportSnapshot, bool) {
	return s.ReportSnapshotForScope(defaultScope(), id)
}

func (s *Store) ReportSnapshotForScope(scope Scope, id string) (ReportSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return ReportSnapshot{}, false
	}
	snapshot, err := reportSnapshotByIDTx(s.db, scope, strings.TrimSpace(id))
	if err != nil {
		return ReportSnapshot{}, false
	}
	return snapshot, true
}

func normalizeReportReleaseInput(input ReportReleaseInput) ReportReleaseInput {
	input.SampleID = strings.TrimSpace(input.SampleID)
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.TemplateVersion = strings.TrimSpace(input.TemplateVersion)
	input.ArtifactFormat = strings.TrimSpace(input.ArtifactFormat)
	input.ReleaseSignature = strings.TrimSpace(input.ReleaseSignature)
	input.SupersessionReason = strings.TrimSpace(input.SupersessionReason)
	if input.GenerationInputs == nil {
		input.GenerationInputs = map[string]string{}
	}
	normalized := map[string]string{}
	for key, value := range input.GenerationInputs {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = strings.TrimSpace(value)
	}
	input.GenerationInputs = normalized
	return input
}

func reportReleaseSignature(actor ActorContext) string {
	userID := normalizeActorContext(actor, "report-release").UserID
	if userID == "" {
		userID = "report-release"
	}
	return userID + " signed report release approval"
}

func validateReportReleaseInput(input ReportReleaseInput) error {
	if input.SampleID == "" {
		return errors.New("sample id is required")
	}
	if input.TemplateID == "" {
		return errors.New("report template id is required")
	}
	if input.TemplateVersion == "" {
		return errors.New("report template version is required")
	}
	if input.ArtifactFormat == "" {
		return errors.New("report artifact format is required")
	}
	if len(input.ArtifactContent) == 0 {
		return errors.New("report artifact content is required")
	}
	if input.ReleaseSignature == "" {
		return errors.New("report release signature is required")
	}
	return nil
}

func buildReportSnapshotPayload(sample Sample, results []Result, templateID, templateVersion string, generationInputs map[string]string, releasedBy, releaseSignature string, releasedAt time.Time) (reportSnapshotPayload, error) {
	reviewedBy := joinedReviewedBy(results)
	data := ReportDataSnapshot{Sample: sample, Results: append([]Result(nil), results...)}
	canonical := struct {
		Sample           Sample            `json:"sample"`
		Results          []Result          `json:"results"`
		TemplateID       string            `json:"template_id"`
		TemplateVersion  string            `json:"template_version"`
		GenerationInputs map[string]string `json:"generation_inputs"`
		ReviewedBy       string            `json:"reviewed_by"`
		ReleasedBy       string            `json:"released_by"`
		ReleaseSignature string            `json:"release_signature"`
		ReleasedAt       string            `json:"released_at"`
	}{Sample: data.Sample, Results: data.Results, TemplateID: strings.TrimSpace(templateID), TemplateVersion: strings.TrimSpace(templateVersion), GenerationInputs: generationInputs, ReviewedBy: reviewedBy, ReleasedBy: strings.TrimSpace(releasedBy), ReleaseSignature: strings.TrimSpace(releaseSignature), ReleasedAt: formatTime(releasedAt)}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return reportSnapshotPayload{}, err
	}
	return reportSnapshotPayload{CanonicalJSON: payload, ContentHash: hashBytes(payload), DataSnapshot: data, ReviewedBy: reviewedBy}, nil
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func joinedReviewedBy(results []Result) string {
	seen := map[string]bool{}
	out := []string{}
	for _, result := range results {
		if reviewer := strings.TrimSpace(result.ReviewedBy); reviewer != "" && !seen[reviewer] {
			seen[reviewer] = true
			out = append(out, reviewer)
		}
	}
	return strings.Join(out, ",")
}

type reportCurrentRow struct {
	SnapshotID string
	ArtifactID string
}

func currentReportForSampleTx(tx *sql.Tx, scope Scope, sampleID string) (*reportCurrentRow, error) {
	row := tx.QueryRow(`SELECT s.id, a.id FROM report_snapshots s JOIN report_artifacts a ON a.snapshot_id = s.id WHERE s.tenant_id = ? AND s.lab_id = ? AND s.sample_id = ? AND NOT EXISTS (SELECT 1 FROM report_supersessions x WHERE x.superseded_snapshot_id = s.id) ORDER BY s.released_at DESC, s.id DESC LIMIT 1`, scope.TenantID, scope.LabID, sampleID)
	var current reportCurrentRow
	if err := row.Scan(&current.SnapshotID, &current.ArtifactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &current, nil
}

func resultsForSampleTx(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, scope Scope, sampleID string) ([]Result, error) {
	rows, err := q.Query(resultSelectSQL+` FROM results WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? ORDER BY id`, scope.TenantID, scope.LabID, strings.TrimSpace(sampleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

type reportSnapshotQueryer interface{ QueryRow(string, ...any) *sql.Row }

func reportSnapshotByIDTx(q reportSnapshotQueryer, scope Scope, id string) (ReportSnapshot, error) {
	row := q.QueryRow(`SELECT s.id, s.tenant_id, s.lab_id, s.sample_id, s.template_id, s.template_version, s.generation_inputs_json, s.data_snapshot_json, s.reviewed_by, s.released_by, s.release_signature, s.released_at, s.content_hash, s.created_at, COALESCE(prev.superseded_snapshot_id, ''), COALESCE(next.superseding_snapshot_id, '') FROM report_snapshots s LEFT JOIN report_supersessions prev ON prev.superseding_snapshot_id = s.id LEFT JOIN report_supersessions next ON next.superseded_snapshot_id = s.id WHERE s.tenant_id = ? AND s.lab_id = ? AND s.id = ?`, scope.TenantID, scope.LabID, id)
	var snapshot ReportSnapshot
	var generationJSON, dataJSON, releasedAt, createdAt string
	if err := row.Scan(&snapshot.ID, &snapshot.TenantID, &snapshot.LabID, &snapshot.SampleID, &snapshot.TemplateID, &snapshot.TemplateVersion, &generationJSON, &dataJSON, &snapshot.ReviewedBy, &snapshot.ReleasedBy, &snapshot.ReleaseSignature, &releasedAt, &snapshot.ContentHash, &createdAt, &snapshot.SupersedesSnapshotID, &snapshot.SupersededBySnapshotID); err != nil {
		return ReportSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(generationJSON), &snapshot.GenerationInputs); err != nil {
		return ReportSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(dataJSON), &snapshot.DataSnapshot); err != nil {
		return ReportSnapshot{}, err
	}
	snapshot.ReleasedAt, _ = parseTime(releasedAt)
	snapshot.CreatedAt, _ = parseTime(createdAt)
	return snapshot, nil
}
