# PSC-RM-084 audit checkpoint proof

Scope: Project Scientist lab-development only. This proof uses synthetic/local audit data and does not approve external staging, customer pilots, customer data handling, production readiness, DNS/auth/security changes, deployment, or any external service exposure.

## What was added

Project Scientist now has a repo-tracked internal checkpoint artifact mechanism outside the mutable primary SQLite rows:

- `Store.GenerateAuditCheckpointArtifact(signer)` reads the SQLite audit stream, verifies the existing hash chain, and creates a deterministic checkpoint payload from the audit event count, tail sequence, tail hash, and tail timestamp.
- `Store.WriteAuditCheckpointArtifact(path, signer)` writes that proof as JSON outside the database file.
- `ReadAuditCheckpointArtifact(path)` reads the JSON artifact back.
- `Store.VerifyAuditCheckpointArtifact(artifact)` verifies artifact checksum, Ed25519 signature, SQLite chain integrity, and that the current SQLite tail still matches the external artifact.
- `VerifyAuditCheckpointArtifactProof(artifact)` verifies the artifact's own checksum/signature without opening SQLite.

The checkpoint artifact contains:

- schema version and algorithm id (`ed25519-sha256-json-v1`),
- audit event count,
- tail sequence,
- tail audit hash,
- tail event timestamp,
- signer key id,
- signer public key,
- SHA-256 checksum of the canonical checkpoint payload,
- deterministic Ed25519 signature of the same payload.

## What this proves

The primary SQLite audit table already catches ordinary row edits, sequence gaps, duplicate sequences, malformed JSON, and local checkpoint-tail mismatch. The new artifact adds a second proof boundary: a checksum/signed JSON checkpoint that can be stored outside the database file. If a local writer mutates SQLite and regenerates the in-database hash/checkpoint tail, verification against the previously written artifact fails with a checkpoint artifact mismatch.

## What this does not prove

This is intentionally not a production-grade trust anchor:

- It does not use external object storage, WORM storage, KMS/HSM-backed signing, a real key rotation policy, or an independent timestamping authority.
- The deterministic signer helper is for synthetic lab tests and repeatable internal validation only; it is not production key custody.
- The JSON file is only as durable as the operator-controlled storage location chosen for it.
- This work does not approve external staging, customer data, live migration, production deployment, customer-facing readiness claims, or any security/auth/DNS/billing mutation.

## Required validation

The repo tests prove:

- artifact generation/write/read/verification succeeds against an untampered audit tail,
- direct artifact tampering fails checksum verification,
- a regenerated SQLite audit-chain tail fails when checked against a prior external artifact.

Run:

```bash
go test ./...
```
