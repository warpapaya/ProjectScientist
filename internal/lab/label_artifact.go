package lab

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const labelArtifactFormat = "text/psc-label-v1"

type LabelArtifact struct {
	ArtifactID    string `json:"artifact_id"`
	SampleID      string `json:"sample_id"`
	Format        string `json:"format"`
	ContentHash   string `json:"content_hash"`
	BarcodeValue  string `json:"barcode_value"`
	QRPayload     string `json:"qr_payload"`
	PrintableText string `json:"printable_text"`
}

func (s *Store) GenerateSampleLabelArtifact(sampleID string, actor ActorContext) (LabelArtifact, error) {
	return s.GenerateSampleLabelArtifactForScope(defaultScope(), sampleID, actor)
}

func (s *Store) GenerateSampleLabelArtifactForScope(scope Scope, sampleID string, actor ActorContext) (LabelArtifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, err := normalizeScope(scope)
	if err != nil {
		return LabelArtifact{}, err
	}
	sampleID = strings.TrimSpace(sampleID)
	if sampleID == "" {
		return LabelArtifact{}, errors.New("sample id is required")
	}
	var artifact LabelArtifact
	var deniedErr error
	err = s.withTx(func(tx *sql.Tx) error {
		allowed, authErr := authorizeOperationTx(tx, scope, OperationReportGenerate, actor, AuditResource{Type: "sample", ID: sampleID}, nil)
		if authErr != nil {
			return authErr
		}
		if !allowed {
			deniedErr = ErrAuthorizationDenied
			return nil
		}
		sample, err := sampleByIDForScopeTx(tx, scope, sampleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				deniedErr = fmt.Errorf("unknown sample %q", sampleID)
				return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.label_artifact.requested", Outcome: AuditOutcomeDenied, Reason: "sample_not_found", Resource: AuditResource{Type: "sample", ID: sampleID}})
			}
			return err
		}
		containerReferenceCodes, err := sampleContainerReferenceCodesTx(tx, scope, sample)
		if err != nil {
			return err
		}
		artifact, err = buildSampleLabelArtifact(scope, sample, containerReferenceCodes)
		if err != nil {
			return err
		}
		return appendAuditTx(tx, auditWrite{Scope: scope, Actor: actor, Action: "sample.label_artifact.generated", Outcome: AuditOutcomeAllowed, Resource: AuditResource{Type: "label_artifact", ID: artifact.ArtifactID}, Details: map[string]any{"sample_id": sample.ID, "content_hash": artifact.ContentHash, "barcode_value": artifact.BarcodeValue, "qr_payload": artifact.QRPayload, "format": artifact.Format}})
	})
	if err != nil {
		return LabelArtifact{}, err
	}
	if deniedErr != nil {
		return LabelArtifact{}, deniedErr
	}
	return artifact, nil
}

func buildSampleLabelArtifact(scope Scope, sample Sample, containerReferenceCodes map[string]string) (LabelArtifact, error) {
	barcodeValue := strings.TrimSpace(sample.LabSampleID)
	if barcodeValue == "" {
		barcodeValue = sample.ID
	}
	if !isBarcodeSafeIdentifier(barcodeValue) {
		return LabelArtifact{}, fmt.Errorf("barcode-safe identifier required for sample %q: %q", sample.ID, barcodeValue)
	}
	qrPayload := fmt.Sprintf("PSC:%s:%s:%s:%s", scope.TenantID, scope.LabID, sample.ID, barcodeValue)
	if !isQRPayloadSafe(qrPayload) {
		return LabelArtifact{}, fmt.Errorf("QR-safe payload could not be produced for sample %q", sample.ID)
	}
	artifactID := "LBL-" + strings.ReplaceAll(sample.ID, "-", "-")
	printable := printableSampleLabelText(artifactID, sample, containerReferenceCodes, barcodeValue, qrPayload)
	sum := sha256.Sum256([]byte(printable))
	return LabelArtifact{ArtifactID: artifactID, SampleID: sample.ID, Format: labelArtifactFormat, ContentHash: "sha256:" + hex.EncodeToString(sum[:]), BarcodeValue: barcodeValue, QRPayload: qrPayload, PrintableText: printable}, nil
}

func printableSampleLabelText(artifactID string, sample Sample, containerReferenceCodes map[string]string, barcodeValue, qrPayload string) string {
	var b strings.Builder
	b.WriteString("PROJECT SCIENTIST SAMPLE LABEL\n")
	fmt.Fprintf(&b, "Artifact: %s\n", artifactID)
	fmt.Fprintf(&b, "Sample: %s\n", sample.ID)
	fmt.Fprintf(&b, "Lab sample: %s\n", emptyLabelValue(sample.LabSampleID))
	fmt.Fprintf(&b, "Client sample: %s\n", emptyLabelValue(sample.ClientSampleID))
	fmt.Fprintf(&b, "Project: %s\n", emptyLabelValue(sample.Project))
	fmt.Fprintf(&b, "Matrix: %s\n", emptyLabelValue(sample.Matrix))
	for _, container := range sample.Containers {
		containerRef := container.ContainerReferenceID
		if code := strings.TrimSpace(containerReferenceCodes[container.ContainerReferenceID]); code != "" {
			containerRef = code
		}
		parts := []string{container.ID, containerRef, container.Volume, container.Condition}
		fmt.Fprintf(&b, "Container: %s\n", strings.Join(nonEmptyLabelParts(parts), " "))
	}
	fmt.Fprintf(&b, "Barcode: %s\n", barcodeValue)
	fmt.Fprintf(&b, "QR: %s\n", qrPayload)
	return b.String()
}

func sampleContainerReferenceCodesTx(tx *sql.Tx, scope Scope, sample Sample) (map[string]string, error) {
	ids := map[string]bool{}
	for _, container := range sample.Containers {
		if id := strings.TrimSpace(container.ContainerReferenceID); id != "" {
			ids[id] = true
		}
	}
	codes := map[string]string{}
	for id := range ids {
		var code string
		err := tx.QueryRow(`SELECT code FROM sample_reference_items WHERE tenant_id = ? AND lab_id = ? AND id = ? AND kind = ?`, scope.TenantID, scope.LabID, id, string(SampleReferenceContainer)).Scan(&code)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("unknown container reference %q", id)
			}
			return nil, err
		}
		codes[id] = code
	}
	return codes, nil
}

func emptyLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func nonEmptyLabelParts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func isBarcodeSafeIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isQRPayloadSafe(value string) bool {
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\t") {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}
