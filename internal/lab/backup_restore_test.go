package lab

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupSQLiteStoreCreatesRestorableManifestWithAuditTailAndFiles(t *testing.T) {
	tmp := t.TempDir()
	sourceDB := filepath.Join(tmp, "source", "project-scientist.db")
	store, err := OpenSQLiteStore(sourceDB)
	if err != nil {
		t.Fatalf("open source store: %v", err)
	}
	actor := testActor("backup-operator")
	client, err := store.CreateClient("Backup Proof Client", "backup@example.test", actor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Restore Drill", Matrix: "Water", Tests: []string{"pH"}}, actor); err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close source store: %v", err)
	}

	configPath := filepath.Join(tmp, "config", "project-scientist.env")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("PSC_ADDR=:8080\nPSC_DATA_DIR=/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(tmp, "artifacts", "sample-report.txt")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("synthetic lab-test artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := BackupSQLiteStore(context.Background(), sourceDB, filepath.Join(tmp, "backup"), []string{configPath}, []string{artifactPath})
	if err != nil {
		t.Fatalf("backup store: %v", err)
	}
	if manifest.DatabaseSHA256 == "" {
		t.Fatalf("database checksum was not recorded: %#v", manifest)
	}
	if manifest.AuditEventCount != 2 || manifest.AuditTailSequence != 2 || manifest.AuditTailHash == "" {
		t.Fatalf("audit tail not captured: %#v", manifest)
	}
	if len(manifest.ConfigFiles) != 1 || len(manifest.ArtifactFiles) != 1 {
		t.Fatalf("config/artifact manifest incomplete: %#v", manifest)
	}
	if _, err := os.Stat(filepath.Join(tmp, "backup", "config", "project-scientist.env")); err != nil {
		t.Fatalf("config file was not copied into backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "backup", "artifacts", "sample-report.txt")); err != nil {
		t.Fatalf("artifact file was not copied into backup: %v", err)
	}

	restore, err := VerifyBackupRestore(context.Background(), manifest)
	if err != nil {
		t.Fatalf("verify restore: %v", err)
	}
	if restore.AuditEventCount != manifest.AuditEventCount || restore.AuditTailHash != manifest.AuditTailHash {
		t.Fatalf("restore audit tail mismatch: got %#v want %#v", restore, manifest)
	}
}

func TestBackupRestoreProofScriptUsesAuthorizedSyntheticActor(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	script := filepath.Join(root, "scripts", "backup-restore-proof.sh")
	proofDir := filepath.Join(root, "tmp", "backup-restore-proof-test", t.Name())
	if err := os.RemoveAll(proofDir); err != nil {
		t.Fatalf("clean proof directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(proofDir) })

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("backup restore proof script requires bash: %v", err)
	}
	cmd := exec.Command(bashPath, script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PSC_BACKUP_PROOF_DIR="+proofDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup restore proof script failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "backup/restore proof ok:") {
		t.Fatalf("proof script did not report success: %s", output)
	}
}

func TestVerifyBackupRestoreRejectsTamperedAuditChain(t *testing.T) {
	tmp := t.TempDir()
	sourceDB := filepath.Join(tmp, "source.db")
	store, err := OpenSQLiteStore(sourceDB)
	if err != nil {
		t.Fatalf("open source store: %v", err)
	}
	if _, err := store.CreateClient("Tamper Client", "tamper@example.test", testActor("backup-operator")); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close source store: %v", err)
	}
	manifest, err := BackupSQLiteStore(context.Background(), sourceDB, filepath.Join(tmp, "backup"), nil, nil)
	if err != nil {
		t.Fatalf("backup store: %v", err)
	}

	tampered, err := OpenSQLiteStoreWithoutVerification(manifest.BackupDatabase)
	if err != nil {
		t.Fatalf("open tampered backup: %v", err)
	}
	if _, err := tampered.db.ExecContext(context.Background(), `UPDATE audit_events SET action = 'client.tampered' WHERE sequence = 1`); err != nil {
		t.Fatalf("tamper backup audit: %v", err)
	}
	if err := tampered.Close(); err != nil {
		t.Fatalf("close tampered backup: %v", err)
	}

	if _, err := VerifyBackupRestore(context.Background(), manifest); err == nil {
		t.Fatal("VerifyBackupRestore succeeded after audit tampering")
	}
}
