# K8s Namespace Sync

![Top Language](https://img.shields.io/github/languages/top/somaz94/k8s-namespace-sync?color=green&logo=go&logoColor=b)

K8s Namespace Sync is a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across multiple namespaces within a Kubernetes cluster.

## Features

- Automatic synchronization of Secrets and ConfigMaps across namespaces
- Automatic detection and synchronization of changes in source namespace
- Automatic exclusion of system namespaces (kube-system, kube-public, etc.)
- Support for manually excluding specific namespaces
- Prometheus metrics support
- Synchronization status monitoring

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

2. Create a NamespaceSync CR:

Basic synchronization:

```yaml
apiVersion: sync.nsync.dev/v1
kind: NamespaceSync
metadata:
  name: namespacesync-sample
spec:
  sourceNamespace: default
  configMapName: test-configmap
  secretName: test-secret
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
  name: namespacesync-sample-with-exclude
spec:
  sourceNamespace: default
  configMapName: test-configmap
  secretName: test-secret
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

