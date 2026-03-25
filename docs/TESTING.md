# k8s-namespace-sync - Test Guide

<br/>

## Prerequisites

- Kubernetes cluster (Kind, Minikube, EKS, GKE, etc.)
- `kubectl` configured
- `make`, `helm`, `go` installed

<br/>

## 1. Unit Tests

```bash
make test
```

Runs all unit and envtest-based tests with coverage report (`cover.out`).

<br/>

## 2. Integration Test (Automated)

Run all sample tests automatically against a live cluster:

```bash
make test-integration
```

The script will:
- Auto-install CRD if not found (`make install`)
- Auto-deploy controller if not running (`make deploy`)
- Apply sample NamespaceSync resources and verify resource synchronization

<br/>

## 3. Helm Chart Test (Automated)

Run Helm chart tests (lint, template, install, sync tests, uninstall):

```bash
make test-helm
```

Test coverage:
- Helm lint, template render
- Helm install & release verification
- Controller pod, CRD, RBAC verification
- NamespaceSync CR tests (basic, exclude, filter, target)
- ConfigMap and Secret synchronization checks
- Helm uninstall & cleanup verification

<br/>

## 4. E2E Tests

Requires a Kind cluster:

```bash
make test-e2e
```

The test will verify that Kind is installed and a cluster is running before executing Ginkgo-based end-to-end tests.

<br/>

## 5. Manual Deploy Test

<br/>

### Step 1: Deploy Controller (CRD + RBAC + Controller)

```bash
make deploy
```

<br/>

### Step 2: Verify Controller is Running

```bash
kubectl get pods -n k8s-namespace-sync-system
```

<br/>

### Step 3: Create Test Resources

```bash
kubectl apply -f config/samples/test-configmap.yaml
kubectl apply -f config/samples/test-secret.yaml
```

<br/>

### Step 4: Apply NamespaceSync CR

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check:

```bash
kubectl get namespacesync
kubectl get configmaps -A | grep test-configmap
kubectl get secrets -A | grep test-secret
```

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test A: Basic Sync (All Namespaces)

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check:

```bash
kubectl get namespacesync
kubectl get configmaps -A | grep test-configmap
kubectl get secrets -A | grep test-secret
```

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test B: Namespace Exclusion

```bash
kubectl apply -f config/samples/sync_v1_namespacesync_exclude.yaml
```

Check:

```bash
kubectl get namespacesync
kubectl describe namespacesync namespacesync-sample-exclude
```

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test C: Resource Filters

```bash
kubectl apply -f config/samples/sync_v1_namespacesync_filter.yaml
```

Check:

```bash
kubectl get namespacesync
kubectl describe namespacesync namespacesync-sample-filter
```

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test D: Target Namespaces

```bash
kubectl apply -f config/samples/sync_v1_namespacesync_target.yaml
```

Check:

```bash
kubectl get namespacesync
kubectl describe namespacesync namespacesync-sample-target
```

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test E: Cleanup on Deletion

Verifies that synced resources are deleted when the NamespaceSync CR is deleted via finalizer.

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check that resources are synced:

```bash
kubectl get configmaps -A | grep test-configmap
kubectl get secrets -A | grep test-secret
```

Delete the CR and verify cleanup:

```bash
kubectl delete namespacesync namespacesync-sample
kubectl get configmaps -A | grep test-configmap
kubectl get secrets -A | grep test-secret
```

The synced resources should be removed from all target namespaces after deletion.

<br/>

### Test F: Dynamic Namespace Detection

Verifies that resources are synced to newly created namespaces at runtime.

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Create a new namespace after the CR is applied:

```bash
kubectl create ns dynamic-test-ns
```

Check that resources are automatically synced to the new namespace:

```bash
kubectl get configmap test-configmap -n dynamic-test-ns
kubectl get secret test-secret -n dynamic-test-ns
```

Cleanup:

```bash
kubectl delete namespacesync --all
kubectl delete ns dynamic-test-ns
```

<br/>

### Test G: Event Recording

Verifies that Kubernetes events (SyncComplete) are emitted after sync operations.

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check:

```bash
kubectl describe namespacesync namespacesync-sample
kubectl get events --field-selector involvedObject.kind=NamespaceSync
```

You should see `SyncComplete` events recorded on the NamespaceSync resource.

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test H: Status Conditions

Verifies that the Ready condition reason (SyncComplete) and message contains namespace info.

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check:

```bash
kubectl get namespacesync namespacesync-sample -o jsonpath='{.status.conditions}'
```

The Ready condition should have:
- `status: "True"`
- `reason: "SyncComplete"`
- `message` containing the number of synced namespaces

Cleanup:

```bash
kubectl delete namespacesync --all
```

<br/>

### Test I: Sync Metadata Annotations

Verifies that `namespacesync.nsync.dev/source-namespace` and `namespacesync.nsync.dev/last-sync` annotations are present on synced resources.

```bash
kubectl apply -f config/samples/sync_v1_namespacesync.yaml
```

Check annotations on a synced resource in a target namespace:

```bash
kubectl get configmap test-configmap -n <target-namespace> -o jsonpath='{.metadata.annotations}'
```

You should see:
- `namespacesync.nsync.dev/source-namespace` set to the source namespace name
- `namespacesync.nsync.dev/last-sync` set to a timestamp in RFC3339 format

Cleanup:

```bash
kubectl delete namespacesync --all
```

---

## 6. Full Cleanup

```bash
# Remove all test resources
kubectl delete -f config/samples/test-configmap.yaml --ignore-not-found
kubectl delete -f config/samples/test-configmap2.yaml --ignore-not-found
kubectl delete -f config/samples/test-secret.yaml --ignore-not-found
kubectl delete -f config/samples/test-secret2.yaml --ignore-not-found
kubectl delete namespacesync --all --ignore-not-found

# Undeploy controller (removes CRD + RBAC + Controller)
make undeploy
```

---

## Sample Files

| File | Type | Description |
|------|------|-------------|
| `sync_v1_namespacesync.yaml` | Basic | Sync ConfigMap and Secret to all namespaces |
| `sync_v1_namespacesync_exclude.yaml` | Exclude | Sync with namespace exclusion list |
| `sync_v1_namespacesync_filter.yaml` | Filter | Sync with resource include/exclude filters |
| `sync_v1_namespacesync_target.yaml` | Target | Sync to specific target namespaces only |
| `test-configmap.yaml` | Test | Sample ConfigMap with app properties and JSON config |
| `test-configmap2.yaml` | Test | Second sample ConfigMap for multi-resource tests |
| `test-secret.yaml` | Test | Sample Secret with credentials and JSON config |
| `test-secret2.yaml` | Test | Second sample Secret for multi-resource tests |
| `kustomization.yaml` | Kustomize | Kustomize configuration for sample resources |
