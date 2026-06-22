package lab

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BackupFile struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type BackupManifest struct {
	CreatedAt         time.Time       `json:"created_at"`
	SourceDatabase    string          `json:"source_database"`
	BackupDirectory   string          `json:"backup_directory"`
	BackupDatabase    string          `json:"backup_database"`
	DatabaseSHA256    string          `json:"database_sha256"`
	DatabaseBytes     int64           `json:"database_bytes"`
	AuditEventCount   int             `json:"audit_event_count"`
	AuditTailSequence int64           `json:"audit_tail_sequence"`
	AuditTailHash     string          `json:"audit_tail_hash"`
	Checkpoint        AuditCheckpoint `json:"checkpoint"`
	ConfigFiles       []BackupFile    `json:"config_files,omitempty"`
	ArtifactFiles     []BackupFile    `json:"artifact_files,omitempty"`
	ManifestPath      string          `json:"manifest_path"`
}

type RestoreVerification struct {
	BackupDatabase    string `json:"backup_database"`
	DatabaseSHA256    string `json:"database_sha256"`
	AuditEventCount   int    `json:"audit_event_count"`
	AuditTailSequence int64  `json:"audit_tail_sequence"`
	AuditTailHash     string `json:"audit_tail_hash"`
}

func BackupSQLiteStore(ctx context.Context, sourceDBPath, backupDir string, configPaths, artifactPaths []string) (BackupManifest, error) {
	if strings.TrimSpace(sourceDBPath) == "" {
		return BackupManifest{}, fmt.Errorf("source database path is required")
	}
	if strings.TrimSpace(backupDir) == "" {
		return BackupManifest{}, fmt.Errorf("backup directory is required")
	}
	sourceAbs, err := filepath.Abs(sourceDBPath)
	if err != nil {
		return BackupManifest{}, err
	}
	backupAbs, err := filepath.Abs(backupDir)
	if err != nil {
		return BackupManifest{}, err
	}
	if err := os.MkdirAll(backupAbs, 0o755); err != nil {
		return BackupManifest{}, err
	}
	backupDB := filepath.Join(backupAbs, "project-scientist.db")
	if err := os.Remove(backupDB); err != nil && !os.IsNotExist(err) {
		return BackupManifest{}, err
	}

	db, err := sql.Open("sqlite3", sourceAbs+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return BackupManifest{}, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "VACUUM main INTO "+sqliteStringLiteral(backupDB)); err != nil {
		return BackupManifest{}, fmt.Errorf("snapshot sqlite database: %w", err)
	}

	store, err := OpenSQLiteStore(backupDB)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("verify backup audit chain: %w", err)
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		_ = store.Close()
		return BackupManifest{}, err
	}
	checkpoint, _, err := store.latestCheckpoint()
	if err != nil {
		_ = store.Close()
		return BackupManifest{}, err
	}
	if err := store.Close(); err != nil {
		return BackupManifest{}, err
	}

	dbFile, err := fileDigest(backupDB)
	if err != nil {
		return BackupManifest{}, err
	}
	manifest := BackupManifest{
		CreatedAt:       time.Now().UTC(),
		SourceDatabase:  sourceAbs,
		BackupDirectory: backupAbs,
		BackupDatabase:  backupDB,
		DatabaseSHA256:  dbFile.SHA256,
		DatabaseBytes:   dbFile.Bytes,
		AuditEventCount: len(events),
		Checkpoint:      checkpoint,
		ManifestPath:    filepath.Join(backupAbs, "backup-manifest.json"),
	}
	if len(events) > 0 {
		manifest.AuditTailSequence = events[len(events)-1].Sequence
		manifest.AuditTailHash = events[len(events)-1].Hash
	}
	manifest.ConfigFiles, err = copyBackupFiles(configPaths, filepath.Join(backupAbs, "config"))
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.ArtifactFiles, err = copyBackupFiles(artifactPaths, filepath.Join(backupAbs, "artifacts"))
	if err != nil {
		return BackupManifest{}, err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupManifest{}, err
	}
	if err := os.WriteFile(manifest.ManifestPath, append(body, '\n'), 0o644); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

func VerifyBackupRestore(ctx context.Context, manifest BackupManifest) (RestoreVerification, error) {
	if manifest.BackupDatabase == "" {
		return RestoreVerification{}, fmt.Errorf("backup database path is required")
	}
	if digest, err := fileDigest(manifest.BackupDatabase); err != nil {
		return RestoreVerification{}, err
	} else if digest.SHA256 != manifest.DatabaseSHA256 {
		return RestoreVerification{}, fmt.Errorf("backup database checksum mismatch: got %s want %s", digest.SHA256, manifest.DatabaseSHA256)
	}
	store, err := OpenSQLiteStore(manifest.BackupDatabase)
	if err != nil {
		return RestoreVerification{}, err
	}
	defer store.Close()
	if err := store.VerifyAuditChain(); err != nil {
		return RestoreVerification{}, err
	}
	events, err := store.AuditEvents(0)
	if err != nil {
		return RestoreVerification{}, err
	}
	verification := RestoreVerification{BackupDatabase: manifest.BackupDatabase, DatabaseSHA256: manifest.DatabaseSHA256, AuditEventCount: len(events)}
	if len(events) > 0 {
		verification.AuditTailSequence = events[len(events)-1].Sequence
		verification.AuditTailHash = events[len(events)-1].Hash
	}
	if verification.AuditEventCount != manifest.AuditEventCount || verification.AuditTailSequence != manifest.AuditTailSequence || verification.AuditTailHash != manifest.AuditTailHash {
		return RestoreVerification{}, fmt.Errorf("restored audit tail mismatch")
	}
	if ctx.Err() != nil {
		return RestoreVerification{}, ctx.Err()
	}
	return verification, nil
}

func copyBackupFiles(paths []string, destDir string) ([]BackupFile, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	out := make([]BackupFile, 0, len(paths))
	for _, source := range paths {
		if strings.TrimSpace(source) == "" {
			continue
		}
		sourceAbs, err := filepath.Abs(source)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(sourceAbs)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("backup file %s is a directory", sourceAbs)
		}
		dest := filepath.Join(destDir, filepath.Base(sourceAbs))
		if err := copyFile(sourceAbs, dest); err != nil {
			return nil, err
		}
		digest, err := fileDigest(dest)
		if err != nil {
			return nil, err
		}
		out = append(out, BackupFile{Source: sourceAbs, Path: dest, SHA256: digest.SHA256, Bytes: digest.Bytes})
	}
	return out, nil
}

func copyFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileDigest(path string) (BackupFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return BackupFile{}, err
	}
	defer file.Close()
	h := sha256.New()
	bytes, err := io.Copy(h, file)
	if err != nil {
		return BackupFile{}, err
	}
	return BackupFile{Path: path, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: bytes}, nil
}

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
