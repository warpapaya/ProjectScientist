# PSC-005 Current-State Integration Review

Date: 2026-06-22 UTC
Task: t_a6318322
Source review: t_e3a9c1c5

## Verdict

PSC-005 is reconciled as current-state lab evidence only. The repository now has passing Go unit tests, passing `go vet`, and a Docker local smoke that starts `project-scientist:dev-local`, returns `ok` from `/healthz`, and shuts down cleanly.

This is not production-ready evidence. Scope remains lab-test/internal only: no customer/prod data, no external exposure, and no customer-facing readiness claim.

## Repository state

- HEAD: `6288d62 feat: add authenticated actor context`
- Branch observed: `feat/psc-rm-003-actor-context`
- Working tree is not clean; current WIP is deliberately treated as local lab evidence, not a release artifact.
- Current uncommitted/untracked state observed after verification:

```text
 M Dockerfile
?? .hermes-validation/
?? docs/PSC-005-current-state-review.md
?? docs/PSC-MVP-001-productionization-review.md
?? docs/PSC-MVP-001-productionization-review.pdf
?? docs/PSC-MVP-003-productionization-review.md
?? docs/PSC-MVP-003-productionization-review.pdf
?? internal/lab/audit_schema_test.go
?? internal/lab/test_helpers_test.go
```

## Reconciliation performed

The in-progress actor/audit work was reconciled enough for current-state acceptance evidence:

- Actor-context audit paths compile and run.
- Audit schema verifier tests compile against the current `AuditEvent` shape.
- Legacy/local Docker state issue was diagnosed during smoke: an old `project-scientist-dev` container from another worktree was holding the fixed name/port and reporting an older migration error. It was removed as stale local dev state, then the current worktree image started and passed healthcheck.
- The code path now includes v1-to-v2 SQLite audit/schema migration handling in `internal/lab/store.go` so existing local v1 data can acquire tenant/lab/audit context columns and recomputed local audit hashes/checkpoints.

## Verified artifacts from parent review

```text
20a7936f8140bf78791232799404721898beb2b77ce08c1a5b402b8ae071e540  docs/senaite-parity-roadmap.md
d58b1ee54b91224c14853245fc0f01d1aa21528ad373ad285857bce630073c40  docs/kanban-roadmap-tasks.json
003cc00601115116d2897440a1491c2a421bb4d38a856e52dc62aac7f3edf0ee  docs/mvp-test-scope.md
fbe7b40b0879aa561bb17b39eae1ba5d4344469e65bab8a91c01bd2a3f550673  docs/mvp-kanban-tasks.json
```

Card counts:

- `docs/kanban-roadmap-tasks.json`: 44 next-wave roadmap entries
- `docs/mvp-kanban-tasks.json`: 9 MVP closure entries

## Verification commands and exact outputs

Full raw log:

- `.hermes-validation/psc-005-verification-final-20260622T192226Z.log`
- sha256: `900fac16df270ad20083a7f7c62217506a6aad3d8aca145859b2be0c1077e9c1`

Key excerpts:

```text
$ go test ./...
ok  	github.com/warpapaya/ProjectScientist/cmd/project-scientist	(cached)
ok  	github.com/warpapaya/ProjectScientist/internal/lab	(cached)

$ go vet ./...

$ docker compose config --quiet
docker compose config --quiet: exit 0

$ curl -fsS http://127.0.0.1:8097/healthz
ok

$ docker compose ps
NAME                    IMAGE                         COMMAND                  SERVICE             CREATED         STATUS                   PORTS
project-scientist-dev   project-scientist:dev-local   "/app/project-scient…"   project-scientist   8 seconds ago   Up 7 seconds (healthy)   127.0.0.1:8097->8080/tcp

$ docker compose down
 Container project-scientist-dev Stopping 
 Container project-scientist-dev Stopped 
 Container project-scientist-dev Removing 
 Container project-scientist-dev Removed 
 Network project-scientist_default Removing 
 Network project-scientist_default Removed
```

## Parity gaps / next-wave status

PSC-005 artifacts remain roadmap/spec evidence, not implementation closure. The parity roadmap still carries 44 next-wave entries, and the MVP closure map carries 9 entries. The explicit stop-line remains: Project Scientist is a local lab-test prototype until security, tenant isolation, RBAC, audit review, migration reconciliation, reporting, and pilot-readiness cards are completed and reviewed.
