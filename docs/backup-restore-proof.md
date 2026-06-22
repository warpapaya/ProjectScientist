# Backup/restore and audit verification proof

Status: lab-test/local-dev proof only. This does not approve Project Scientist for production, customer data, customer migration, external exposure, or customer-facing reliability claims.

## Scope

The proof covers a local/dev/staging-shaped deployment shape:

- SQLite application data: `project-scientist.db`.
- Audit stream and local checkpoint stored in SQLite.
- Deployment config captured as files, not secrets.
- Generated/report-like artifacts captured as files.

The proof intentionally uses synthetic data only and does not mutate customer or production systems.

## Scripted proof

Run from the repo root:

```bash
./scripts/backup-restore-proof.sh
```

Optional output location:

```bash
PSC_BACKUP_PROOF_DIR=/tmp/psc-backup-proof ./scripts/backup-restore-proof.sh
```

The script creates synthetic lab-test state, snapshots the SQLite database, copies config/artifact files, writes a manifest, and re-opens the restored snapshot through `OpenSQLiteStore`. Startup verification runs the audit-chain verifier before the restored store can serve writes.

Generated files under the proof directory:

- `backup/project-scientist.db` — SQLite snapshot created with `VACUUM INTO`.
- `backup/backup-manifest.json` — manifest with SHA-256 checksums, audit event count, checkpoint, tail sequence, and tail hash.
- `backup/config/project-scientist.env` — non-secret local config copy.
- `backup/artifacts/synthetic-coa.txt` — synthetic artifact copy proving artifact capture path.
- `proof-result.json` — machine-readable proof result.
- `proof-summary.md` — human-readable proof summary.

## RPO/RTO assumptions

RPO for this lab-test lane: data loss is bounded by time since the last completed SQLite snapshot. The snapshot is point-in-time and only counted as valid after the manifest is written and restore verification passes.

RTO for this lab-test lane: restore time is the time to copy `project-scientist.db`, config files, and artifact files into a clean data directory, then start the app and pass audit-chain verification. The current proof validates restore by opening the snapshot with `OpenSQLiteStore`, which fails closed on audit hash/checkpoint damage.

Production-candidate RPO/RTO is not claimed yet. Before any production use, define backup cadence, retention, off-host storage, encryption/key handling, external checkpoint anchoring, operator runbook steps, and measured restore timings on the target deployment shape.

## Audit restore checks

The backup API records:

- SHA-256 of the restored SQLite snapshot.
- Audit event count.
- Audit tail sequence.
- Audit tail hash.
- Current local checkpoint.

`VerifyBackupRestore` rejects a backup when:

- The database checksum differs from the manifest.
- `OpenSQLiteStore` fails startup audit-chain/checkpoint verification.
- The restored audit event count, tail sequence, or tail hash differs from the manifest.

This keeps the proof aligned with `docs/security-audit-model.md`: local checkpoints are a fast lab-test guard, not a replacement for future signed/external checkpoint anchoring.

## Current limitations / stop-lines

- Local SQLite snapshot only; no object storage, WORM retention, or cross-host replication yet.
- Config capture is file-based and must not include secrets.
- Artifact capture currently proves file-copy discipline with synthetic files; future report packages need explicit artifact inventory rules.
- No production mutation and no customer data are allowed in this proof.
- External checkpoint anchoring remains a future security gate.
