#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${PSC_BACKUP_PROOF_DIR:-$ROOT/tmp/backup-restore-proof/$STAMP}"
DATA_DIR="$OUT_DIR/source-data"
BACKUP_DIR="$OUT_DIR/backup"
CONFIG_DIR="$OUT_DIR/config"
ARTIFACT_DIR="$OUT_DIR/artifacts"
RUNNER="$OUT_DIR/backup_restore_proof_runner.go"
RESULT_JSON="$OUT_DIR/proof-result.json"
SUMMARY_MD="$OUT_DIR/proof-summary.md"

cleanup() {
  rm -f "$RUNNER"
}
trap cleanup EXIT

mkdir -p "$DATA_DIR" "$BACKUP_DIR" "$CONFIG_DIR" "$ARTIFACT_DIR"

cat > "$CONFIG_DIR/project-scientist.env" <<'CONFIG'
PSC_ADDR=:8080
PSC_DATA_DIR=/data
PSC_ENABLE_DEMO_RESET=false
PSC_SYNTHETIC_FIXTURE_PATH=/app/fixtures/mvp_synthetic_lab.json
CONFIG

cat > "$ARTIFACT_DIR/synthetic-coa.txt" <<'ARTIFACT'
Synthetic lab-test artifact for backup/restore proof.
No customer data. No production mutation.
ARTIFACT

cat > "$RUNNER" <<'GO'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/warpapaya/ProjectScientist/internal/lab"
)

type proofResult struct {
	ProofStartedAt string                  `json:"proof_started_at"`
	Scope          string                  `json:"scope"`
	RPO            string                  `json:"rpo"`
	RTO            string                  `json:"rto"`
	Manifest       lab.BackupManifest      `json:"manifest"`
	Restore        lab.RestoreVerification `json:"restore"`
}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintf(os.Stderr, "usage: %s <data-dir> <backup-dir> <config-path> <artifact-path>\n", os.Args[0])
		os.Exit(2)
	}
	dataDir, backupDir, configPath, artifactPath := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	dbPath := filepath.Join(dataDir, "project-scientist.db")
	ctx := context.Background()
	store, err := lab.OpenSQLiteStore(dbPath)
	must("open source store", err)
	actor := lab.MustActorContext(lab.ActorContextInput{
		UserID: "backup-proof-operator", DisplayName: "Backup Proof Operator", AuthProvider: "local-dev",
		RequestID: "backup-restore-proof", CorrelationID: "backup-restore-proof",
		TenantMemberships: []lab.TenantMembership{{TenantID: lab.DefaultTenantID}},
	})
	client, err := store.CreateClient("Okefenokee Backup Proof Lab", "backup-proof@example.test", actor)
	must("create proof client", err)
	sample, err := store.CreateSample(lab.CreateSampleInput{ClientID: client.ID, Project: "Backup Restore Drill", Matrix: "Water", Tests: []string{"pH", "TSS"}}, actor)
	must("create proof sample", err)
	must("transition proof sample", store.TransitionSample(sample.ID, lab.StatusInPrep, actor))
	must("close source store", store.Close())

	manifest, err := lab.BackupSQLiteStore(ctx, dbPath, backupDir, []string{configPath}, []string{artifactPath})
	must("backup sqlite store", err)
	restore, err := lab.VerifyBackupRestore(ctx, manifest)
	must("verify restore", err)

	result := proofResult{
		ProofStartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Scope: "local/dev/staging-shaped lab-test drill; synthetic data only; no customer/prod mutation",
		RPO: "SQLite snapshot via VACUUM INTO after writes are closed: expected data loss is bounded by time since the last completed proof/backup snapshot.",
		RTO: "Restore target is a copied SQLite DB plus config/artifact files; lab-test target is operator-verified startup/audit-chain validation before serving writes.",
		Manifest: manifest,
		Restore: restore,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must("write result", enc.Encode(result))
}

func must(step string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
		os.Exit(1)
	}
}
GO

go run "$RUNNER" "$DATA_DIR" "$BACKUP_DIR" "$CONFIG_DIR/project-scientist.env" "$ARTIFACT_DIR/synthetic-coa.txt" | tee "$RESULT_JSON" >/dev/null

cat > "$SUMMARY_MD" <<EOF
# Project Scientist backup/restore proof

Generated: $STAMP
Scope: local/dev/staging-shaped lab-test drill only; synthetic data only; no customer/prod mutation.

Artifacts:
- Result JSON: $RESULT_JSON
- Backup manifest: $BACKUP_DIR/backup-manifest.json
- SQLite snapshot: $BACKUP_DIR/project-scientist.db
- Config copy: $BACKUP_DIR/config/project-scientist.env
- Artifact copy: $BACKUP_DIR/artifacts/synthetic-coa.txt

RPO assumption: the scripted SQLite snapshot is point-in-time; expected data loss is bounded by time since the last completed backup snapshot.
RTO assumption: restore is copying the SQLite DB/config/artifacts to a clean data directory and passing startup audit-chain verification before serving writes.

Verification performed:
- Created synthetic client/sample/workflow data.
- Captured SQLite database with VACUUM INTO.
- Copied local config and synthetic artifact files.
- Wrote manifest with SHA-256 checksums and audit tail sequence/hash.
- Re-opened the restored snapshot through OpenSQLiteStore, which verifies the audit chain and checkpoint before serving writes.
EOF

printf 'backup/restore proof ok: %s\n' "$OUT_DIR"
printf 'result: %s\n' "$RESULT_JSON"
printf 'summary: %s\n' "$SUMMARY_MD"
