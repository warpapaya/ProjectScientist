package lab

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ReportPackageAttachmentInput struct {
	Name             string `json:"name"`
	MediaType        string `json:"media_type"`
	Content          []byte `json:"content"`
	SourceArtifactID string `json:"source_artifact_id,omitempty"`
}

type ReportPackageAttachment struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	LabID            string    `json:"lab_id"`
	PackageID        string    `json:"package_id"`
	Name             string    `json:"name"`
	MediaType        string    `json:"media_type"`
	ContentHash      string    `json:"content_hash"`
	Content          []byte    `json:"content"`
	SourceArtifactID string    `json:"source_artifact_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type COCPackageInput struct {
	SampleID      string                         `json:"sample_id"`
	PackageFormat string                         `json:"package_format"`
	Attachments   []ReportPackageAttachmentInput `json:"attachments"`
}

type COCPackage struct {
	ID            string                    `json:"id"`
	TenantID      string                    `json:"tenant_id"`
	LabID         string                    `json:"lab_id"`
	SampleID      string                    `json:"sample_id"`
	PackageFormat string                    `json:"package_format"`
	ContentHash   string                    `json:"content_hash"`
	Content       []byte                    `json:"content"`
	CustodyEvents []CustodyEvent            `json:"custody_events"`
	Attachments   []ReportPackageAttachment `json:"attachments"`
	CreatedAt     time.Time                 `json:"created_at"`
}

type cocPackagePayload struct {
	CanonicalJSON []byte
	ContentHash   string
	Attachments   []ReportPackageAttachment
}

func (s *Store) GenerateCOCPackage(input COCPackageInput, actor ActorContext) (COCPackage, error) {
	return s.GenerateCOCPackageForScope(defaultScope(), input, actor)
}

func (s *Store) GenerateCOCPackageForScope(scope Scope, input COCPackageInput, actor ActorContext) (COCPackage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return COCPackage{}, err
	}
	input = normalizeCOCPackageInput(input)
	if err := validateCOCPackageInput(input); err != nil {
		return COCPackage{}, err
	}
	var pkg COCPackage
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationReportGenerate, actor, AuditResource{Type: "sample", ID: input.SampleID}, map[string]any{"package_format": input.PackageFormat, "attachment_count": len(input.Attachments)})
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = fmt.Errorf("%w: COC package generation requires report generation role", ErrAuthorizationDenied)
			return nil
		}
		sample, err := sampleByIDForScopeTx(tx, scope, input.SampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				deniedErr = fmt.Errorf("unknown sample %q", input.SampleID)
				return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "coc.package.generate.requested", Outcome: AuditOutcomeDenied, Reason: "sample_not_found", Resource: AuditResource{Type: "sample", ID: input.SampleID}, Details: map[string]any{"package_format": input.PackageFormat, "attachment_count": len(input.Attachments)}})
			}
			return err
		}
		custodyEvents, err := custodyEventsForSampleQuery(tx, scope, sample.ID)
		if err != nil {
			return err
		}
		if len(custodyEvents) == 0 {
			deniedErr = fmt.Errorf("COC package requires custody history for sample %q", sample.ID)
			return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "coc.package.generate.denied", Outcome: AuditOutcomeDenied, Reason: "missing_custody_history", Resource: AuditResource{Type: "sample", ID: sample.ID}, Details: map[string]any{"package_format": input.PackageFormat}})
		}
		now := time.Now().UTC()
		payload, err := buildCOCPackagePayload(sample, custodyEvents, input, now)
		if err != nil {
			return err
		}
		nextPackage, err := nextCounter(tx, "next_coc_package")
		if err != nil {
			return err
		}
		pkg = COCPackage{ID: fmt.Sprintf("CP-%06d", nextPackage), TenantID: scope.TenantID, LabID: scope.LabID, SampleID: sample.ID, PackageFormat: input.PackageFormat, ContentHash: payload.ContentHash, Content: append([]byte(nil), payload.CanonicalJSON...), CustodyEvents: append([]CustodyEvent(nil), custodyEvents...), CreatedAt: now}
		if _, err := tx.Exec(`INSERT INTO coc_packages(id, tenant_id, lab_id, sample_id, package_format, content_hash, content_blob, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, pkg.ID, pkg.TenantID, pkg.LabID, pkg.SampleID, pkg.PackageFormat, pkg.ContentHash, pkg.Content, formatTime(pkg.CreatedAt)); err != nil {
			return err
		}
		pkg.Attachments = make([]ReportPackageAttachment, 0, len(payload.Attachments))
		for i, attachment := range payload.Attachments {
			nextAttachment, err := nextCounter(tx, "next_report_package_attachment")
			if err != nil {
				return err
			}
			attachment.ID = fmt.Sprintf("PA-%06d", nextAttachment)
			attachment.TenantID = scope.TenantID
			attachment.LabID = scope.LabID
			attachment.PackageID = pkg.ID
			attachment.CreatedAt = now
			if _, err := tx.Exec(`INSERT INTO report_package_attachments(id, tenant_id, lab_id, package_id, name, media_type, content_hash, content_blob, source_artifact_id, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, attachment.ID, attachment.TenantID, attachment.LabID, attachment.PackageID, attachment.Name, attachment.MediaType, attachment.ContentHash, attachment.Content, attachment.SourceArtifactID, i+1, formatTime(attachment.CreatedAt)); err != nil {
				return err
			}
			pkg.Attachments = append(pkg.Attachments, attachment)
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "coc.package.generated", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "coc_package", ID: pkg.ID}, Details: map[string]any{"sample_id": sample.ID, "package_hash": pkg.ContentHash, "custody_event_count": len(custodyEvents), "attachment_count": len(pkg.Attachments), "attachment_hashes": attachmentHashes(pkg.Attachments)}})
	})
	if err != nil {
		return COCPackage{}, err
	}
	if deniedErr != nil {
		return COCPackage{}, deniedErr
	}
	return pkg, nil
}

func (s *Store) COCPackage(id string) (COCPackage, bool) {
	return s.COCPackageForScope(defaultScope(), id)
}

func (s *Store) COCPackageForScope(scope Scope, id string) (COCPackage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return COCPackage{}, false
	}
	pkg, err := cocPackageByID(s.db, scope, strings.TrimSpace(id))
	if err != nil {
		return COCPackage{}, false
	}
	return pkg, true
}

func normalizeCOCPackageInput(input COCPackageInput) COCPackageInput {
	input.SampleID = strings.TrimSpace(input.SampleID)
	input.PackageFormat = strings.TrimSpace(input.PackageFormat)
	attachments := make([]ReportPackageAttachmentInput, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		attachment.Name = strings.TrimSpace(attachment.Name)
		attachment.MediaType = strings.TrimSpace(attachment.MediaType)
		attachment.SourceArtifactID = strings.TrimSpace(attachment.SourceArtifactID)
		attachments = append(attachments, attachment)
	}
	sort.SliceStable(attachments, func(i, j int) bool {
		if attachments[i].Name != attachments[j].Name {
			return attachments[i].Name < attachments[j].Name
		}
		if attachments[i].MediaType != attachments[j].MediaType {
			return attachments[i].MediaType < attachments[j].MediaType
		}
		return hashBytes(attachments[i].Content) < hashBytes(attachments[j].Content)
	})
	input.Attachments = attachments
	return input
}

func validateCOCPackageInput(input COCPackageInput) error {
	if input.SampleID == "" {
		return errors.New("sample id is required")
	}
	if input.PackageFormat == "" {
		return errors.New("COC package format is required")
	}
	if len(input.Attachments) == 0 {
		return errors.New("COC package requires at least one attachment")
	}
	for _, attachment := range input.Attachments {
		if attachment.Name == "" {
			return errors.New("attachment name is required")
		}
		if attachment.MediaType == "" {
			return errors.New("attachment media type is required")
		}
		if len(attachment.Content) == 0 {
			return fmt.Errorf("attachment %q content is required", attachment.Name)
		}
	}
	return nil
}

func buildCOCPackagePayload(sample Sample, custodyEvents []CustodyEvent, input COCPackageInput, generatedAt time.Time) (cocPackagePayload, error) {
	attachments := make([]ReportPackageAttachment, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		attachments = append(attachments, ReportPackageAttachment{Name: attachment.Name, MediaType: attachment.MediaType, ContentHash: hashBytes(attachment.Content), Content: append([]byte(nil), attachment.Content...), SourceArtifactID: attachment.SourceArtifactID})
	}
	canonicalAttachments := make([]struct {
		Name             string `json:"name"`
		MediaType        string `json:"media_type"`
		ContentHash      string `json:"content_hash"`
		SourceArtifactID string `json:"source_artifact_id,omitempty"`
	}, 0, len(attachments))
	for _, attachment := range attachments {
		canonicalAttachments = append(canonicalAttachments, struct {
			Name             string `json:"name"`
			MediaType        string `json:"media_type"`
			ContentHash      string `json:"content_hash"`
			SourceArtifactID string `json:"source_artifact_id,omitempty"`
		}{Name: attachment.Name, MediaType: attachment.MediaType, ContentHash: attachment.ContentHash, SourceArtifactID: attachment.SourceArtifactID})
	}
	canonicalCustody := make([]struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Actor      string `json:"actor"`
		OccurredAt string `json:"occurred_at"`
		Location   string `json:"location"`
		Reason     string `json:"reason"`
		Sequence   int64  `json:"sequence"`
	}, 0, len(custodyEvents))
	for _, event := range custodyEvents {
		canonicalCustody = append(canonicalCustody, struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Actor      string `json:"actor"`
			OccurredAt string `json:"occurred_at"`
			Location   string `json:"location"`
			Reason     string `json:"reason"`
			Sequence   int64  `json:"sequence"`
		}{ID: event.ID, Type: string(event.Type), Actor: event.Actor.UserID, OccurredAt: formatTime(event.OccurredAt), Location: event.Location, Reason: event.Reason, Sequence: event.Sequence})
	}
	canonical := struct {
		Sample        Sample `json:"sample"`
		PackageFormat string `json:"package_format"`
		GeneratedAt   string `json:"generated_at"`
		CustodyEvents any    `json:"custody_events"`
		Attachments   any    `json:"attachments"`
	}{Sample: sample, PackageFormat: input.PackageFormat, GeneratedAt: formatTime(generatedAt), CustodyEvents: canonicalCustody, Attachments: canonicalAttachments}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return cocPackagePayload{}, err
	}
	return cocPackagePayload{CanonicalJSON: payload, ContentHash: hashBytes(payload), Attachments: attachments}, nil
}

func attachmentHashes(attachments []ReportPackageAttachment) []string {
	hashes := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		hashes = append(hashes, attachment.ContentHash)
	}
	return hashes
}

type custodyEventQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func custodyEventsForSampleQuery(q custodyEventQueryer, scope Scope, sampleID string) ([]CustodyEvent, error) {
	rows, err := q.Query(`SELECT id, tenant_id, lab_id, sample_id, type, actor_json, occurred_at, location, reason, sequence, created_at FROM custody_events WHERE tenant_id = ? AND lab_id = ? AND sample_id = ? ORDER BY sequence`, scope.TenantID, scope.LabID, sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustodyEvents(rows)
}

func cocPackageByID(db *sql.DB, scope Scope, id string) (COCPackage, error) {
	row := db.QueryRow(`SELECT id, tenant_id, lab_id, sample_id, package_format, content_hash, content_blob, created_at FROM coc_packages WHERE tenant_id = ? AND lab_id = ? AND id = ?`, scope.TenantID, scope.LabID, id)
	var pkg COCPackage
	var createdAt string
	if err := row.Scan(&pkg.ID, &pkg.TenantID, &pkg.LabID, &pkg.SampleID, &pkg.PackageFormat, &pkg.ContentHash, &pkg.Content, &createdAt); err != nil {
		return COCPackage{}, err
	}
	pkg.CreatedAt, _ = parseTime(createdAt)
	custodyEvents, err := custodyEventsForSampleQuery(db, scope, pkg.SampleID)
	if err != nil {
		return COCPackage{}, err
	}
	pkg.CustodyEvents = custodyEvents
	attachments, err := packageAttachmentsForPackage(db, scope, pkg.ID)
	if err != nil {
		return COCPackage{}, err
	}
	pkg.Attachments = attachments
	return pkg, nil
}

func packageAttachmentsForPackage(db *sql.DB, scope Scope, packageID string) ([]ReportPackageAttachment, error) {
	rows, err := db.Query(`SELECT id, tenant_id, lab_id, package_id, name, media_type, content_hash, content_blob, source_artifact_id, created_at FROM report_package_attachments WHERE tenant_id = ? AND lab_id = ? AND package_id = ? ORDER BY sort_order, id`, scope.TenantID, scope.LabID, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attachments := []ReportPackageAttachment{}
	for rows.Next() {
		var attachment ReportPackageAttachment
		var createdAt string
		if err := rows.Scan(&attachment.ID, &attachment.TenantID, &attachment.LabID, &attachment.PackageID, &attachment.Name, &attachment.MediaType, &attachment.ContentHash, &attachment.Content, &attachment.SourceArtifactID, &createdAt); err != nil {
			return nil, err
		}
		attachment.CreatedAt, _ = parseTime(createdAt)
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}
