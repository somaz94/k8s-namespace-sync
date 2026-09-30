# K8s Namespace Sync

> Kubernetes operator that automatically syncs Secrets & ConfigMaps across namespaces — glob filtering, auto-detection, and Prometheus metrics.

![Top Language](https://img.shields.io/github/languages/top/somaz94/k8s-namespace-sync?color=green&logo=go&logoColor=b)
![k8s-namespace-sync](https://img.shields.io/github/v/tag/somaz94/k8s-namespace-sync?label=k8s-namespace-sync&logo=kubernetes&logoColor=white)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/somaz94/k8s-namespace-sync)](https://goreportcard.com/report/github.com/somaz94/k8s-namespace-sync)
![Docker Pulls](https://img.shields.io/docker/pulls/somaz940/k8s-namespace-sync?logo=docker&logoColor=white)
![GitHub Release](https://img.shields.io/github/release/somaz94/k8s-namespace-sync?logo=github)
![GitHub Stars](https://img.shields.io/github/stars/somaz94/k8s-namespace-sync?style=social)

K8s Namespace Sync is a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across multiple namespaces within a Kubernetes cluster.

<br/>

## Features

![Secret Sync](https://img.shields.io/badge/Secret_Sync-blue?logo=kubernetes&logoColor=white)
![ConfigMap Sync](https://img.shields.io/badge/ConfigMap_Sync-blue?logo=kubernetes&logoColor=white)
![Glob Filter](https://img.shields.io/badge/Glob_Filter-green?logo=kubernetes&logoColor=white)
![Auto Detect](https://img.shields.io/badge/Auto_Detect-green?logo=kubernetes&logoColor=white)
![Namespace Target](https://img.shields.io/badge/Namespace_Target-orange?logo=kubernetes&logoColor=white)
![Prometheus](https://img.shields.io/badge/Prometheus_Metrics-E6522C?logo=prometheus&logoColor=white)
![Finalizer](https://img.shields.io/badge/Finalizer_Cleanup-326CE5?logo=kubernetes&logoColor=white)

- Automatic synchronization of Secrets and ConfigMaps across namespaces
- Automatic detection and synchronization of changes in source namespace
- Support for selective namespace targeting
- Automatic exclusion of system namespaces (kube-system, kube-public, etc.)
- Support for manually excluding specific namespaces
- Prometheus metrics support
- Synchronization status monitoring
- Resource filtering support with glob pattern matching
- Selective resource synchronization based on name patterns

<br/>

## How it Works

The controller:
1. Watches for changes in the source namespace's Secrets and ConfigMaps
2. Automatically syncs changes to all target namespaces
3. Maintains consistency by cleaning up resources when source is deleted
4. Uses finalizers to ensure proper cleanup during deletion

<br/>

## Installation

<br/>

### Prerequisites
- Kubernetes v1.25+ (the CRD ships CEL validation rules, which require `x-kubernetes-validations`)
- kubectl v1.11.3+

<br/>

### Option 1: Helm (Recommended)

**Recommended: OCI registry (Helm 3.8+)**

```bash
# Single-command install — no helm repo add needed
helm install k8s-namespace-sync oci://ghcr.io/somaz94/charts/k8s-namespace-sync \
  --version 0.5.0
```

**Alternative: classic Helm repo**

```bash
# Add the Helm repository
helm repo add k8s-namespace-sync https://somaz94.github.io/k8s-namespace-sync/helm-repo
helm repo update

# Install with default values
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync

# Or install with custom values
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync \
  --set image.tag=v0.5.0
```

The chart creates the namespace named by its `namespace` value (`k8s-namespace-sync-system` unless you change it), so do not add `--namespace k8s-namespace-sync-system --create-namespace`: Helm would create that namespace first, and the install then fails with `namespaces "k8s-namespace-sync-system" already exists`.

For full Helm chart options, see [Helm README](docs/HELM.md).

<br/>

### Option 2: kubectl apply (Quick Install)

```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/dist/install.yaml
```

<br/>

### Option 3: Build from Source

```bash
# Clone the repository
git clone https://github.com/somaz94/k8s-namespace-sync.git
cd k8s-namespace-sync

# Install CRDs
make install

# Deploy the controller
make deploy IMG=somaz940/k8s-namespace-sync:v0.5.0
```

<br/>

## Usage

<br/>

### 1. Create a Secret or ConfigMap in the source namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: test-secret
  namespace: default
type: Opaque
stringData:
  username: admin
  password: secret123
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: default
data:
  key1: value1
  key2: value2
```

if you want to test with multiple resources, you can apply the following resources:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/test-configmap-secret/test-configmap.yaml
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/test-configmap-secret/test-configmap2.yaml
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/test-configmap-secret/test-secret.yaml
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/test-configmap-secret/test-secret2.yaml
```

<br/>

### 2. Create a NamespaceSync CR:

- **Important Note**: Each CR should be tested individually to avoid conflicts. After testing each CR, make sure to delete it before testing the next one.

Basic synchronization (sync to all namespaces):

```yaml
apiVersion: sync.nsync.dev/v1
kind: NamespaceSync
metadata:
  name: namespacesync-sample
  finalizers:
    - namespacesync.nsync.dev/finalizer
spec:
  sourceNamespace: default
  configMapName:
    - test-configmap
  secretName:
    - test-secret
```

Basic apply and test:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync.yaml

# Check the resources in the target namespaces
k get namespacesyncs.sync.nsync.dev

# Delete the CR after testing
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync.yaml
```

With specific target namespaces:

```yaml
apiVersion: sync.nsync.dev/v1
kind: NamespaceSync
metadata:
  name: namespacesync-sample-targets
  finalizers:
    - namespacesync.nsync.dev/finalizer
spec:
  sourceNamespace: default
  targetNamespaces:  # Only sync to these namespaces
    - test-ns1
    - test-ns2
  configMapName:
    - test-configmap
  secretName:
    - test-secret
```

Target apply and test:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_target.yaml

# Check the resources in the target namespaces
kubectl get namespacesyncs.sync.nsync.dev

# Create target namespaces
kubectl create ns test-ns1
kubectl create ns test-ns2

# Check the resources in the target namespaces
kubectl get secret,configmap -n test-ns1
kubectl get secret,configmap -n test-ns2

# Check the resources in the another namespace
kubectl get secret,configmap -n <another-namespace>

# Delete the CR after testing
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_target.yaml
```

With excluded namespaces:

```yaml
apiVersion: sync.nsync.dev/v1
kind: NamespaceSync
metadata:
  name: namespacesync-sample
  finalizers:
    - namespacesync.nsync.dev/finalizer
spec:
  sourceNamespace: default
  configMapName:
    - test-configmap
    - test-configmap2
  secretName:
    - test-secret
    - test-secret2
  exclude:
    - test-ns2
    - test-ns3

```

Exclude apply and test:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_exclude.yaml

# Check the resources in the target namespaces
k get namespacesyncs.sync.nsync.dev

# Create target namespaces
kubectl create ns test-ns2
kubectl create ns test-ns3

# Check the resources in the target namespaces
kubectl get secret,configmap -n test-ns2
kubectl get secret,configmap -n test-ns3

# Check the resources in the another namespace
kubectl get secret,configmap -n <another-namespace>

# Delete the CR after testing
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_exclude.yaml
```

With resource filters:

```yaml
apiVersion: sync.nsync.dev/v1
kind: NamespaceSync
metadata:
  name: namespacesync-filter-sample
  finalizers:
    - namespacesync.nsync.dev/finalizer
spec:
  sourceNamespace: default
  configMapName:
    - test-configmap
    - test-configmap2
  secretName:
    - test-secret
    - test-secret2
  resourceFilters:
    configMaps:
      # include:
      #   - "test-configmap*"    # Listed ConfigMaps starting with test-configmap
      exclude:
        - "*2"            # Exclude ConfigMaps ending with 2
    secrets:
      # include:
      #   - "test-secret*"   # Listed Secrets starting with test-secret
      exclude:
        - "*2"            # Exclude Secrets ending with 2
  exclude:
    - test-ns2
    - test-ns3
```

Filter apply and test:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_filter.yaml

# Check the resources in the target namespaces
kubectl get namespacesyncs.sync.nsync.dev

# Create target namespaces
kubectl create ns test-ns2
kubectl create ns test-ns3

# Check the resources in the target namespaces
kubectl get secret,configmap -n test-ns2
kubectl get secret,configmap -n test-ns3

# Check the resources in the another namespace
# Check the resources in the filtered configmap
kubectl get secret,configmap -n <another-namespace>

# Delete the CR after testing
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_filter.yaml
```

<br/>

### Sync Behavior
- If `targetNamespaces` is not specified, resources will be synced to all namespaces (except excluded ones)
- If `targetNamespaces` is specified, resources will only be synced to the listed namespaces
- System namespaces and source namespace are always excluded
- `exclude` list takes precedence over `targetNamespaces`
- Changes in source resources are automatically detected and synced in real-time
- Deleting a resource from the source namespace removes its synced copies from the target namespaces
- Labels and annotations from the source resources are preserved in synced resources, except keys under `kubernetes.io/` and `kubectl.kubernetes.io/` (such as `kubectl.kubernetes.io/last-applied-configuration`), which describe the source object itself
- When the NamespaceSync CR is deleted, all synced resources are automatically cleaned up
- Deletion only touches copies the controller created, identified by the `namespacesync.nsync.dev/source-namespace` and `namespacesync.nsync.dev/source-name` annotations. A same-named object it never synced, such as one a resource filter excluded, is left in place. Syncing still overwrites a same-named object that no other NamespaceSync owns, which from then on counts as a synced copy
- Deleting a NamespaceSync keeps a copy that another NamespaceSync still syncs from the same source into that namespace, and an object another NamespaceSync reads as its source
- NamespaceSyncs that sync the same name into the same namespace do not overwrite each other; see [Overlapping NamespaceSyncs](#overlapping-namespacesyncs)
- `spec.sourceNamespace` cannot be changed after creation. To sync from another namespace, delete the NamespaceSync, whose finalizer removes the old copies, and create a new one
- Finalizer ensures proper cleanup of synced resources before CR deletion
- The controller performs periodic reconciliation (default: every 5 minutes) to catch any external drift, ensuring resources remain in sync even if events are missed. The interval is configurable via the `RECONCILE_INTERVAL` environment variable (e.g., `RECONCILE_INTERVAL=10m`)

<br/>

### Overlapping NamespaceSyncs

When several NamespaceSyncs sync the same name into the same namespace:
- A namespace a NamespaceSync reaches only because its `targetNamespaces` is empty never receives an object another NamespaceSync reads as its source, or a copy another NamespaceSync still syncs from a different source namespace
- A namespace listed in `targetNamespaces` is treated as intent. The NamespaceSync keeps its own copy there even when another NamespaceSync reads that copy as its source, so syncs can chain (`a` to `b`, then `b` to `c`), and it takes over a copy from a NamespaceSync that reaches the namespace only by default
- An object created by hand that another NamespaceSync reads as its source is never overwritten
- When two NamespaceSyncs both list the namespace, the copy written first stays
- A skipped object is listed in `status.failedNamespaces` with a `SyncConflict` event, and the other resources in that namespace still sync. A copy whose source no NamespaceSync syncs any more is taken over
- The protection lasts only while the other NamespaceSync exists, so while it is deleted and recreated, a NamespaceSync with an empty `targetNamespaces` can overwrite its source
- One NamespaceSync's spec can block another's writes, so give NamespaceSync edit rights only to people trusted with every namespace it can reach

<br/>

### Kubernetes Events

The controller emits Kubernetes events on NamespaceSync resources to provide visibility into sync operations:

| Event Type | Reason | Description |
|------------|--------|-------------|
| Normal | `SyncComplete` | Emitted after resources are successfully synced to target namespaces |
| Warning | `SyncFailed` | Emitted when sync fails for one or more target namespaces |
| Warning | `ValidationFailed` | Emitted when the NamespaceSync spec fails validation |
| Warning | `SyncConflict` | Emitted when a sync skips an object another NamespaceSync owns (see Sync Behavior) |
| Normal | `CleanupComplete` | Emitted after synced resources are successfully cleaned up during deletion |
| Warning | `CleanupFailed` | Emitted when cleanup of synced resources fails during deletion |

You can view events with:
```bash
kubectl describe namespacesync <name>
# or
kubectl get events --field-selector involvedObject.kind=NamespaceSync
```

<br/>

### Sync Metadata

Synced resources are annotated with metadata that tracks their origin and sync status:

| Annotation | Description |
|------------|-------------|
| `namespacesync.nsync.dev/source-namespace` | The namespace from which the resource was synced |
| `namespacesync.nsync.dev/source-name` | The name of the source resource |
| `namespacesync.nsync.dev/last-sync` | When the controller last wrote the copy's data (RFC3339 format); an update that changes only labels or annotations keeps it |

These annotations allow you to identify which resources are managed by NamespaceSync and trace them back to their source.

Copies keep exactly the labels and annotations of their source. When a mutating admission policy or webhook adds labels or annotations to the copies, a sync removes them and admission adds them back in the same request, so as long as admission writes the same values every time, the stored copy does not change and nothing is rewritten. Admission that changes the data or writes a different value each time, and another controller that edits the copies after they are written, still make every sync rewrite the copy; exclude the copies from those.

<br/>

### Status Conditions

The controller sets a `Ready` condition on each NamespaceSync resource to reflect the current sync state:

| Reason | Status | Description |
|--------|--------|-------------|
| `SyncComplete` | `True` | All target namespaces were synced successfully |
| `PartialSync` | `True` | Some target namespaces were synced, but others failed |
| `SyncFailed` | `False` | All target namespaces failed to sync |
| `InvalidSpec` | `False` | The spec failed validation; the message holds the error |

You can inspect the condition with:
```bash
kubectl get namespacesync <name> -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

<br/>

### Prometheus Metrics

The controller exposes the following Prometheus metrics:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `namespacesync_sync_success_total` | Counter | `namespace`, `resource_type` | Number of successful resource synchronizations |
| `namespacesync_sync_failure_total` | Counter | `namespace`, `resource_type` | Number of failed resource synchronizations, sync conflicts included |
| `namespacesync_cleanup_success_total` | Counter | `namespace`, `resource_type` | Number of synced copies deleted during cleanup |
| `namespacesync_cleanup_failure_total` | Counter | `namespace`, `resource_type` | Number of failed resource cleanups |
| `namespacesync_sync_duration_seconds` | Histogram | `namespace`, `resource_type` | Duration of sync operations in seconds |
| `namespacesync_managed_resources` | Gauge | `namespace`, `resource_type` | Number of resources being managed by NamespaceSync |

<br/>

### Resource Filtering
- Supports both include and exclude patterns for ConfigMaps and Secrets
- Uses glob pattern matching (e.g., "*" for any characters)
- Include patterns specify which resources to sync
- Exclude patterns specify which resources to skip
- If both include and exclude patterns are specified, exclude takes precedence
- Patterns are matched against resource names
- Can be combined with namespace targeting and exclusion
- A pattern that is not a valid glob (for example `[abc`) fails validation: the controller emits a `ValidationFailed` event and sets the Ready condition to `False` with reason `InvalidSpec` instead of syncing

<br/>

## Verification

<br/>

### 1. Check synchronization status:

```bash
kubectl get namespacesync namespacesync-sample -o yaml
```

<br/>

### 2. Verify resources in other namespaces:

```bash
kubectl get secret test-secret -n target-namespace
kubectl get configmap test-configmap -n target-namespace
```

<br/>

## Validation Rules

The CRD enforces the following rules at admission time. They are part of the CRD schema (field constraints and CEL `x-kubernetes-validations`), so an invalid resource is rejected by `kubectl apply` rather than being accepted and then failing on every reconcile.

| Rule | Message |
|---|---|
| `spec.sourceNamespace` must be a non-empty DNS-1123 label (1–63 chars) | `spec.sourceNamespace in body should match ...` |
| At least one of `spec.secretName` or `spec.configMapName` must be non-empty | `at least one secret or configmap must be specified` |
| `spec.sourceNamespace` cannot change after creation | `sourceNamespace is immutable; delete and recreate the NamespaceSync to change it` |

The controller keeps the same checks, except immutability, as a reconcile-time backstop, so a resource created before these rules existed still reports the error in its status instead of syncing nothing silently.

The controller also rejects resource filter patterns that are not valid globs. That check has no admission-time equivalent, so such a resource is accepted by `kubectl apply` and then reports the error in its status.

Note that listing the source namespace in `spec.targetNamespaces`, or a namespace in both `spec.targetNamespaces` and `spec.exclude`, is **not** an error — the source namespace is always skipped and `exclude` is evaluated first.

<br/>

## Excluded Namespaces

The following namespaces are automatically excluded from synchronization:
- kube-system
- kube-public
- kube-node-lease
- k8s-namespace-sync-system

Additionally, you can manually exclude specific namespaces using the `exclude` field in the NamespaceSync CR.

<br/>

## Troubleshooting

Common issues and solutions:

1. **Resources not syncing:**
   - Check if namespace is in exclude list
   - Verify controller logs: `kubectl logs -n namespacesync-system -l control-plane=controller-manager`
   - Check NamespaceSync status: `kubectl get namespacesync <name> -o yaml`

2. **Permission issues:**
   - Ensure RBAC permissions are properly configured
   - Check if ServiceAccount has necessary permissions

3. **Cleanup issues:**
   - Ensure finalizer is present in CR
   - Check controller logs for cleanup errors

<br/>

## Cleanup

1. Delete the ConfigMap and Secret resources:

```bash
kubectl delete configmap test-configmap
kubectl delete secret test-secret
kubectl delete configmap test-configmap2
kubectl delete secret test-secret2
```

2. Remove the NamespaceSync CR:

```bash
kubectl delete namespacesync --all -A
```

3. Remove the controller:

```bash
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/dist/install.yaml
```

<br/>

# Development Setup

<br/>

### Install Required Tools

All required tools will be automatically downloaded to `./bin` directory when running:
```bash
make install-tools
```

Or you can install individual tools:
```bash
# Install controller-gen
make controller-gen  # v0.16.4

# Install kustomize
make kustomize      # v5.5.0

# Install setup-envtest
make envtest        # v0.19.0

# Install golangci-lint
make golangci-lint  # v1.61.0
```

Manual installation locations:
- All tools will be installed in `./bin` directory
- Specific versions:
  - controller-gen v0.16.4
  - kustomize v5.5.0
  - setup-envtest v0.19.0
  - golangci-lint v2.1.6

Note: The binary directory (`./bin`) is git-ignored and will be created when needed.

<br/>

## Documentation

| Document | Description |
|----------|-------------|
| [Helm Chart](docs/HELM.md) | Helm chart installation, configuration, and values reference |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common issues and solutions |
| [Version Bump](docs/VERSION_BUMP.md) | Checklist for releasing a new version |
| [Contributing](CONTRIBUTING.md) | How to contribute to this project |

<br/>

## Contributing

Issues and pull requests are welcome.

<br/>

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

