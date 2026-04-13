# Changelog

All notable changes to this project will be documented in this file.

## Unreleased (2026-04-13)

### Documentation

- remove duplicate rules covered by global CLAUDE.md ([c0df548](https://github.com/somaz94/k8s-namespace-sync/commit/c0df548b3336b292b7b88b26246fa0b0fc74b462))

### Continuous Integration

- skip auto-generated changelog and contributors commits in release notes ([9acdd4b](https://github.com/somaz94/k8s-namespace-sync/commit/9acdd4b8be81aa10e939f565bb7ce958fee0b32c))

### Chores

- **deps:** bump softprops/action-gh-release from 2 to 3 ([d02c266](https://github.com/somaz94/k8s-namespace-sync/commit/d02c266a4ff37289517968441d59b4d6a9f252b2))
- **deps:** bump dependabot/fetch-metadata from 2 to 3 ([e821e0a](https://github.com/somaz94/k8s-namespace-sync/commit/e821e0a06079dab8e03877f69adc2805951d2466))
- **deps:** bump azure/setup-helm from 4 to 5 ([788af22](https://github.com/somaz94/k8s-namespace-sync/commit/788af227a5931983d09ca123e0acd20d2cdcd587))
- remove duplicate rules from CLAUDE.md (moved to global) ([d1777a0](https://github.com/somaz94/k8s-namespace-sync/commit/d1777a00587d0b20c2cf19a0416379bb29b1b0b7))
- add git config protection to CLAUDE.md ([d5d6de4](https://github.com/somaz94/k8s-namespace-sync/commit/d5d6de4b6b18b39cf77a830de0d5c08272ebe238))

### Contributors

- somaz

<br/>

## [v0.3.1](https://github.com/somaz94/k8s-namespace-sync/compare/v0.3.0...v0.3.1) (2026-03-25)

### Code Refactoring

- cleanup duplication, structured logging, and errors.Join ([2211027](https://github.com/somaz94/k8s-namespace-sync/commit/22110271a6eed70575bb190125b2c5df74db235e))
- eliminate code duplication across controller with generics and shared helpers ([5d3539d](https://github.com/somaz94/k8s-namespace-sync/commit/5d3539d38c5e3384848f9473b5fbb080f6d44113))

### Tests

- add coverage for cleanup error paths and createOrUpdateResource Get error ([58ddddf](https://github.com/somaz94/k8s-namespace-sync/commit/58ddddf7f05a74a22fd0a2e7ba3cb534fd5234da))

### Continuous Integration

- revert to body_path RELEASE.md in release workflow ([a84d69b](https://github.com/somaz94/k8s-namespace-sync/commit/a84d69be701875dc1125f11db297c743964f5d7d))
- use generate_release_notes instead of RELEASE.md ([02f7ea8](https://github.com/somaz94/k8s-namespace-sync/commit/02f7ea8ca85aa1c5e7751b6a5cbdec79ad7035cf))

### Chores

- bump version to v0.3.1 ([6f68a38](https://github.com/somaz94/k8s-namespace-sync/commit/6f68a383ffc7baeefe44dba60438a0b47bb0b929))

### Contributors

- somaz

<br/>

## [v0.3.0](https://github.com/somaz94/k8s-namespace-sync/compare/v0.2.1...v0.3.0) (2026-03-25)

### Features

- improve status conditions with PartialSync and SyncFailed reasons ([52cb987](https://github.com/somaz94/k8s-namespace-sync/commit/52cb9879cc77804f3b9746e58662f14d31389ba4))
- make reconciliation interval configurable via RECONCILE_INTERVAL env var ([a787e0b](https://github.com/somaz94/k8s-namespace-sync/commit/a787e0b03d7ebdf42ecee28eaddfa7ec6793a8de))
- add event recording, complete metrics, periodic reconciliation ([105583b](https://github.com/somaz94/k8s-namespace-sync/commit/105583b4ad4722098316f4f072d496a788f4572e))
- add CODEOWNERS ([95c4d74](https://github.com/somaz94/k8s-namespace-sync/commit/95c4d74f5adc8ce33a3d6b689404eb70c004bac5))

### Bug Fixes

- prevent source-namespace annotation from being overwritten with target namespace ([34c6118](https://github.com/somaz94/k8s-namespace-sync/commit/34c61188ffb83283eafd162744a577bc6f567750))
- restore Chart.yaml before gh-pages checkout in helm-release workflow ([1fb00b7](https://github.com/somaz94/k8s-namespace-sync/commit/1fb00b7a840297512720d407a8df99a4feed8f07))
- prevent duplicate quote in bump-version.sh values.yaml sed pattern ([7b85fcf](https://github.com/somaz94/k8s-namespace-sync/commit/7b85fcf3fb07603beb8ec0bf7790fa98147db57b))
- use GITHUB_TOKEN for dependabot auto merge ([896a939](https://github.com/somaz94/k8s-namespace-sync/commit/896a939eb8fd5dbc130b6a4fe4a71704d5ecdf1f))
- re-declare ARG in final stage for OCI labels ([d7d288b](https://github.com/somaz94/k8s-namespace-sync/commit/d7d288b616d3068c375e51b9014610c6212309b6))
- set imagePullPolicy=Always in helm test script ([c4dc522](https://github.com/somaz94/k8s-namespace-sync/commit/c4dc522a8f7fb959f0be04bd101aed0be923e25e))
- skip major version tag deletion on first release ([f316438](https://github.com/somaz94/k8s-namespace-sync/commit/f3164380eb9827b155bd15e7fc97718a9d96760e))
- add trap cleanup to Helm test ([3173134](https://github.com/somaz94/k8s-namespace-sync/commit/3173134926e18f9113fb462253cae304f155db14))
- show full undeploy output in integration test cleanup ([3863411](https://github.com/somaz94/k8s-namespace-sync/commit/3863411c81efe04cec2e2f00284f5bb17800a358))
- add trap for cleanup and undeploy on test exit ([69ac84b](https://github.com/somaz94/k8s-namespace-sync/commit/69ac84b27a79655bbe283519027db4e3728efc59))
- force imagePullPolicy Always during integration tests ([c0b4200](https://github.com/somaz94/k8s-namespace-sync/commit/c0b420088365041faf8acb1e8e2751c709791182))

### Code Refactoring

- extract syncResourceList to reduce duplication in sync.go ([2dceab4](https://github.com/somaz94/k8s-namespace-sync/commit/2dceab434bd8d1afb948133f4f7e0afae4a458f4))
- translate Korean comments to English ([ed8e30f](https://github.com/somaz94/k8s-namespace-sync/commit/ed8e30facfcaae172e7d6296e7195143b707ffa0))

### Documentation

- update TESTING.md, HELM.md, README.md; add status condition unit tests ([53b0db5](https://github.com/somaz94/k8s-namespace-sync/commit/53b0db5b7844cf7e2e685e2777dce6f16a27514c))
- update documentation for configurable reconcile interval and improved status ([a2ea840](https://github.com/somaz94/k8s-namespace-sync/commit/a2ea8402d95e2c9ba17f5e71ccb31bc2f714a1b6))
- add DEVELOPMENT.md ([942d2a0](https://github.com/somaz94/k8s-namespace-sync/commit/942d2a0ea0ad59c0e39f7d7ba590186190a40a69))
- add no-push rule to CLAUDE.md ([9d136aa](https://github.com/somaz94/k8s-namespace-sync/commit/9d136aadc77a933820b265baef2b92c164fa62ef))
- add CLAUDE.md project guide ([eafd881](https://github.com/somaz94/k8s-namespace-sync/commit/eafd88131946c4b223d5c6b249dd1559cedaeefe))
- add badges to README ([0ef59cc](https://github.com/somaz94/k8s-namespace-sync/commit/0ef59cca4e2263ef2169c559271baee543fb04a6))
- unify installation structure and migrate to dist/ ([c100c61](https://github.com/somaz94/k8s-namespace-sync/commit/c100c61fb768bd124f9c3ce96ff030a5d32dfc7a))
- add TESTING.md for test guide and procedures ([c81b63f](https://github.com/somaz94/k8s-namespace-sync/commit/c81b63fffb859dac27e8de361e8c9be2244bdaa3))

### Tests

- add event recording, status conditions, and metadata annotation tests ([64017a2](https://github.com/somaz94/k8s-namespace-sync/commit/64017a270ddd8bbe223e3b27ff6de7e338dbad6c))

### Continuous Integration

- restrict push trigger to main branch to prevent duplicate CI runs ([bd75245](https://github.com/somaz94/k8s-namespace-sync/commit/bd7524541f3c68a7a7cc3f81098e5a124a2a87c9))
- add auto-generated PR body script for make pr ([51f9e2e](https://github.com/somaz94/k8s-namespace-sync/commit/51f9e2e3538b7b4fb8373466c4e8f9cc174550a8))
- migrate gitlab-mirror workflow to multi-git-mirror action ([10c5696](https://github.com/somaz94/k8s-namespace-sync/commit/10c569622a0ce7e1a362edd606e99912bedc59f8))
- use somaz94/contributors-action@v1 for contributors generation ([faeaf6b](https://github.com/somaz94/k8s-namespace-sync/commit/faeaf6b1f5f7e320003cebbfddaaf65115bbd7b4))
- use major-tag-action for version tag updates ([b6d1ead](https://github.com/somaz94/k8s-namespace-sync/commit/b6d1ead6dd42d0f96212c97de4d1b23ba45e6730))
- migrate changelog generator to go-changelog-action ([6048012](https://github.com/somaz94/k8s-namespace-sync/commit/60480128efdfa91147288b5e6bfbc6ed57c82e87))
- unify changelog-generator with flexible tag pattern ([a0522ae](https://github.com/somaz94/k8s-namespace-sync/commit/a0522ae693cf9d04e19a94f1bfe6eee9c69e828d))

### Chores

- bump version to v0.3.0 ([ec69e99](https://github.com/somaz94/k8s-namespace-sync/commit/ec69e99887fe19639f6fac1a4687b0a882ab96ef))
- **deps:** bump the go-minor group with 3 updates (#41) ([#41](https://github.com/somaz94/k8s-namespace-sync/pull/41)) ([77a77c7](https://github.com/somaz94/k8s-namespace-sync/commit/77a77c701fefe7cccadbe98def72c05a12901e09))
- add workflow Makefile targets (check-gh, branch, pr) ([573ff63](https://github.com/somaz94/k8s-namespace-sync/commit/573ff63af60ad64e5f4b4e8165e91eca6344efe9))
- add build-time version injection and OCI labels to Dockerfile ([691cba0](https://github.com/somaz94/k8s-namespace-sync/commit/691cba0c3755bba00551b43109dc08709994c128))
- add version check and bump-version script ([46264a7](https://github.com/somaz94/k8s-namespace-sync/commit/46264a7c4a4eecc169a44abe424000367a9428df))
- change license from MIT to Apache 2.0 ([a906c5b](https://github.com/somaz94/k8s-namespace-sync/commit/a906c5bd51ae7921f7f5fe130ee70a3e8518bbf3))

### Contributors

- somaz

<br/>

## [v0.2.1](https://github.com/somaz94/k8s-namespace-sync/compare/v0.2.0...v0.2.1) (2026-03-11)

### Features

- add integration and helm test scripts ([3a646f5](https://github.com/somaz94/k8s-namespace-sync/commit/3a646f5155e2c6408230aaf0e1b2e15bd5a467a0))

### Bug Fixes

- add pod creation wait loop before kubectl wait in integration test ([31e593f](https://github.com/somaz94/k8s-namespace-sync/commit/31e593fcb46dfe1b4b5d0ba62ba3989aeb589d4f))
- auto-install CRD and deploy controller in integration test ([d8a35c9](https://github.com/somaz94/k8s-namespace-sync/commit/d8a35c9233b37b0f9306ebfd37af1fd56e0e6fc7))
- set GOTOOLCHAIN from go.mod to fix covdata errors ([016ad85](https://github.com/somaz94/k8s-namespace-sync/commit/016ad8573f8e326789b1a7a3885174e2175f08a1))
- clean up CRD before helm install in test ([f984093](https://github.com/somaz94/k8s-namespace-sync/commit/f9840930c90b13b167b7c485a6746519d86734b4))
- use --no-hooks for helm uninstall in test to prevent hang ([c1b209c](https://github.com/somaz94/k8s-namespace-sync/commit/c1b209ce6f9fdfbcd87b101e1ffa5e9a61d3c456))
- add dedicated RBAC for CRD cleanup hook job ([cdf802a](https://github.com/somaz94/k8s-namespace-sync/commit/cdf802aa6dd070f61355c808ee14a6273f9f380f))
- remove helm-repo before checkout to prevent untracked file conflict ([cf75a3f](https://github.com/somaz94/k8s-namespace-sync/commit/cf75a3f98888a42c73cb1f75ff518cd829813d61))
- resolve helm-repo checkout conflict in helm-release workflow ([029e1c7](https://github.com/somaz94/k8s-namespace-sync/commit/029e1c74ef8b31fbe6a44ec95bda5e1d41cfd83e))
- dynamically resolve tags in changelog and release workflows ([29973ee](https://github.com/somaz94/k8s-namespace-sync/commit/29973eef75dc3237c03d98678a0ccf637cb92bf3))

### Performance Improvements

- optimize Dockerfile with build cache and smaller binary ([36c7da8](https://github.com/somaz94/k8s-namespace-sync/commit/36c7da81af979222a7ea478d59d980e6cbdd9bff))

### Documentation

- update VERSION_BUMP.md to v0.2.1 ([e30d3d0](https://github.com/somaz94/k8s-namespace-sync/commit/e30d3d096d6d38d9697d316993cdd337e1ab487a))
- docs/VERSION_BUMP.md ([b4ca1b3](https://github.com/somaz94/k8s-namespace-sync/commit/b4ca1b3d95b2d373d405612276835e72484697de))
- add TROUBLESHOOTING.md and CONTRIBUTING.md, migrate golangci-lint to v2 ([6d3b629](https://github.com/somaz94/k8s-namespace-sync/commit/6d3b6298e80ee47e6eb6cb2601c6b7261130af2e))
- consolidate documentation into docs/ directory ([c0f2994](https://github.com/somaz94/k8s-namespace-sync/commit/c0f299439f0cd054a05e3f1408dee0e45dcc7d2a))
- README.md ([35aa88f](https://github.com/somaz94/k8s-namespace-sync/commit/35aa88f542aec77dd36476c126c64d970ab6f00d))

### Tests

- enhance controller test coverage from 84% to 90% ([cb365f6](https://github.com/somaz94/k8s-namespace-sync/commit/cb365f691443c098a413b80416f6e6685e6d7651))

### Continuous Integration

- add automation workflows and manifests verification ([5194f21](https://github.com/somaz94/k8s-namespace-sync/commit/5194f21cda55b0670d355326f862ca158349005c))
- migrate release workflow to git-cliff and softprops/action-gh-release ([31471ac](https://github.com/somaz94/k8s-namespace-sync/commit/31471ac11153174a535b920bc5710164c698da28))
- add release notes categorization config ([a11d568](https://github.com/somaz94/k8s-namespace-sync/commit/a11d568fe022ff186cf132736772e87a95da7659))
- add helm chart release workflow for automated packaging ([8711b1b](https://github.com/somaz94/k8s-namespace-sync/commit/8711b1b883d8bab146223580aef6631f138cb797))
- use conventional commit message in changelog-generator workflow ([23e7098](https://github.com/somaz94/k8s-namespace-sync/commit/23e70987cef69c62d3209ce6b27f0dbfebd699bb))

### Chores

- sync workflow parity and update Go version in docs ([b88bd8b](https://github.com/somaz94/k8s-namespace-sync/commit/b88bd8b6083a3af74fbe68ffe05ca3968e705a04))
- upgrade Go to 1.26, use go-version-file in CI workflows ([87dcace](https://github.com/somaz94/k8s-namespace-sync/commit/87dcace110d5b77a2900c4ea69538284501839c2))
- bump version to v0.2.1 ([f353f73](https://github.com/somaz94/k8s-namespace-sync/commit/f353f73777c7af4e81143173d103fe863233954a))
- **deps:** bump sigs.k8s.io/controller-runtime in the go-minor group ([426f366](https://github.com/somaz94/k8s-namespace-sync/commit/426f3665fba33e2af54e9c4ff617e1c1368861c9))
- **deps:** bump the go-minor group across 1 directory with 4 updates ([c8d7b7a](https://github.com/somaz94/k8s-namespace-sync/commit/c8d7b7a69556a438500f64f93ba10d89ad8fa98c))
- **deps:** bump the go-minor group across 1 directory with 6 updates ([414d10b](https://github.com/somaz94/k8s-namespace-sync/commit/414d10b6d42eacb9adf3558e384bb5db10c88e9d))
- **deps:** bump golang from 1.25 to 1.26 in the docker-minor group ([cd64afa](https://github.com/somaz94/k8s-namespace-sync/commit/cd64afa2aed838461d11eabf8679aefe483f142f))
- **deps:** bump the go-minor group with 3 updates ([ecf6744](https://github.com/somaz94/k8s-namespace-sync/commit/ecf67445948f3f0a823fd87b1165b5830c76ec2c))
- **deps:** bump the go-minor group with 5 updates ([fc83b98](https://github.com/somaz94/k8s-namespace-sync/commit/fc83b98deb9351786a4a73f0d1cae797b366a29e))
- test, test-e2e ([24d98c2](https://github.com/somaz94/k8s-namespace-sync/commit/24d98c2f63c5e07a77f5a35514ed64a4abc2d034))
- workflows ([982c37e](https://github.com/somaz94/k8s-namespace-sync/commit/982c37e42b8776f51a08c37f0d6e2217bf7dced2))
- **deps:** bump actions/checkout from 5 to 6 ([5b19024](https://github.com/somaz94/k8s-namespace-sync/commit/5b190242bef1b5242180b7b92617d034b3e652ee))
- **deps:** bump the go-minor group with 6 updates ([c98f863](https://github.com/somaz94/k8s-namespace-sync/commit/c98f863fe6811af1c551a62a6be30cbe18794c67))
- **deps:** bump golangci/golangci-lint-action from 8 to 9 ([babdf37](https://github.com/somaz94/k8s-namespace-sync/commit/babdf3762118605e16153166e4386f2482d2820e))
- **deps:** bump the go-minor group across 1 directory with 6 updates ([023fcd0](https://github.com/somaz94/k8s-namespace-sync/commit/023fcd0426e1f851cbf3504202e4574a01ecee00))
- **deps:** bump actions/setup-go from 5 to 6 ([6e91b64](https://github.com/somaz94/k8s-namespace-sync/commit/6e91b64d266d91958e5575d202f6e7786e5b94a7))
- **deps:** bump the go-minor group with 6 updates ([c3d0b7f](https://github.com/somaz94/k8s-namespace-sync/commit/c3d0b7f68990caf122274ffb3879f690dc4b7eaf))
- **deps:** bump the go-minor group with 5 updates ([6ef733e](https://github.com/somaz94/k8s-namespace-sync/commit/6ef733e2ebc30e8f8ce333f9659380f3f4506901))
- **deps:** bump golang from 1.24 to 1.25 in the docker-minor group ([d8a569d](https://github.com/somaz94/k8s-namespace-sync/commit/d8a569dec95221a1b79e60dad55ccf6690350a0d))
- **deps:** bump actions/checkout from 4 to 5 ([34630df](https://github.com/somaz94/k8s-namespace-sync/commit/34630df77396d5d1b5f9696256329a5ec2540290))
- **deps:** bump the go-minor group across 1 directory with 5 updates ([29eeb6e](https://github.com/somaz94/k8s-namespace-sync/commit/29eeb6e18f84dbcbf88aa0dc3f62707e478dfd71))
- **deps:** bump the go-minor group across 1 directory with 4 updates ([a246963](https://github.com/somaz94/k8s-namespace-sync/commit/a24696351fb47f3c08be4654a1615248feed6720))
- **deps:** bump the go-minor group with 3 updates ([b8ea08d](https://github.com/somaz94/k8s-namespace-sync/commit/b8ea08d3cd4e45f3d71cb34a3a674f7ef947198e))
- **deps:** bump golangci/golangci-lint-action from 7 to 8 ([70b609e](https://github.com/somaz94/k8s-namespace-sync/commit/70b609e7d58d1352173c0da3c75e32a50d642365))
- **deps:** bump the go-minor group with 3 updates ([07e3e5a](https://github.com/somaz94/k8s-namespace-sync/commit/07e3e5aef55ee4d5bcaa3f68e2ef36e13f926aa0))
- **deps:** bump github.com/prometheus/client_golang ([d75195f](https://github.com/somaz94/k8s-namespace-sync/commit/d75195fba23c3ec7b2cdc643a3554255a50f0486))
- **deps:** bump the go-minor group with 2 updates ([51afa0a](https://github.com/somaz94/k8s-namespace-sync/commit/51afa0a1fa5db60290eb844f371b993a9fc2b9e4))
- **deps:** bump sigs.k8s.io/controller-runtime in the go-minor group ([8f2d971](https://github.com/somaz94/k8s-namespace-sync/commit/8f2d9710e64cd36dbbc3b80e14c73d05edb20f46))
- **deps:** bump golangci/golangci-lint-action from 6 to 7 ([c35ab49](https://github.com/somaz94/k8s-namespace-sync/commit/c35ab49328522303fdeade0a33eed0ed659c268c))
- **deps:** bump the go-minor group across 1 directory with 7 updates ([785433c](https://github.com/somaz94/k8s-namespace-sync/commit/785433c6a99cfcd5dc9abcf244d5433659aa9678))
- **deps:** bump the go-minor group with 5 updates ([f7511f0](https://github.com/somaz94/k8s-namespace-sync/commit/f7511f019fa2d4103b7720d80d55d7601e67da40))

### Contributors

- somaz

<br/>

## [v0.2.0](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.7...v0.2.0) (2025-02-24)

### Bug Fixes

- changelog-generator.yml ([0a496c7](https://github.com/somaz94/k8s-namespace-sync/commit/0a496c7df15702ebabfeb7c377fa671abc2c2eea))
- chnagelog-generator.yml ([3c9c9fb](https://github.com/somaz94/k8s-namespace-sync/commit/3c9c9fbfed2cd1180447ce44856bb4a07d0e646d))
- chnagelog-generator.yml ([5187cdf](https://github.com/somaz94/k8s-namespace-sync/commit/5187cdf0ee67811e75639889d34b128db9d5a598))
- changelog-generator.yml ([eece9c6](https://github.com/somaz94/k8s-namespace-sync/commit/eece9c6d6646f6a1b85a4c30b97514e76118dd12))
- changelog-generator.yml ([6620ff8](https://github.com/somaz94/k8s-namespace-sync/commit/6620ff82018ebde182f1ee621bb6de8f0fc4c2da))
- changelog-generator.yml ([6ec056b](https://github.com/somaz94/k8s-namespace-sync/commit/6ec056bc5de3d591119ef1b5fcc1b50dd6f79d54))
- changelog-generator.yml ([39ee572](https://github.com/somaz94/k8s-namespace-sync/commit/39ee572b1a90947fbc2f4cdb2b995f130caab6c9))
- gitlab-mirror.yml ([1c4ef7e](https://github.com/somaz94/k8s-namespace-sync/commit/1c4ef7ea80117ed2c3b2e91f05611f42f687a52d))
- gitlab-mirror.yml, .gitignore ([b4b610d](https://github.com/somaz94/k8s-namespace-sync/commit/b4b610d45d5cfdbaeb55d4ab4fc083654aa16f57))
- changelog-generator.yml ([6c7ac0f](https://github.com/somaz94/k8s-namespace-sync/commit/6c7ac0f3c373e720db3fb53e3d9861f297db41b7))
- chagnelog-generator.yml ([66db095](https://github.com/somaz94/k8s-namespace-sync/commit/66db095fabe78718cb05c4b91316084851d49a0e))
- release.yml & changelog-generator.yml ([0c18ee1](https://github.com/somaz94/k8s-namespace-sync/commit/0c18ee174c1d3141c48fed80cc782e2a74e8de2d))

### Chores

- **deps:** bump golang from 1.23 to 1.24 in the docker-minor group ([9bee690](https://github.com/somaz94/k8s-namespace-sync/commit/9bee690df819ae9012ec13b3a459b59ef543ada7))

### Add

- gitlab-mirror.yml ([c82d08e](https://github.com/somaz94/k8s-namespace-sync/commit/c82d08e07698b21ef959f6304e82409a719fa0ed))

### Contributors

- somaz

<br/>

## [v0.1.7](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.6...v0.1.7) (2025-02-21)

### Chores

- fix release/install.yaml ([03a974b](https://github.com/somaz94/k8s-namespace-sync/commit/03a974bebf7b434470c2503c0a3343fd344f111d))
- fix helm version ([f06c664](https://github.com/somaz94/k8s-namespace-sync/commit/f06c66417da3152db5403c14c4185bb9171c57ad))
- test upgrade go package version ([f2d91af](https://github.com/somaz94/k8s-namespace-sync/commit/f2d91af2a1fa25b80f123bdb190614b167a1abef))
- **deps:** bump the go-minor group with 8 updates ([5de4632](https://github.com/somaz94/k8s-namespace-sync/commit/5de4632aeef7c42f3ea66cafe5a1ed2d64c0277c))
- **deps:** bump golang from 1.22 to 1.23 in the docker-minor group ([abebe0d](https://github.com/somaz94/k8s-namespace-sync/commit/abebe0d2c0131ed6b99582fce0649cf5c5f91af5))
- **deps:** bump janheinrichmerker/action-github-changelog-generator ([60e6406](https://github.com/somaz94/k8s-namespace-sync/commit/60e6406a33cfeb7f0d9c2a918746b88d973e5008))
- add dependabot.yml & fix changelog-generator.yml ([297926d](https://github.com/somaz94/k8s-namespace-sync/commit/297926d87c82d1121122d9e7aac0539353ede24d))
- fix changelog workflow ([a0558b1](https://github.com/somaz94/k8s-namespace-sync/commit/a0558b1d48775aa911ec435ead93ee473a2ddf2d))
- test github hosted runner complete ([e8b43d6](https://github.com/somaz94/k8s-namespace-sync/commit/e8b43d673aa16c791b7a14ea8891dbcd8c165041))
- test github hosted runner ([6fe2397](https://github.com/somaz94/k8s-namespace-sync/commit/6fe2397cc40258b8e7e6ccaf09d1296c01266847))
- fix changelog workflow ([c7339e2](https://github.com/somaz94/k8s-namespace-sync/commit/c7339e29a988a53fb909954a5800465735e7dc17))
- fix changelog workflow ([eadca56](https://github.com/somaz94/k8s-namespace-sync/commit/eadca56f4c94d8e36d6a6d8423998f75bfc873d5))
- fix changelog workflow ([1609a04](https://github.com/somaz94/k8s-namespace-sync/commit/1609a04627d971027249761dd683987099b73006))
- fix changelog workflow ([5c48e5d](https://github.com/somaz94/k8s-namespace-sync/commit/5c48e5d3dbfef3682123aa10ba6835867f499c26))
- fix changelog-generator workflow ([91e8c6b](https://github.com/somaz94/k8s-namespace-sync/commit/91e8c6b2e934ac8804da70602637729f9673f6c2))
- fix test, test-e2e workflow ([74ec97b](https://github.com/somaz94/k8s-namespace-sync/commit/74ec97b337ec7713c9a165d4d8405c200891610a))
- fix changelog workflow ([fe70de1](https://github.com/somaz94/k8s-namespace-sync/commit/fe70de1ec1d8d2afc347618c4a6eddbc3bdc6d28))
- fix changelog workflow ([98b6a78](https://github.com/somaz94/k8s-namespace-sync/commit/98b6a7884af6626d289da0c9ea4bdf54bcd95108))
- fix changelog workflow ([42b3bd5](https://github.com/somaz94/k8s-namespace-sync/commit/42b3bd5993ae5c45c70563d6e220803e81efff24))
- fix changelog workflow ([13527ed](https://github.com/somaz94/k8s-namespace-sync/commit/13527edf8bff3d9c07368415251db3fffce3e176))
- fix changelog workflow ([fe42f51](https://github.com/somaz94/k8s-namespace-sync/commit/fe42f51333509f23e25b77f8a1baebcfa87d483b))
- add changelog workflow ([b5a3597](https://github.com/somaz94/k8s-namespace-sync/commit/b5a3597aa5435f15b6ea5cef92dff591b3b5659e))

### Contributors

- somaz

<br/>

## [v0.1.6](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.5...v0.1.6) (2024-12-20)

### Bug Fixes

- bin ([f215f5e](https://github.com/somaz94/k8s-namespace-sync/commit/f215f5e78fbab55982e0415e2743098f3bba7277))
- github workflow ([8553a2a](https://github.com/somaz94/k8s-namespace-sync/commit/8553a2a68ba4eefdd05c4fde5192a14f12b5dc5f))
- github workflow ([9c07067](https://github.com/somaz94/k8s-namespace-sync/commit/9c070678a100593b2aa1ce029578ae9887bd6971))
- helm ([5112a3e](https://github.com/somaz94/k8s-namespace-sync/commit/5112a3ee065b0e238aa27144f9c369f27e5be385))
- Makefile & README.md ([e7f9c4a](https://github.com/somaz94/k8s-namespace-sync/commit/e7f9c4a50a78decd3c6cd8b54d1b1a40ae4f73e5))
- helm/k8s-namespace-sync/values.yaml ([55d0513](https://github.com/somaz94/k8s-namespace-sync/commit/55d0513011ba1f3cea15f892b672a91c89f90a1e))

### Documentation

- helm/README.md ([8b03747](https://github.com/somaz94/k8s-namespace-sync/commit/8b03747e5477b255f9e6953f400727356f67759a))
- README.md ([93fa515](https://github.com/somaz94/k8s-namespace-sync/commit/93fa515c264ec485a65695914259adb2d38337a9))
- README.md ([a6db6d5](https://github.com/somaz94/k8s-namespace-sync/commit/a6db6d50a38f3a16d7a3e80bcc150488c7bcca1b))

### Chores

- fix go mod ([3951719](https://github.com/somaz94/k8s-namespace-sync/commit/3951719000fbb358a3f78511d69e26932c6eedf7))
- linting ([08c4aac](https://github.com/somaz94/k8s-namespace-sync/commit/08c4aac4450546b98d9ca00ad55d04a2070942c5))
- divide test code ([0ec81c3](https://github.com/somaz94/k8s-namespace-sync/commit/0ec81c3b222f5e696c5c8b28b609c97742826b55))
- fix Makefile ([34fe237](https://github.com/somaz94/k8s-namespace-sync/commit/34fe2377ee544e5519d0b3c4adba39dcd3eedf58))

### Contributors

- somaz

<br/>

## [v0.1.5](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.4...v0.1.5) (2025-02-21)

### Bug Fixes

- helm/k8s-namespace-sync/values.yaml ([d94c334](https://github.com/somaz94/k8s-namespace-sync/commit/d94c33434d9e815653dea6918d583521d85dce3a))

### Documentation

- helm/README.md ([2d0c650](https://github.com/somaz94/k8s-namespace-sync/commit/2d0c6503e0a6e65c8fb5b9e14fb2f6e9ecc07989))
- helm/README.md ([bfeb72a](https://github.com/somaz94/k8s-namespace-sync/commit/bfeb72aa5909488edd137cbe177ba8996c85697f))

### Chores

- fix filter ([0232203](https://github.com/somaz94/k8s-namespace-sync/commit/0232203a8917d4ab4a1434b584f4f9807ae4bbac))

### Contributors

- somaz

<br/>

## [v0.1.4](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.3...v0.1.4) (2025-02-21)

### Documentation

- helm/README.md ([5b5a9d6](https://github.com/somaz94/k8s-namespace-sync/commit/5b5a9d6a3edbfd21795ff37bf94d252fa2e40036))
- README.md ([d635a33](https://github.com/somaz94/k8s-namespace-sync/commit/d635a33c158d096e56b0f8bb204008847f82c4e9))
- README.md ([8bd5cb5](https://github.com/somaz94/k8s-namespace-sync/commit/8bd5cb530e8dfd300808be6bb2fbf6f3eac58b9b))
- README.md ([1716202](https://github.com/somaz94/k8s-namespace-sync/commit/1716202453ad6af49ba8893c62653bb3eb146f27))

### Chores

- fix filter ([bdb6bc9](https://github.com/somaz94/k8s-namespace-sync/commit/bdb6bc9a3b960f6e90012259809292de4c20fada))
- add customresource template ([2e68945](https://github.com/somaz94/k8s-namespace-sync/commit/2e68945d52cc02bc61a4fa08c2f7ce29119507b0))
- add helm ([adf632f](https://github.com/somaz94/k8s-namespace-sync/commit/adf632f799de11e9843514ea0575b11a8a4eea8f))
- fix github workflow ([af87e3f](https://github.com/somaz94/k8s-namespace-sync/commit/af87e3fbd1097c1a29655208396f07ecbb6b2be3))

### Release

- v0.1.3 ([737d919](https://github.com/somaz94/k8s-namespace-sync/commit/737d91939e0669e64129cd87231c1af13250515c))

### Contributors

- somaz

<br/>

## [v0.1.3](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.2...v0.1.3) (2025-02-21)

### Documentation

- README.md ([8129bbc](https://github.com/somaz94/k8s-namespace-sync/commit/8129bbcbf97ee347bc6d9a840082438665a8fe02))

### Release

- v0.1.3 ([4dd4fbf](https://github.com/somaz94/k8s-namespace-sync/commit/4dd4fbf7a3d14ddec6aa0853064a5a4b33bd9e80))
- v0.1.2 ([c1976ca](https://github.com/somaz94/k8s-namespace-sync/commit/c1976ca6f6ed0111739332e17795aa8c5bd53bc8))

### Contributors

- somaz

<br/>

## [v0.1.2](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.1...v0.1.2) (2025-02-21)

### Chores

- delete dist/install.yaml ([7f353d2](https://github.com/somaz94/k8s-namespace-sync/commit/7f353d27a493bd79a7421a8cdb996f4dabb6bcd3))
- add exclude namespace function ([a58abba](https://github.com/somaz94/k8s-namespace-sync/commit/a58abbaf6c3e35555d07d8a171802ff5a9e52153))

### Release

- v0.1.2 ([6e6b4f0](https://github.com/somaz94/k8s-namespace-sync/commit/6e6b4f07a1cd9b43601fdf8f82cfcc6e1261d7b7))

### Contributors

- somaz

<br/>

## [v0.1.1](https://github.com/somaz94/k8s-namespace-sync/compare/v0.1.0...v0.1.1) (2025-02-21)

### Documentation

- README.md ([f990756](https://github.com/somaz94/k8s-namespace-sync/commit/f9907564d99b5d51ac3eb4e334e7fac0920f2b8e))

### Chores

- add exclude namespace function ([7cc94ad](https://github.com/somaz94/k8s-namespace-sync/commit/7cc94ad2d94afd8240072cb1d471de4c2a7613da))

### Contributors

- somaz

<br/>

## [v0.1.0](https://github.com/somaz94/k8s-namespace-sync/releases/tag/v0.1.0) (2025-02-21)

### Bug Fixes

- api,cm,config,internal ([ff78dcb](https://github.com/somaz94/k8s-namespace-sync/commit/ff78dcb93e455afd8e7e15e276fe5d6bc3c71fff))
- all ([4bf937f](https://github.com/somaz94/k8s-namespace-sync/commit/4bf937f430955cb38fbd26505c3d56d8f1fc013d))

### Documentation

- README.md ([3fb0353](https://github.com/somaz94/k8s-namespace-sync/commit/3fb0353d08d73da04645f8bae36e3c94008f5964))

### Chores

- add release ([991a869](https://github.com/somaz94/k8s-namespace-sync/commit/991a869a1dd9863657b6b26dfb80b4acea8d2bd6))

### Contributors

- somaz

<br/>

