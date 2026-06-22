# PSC-RM-001 Storage Architecture: SQLite Transaction Spine

Status: implemented for the lab-test lane. This does not approve customer data, production deployment, external exposure, or customer-facing claims.

## Decision

Project Scientist uses SQLite as the first production-shaped persistence spine.

SQLite wins for the current RED/ORANGE lab-test stage because it gives the app a real transactional database without creating a service dependency or operational surface area before the product shape is proven. The current goal is to prove LIMS domain semantics, auditability, migrations, backup/restore, and workflow boundaries locally. A networked database is premature until concurrency, tenancy, and hosting constraints are measured.

## Why SQLite before Postgres

SQLite is the right default now:

- One local database file is easy to inspect, back up, copy, and reset during architecture pressure-testing.
- ACID transactions let domain state and audit events commit or roll back together.
- WAL mode is enough for the current local single-process app and Docker smoke lane.
- Embedded migrations keep startup deterministic without adding a migration service.
- The repository boundary keeps the later Postgres path open.

Postgres becomes the right call when at least one of these is true:

- Multiple app instances need concurrent writes against the same tenant data.
- Hosting requires managed backups, point-in-time recovery, read replicas, or stronger operational controls than a file DB provides.
- Tenant isolation moves to separate schemas/databases or needs central database administration.
- Query/reporting workload outgrows the simple local DB pattern.
- External integrations require stronger transactional concurrency than SQLite can comfortably provide.

## Implementation shape

The storage spine is in `internal/lab/store.go`.

- `OpenSQLiteStore(path)` opens a SQLite database file, applies embedded schema migrations, and verifies the audit hash chain before serving writes.
- Domain tables currently include `clients` and `samples`.
- `audit_events` stores the append-only hash-chained event stream.
- `store_meta` stores monotonic counters and the current `last_hash`.
- Mutating commands (`CreateClient`, `CreateSample`, `TransitionSample`) run inside one database transaction.
- Each mutation writes domain state and its audit event before commit.
- If the audit insert fails, the domain write rolls back with it.
- Startup fails closed when audit verification detects tampering or sequence/hash damage.

The current schema is embedded as Go migration statements. That is intentional for this stage: it keeps the app one-binary/simple-Docker while still giving us a real migration surface (`schema_migrations`). If schema churn accelerates, move migrations to versioned `.sql` files before adding more domain breadth.

## Dependency decision

The app adds one external dependency: `github.com/mattn/go-sqlite3`.

Justification:

- Go's standard library has `database/sql` interfaces but no SQLite driver.
- `mattn/go-sqlite3` is the established driver for SQLite-backed Go apps.
- Citadel/macOS and the Docker image both support CGO for this local lab-test lane.
- Avoiding a driver would force shelling out to `sqlite3` or writing C bindings by hand, which is more fragile than one explicit database driver dependency.

Keep this dependency under review if cross-compilation or distroless deployment becomes a priority. At that point, compare `modernc.org/sqlite` against the CGO operational cost.

## Repository boundary

The current store exposes a small repository-style API:

- `CreateClient`
- `CreateSample`
- `TransitionSample`
- `GetSample`
- `Clients`
- `Samples`
- `AuditEvents`
- `Close`

That boundary is deliberately narrow. Future Phase 1 work should split command handlers, authenticated actor context, tenant scoping, and authorization policy from persistence rather than letting HTTP handlers write SQL directly.

## Rollback-safe startup

Startup behavior is fail-closed:

1. Open DB.
2. Apply embedded migrations idempotently.
3. Read audit events in sequence order.
4. Verify sequence is contiguous starting at 1.
5. Verify each `previous_hash` matches the prior event hash.
6. Recompute each event hash from canonical JSON fields.
7. Refuse to open the store if verification fails.

This is not the final audit gate. It does not yet provide external checkpoints, tenant-aware streams, denied-event schema, authenticated actor context, or operator recovery tooling. It does close the immediate bootstrap flaw where JSON state and JSONL audit could split.

## Tests

The SQLite spine is covered by TDD tests in `internal/lab/store_sqlite_test.go`:

- `TestSQLiteStoreCommitsDomainStateAndAuditAtomically` installs an audit-failure trigger and proves a client row does not commit without its audit event.
- `TestSQLiteStorePersistsDomainStateAndHashChainedAudit` proves state and audit survive close/reopen and preserve hash chaining.
- `TestSQLiteStoreRefusesStartupWhenAuditHashChainIsDamaged` tampers with an audit row and proves verified startup fails closed.

Existing workflow tests in `internal/lab/store_test.go` now run against SQLite instead of the retired JSON file-store prototype.

## Current stop lines

Still lab-test only. Do not use this with customer data or customer-facing claims until the security gates in `docs/security-audit-model.md` are implemented and reviewed.

Known remaining gaps:

- No authenticated actor context.
- No tenant/lab scoping.
- No RBAC/ABAC.
- No denial/failed-operation audit events.
- No external/signed checkpoints.
- No backup/restore drill.
- No migration/import provenance.
