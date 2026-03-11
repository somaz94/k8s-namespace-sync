
### Bug Fixes
- dynamically resolve tags in changelog and release workflows by @somaz94
- resolve helm-repo checkout conflict in helm-release workflow by @somaz94
- remove helm-repo before checkout to prevent untracked file conflict by @somaz94
- add dedicated RBAC for CRD cleanup hook job by @somaz94
- use --no-hooks for helm uninstall in test to prevent hang by @somaz94
- clean up CRD before helm install in test by @somaz94
- set GOTOOLCHAIN from go.mod to fix covdata errors by @somaz94
- auto-install CRD and deploy controller in integration test by @somaz94
- add pod creation wait loop before kubectl wait in integration test by @somaz94

### CI/CD
- use conventional commit message in changelog-generator workflow by @somaz94
- add helm chart release workflow for automated packaging by @somaz94
- add release notes categorization config by @somaz94
- migrate release workflow to git-cliff and softprops/action-gh-release by @somaz94
- add automation workflows and manifests verification by @somaz94

### Chore
- bump the go-minor group with 5 updates by @dependabot[bot]
- bump the go-minor group across 1 directory with 7 updates by @dependabot[bot]
- bump golangci/golangci-lint-action from 6 to 7 by @dependabot[bot]
- bump sigs.k8s.io/controller-runtime in the go-minor group by @dependabot[bot]
- bump the go-minor group with 2 updates by @dependabot[bot]
- bump github.com/prometheus/client_golang by @dependabot[bot]
- bump golangci/golangci-lint-action from 7 to 8 by @dependabot[bot]
- bump the go-minor group with 3 updates by @dependabot[bot]
- bump the go-minor group with 3 updates by @dependabot[bot]
- bump the go-minor group across 1 directory with 4 updates by @dependabot[bot]
- bump the go-minor group across 1 directory with 5 updates by @dependabot[bot]
- bump actions/checkout from 4 to 5 by @dependabot[bot]
- bump golang from 1.24 to 1.25 in the docker-minor group by @dependabot[bot]
- bump the go-minor group with 5 updates by @dependabot[bot]
- bump the go-minor group with 6 updates by @dependabot[bot]
- workflows by @somaz94
- bump golangci/golangci-lint-action from 8 to 9 by @dependabot[bot]
- bump actions/setup-go from 5 to 6 by @dependabot[bot]
- bump the go-minor group across 1 directory with 6 updates by @dependabot[bot]
- test, test-e2e by @somaz94
- bump the go-minor group with 6 updates by @dependabot[bot]
- bump actions/checkout from 5 to 6 by @dependabot[bot]
- bump the go-minor group with 5 updates by @dependabot[bot]
- bump the go-minor group with 3 updates by @dependabot[bot]
- bump golang from 1.25 to 1.26 in the docker-minor group by @dependabot[bot]
- bump the go-minor group across 1 directory with 6 updates by @dependabot[bot]
- bump the go-minor group across 1 directory with 4 updates by @dependabot[bot]
- bump sigs.k8s.io/controller-runtime in the go-minor group by @dependabot[bot]
- bump version to v0.2.1 by @somaz94
- upgrade Go to 1.26, use go-version-file in CI workflows by @somaz94
- sync workflow parity and update Go version in docs by @somaz94

### Documentation
- README.md by @somaz94
- consolidate documentation into docs/ directory by @somaz94
- add TROUBLESHOOTING.md and CONTRIBUTING.md, migrate golangci-lint to v2 by @somaz94
- docs/VERSION_BUMP.md by @somaz94
- update VERSION_BUMP.md to v0.2.1 by @somaz94

### Features
- add integration and helm test scripts by @somaz94

### Performance
- optimize Dockerfile with build cache and smaller binary by @somaz94

### Testing
- enhance controller test coverage from 84% to 90% by @somaz94

**Full Changelog**: https://github.com/somaz94/k8s-namespace-sync/compare/v0.2.0...v0.2.1
