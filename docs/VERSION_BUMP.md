# Version Bump Checklist

When releasing a new version, update the following files:

<br/>

## Required Files

| File | Field | Example |
|------|-------|---------|
| `Makefile` | `IMG ?= somaz940/k8s-namespace-sync:<version>` | `v0.3.0` |
| `helm/k8s-namespace-sync/Chart.yaml` | `version` (chart version, without `v` prefix) | `0.3.0` |
| `helm/k8s-namespace-sync/Chart.yaml` | `appVersion` (app version, with `v` prefix) | `"v0.3.0"` |
| `helm/k8s-namespace-sync/values.yaml` | `image.tag` | `v0.3.0` |
| `config/manager/kustomization.yaml` | `newTag` | `v0.3.0` |
| `release/install.yaml` | `image:` (rebuild with `make build-installer`) | `v0.3.0` |

<br/>

## Documentation Files

| File | Location | Note |
|------|----------|------|
| `docs/HELM.md` | Configuration table (`image.tag` default) | Update default value |

<br/>

## Release Steps

1. Update all files listed above
2. Commit: `git commit -m "chore: bump version to v0.x.x"`
3. Push: `git push origin main`
4. Build and push Docker image: `make docker-buildx` (or `make docker-build docker-push`)
5. Create and push tag: `git tag v0.x.x && git push origin v0.x.x`
6. Workflows triggered automatically:
   - `release.yml` - Creates GitHub Release with git-cliff changelog
   - `helm-release.yml` - Packages and publishes Helm chart to gh-pages
   - `changelog-generator.yml` - Updates CHANGELOG.md
