# Project Scientist local operations runbook

Project Scientist remains a lab-test/local prototype lane. These commands are for synthetic local data only. Do not point them at customer, production, shared-hosting, public-demo, or SENAITE data.

## Binary command surface

All commands are available through the checked-in Go binary:

```bash
go run ./cmd/project-scientist <command>
```

The default database path is `data/project-scientist.db` unless `--db` is provided or `PSC_DATA_DIR` changes the default data directory.

### Audit verification

Verify the append-only audit hash chain before treating a local DB as usable:

```bash
go run ./cmd/project-scientist audit verify --db data/project-scientist.db
```

Expected output includes:

```text
audit verify ok
```

A hash/sequence mismatch exits non-zero and blocks startup-style verification.

### Database migrate/status

Run SQLite migrations explicitly:

```bash
go run ./cmd/project-scientist db migrate --db data/project-scientist.db
```

Check schema/data counts:

```bash
go run ./cmd/project-scientist db status --db data/project-scientist.db
```

Expected output shape:

```text
db status db=data/project-scientist.db schema_version=2 clients=0 samples=0 audit_events=0
```

### Seed/reset

Seed deterministic synthetic demo data directly into the local SQLite database:

```bash
go run ./cmd/project-scientist seed --db data/project-scientist.db
```

Reset is destructive and requires an explicit force flag:

```bash
go run ./cmd/project-scientist reset --db data/project-scientist.db --force
```

Reset removes the SQLite DB plus `-wal` and `-shm` sidecars, then recreates an empty migrated DB. Use only for local prototype data.

### Backup/restore

Create a consistent SQLite backup using `VACUUM INTO`:

```bash
mkdir -p var/backups
go run ./cmd/project-scientist backup --db data/project-scientist.db --out var/backups/project-scientist.db
```

Restore verifies the backup audit chain before overwriting the target DB. Restore is destructive and requires `--force`:

```bash
go run ./cmd/project-scientist restore --db data/project-scientist.db --backup var/backups/project-scientist.db --force
```

### HTTP smoke

Validate a running local server:

```bash
go run ./cmd/project-scientist smoke --base-url http://127.0.0.1:8097
```

The smoke command checks `/healthz` returns `ok` and `/api/state` returns a 2xx response.

## Make wrappers

Convenience wrappers default to `PSC_DB=data/project-scientist.db` and `PSC_BACKUP=var/backups/project-scientist.db`:

```bash
make ops-db-migrate
make ops-db-status
make ops-seed
make ops-audit-verify
make ops-backup
make ops-reset
make ops-restore
make ops-smoke
```

Override paths explicitly when needed:

```bash
make ops-backup PSC_DB=/tmp/psc.db PSC_BACKUP=/tmp/psc-backup.db
```

## Logs and metrics plan

Current implemented observability:

- `/healthz` for container and HTTP smoke checks.
- Structured-ish operator command output with stable tokens: `audit verify ok`, `db status ...`, `backup ok`, `restore ok`, `smoke ok`.
- Server startup log includes the listen address.
- Docker healthcheck polls `/healthz` inside the runtime container.

Near-term metrics/logging plan before any production lane exists:

1. Add one request log line per HTTP request with method, path, status, duration, tenant_id, lab_id, and actor when present. Keep values non-sensitive.
2. Add `/metrics` in Prometheus text format using the standard library only unless a later task approves a dependency. Initial counters/gauges: request count by route/status, request duration buckets, audit verification failures, SQLite open failures, client/sample counts by local scope.
3. Add command duration and target DB path to operator command logs; never log sample/client payloads or customer identifiers.
4. Keep health distinct from readiness: `/healthz` stays shallow, future `/readyz` may verify DB open + audit chain.
5. Wire Docker/local smoke to check health and API state only; no public exposure, customer data, auth proxy, or production claims in this lane.
