# PSC-003 productionization review packet

Task: t_4d27afdd
Scope: local Docker/dev workflow hardening for Friday final review.

## Scope boundary

This hardens Project Scientist's local development workflow only. It is lab-only and not a customer, public, or production deployment path.

Stop-lines preserved:

- No customer/prod data.
- No external exposure.
- No Tindall/CENLA/RJ Lee integration.
- No production-ready claims.
- No DB/Redis/Traefik/Auth/external services added.

## Repo changes

- `Makefile`: added durable workflow targets `test`, `vet`, `fmt-check`, `docker-test`, `dev-up`, and `dev-down`.
- `Dockerfile`: added `test` stage that runs `go test -mod=readonly ./...` inside Docker.
- `docker-compose.yml`: added profiled `project-scientist-test` service using the Dockerfile `test` target.
- `README.md`: corrected local workflow to loopback `127.0.0.1:8097`, Make targets, and dev docs link.
- `docs/dev.md`: added durable local workflow docs with prerequisites, first run, host/Docker gates, health check URL, persistence/reset caveats, and lab-only stop-lines.
- `.dockerignore` and `.gitignore`: tightened repo-local runtime/build/editor/cache ignore policy for `data/`, `var/`, `tmp/`, build outputs, logs, and editor folders without hiding source needed by Docker builds.

## Verification evidence

Commands run from `/Users/citadel/Projects/ProjectScientist`:

```bash
make fmt-check && make test && make vet
```

Result: passed.

Observed output summary:

```text
go test -mod=readonly ./...
?   github.com/warpapaya/ProjectScientist/cmd/project-scientist [no test files]
ok  github.com/warpapaya/ProjectScientist/internal/lab (cached)
go vet ./...
```

```bash
docker compose config --quiet && make docker-test
```

Result: passed.

Observed output summary:

```text
docker compose run --build --rm project-scientist-test
Image projectscientist-project-scientist-test Built
?   github.com/warpapaya/ProjectScientist/cmd/project-scientist [no test files]
ok  github.com/warpapaya/ProjectScientist/internal/lab 0.003s
```

```bash
make dev-up \
  && docker inspect project-scientist-dev --format 'User={{.Config.User}} Restart={{.HostConfig.RestartPolicy.Name}} Mounts={{range .Mounts}}{{.Destination}}:{{.Type}} {{end}} Ports={{json .NetworkSettings.Ports}}' \
  && curl -fsS http://127.0.0.1:8097/healthz \
  && printf '\n' \
  && make dev-down
```

Result: passed.

Observed output summary:

```text
Container project-scientist-dev Started
ok
User=scientist Restart=no Mounts=/data:volume  Ports={"8080/tcp":[{"HostIp":"127.0.0.1","HostPort":"8097"}]}
ok
Container project-scientist-dev Removed
```

`docker compose down` preserved the named `project-scientist-data` volume. The empty `projectscientist_default` network initially lingered with no attached containers and was removed with `docker network rm projectscientist_default`; no Docker volumes were removed.

## Rollback/reset notes

- Roll back repo changes with git checkout/restore for the files listed above.
- Normal local shutdown: `make dev-down` / `docker compose down`; preserves `project-scientist-data`.
- Destructive local reset, only for prototype data: `docker compose down --volumes`.
- Docker environment touched: built images `projectscientist-project-scientist` and `projectscientist-project-scientist-test`; started/stopped/removed container `project-scientist-dev`; created and removed empty network `projectscientist_default`; preserved named volume `project-scientist-data`.
