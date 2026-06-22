# PSC-000 Local Docker LIMS Prototype Handoff

Generated: 2026-06-22 14:55 local
Repository: `/Users/citadel/Projects/ProjectScientist`
GitHub: <https://github.com/warpapaya/ProjectScientist>
Current HEAD validated: `2428ebeb4de6d62155fc481d8d968d3f5c67458e` (`docs: add SENAITE parity roadmap and security gates`)
Original Friday-reviewed bootstrap commit: `f39b51b1c7b8c4a2ed1bb5fed212982c06abd0ad`

## Readiness statement

This packet is a local lab-test/bootstrap handoff only. It is not production-ready, not customer-ready, and not approved for any public, client, Tindall, CENLA, RJ Lee, or other customer environment.

Do not use this prototype with customer/production data. Do not expose it through Traefik/Auth/Internet-facing infrastructure. Do not message customers from this evidence.

## What was verified in this productionization pass

Validation was re-run from the repository root on the current repository state.

| Gate | Result | Evidence |
| --- | --- | --- |
| Git identity | PASS | HEAD `2428ebeb4de6d62155fc481d8d968d3f5c67458e`; original reviewed commit `f39b51b1c7b8c4a2ed1bb5fed212982c06abd0ad` still in history. |
| Go version | PASS | `go version go1.26.0 darwin/arm64` |
| Format gate | PASS | `make fmt-check` exited 0. |
| Host tests | PASS | `make test` -> `go test -mod=readonly ./...`; `cmd/project-scientist` no test files; `internal/lab` ok. |
| Static vet | PASS | `make vet` exited 0. |
| Compose config | PASS | `docker compose config --quiet` exited 0. |
| Docker test image | PASS | `make docker-test` built `projectscientist-project-scientist-test:latest` and ran Go tests inside Docker. |
| Docker app image | PASS | `make dev-up` built `projectscientist-project-scientist:latest`, started `project-scientist-dev` bound to `127.0.0.1:8097`. |
| Health endpoint | PASS | `curl -fsS http://127.0.0.1:8097/healthz` returned `ok`. |
| Minimal API smoke | PASS | Created client `C-00003`, sample `S-000003`, transitioned `in_prep -> in_analysis -> in_review -> released`, latest sample had 2 analyses. |
| Audit chain visibility | PASS | Smoke saw 6 audit events for the smoke actor; latest audit action `sample.transitioned`; `hash` and `previous_hash` present. |
| Cleanup | PASS | `make dev-down` stopped and removed local container/network while preserving the named dev volume. |

Raw logs:

- `/tmp/psc-000-validation-20260622T145057.log`
- `/tmp/psc-000-dev-smoke-20260622T145127.log`

Docker images produced locally:

- `projectscientist-project-scientist:latest`, image id `43f21e148056`, digest `sha256:43f21e148056e39b0adabef2f4e9e0e3c26d8657b2de6da87c843818b5a1275e`
- `projectscientist-project-scientist-test:latest`, image id `a699eeb8cd0d`, digest `sha256:a699eeb8cd0d33f7334d79b7c1bfa949ce46603e2e907c4036ca6c435ed42678`

## Current repository state

The repository is intentionally documented as not clean. No destructive cleanup was performed.

Current tracked modifications/untracked files at validation time:

```text
 M .dockerignore
 M .gitignore
 M Dockerfile
 M README.md
 M docker-compose.yml
?? Makefile
?? docs/PSC-003-productionization-review.md
?? docs/dev.md
?? docs/psc-000-local-prototype-handoff.md
?? docs/psc-000-local-prototype-handoff.pdf
?? internal/lab/store_sqlite_test.go
```

Observed meaning of those changes:

- `.dockerignore` and `.gitignore` exclude local runtime/build/editor artifacts.
- `Dockerfile` adds a Docker `test` target.
- `docker-compose.yml` adds `project-scientist-test` under the `test` profile.
- `Makefile` adds repeatable host and Docker development gates.
- `README.md` points to the local-only workflow and caveats.
- `docs/dev.md` documents local development workflow, stop-lines, persistence/reset caveats, and validation commands.
- `docs/PSC-003-productionization-review.md` and `internal/lab/store_sqlite_test.go` are unrelated untracked outputs from other PSC cards and were left untouched.
- This handoff packet adds the PSC-000 evidence artifact.

Related already-committed docs exist at HEAD:

- `docs/senaite-parity-roadmap.md`
- `docs/security-audit-model.md`

## Reproducible local workflow

From `/Users/citadel/Projects/ProjectScientist`:

```bash
make fmt-check
make test
make vet
docker compose config --quiet
make docker-test
make dev-up
curl -fsS http://127.0.0.1:8097/healthz
python3 /tmp/psc_smoke.py
make dev-down
```

Expected health response:

```text
ok
```

Expected smoke behavior:

- HTTP 201 for client creation.
- HTTP 201 for sample creation.
- HTTP 200 for each legal transition.
- Final sample status `released`.
- Two analyses on the sample.
- Latest audit event contains `hash`; post-genesis transition event contains `previous_hash`.

## Remaining gates and risks

These are blockers before any production/customer claim:

1. Auth/RBAC/e-signature model is absent.
2. Results entry, QC, worksheets, instruments, calculations, and specs/limits are absent.
3. COA/reporting, COC packaging, labels, attachments, and custody chain are absent.
4. Multi-tenant hosting, backup/restore, operational monitoring, and incident model are absent.
5. Audit chain is a prototype JSONL hash chain, not a hardened WORM/tamper-evident compliance system.
6. File-backed local state is acceptable only for lab/bootstrap pressure testing.
7. Repository has review-pending local workflow changes and this evidence packet; do not present as a merged/released product without review.

## Recommendation

Treat PSC-000 as a validated local architecture bootstrap. It is good enough to keep pressure-testing the greenfield Clearline LIMS lane and to compare against SENAITE parity requirements. It is not good enough to deploy, demo externally as production software, or use with customer data.
