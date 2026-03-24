# CLAUDE.md

<br/>

## Commit Guidelines

- Do not include `Co-Authored-By` lines in commit messages.
- Do not push to remote. Only commit. The user will push manually.

<br/>

## Project Structure

- Kubernetes operator built with controller-runtime (kubebuilder)
- CRD: `NamespaceSync` (apiGroup: `sync.nsync.dev/v1`)
- Automatically synchronizes Secrets and ConfigMaps across namespaces
- Features: resource filtering (glob patterns), namespace targeting, finalizer-based cleanup, Prometheus metrics

<br/>

## Build & Test

```bash
make test                # Unit tests (envtest)
make test-e2e            # E2E tests (requires Kind cluster)
make test-integration    # Integration tests (uses make deploy, local source)
make test-helm           # Helm chart tests (uses local chart path)
make manifests generate  # Regenerate CRD and RBAC manifests
make lint                # Run golangci-lint
make bump-version VERSION=vX.Y.Z  # Bump version across all files
```

<br/>

## Key Directories

- `api/v1/` — CRD type definitions (NamespaceSync spec/status)
- `internal/controller/` — Reconciler logic, metrics, filters, sync handlers
- `config/samples/` — Sample CR YAML files
- `helm/k8s-namespace-sync/` — Helm chart
- `hack/` — Test and utility scripts (bump-version, test-integration, test-helm)
- `docs/` — Documentation (HELM.md, TROUBLESHOOTING.md, VERSION_BUMP.md)

<br/>

## Code Style

- Linter: `golangci-lint` — prefer `switch` over `if-else if` chains.
- Run `make lint` before committing.

<br/>

## Common Pitfalls

- **Helm CRD sync**: When adding/changing CRD fields, always copy from `config/crd/bases/` to `helm/k8s-namespace-sync/crds/`.
- **Dockerfile ARG scope**: In multi-stage builds, ARG must be re-declared after each `FROM`.
- **Version bump**: `make bump-version` auto-updates all files (Makefile, Chart.yaml, values.yaml, README, docs, dist/install.yaml). No manual edits needed.
- **Finalizer**: Uses `namespacesync.nsync.dev/finalizer` for cleanup — do not skip finalizer logic on deletion.
- **Reconcile interval**: Configurable via `RECONCILE_INTERVAL` env var (default `5m`). The variable is in `internal/controller/namespacesync_controller.go` (`ReconcileInterval`), read in `cmd/main.go`.
- **Status conditions**: `Ready` condition uses three reasons — `SyncComplete` (all succeeded), `PartialSync` (some failed), `SyncFailed` (all failed).

<br/>

## Release Workflow

1. `make bump-version VERSION=vX.Y.Z`
2. Commit & push
3. `make docker-buildx` (build & push image)
4. `git tag vX.Y.Z && git push origin vX.Y.Z`
5. Tag push auto-triggers: `release.yml`, `helm-release.yml`, `changelog-generator.yml`

<br/>

## Language

- Communicate with the user in Korean.
- All documentation and code comments must be written in English.
