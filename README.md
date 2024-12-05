 K8s Namespace Sync

K8s Namespace Sync is a Kubernetes controller that automatically synchronizes Secrets and ConfigMaps across multiple namespaces within a Kubernetes cluster.

## Features

- Automatic synchronization of Secrets and ConfigMaps across namespaces
- Automatic detection and synchronization of changes in source namespace
- Automatic exclusion of system namespaces (kube-system, kube-public, etc.)
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

```bash
kubectl apply -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync.yaml
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

## Metrics

The following Prometheus metrics are available:
- `namespacesync_sync_success_total`: Number of successful synchronizations
- `namespacesync_sync_failure_total`: Number of failed synchronizations

## Cleanup

1. Delete the NamespaceSync CR:

```bash
kubectl delete namespacesync namespacesync-sample
```

2. Remove the controller:

```bash
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/examples/sync_v1_namespacesync.yaml
kubectl delete -f https://raw.githubusercontent.com/somaz94/k8s-namespace-sync/main/release/install.yaml
```

## Contributing

Issues and pull requests are welcome.

## License

Apache License 2.0

