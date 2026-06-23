# Project Scientist backup/restore proof

Generated: 20260623T052213Z
Scope: local/dev/staging-shaped lab-test drill only; synthetic data only; no customer/prod mutation.

Artifacts:
- Result JSON: docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/proof-result.json
- Backup manifest: docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/backup/backup-manifest.json
- SQLite snapshot: docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/backup/project-scientist.db
- Config copy: docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/backup/config/project-scientist.env
- Artifact copy: docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/backup/artifacts/synthetic-coa.txt

RPO assumption: the scripted SQLite snapshot is point-in-time; expected data loss is bounded by time since the last completed backup snapshot.
RTO assumption: restore is copying the SQLite DB/config/artifacts to a clean data directory and passing startup audit-chain verification before serving writes.

Verification performed:
- Created synthetic client/sample/workflow data.
- Captured SQLite database with VACUUM INTO.
- Copied local config and synthetic artifact files.
- Wrote manifest with SHA-256 checksums and audit tail sequence/hash.
- Re-opened the restored snapshot through OpenSQLiteStore, which verifies the audit chain and checkpoint before serving writes.
