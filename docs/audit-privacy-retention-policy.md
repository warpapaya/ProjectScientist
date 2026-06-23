# PSC-RM-084 Audit Privacy, Retention, Redaction, and Backup Policy

Status: implemented lab-development proof for the synthetic Project Scientist lane. This is not a production/customer-data policy approval.

Scope: Project Scientist local lab-development only. The committed implementation and tests use synthetic data, synthetic operators, local files, and an in-process AES-GCM proof. No real credentials, no customer data, no external exposure, no deployment, and no production/customer claims are authorized by this document.

## Policy summary

Project Scientist audit events are tamper-evident operational records, but they are not a dumping ground. The durable policy is:

1. Retain audit events for 397 days by default in the synthetic lab lane.
2. Do not purge events under an active legal hold.
3. Redact audit disclosure copies instead of editing canonical audit events in place.
4. Preserve provenance fields during redaction: event id, tenant/lab id, timestamp, action, outcome, sequence, hashes, and safe detail hashes.
5. Strip or hash operator/customer-identifying fields and raw payload details before support/review disclosure.
6. Treat backups as encrypted-at-rest artifacts with manifest/readback proof before restore claims.
7. Restrict audit read/export expectations to admin, lab manager, security reviewer, and privacy officer roles.

## Retention windows

Default lab-development policy is `397` days. That is intentionally longer than one calendar year to allow year-close review plus delayed audit/security review.

Retention evaluation is implemented in `internal/lab/audit_privacy.go`:

- `DefaultAuditPrivacyPolicy()` returns the v1 policy.
- `EvaluateAuditRetention(policy, event, now, holds)` returns one of:
  - `keep` when the event remains inside the retention window.
  - `purge` when the retention window is expired and no legal hold applies.
  - `legal_hold` when a hold overrides purge.

The purge decision is a policy decision only in this lab-development slice. There is no destructive purge job and no production data mutation in PSC-RM-084.

## Legal hold behavior

A legal hold must include:

- hold id;
- reason;
- effective time;
- event ids and/or tenant/lab scope;
- optional expiration time.

Legal hold behavior is fail-closed for retention: if a hold applies to an expired event, `EvaluateAuditRetention` returns `legal_hold` and records the hold id. Holds with a blank id or reason are ignored because unverifiable holds should not silently block retention decisions.

## Redaction workflow

Canonical audit rows remain append-only/tamper-evident. Redaction creates a disclosure copy via `RedactAuditEventForDisclosure`; it does not rewrite the canonical event.

Redaction behavior:

- replaces `actor`, `actor_context.user_id`, `actor_context.display_name_snapshot`, request id, and correlation id with `[REDACTED]` in the disclosure copy;
- reuses the existing audit detail allowlist/sanitizer so safe provenance such as `payload_hash`, `source_hash`, row number, and sanitized source name can survive;
- removes raw payload, client/customer name, email, token, secret, authorization, legacy, and other unsafe detail keys when not explicitly allowlisted;
- attaches redaction metadata: requester, reason, UTC redaction time, policy name, and hash of the original event.

This preserves reviewability without pretending that a redacted packet is the original audit record.

## Encryption-at-rest and backup proof

PSC-RM-084 implements a local proof, not a production KMS design.

Implemented local proof:

- `EncryptBackupFile` encrypts a backup/manifest file using AES-256-GCM and a caller-provided synthetic 32-byte key.
- The proof records plaintext and ciphertext SHA256 digests, byte counts, algorithm, key id, nonce, creation time, synthetic-only scope, and customer-data status.
- `VerifyEncryptedBackupReadback` verifies ciphertext digest, decrypts with authenticated additional data bound to the key id, and checks plaintext digest before returning readback bytes.
- Tampered ciphertext fails verification.

Production design that remains explicitly out of scope:

- managed KMS/age recipient policy;
- key rotation;
- off-host/WORM storage;
- customer-data handling;
- restore into any externally exposed environment;
- destructive cleanup jobs.

## Restore/readback evidence

Restore/readback proof is considered valid only when all of these are true:

1. Backup manifest/database digest exists.
2. Audit chain verification passes on the backup or restored store.
3. Encrypted backup proof verifies ciphertext checksum and decrypts to the expected plaintext checksum.
4. The proof states whether plaintext was retained and confirms synthetic-only data status.
5. The operator records the command/test result before claiming restore readiness.

Current test evidence:

- `go test ./internal/lab -run TestBackupSQLiteStoreCreatesRestorableManifestWithAuditTailAndFiles` covers database backup manifest and audit-tail restore verification.
- `go test ./internal/lab -run TestVerifyBackupRestoreRejectsTamperedAuditChain` covers damaged backup rejection.
- `go test ./internal/lab -run TestEncryptedBackupProofRoundTripAndTamperDetection` covers local encrypted backup readback and tamper failure.

## Operator access expectations

Until a real authentication/session layer and production deployment model exists, the only acceptable operator posture is local synthetic lab-development:

- audit read/export belongs to admin, lab manager, security reviewer, or privacy officer roles;
- support packets receive redacted disclosure copies, never canonical raw event dumps;
- recovery operators use `OpenSQLiteStoreWithoutVerification` only for controlled tests or documented recovery drills, never normal serving paths;
- no real credentials or customer data are committed, pasted into audit details, or used in backup proofs;
- any future production/customer-data backup encryption requires Petie approval plus Aegis review.

## Limits and non-goals

This PSC-RM-084 slice does not:

- approve Project Scientist for production;
- approve customer data, real lab data, real credentials, or external exposure;
- implement a destructive retention purge worker;
- implement production KMS, WORM storage, or external checkpoint anchoring;
- claim compliance certification;
- replace Aegis/security review or Petie approval gates.
