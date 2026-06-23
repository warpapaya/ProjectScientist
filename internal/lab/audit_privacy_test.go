package lab

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditRetentionHonorsWindowAndLegalHold(t *testing.T) {
	policy := DefaultAuditPrivacyPolicy()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	expired := AuditEvent{EventID: "evt-old", TenantID: DefaultTenantID, LabID: DefaultLabID, Timestamp: now.AddDate(0, 0, -policy.RetentionDays-1), Sequence: 7}

	decision := EvaluateAuditRetention(policy, expired, now, nil)
	if decision.Action != AuditRetentionPurge || decision.Reason != "retention_window_expired" {
		t.Fatalf("expired event should be purged absent hold: %#v", decision)
	}

	hold := AuditLegalHold{ID: "hold-psc-rm-084", Reason: "synthetic lab security review", EventIDs: []string{"evt-old"}, EffectiveAt: now.Add(-time.Hour)}
	decision = EvaluateAuditRetention(policy, expired, now, []AuditLegalHold{hold})
	if decision.Action != AuditRetentionHold || decision.HoldID != hold.ID || !strings.Contains(decision.Reason, "legal_hold") {
		t.Fatalf("legal hold should override purge: %#v", decision)
	}
}

func TestAuditRedactionWorkflowPreservesNonSecretProvenance(t *testing.T) {
	event := AuditEvent{
		EventID:   "evt-redact-1",
		TenantID:  DefaultTenantID,
		LabID:     DefaultLabID,
		Timestamp: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
		Actor:     "analyst-1",
		ActorContext: ActorContext{
			UserID:              "analyst-1",
			DisplayNameSnapshot: "Ada Analyst",
			AuthProvider:        "lab-dev",
			Roles:               []string{"analyst"},
			RequestID:           "req-redact-1",
			CorrelationID:       "req-redact-1",
		},
		Resource: AuditResource{Type: "sample", ID: "sample-secret-123"},
		Action:   "sample.imported",
		Outcome:  AuditOutcomeAllowed,
		Sequence: 11,
		Details: map[string]any{
			"source":       "file:///tmp/import.csv?token=secret",
			"payload_hash": "sha256:abc123",
			"client_name":  "Customer Nobody Should See",
			"raw_payload":  "secret raw data",
		},
	}

	redacted := RedactAuditEventForDisclosure(event, AuditRedactionRequest{RequestedBy: "privacy-officer", Reason: "support packet minimization"})
	if redacted.EventID != event.EventID || redacted.Sequence != event.Sequence || redacted.TenantID != event.TenantID || redacted.LabID != event.LabID {
		t.Fatalf("redaction changed audit provenance: %#v", redacted)
	}
	if redacted.Actor != AuditRedactedValue || redacted.ActorContext.DisplayNameSnapshot != AuditRedactedValue || redacted.ActorContext.UserID != AuditRedactedValue {
		t.Fatalf("actor context was not redacted: %#v", redacted.ActorContext)
	}
	if _, ok := redacted.Details["raw_payload"]; ok {
		t.Fatalf("raw payload leaked after redaction: %#v", redacted.Details)
	}
	if _, ok := redacted.Details["client_name"]; ok {
		t.Fatalf("client name leaked after redaction: %#v", redacted.Details)
	}
	if redacted.Details["payload_hash"] != "sha256:abc123" {
		t.Fatalf("safe payload hash was not preserved: %#v", redacted.Details)
	}
	if source, ok := redacted.Details["source"].(string); !ok || strings.Contains(source, "token=") {
		t.Fatalf("source query secret leaked: %#v", redacted.Details)
	}
	if redacted.Redaction == nil || redacted.Redaction.Reason == "" || redacted.Redaction.OriginalEventHash == "" {
		t.Fatalf("redaction metadata missing: %#v", redacted.Redaction)
	}
}

func TestEncryptedBackupProofRoundTripAndTamperDetection(t *testing.T) {
	tmp := t.TempDir()
	plainPath := filepath.Join(tmp, "backup-manifest.json")
	plain := []byte(`{"scope":"synthetic-lab-test","database_sha256":"sha256:abc123"}`)
	if err := os.WriteFile(plainPath, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x42}, 32)

	proof, err := EncryptBackupFile(plainPath, filepath.Join(tmp, "backup-manifest.json.age-local"), key, "synthetic-local-aes-gcm")
	if err != nil {
		t.Fatalf("encrypt backup file: %v", err)
	}
	if proof.Algorithm != "AES-256-GCM" || proof.PlaintextSHA256 == "" || proof.CiphertextSHA256 == "" || proof.KeyID != "synthetic-local-aes-gcm" {
		t.Fatalf("encryption proof incomplete: %#v", proof)
	}

	readback, err := VerifyEncryptedBackupReadback(proof, key)
	if err != nil {
		t.Fatalf("verify encrypted readback: %v", err)
	}
	if string(readback.Plaintext) != string(plain) || readback.PlaintextSHA256 != proof.PlaintextSHA256 {
		t.Fatalf("restore/readback mismatch: %#v", readback)
	}

	ciphertext, err := os.ReadFile(proof.CiphertextPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 0x01
	if err := os.WriteFile(proof.CiphertextPath, ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyEncryptedBackupReadback(proof, key); err == nil {
		t.Fatal("tampered encrypted backup verified successfully")
	}
}
