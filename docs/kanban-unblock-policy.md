# Project Scientist — Kanban Unblock Policy

Status: active for the Project Scientist lab-development lane.

Petie granted standing approval for autonomous Project Scientist development work. Friday/workers should not block only because they need human approval for local development activities.

## Auto-approved in this lane

- Local Docker development and smoke testing on Citadel.
- Repo edits in `/Users/citadel/Projects/ProjectScientist`.
- Go tests, lint/vet/fmt, Docker builds, local seed/reset scripts.
- Kanban comments, unblock/promote/complete actions, and remediation-card creation.
- Git commits and pushes to `https://github.com/warpapaya/ProjectScientist.git` for this lab project.
- Internal docs, specs, roadmaps, test fixtures, synthetic/demo data, and local artifacts.

## Still not auto-approved

- Production or customer-system mutation.
- Tindall/CENLA/RJ Lee/other customer tenant changes.
- External/customer-facing messages.
- Production-readiness claims.
- DNS/auth/billing/payment/security-account changes.
- Secrets or credential handling.
- Destructive cleanup outside local Project Scientist dev artifacts.

## Blocker handling

1. If a card is blocked only for human review/approval and the work is inside the auto-approved scope, Friday may review evidence, comment the approval transition, complete/unblock/promote the card, and dispatch downstream work.
2. If a card is blocked by a technical failure, keep it blocked and create a narrow remediation card unless one already exists.
3. If a card is blocked by a still-active stop-line, leave it blocked and comment the boundary.
4. Always record decisions as Kanban comments before state changes.
5. Do not call a worker handoff complete without verifying concrete evidence: file paths, commits, tests, Docker health, or artifact checksums as applicable.

## Current known technical caution

A local `make docker-test` run returned exit 137 during the sqlite module download after prior workers reported passing Docker gates. Treat that as a technical blocker/remediation signal, not a Petie-approval blocker.
