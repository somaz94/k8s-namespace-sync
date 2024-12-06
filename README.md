# K8s Namespace Sync

![Top Language](https://img.shields.io/github/languages/top/somaz94/k8s-namespace-sync?color=green&logo=go&logoColor=b)
![Latest Tag](https://img.shields.io/github/v/tag/somaz94/k8s-namespace-sync)

K8s Namespace Sync is a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across multiple namespaces within a Kubernetes cluster.

## Features

- Automatic synchronization of Secrets and ConfigMaps across namespaces
- Automatic detection and synchronization of changes in source namespace
- Support for selective namespace targeting
- Automatic exclusion of system namespaces (kube-system, kube-public, etc.)
- Support for manually excluding specific namespaces
- Prometheus metrics support
- Synchronization status monitoring
- Resource filtering support with glob pattern matching
- Selective resource synchronization based on name patterns

## How it Works

The controller:
1. Watches for changes in the source namespace's Secrets and ConfigMaps
2. Automatically syncs changes to all target namespaces
3. Maintains consistency by cleaning up resources when source is deleted
4. Uses finalizers to ensure proper cleanup during deletion

## Installation
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/install.yaml
```

## Usage

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

### 2. Create a NamespaceSync CR:

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

Basic apply the CR:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync.yaml
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
    - production
    - staging
  configMapName:
    - test-configmap
  secretName:
    - test-secret
```

Target apply the CR:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_target.yaml
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

Exclude apply the CR:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_exclude.yaml
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
      #   - "test-configmap*"    # All ConfigMaps starting with test-configmap
      exclude:
        - "*2"            # Exclude ConfigMaps ending with 2
    secrets:
      # include:
      #   - "test-secret*"   # All Secrets starting with test-secret
      exclude:
        - "*2"            # Exclude Secrets ending with 2
  exclude:
    - test-ns2
    - test-ns3
```

Filter apply the CR:
```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_filter.yaml
```

### Sync Behavior
- If `targetNamespaces` is not specified, resources will be synced to all namespaces (except excluded ones)
- If `targetNamespaces` is specified, resources will only be synced to the listed namespaces
- System namespaces and source namespace are always excluded
- `exclude` list takes precedence over `targetNamespaces`
- Changes in source resources are automatically detected and synced in real-time
- Deleting a resource from the source namespace will remove it from all synced namespaces
- Labels and annotations from the source resources are preserved in synced resources
- When the NamespaceSync CR is deleted, all synced resources are automatically cleaned up
- Finalizer ensures proper cleanup of synced resources before CR deletion

### Resource Filtering
- Supports both include and exclude patterns for ConfigMaps and Secrets
- Uses glob pattern matching (e.g., "*" for any characters)
- Include patterns specify which resources to sync
- Exclude patterns specify which resources to skip
- If both include and exclude patterns are specified, exclude takes precedence
- Patterns are matched against resource names
- Can be combined with namespace targeting and exclusion

## Verification

### 1. Check synchronization status:

```bash
kubectl get namespacesync namespacesync-sample -o yaml
```

### 2. Verify resources in other namespaces:

```bash
kubectl get secret test-secret -n target-namespace
kubectl get configmap test-configmap -n target-namespace
```

## Excluded Namespaces

The following namespaces are automatically excluded from synchronization:
- kube-system
- kube-public
- kube-node-lease
- k8s-namespace-sync-system

Additionally, you can manually exclude specific namespaces using the `exclude` field in the NamespaceSync CR.

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

## Cleanup

1. Delete the NamespaceSync CR:

```bash
kubectl delete namespacesync namespacesync-sample
kubectl delete namespacesync namespacesync-sample-targets
kubectl delete namespacesync namespacesync-sample-exclude
```

2. Remove the controller:

```bash
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/install.yaml
```

## Contributing

Issues and pull requests are welcome.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

