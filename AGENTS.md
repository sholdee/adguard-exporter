# Agent Notes for adguard-exporter

This repo is a small Go service that exports Prometheus metrics from AdGuard
Home query logs. It is intended to run as a sidecar next to AdGuard Home in
Kubernetes, with both containers sharing AdGuard's work directory.

Keep changes boring. This project should remain a single-purpose exporter, not
a Kubernetes controller or a general AdGuard API client.

## What To Read First

- `README.md` for the user-facing deployment shape and metric list.
- `main.go` for process wiring, HTTP endpoints, metric registration, and
  graceful shutdown.
- `config/config.go` for environment variables and defaults.
- `loghandler/loghandler.go` for file watching and JSON-line parsing.
- `metrics/` for metric definitions and aggregation behavior.
- `Dockerfile` and `.github/workflows/` for image and CI changes.

## Runtime Model

- Default log file: `/opt/adguardhome/work/data/querylog.json`
- Default metrics port: `8000`
- Health endpoints:
  - `/metrics`
  - `/livez`
  - `/readyz`
- The container runs as distroless nonroot.
- The exporter reads AdGuard Home query log records from disk; it does not call
  the AdGuard HTTP API.

## Code Boundaries

- `config` only parses environment/default configuration.
- `loghandler` owns filesystem watching, file offsets, JSON decoding, and log
  health.
- `metrics` owns Prometheus collectors, top-host tracking, and rolling response
  time aggregation.
- `main` wires packages together and owns HTTP/server lifecycle.

Avoid leaking file-watch or AdGuard log parsing details into `main`. Avoid
putting HTTP concerns into `metrics`.

## Change Map

- Metric names, labels, or meanings:
  - update `metrics/`
  - update registrations in `main.go`
  - update the metric list in `README.md`
- AdGuard query log format assumptions:
  - inspect `loghandler/loghandler.go`
  - inspect `metrics/collector.go`
  - current fields used: `QH`, `QT`, `Result.IsFiltered`, `Result.Reason`,
    `Elapsed`, `Upstream`
- Runtime configuration:
  - update `config/config.go`
  - update Dockerfile `ENV` defaults if applicable
  - update README examples
- CI, Renovate, or image publishing:
  - inspect `.github/workflows/`
  - inspect `.github/renovate.json5`
  - inspect `Dockerfile`, `.dockerignore`, and `go.mod`

Do not scan `assets/` unless the README image itself is part of the task.

## High-Risk Areas

- Log parsing is loosely typed. Bad or changed AdGuard fields may silently turn
  into zero values.
- Label cardinality can grow quickly for host and upstream labels.
- Top-host metrics are reset and repopulated from in-memory state; be careful
  with Prometheus counter semantics.
- Liveness depends on file open/read/watch behavior. Missing-file and watch
  error handling affects Kubernetes restarts.
- Docker base images and GitHub Actions are pinned for supply-chain hygiene.
  Update tag and digest together.

## Local Commands

Use these from the repo root:

```bash
go test ./...
go test -race ./... -v -coverprofile=coverage.txt -covermode=atomic
go tool cover -func=coverage.txt
go vet ./...
go mod verify
go mod tidy
golangci-lint run
actionlint -shellcheck=shellcheck
npx --yes markdownlint-cli2@0.22.1 '**/*.md' '#node_modules'
govulncheck ./...
```

Docker smoke test:

```bash
docker buildx build --platform linux/amd64,linux/arm64 .
```

Enable the local pre-commit hook with:

```bash
git config core.hooksPath .githooks
```

## CI And Release Notes

- The repository keeps `master` as its single long-lived branch.
- The Go build target is the repo root (`.`), not `./cmd/...`.
- Production publishing is manual through the `Publish` workflow.
- The workflow input accepts strict SemVer as `X.Y.Z` or `vX.Y.Z`, normalizes it
  to git tag `vX.Y.Z`, and fails if that tag already exists.
- Published images:
  - `sholdee/adguardexporter`
  - `ghcr.io/sholdee/adguard-exporter`
- Release image tags:
  - `vX.Y.Z`
  - `X.Y.Z`
  - `X.Y`
  - `latest`
- `latest` is release-owned; do not publish it from normal `master` pushes.
- Docker builds target `linux/amd64` and `linux/arm64`.
- Docker Hub publishing expects `DOCKER_USERNAME` and `DOCKER_PASSWORD`.
- GHCR publishing uses `GITHUB_TOKEN`.
- The publish workflow creates the git tag and GitHub Release after image
  manifests verify.

## Current Gaps To Keep In Mind

- Unit tests cover `config`, most of `metrics`, and core `loghandler` file
  processing behavior. Add focused tests before behavior changes.
- `WatchLogFile` and `main` still need refactoring before they can be tested
  cleanly.
- The exporter is intentionally file-log based; adding API calls or Kubernetes
  clients should be treated as a scope expansion.
