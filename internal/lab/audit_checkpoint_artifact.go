package lab

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AuditCheckpointArtifactSchemaVersion = 1

// AuditCheckpointSigner is an internal synthetic-lab signer for checkpoint artifacts.
// It is not a production key-management boundary; production approval still requires
// external anchoring/key custody outside this local prototype.
type AuditCheckpointSigner struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// NewDeterministicAuditCheckpointSigner derives a deterministic Ed25519 signer for
// synthetic validation and repeatable tests. Do not treat this helper as production
// key management.
func NewDeterministicAuditCheckpointSigner(keyID string, seed []byte) AuditCheckpointSigner {
	digest := sha256.Sum256(seed)
	privateKey := ed25519.NewKeyFromSeed(digest[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return AuditCheckpointSigner{KeyID: strings.TrimSpace(keyID), PrivateKey: privateKey, PublicKey: publicKey}
}

type AuditCheckpointArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	Algorithm     string `json:"algorithm"`
	EventCount    int    `json:"event_count"`
	Sequence      int64  `json:"sequence"`
	TailHash      string `json:"tail_hash"`
	TailTimestamp string `json:"tail_timestamp"`
	SignerKeyID   string `json:"signer_key_id"`
	PublicKey     string `json:"public_key"`
	Checksum      string `json:"checksum"`
	Signature     string `json:"signature"`
}

type auditCheckpointPayload struct {
	SchemaVersion int    `json:"schema_version"`
	Algorithm     string `json:"algorithm"`
	EventCount    int    `json:"event_count"`
	Sequence      int64  `json:"sequence"`
	TailHash      string `json:"tail_hash"`
	TailTimestamp string `json:"tail_timestamp"`
	SignerKeyID   string `json:"signer_key_id"`
	PublicKey     string `json:"public_key"`
}

func (s *Store) GenerateAuditCheckpointArtifact(signer AuditCheckpointSigner) (AuditCheckpointArtifact, error) {
	if strings.TrimSpace(signer.KeyID) == "" {
		return AuditCheckpointArtifact{}, errors.New("checkpoint signer key id is required")
	}
	if len(signer.PrivateKey) != ed25519.PrivateKeySize || len(signer.PublicKey) != ed25519.PublicKeySize {
		return AuditCheckpointArtifact{}, errors.New("checkpoint signer requires Ed25519 private and public keys")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := auditEventsQuery(s.db, "", 0)
	if err != nil {
		return AuditCheckpointArtifact{}, err
	}
	if err := VerifyAuditEvents(events); err != nil {
		return AuditCheckpointArtifact{}, err
	}
	payload := auditCheckpointPayload{
		SchemaVersion: AuditCheckpointArtifactSchemaVersion,
		Algorithm:     "ed25519-sha256-json-v1",
		EventCount:    len(events),
		SignerKeyID:   strings.TrimSpace(signer.KeyID),
		PublicKey:     hex.EncodeToString(signer.PublicKey),
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		payload.Sequence = last.Sequence
		payload.TailHash = last.Hash
		payload.TailTimestamp = formatTime(last.Timestamp)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return AuditCheckpointArtifact{}, err
	}
	checksum := sha256.Sum256(encoded)
	signature := ed25519.Sign(signer.PrivateKey, encoded)
	return AuditCheckpointArtifact{
		SchemaVersion: payload.SchemaVersion,
		Algorithm:     payload.Algorithm,
		EventCount:    payload.EventCount,
		Sequence:      payload.Sequence,
		TailHash:      payload.TailHash,
		TailTimestamp: payload.TailTimestamp,
		SignerKeyID:   payload.SignerKeyID,
		PublicKey:     payload.PublicKey,
		Checksum:      hex.EncodeToString(checksum[:]),
		Signature:     hex.EncodeToString(signature),
	}, nil
}

func (s *Store) WriteAuditCheckpointArtifact(path string, signer AuditCheckpointSigner) (AuditCheckpointArtifact, error) {
	artifact, err := s.GenerateAuditCheckpointArtifact(signer)
	if err != nil {
		return AuditCheckpointArtifact{}, err
	}
	if strings.TrimSpace(path) == "" {
		return AuditCheckpointArtifact{}, errors.New("checkpoint artifact path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return AuditCheckpointArtifact{}, err
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return AuditCheckpointArtifact{}, err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return AuditCheckpointArtifact{}, err
	}
	return artifact, nil
}

func ReadAuditCheckpointArtifact(path string) (AuditCheckpointArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AuditCheckpointArtifact{}, err
	}
	var artifact AuditCheckpointArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return AuditCheckpointArtifact{}, err
	}
	return artifact, nil
}

func (s *Store) VerifyAuditCheckpointArtifact(artifact AuditCheckpointArtifact) error {
	if err := VerifyAuditCheckpointArtifactProof(artifact); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := auditEventsQuery(s.db, "", 0)
	if err != nil {
		return err
	}
	if err := VerifyAuditEvents(events); err != nil {
		return err
	}
	var sequence int64
	var tailHash, tailTimestamp string
	if len(events) > 0 {
		last := events[len(events)-1]
		sequence = last.Sequence
		tailHash = last.Hash
		tailTimestamp = formatTime(last.Timestamp)
	}
	if artifact.EventCount != len(events) || artifact.Sequence != sequence || artifact.TailHash != tailHash || artifact.TailTimestamp != tailTimestamp {
		return fmt.Errorf("checkpoint artifact mismatch: artifact count/sequence/hash/timestamp %d/%d/%s/%s does not match audit tail %d/%d/%s/%s", artifact.EventCount, artifact.Sequence, artifact.TailHash, artifact.TailTimestamp, len(events), sequence, tailHash, tailTimestamp)
	}
	return nil
}

func VerifyAuditCheckpointArtifactProof(artifact AuditCheckpointArtifact) error {
	payload, err := auditCheckpointArtifactPayload(artifact)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	checksum := sha256.Sum256(encoded)
	if artifact.Checksum != hex.EncodeToString(checksum[:]) {
		return fmt.Errorf("checksum mismatch for checkpoint artifact")
	}
	publicKey, err := hex.DecodeString(artifact.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("checkpoint artifact public key is invalid")
	}
	signature, err := hex.DecodeString(artifact.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("checkpoint artifact signature is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), encoded, signature) {
		return errors.New("checkpoint artifact signature verification failed")
	}
	return nil
}

func auditCheckpointArtifactPayload(artifact AuditCheckpointArtifact) (auditCheckpointPayload, error) {
	if artifact.SchemaVersion != AuditCheckpointArtifactSchemaVersion {
		return auditCheckpointPayload{}, fmt.Errorf("unsupported checkpoint artifact schema version %d", artifact.SchemaVersion)
	}
	if strings.TrimSpace(artifact.Algorithm) != "ed25519-sha256-json-v1" {
		return auditCheckpointPayload{}, fmt.Errorf("unsupported checkpoint artifact algorithm %q", artifact.Algorithm)
	}
	if strings.TrimSpace(artifact.SignerKeyID) == "" {
		return auditCheckpointPayload{}, errors.New("checkpoint artifact signer key id is required")
	}
	if artifact.EventCount < 0 || artifact.Sequence < 0 {
		return auditCheckpointPayload{}, errors.New("checkpoint artifact count/sequence cannot be negative")
	}
	if artifact.EventCount == 0 && (artifact.Sequence != 0 || artifact.TailHash != "" || artifact.TailTimestamp != "") {
		return auditCheckpointPayload{}, errors.New("empty checkpoint artifact cannot have a tail")
	}
	if artifact.EventCount > 0 && (artifact.Sequence <= 0 || strings.TrimSpace(artifact.TailHash) == "" || strings.TrimSpace(artifact.TailTimestamp) == "") {
		return auditCheckpointPayload{}, errors.New("checkpoint artifact tail sequence/hash/timestamp are required")
	}
	return auditCheckpointPayload{SchemaVersion: artifact.SchemaVersion, Algorithm: artifact.Algorithm, EventCount: artifact.EventCount, Sequence: artifact.Sequence, TailHash: artifact.TailHash, TailTimestamp: artifact.TailTimestamp, SignerKeyID: artifact.SignerKeyID, PublicKey: artifact.PublicKey}, nil
}
