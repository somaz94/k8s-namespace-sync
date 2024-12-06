# K8s Namespace Sync

![Top Language](https://img.shields.io/github/languages/top/somaz94/k8s-namespace-sync?color=green&logo=go&logoColor=b)
![Latest Tag](https://img.shields.io/github/v/tag/somaz94/k8s-namespace-sync)

K8s Namespace Sync is a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across multiple namespaces within a Kubernetes cluster.

## Features

- Automatic synchronization of Secrets and ConfigMaps across namespaces
- Automatic detection and synchronization of changes in source namespace
- Automatic exclusion of system namespaces (kube-system, kube-public, etc.)
- Support for manually excluding specific namespaces
- Prometheus metrics support
- Synchronization status monitoring

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

1. Create a Secret or ConfigMap in the source namespace:

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

2. Create a NamespaceSync CR:

Basic synchronization:

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
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync_with_exclude.yaml
```

## Verification

1. Check synchronization status:

```bash
kubectl get namespacesync namespacesync-sample -o yaml
```

2. Verify resources in other namespaces:

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

## Metrics

The following Prometheus metrics are available:
- `namespacesync_sync_success_total`: Number of successful synchronizations
- `namespacesync_sync_failure_total`: Number of failed synchronizations

## Cleanup

1. Delete the NamespaceSync CR:

```bash
kubectl delete namespacesync namespacesync-sample
kubectl delete namespacesync namespacesync-sample-with-exclude
```

2. Remove the controller:

```bash
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/install.yaml
```

## Contributing

Issues and pull requests are welcome.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

