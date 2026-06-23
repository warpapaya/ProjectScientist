package lab

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const AuditRedactedValue = "[REDACTED]"

type AuditPrivacyPolicy struct {
	Name                   string   `json:"name"`
	Scope                  string   `json:"scope"`
	RetentionDays          int      `json:"retention_days"`
	MinimumLegalHoldReason string   `json:"minimum_legal_hold_reason"`
	AuditReadRoles         []string `json:"audit_read_roles"`
	BackupEncryption       string   `json:"backup_encryption"`
}

type AuditLegalHold struct {
	ID          string    `json:"id"`
	Reason      string    `json:"reason"`
	EventIDs    []string  `json:"event_ids,omitempty"`
	TenantID    string    `json:"tenant_id,omitempty"`
	LabID       string    `json:"lab_id,omitempty"`
	EffectiveAt time.Time `json:"effective_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

type AuditRetentionAction string

const (
	AuditRetentionKeep  AuditRetentionAction = "keep"
	AuditRetentionPurge AuditRetentionAction = "purge"
	AuditRetentionHold  AuditRetentionAction = "legal_hold"
)

type AuditRetentionDecision struct {
	Action    AuditRetentionAction `json:"action"`
	Reason    string               `json:"reason"`
	EventID   string               `json:"event_id"`
	Sequence  int64                `json:"sequence"`
	ExpiresAt time.Time            `json:"expires_at"`
	HoldID    string               `json:"hold_id,omitempty"`
}

type AuditRedactionRequest struct {
	RequestedBy string `json:"requested_by"`
	Reason      string `json:"reason"`
}

type AuditRedactionRecord struct {
	RequestedBy       string    `json:"requested_by"`
	Reason            string    `json:"reason"`
	RedactedAt        time.Time `json:"redacted_at"`
	OriginalEventHash string    `json:"original_event_hash"`
	Policy            string    `json:"policy"`
}

type RedactedAuditEvent struct {
	AuditEvent
	Redaction *AuditRedactionRecord `json:"redaction"`
}

type EncryptedBackupProof struct {
	PlaintextPath      string    `json:"plaintext_path"`
	CiphertextPath     string    `json:"ciphertext_path"`
	Algorithm          string    `json:"algorithm"`
	KeyID              string    `json:"key_id"`
	NonceHex           string    `json:"nonce_hex"`
	PlaintextSHA256    string    `json:"plaintext_sha256"`
	PlaintextBytes     int64     `json:"plaintext_bytes"`
	CiphertextSHA256   string    `json:"ciphertext_sha256"`
	CiphertextBytes    int64     `json:"ciphertext_bytes"`
	CreatedAt          time.Time `json:"created_at"`
	Scope              string    `json:"scope"`
	PlaintextRetained  bool      `json:"plaintext_retained"`
	CustomerDataStatus string    `json:"customer_data_status"`
}

type EncryptedBackupReadback struct {
	Plaintext       []byte `json:"-"`
	PlaintextSHA256 string `json:"plaintext_sha256"`
	PlaintextBytes  int64  `json:"plaintext_bytes"`
}

func DefaultAuditPrivacyPolicy() AuditPrivacyPolicy {
	return AuditPrivacyPolicy{
		Name:                   "PSC-RM-084 synthetic lab audit privacy policy v1",
		Scope:                  "Project Scientist lab-development synthetic data only",
		RetentionDays:          397,
		MinimumLegalHoldReason: "named review, incident, or compliance reason required before retention purge",
		AuditReadRoles:         []string{"admin", "lab_manager", "security_reviewer", "privacy_officer"},
		BackupEncryption:       "AES-256-GCM local proof; production design requires managed KMS or age recipient policy before customer data",
	}
}

func EvaluateAuditRetention(policy AuditPrivacyPolicy, event AuditEvent, now time.Time, holds []AuditLegalHold) AuditRetentionDecision {
	if policy.RetentionDays <= 0 {
		policy = DefaultAuditPrivacyPolicy()
	}
	expiresAt := event.Timestamp.AddDate(0, 0, policy.RetentionDays)
	for _, hold := range holds {
		if legalHoldApplies(hold, event, now) {
			return AuditRetentionDecision{Action: AuditRetentionHold, Reason: "legal_hold_overrides_retention_purge", EventID: event.EventID, Sequence: event.Sequence, ExpiresAt: expiresAt, HoldID: hold.ID}
		}
	}
	if !now.Before(expiresAt) {
		return AuditRetentionDecision{Action: AuditRetentionPurge, Reason: "retention_window_expired", EventID: event.EventID, Sequence: event.Sequence, ExpiresAt: expiresAt}
	}
	return AuditRetentionDecision{Action: AuditRetentionKeep, Reason: "within_retention_window", EventID: event.EventID, Sequence: event.Sequence, ExpiresAt: expiresAt}
}

func legalHoldApplies(hold AuditLegalHold, event AuditEvent, now time.Time) bool {
	if strings.TrimSpace(hold.ID) == "" || strings.TrimSpace(hold.Reason) == "" {
		return false
	}
	if !hold.EffectiveAt.IsZero() && now.Before(hold.EffectiveAt) {
		return false
	}
	if !hold.ExpiresAt.IsZero() && !now.Before(hold.ExpiresAt) {
		return false
	}
	if hold.TenantID != "" && hold.TenantID != event.TenantID {
		return false
	}
	if hold.LabID != "" && hold.LabID != event.LabID {
		return false
	}
	if len(hold.EventIDs) == 0 {
		return hold.TenantID != "" || hold.LabID != ""
	}
	for _, id := range hold.EventIDs {
		if id == event.EventID {
			return true
		}
	}
	return false
}

func RedactAuditEventForDisclosure(event AuditEvent, req AuditRedactionRequest) RedactedAuditEvent {
	redacted := event
	redacted.Actor = AuditRedactedValue
	redacted.ActorContext.UserID = AuditRedactedValue
	redacted.ActorContext.DisplayNameSnapshot = AuditRedactedValue
	redacted.ActorContext.RequestID = AuditRedactedValue
	redacted.ActorContext.CorrelationID = AuditRedactedValue
	redacted.Details = sanitizeAuditDetails(event.Action, event.Details)
	return RedactedAuditEvent{
		AuditEvent: redacted,
		Redaction: &AuditRedactionRecord{
			RequestedBy:       strings.TrimSpace(req.RequestedBy),
			Reason:            strings.TrimSpace(req.Reason),
			RedactedAt:        time.Now().UTC(),
			OriginalEventHash: hashAuditEventForRedaction(event),
			Policy:            DefaultAuditPrivacyPolicy().Name,
		},
	}
}

func hashAuditEventForRedaction(event AuditEvent) string {
	body, err := json.Marshal(event)
	if err != nil {
		return auditStringHash(fmt.Sprintf("%#v", event))
	}
	return auditBytesHash(body)
}

func EncryptBackupFile(plaintextPath, ciphertextPath string, key []byte, keyID string) (EncryptedBackupProof, error) {
	if len(key) != 32 {
		return EncryptedBackupProof{}, fmt.Errorf("backup encryption key must be 32 bytes for AES-256-GCM")
	}
	plain, err := os.ReadFile(plaintextPath)
	if err != nil {
		return EncryptedBackupProof{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedBackupProof{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedBackupProof{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedBackupProof{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, []byte(strings.TrimSpace(keyID)))
	if bytes.Contains(ciphertext, plain) {
		return EncryptedBackupProof{}, fmt.Errorf("ciphertext contains plaintext bytes")
	}
	if err := os.WriteFile(ciphertextPath, ciphertext, 0o600); err != nil {
		return EncryptedBackupProof{}, err
	}
	plainDigest, err := fileDigest(plaintextPath)
	if err != nil {
		return EncryptedBackupProof{}, err
	}
	cipherDigest, err := fileDigest(ciphertextPath)
	if err != nil {
		return EncryptedBackupProof{}, err
	}
	return EncryptedBackupProof{
		PlaintextPath:      plaintextPath,
		CiphertextPath:     ciphertextPath,
		Algorithm:          "AES-256-GCM",
		KeyID:              strings.TrimSpace(keyID),
		NonceHex:           hex.EncodeToString(nonce),
		PlaintextSHA256:    plainDigest.SHA256,
		PlaintextBytes:     plainDigest.Bytes,
		CiphertextSHA256:   cipherDigest.SHA256,
		CiphertextBytes:    cipherDigest.Bytes,
		CreatedAt:          time.Now().UTC(),
		Scope:              DefaultAuditPrivacyPolicy().Scope,
		PlaintextRetained:  true,
		CustomerDataStatus: "synthetic-only; no customer data or credentials",
	}, nil
}

func VerifyEncryptedBackupReadback(proof EncryptedBackupProof, key []byte) (EncryptedBackupReadback, error) {
	if len(key) != 32 {
		return EncryptedBackupReadback{}, fmt.Errorf("backup encryption key must be 32 bytes for AES-256-GCM")
	}
	cipherDigest, err := fileDigest(proof.CiphertextPath)
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	if cipherDigest.SHA256 != proof.CiphertextSHA256 {
		return EncryptedBackupReadback{}, fmt.Errorf("ciphertext checksum mismatch: got %s want %s", cipherDigest.SHA256, proof.CiphertextSHA256)
	}
	nonce, err := hex.DecodeString(proof.NonceHex)
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	ciphertext, err := os.ReadFile(proof.CiphertextPath)
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(strings.TrimSpace(proof.KeyID)))
	if err != nil {
		return EncryptedBackupReadback{}, err
	}
	plainHash := bareSHA256(plain)
	if plainHash != proof.PlaintextSHA256 {
		return EncryptedBackupReadback{}, fmt.Errorf("plaintext checksum mismatch: got %s want %s", plainHash, proof.PlaintextSHA256)
	}
	return EncryptedBackupReadback{Plaintext: plain, PlaintextSHA256: plainHash, PlaintextBytes: int64(len(plain))}, nil
}

func bareSHA256(payload []byte) string {
	return strings.TrimPrefix(auditBytesHash(payload), "sha256:")
}
