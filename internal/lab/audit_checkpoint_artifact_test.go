package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditCheckpointArtifactRoundTripVerifiesSQLiteTail(t *testing.T) {
	dbPath := buildAuditHistory(t)
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	signer := NewDeterministicAuditCheckpointSigner("psc-rm-084-synthetic-lab", []byte("psc-rm-084 audit checkpoint test key"))
	artifactPath := filepath.Join(t.TempDir(), "audit-checkpoint.json")
	written, err := store.WriteAuditCheckpointArtifact(artifactPath, signer)
	if err != nil {
		t.Fatalf("write checkpoint artifact: %v", err)
	}
	if written.SchemaVersion != AuditCheckpointArtifactSchemaVersion || written.Sequence != 3 || written.TailHash == "" || written.Checksum == "" || written.Signature == "" {
		t.Fatalf("checkpoint artifact missing required proof fields: %#v", written)
	}

	read, err := ReadAuditCheckpointArtifact(artifactPath)
	if err != nil {
		t.Fatalf("read checkpoint artifact: %v", err)
	}
	if read.Checksum != written.Checksum || read.Signature != written.Signature {
		t.Fatalf("read checkpoint artifact changed proof material: read=%#v written=%#v", read, written)
	}
	if err := store.VerifyAuditCheckpointArtifact(read); err != nil {
		t.Fatalf("verify checkpoint artifact against store: %v", err)
	}
}

func TestAuditCheckpointArtifactDetectsArtifactTampering(t *testing.T) {
	dbPath := buildAuditHistory(t)
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	signer := NewDeterministicAuditCheckpointSigner("psc-rm-084-synthetic-lab", []byte("psc-rm-084 audit checkpoint test key"))
	artifact, err := store.GenerateAuditCheckpointArtifact(signer)
	if err != nil {
		t.Fatalf("generate checkpoint artifact: %v", err)
	}
	artifact.TailHash = strings.Repeat("0", 64)

	if err := store.VerifyAuditCheckpointArtifact(artifact); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch for tampered checkpoint artifact, got %v", err)
	}
}

func TestAuditCheckpointArtifactDetectsRegeneratedSQLiteChainMismatch(t *testing.T) {
	dbPath := buildAuditHistory(t)
	store, err := OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	signer := NewDeterministicAuditCheckpointSigner("psc-rm-084-synthetic-lab", []byte("psc-rm-084 audit checkpoint test key"))
	artifact, err := store.GenerateAuditCheckpointArtifact(signer)
	if err != nil {
		t.Fatalf("generate checkpoint artifact: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	tampered, err := OpenSQLiteStoreWithoutVerification(dbPath)
	if err != nil {
		t.Fatalf("open tamper store: %v", err)
	}
	events, err := tampered.AuditEvents(0)
	if err != nil {
		t.Fatalf("read audit events for regeneration: %v", err)
	}
	regenerated := events[len(events)-1]
	regenerated.Action = "client.regenerated"
	regenerated.Hash = hashEvent(regenerated)
	if _, err := tampered.db.Exec(`UPDATE audit_events SET action = ?, hash = ? WHERE sequence = ?`, regenerated.Action, regenerated.Hash, regenerated.Sequence); err != nil {
		t.Fatalf("tamper audit tail: %v", err)
	}
	if _, err := tampered.db.Exec(`UPDATE audit_checkpoints SET hash = ? WHERE name = 'latest'`, regenerated.Hash); err != nil {
		t.Fatalf("tamper sqlite checkpoint: %v", err)
	}
	if err := tampered.Close(); err != nil {
		t.Fatalf("close tamper store: %v", err)
	}

	tampered, err = OpenSQLiteStoreWithoutVerification(dbPath)
	if err != nil {
		t.Fatalf("reopen tampered store: %v", err)
	}
	defer tampered.Close()
	if err := tampered.VerifyAuditCheckpointArtifact(artifact); err == nil || !strings.Contains(err.Error(), "checkpoint artifact mismatch") {
		t.Fatalf("expected checkpoint artifact mismatch for regenerated SQLite chain, got %v", err)
	}
}
