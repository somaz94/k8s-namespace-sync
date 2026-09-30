# Troubleshooting

<br/>

## Helm Test Issues

<br/>

### `UPGRADE FAILED: "release-name" has no deployed releases`

A previous failed install/uninstall left a stuck release.

```bash
# Check stuck releases
helm list -a --all-namespaces | grep <release-name>

# Force cleanup
helm uninstall <release-name> --no-hooks
kubectl delete ns k8s-namespace-sync-system --ignore-not-found
```

<br/>

### CRD cleanup hook fails: `BackoffLimitExceeded`

The cleanup job's ServiceAccount lacks `apiextensions.k8s.io` CRD delete permission.

```bash
# Force uninstall without hooks
helm uninstall <release-name> --no-hooks

# Manually delete CRD if needed
kubectl delete crd namespacesyncs.sync.nsync.dev --ignore-not-found

# Delete stuck job
kubectl delete job -n k8s-namespace-sync-system -l app.kubernetes.io/name=k8s-namespace-sync --ignore-not-found
```

<br/>

### Helm uninstall hangs

The pre-delete hook Job, which deletes the CRD, has not finished. Charts before 0.5.0 gave it no permission to watch the CRD, so the `kubectl delete` inside it waited forever. A release keeps the hook it was installed or upgraded with, so upgrade to a newer chart before uninstalling. Otherwise look at the Job's pod: an image it cannot pull, a pod stuck in Pending, or a stopped controller that leaves NamespaceSync finalizers in place all keep it running.

```bash
# Inspect the hook Job; a failed pod is kept for its logs
kubectl logs -n k8s-namespace-sync-system job/<release-name>-k8s-namespace-sync-crd-cleanup

# Cancel and force uninstall
helm uninstall <release-name> --no-hooks

# Clean up namespace
kubectl delete ns k8s-namespace-sync-system --ignore-not-found
```

<br/>

## Controller Issues

<br/>

### Controller pod is CrashLoopBackOff

```bash
# Check logs
kubectl logs -n k8s-namespace-sync-system deployment/k8s-namespace-sync-controller-manager --previous

# Check events
kubectl describe pod -n k8s-namespace-sync-system -l control-plane=controller-manager
```

Common causes:
- CRD not installed: Run `make install` or reinstall Helm chart
- RBAC permission denied: Check ClusterRole and ClusterRoleBinding
- Port conflict: Metrics (8443) or health probe (8081) port already in use

<br/>

### CRD not found

```bash
# Verify CRD exists
kubectl get crd namespacesyncs.sync.nsync.dev

# Reinstall CRDs
make install
```

<br/>

### Resources not syncing to target namespaces

```bash
# Check CR status
kubectl get namespacesync <name> -o yaml

# Verify Ready condition
kubectl get namespacesync <name> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'

# Check controller logs
kubectl logs -n k8s-namespace-sync-system deployment/k8s-namespace-sync-controller-manager -f

# Check if namespace is excluded (system namespaces are auto-excluded)
# Auto-excluded: kube-system, kube-public, kube-node-lease, default, k8s-namespace-sync-system
```

<br/>

### Synced resources not cleaned up after CR deletion

Cleanup deletes only copies that carry the controller's `namespacesync.nsync.dev/source-*` annotations. An object with the same name that the controller never synced is left in place on purpose, and so is a copy that another NamespaceSync still syncs from the same source or an object another NamespaceSync reads as its source.

```bash
# Check whether the object is a synced copy
kubectl get secret <name> -n <target-ns> -o jsonpath='{.metadata.annotations}'

# Check if controller is running
kubectl get pods -n k8s-namespace-sync-system

# If controller is down, manually remove finalizer
kubectl patch namespacesync <name> -p '{"metadata":{"finalizers":null}}' --type=merge

# Manually clean up orphaned resources
kubectl delete configmap <name> -n <target-ns>
kubectl delete secret <name> -n <target-ns>
```

<br/>

### Resource filter not working as expected

- Patterns use glob matching (e.g., `*2` matches `test-configmap2`)
- Exclude takes precedence over include
- Namespace exclude and resource filter are independent
- A pattern that is not a valid glob (e.g., `[abc`) fails validation: the Ready condition shows reason `InvalidSpec` with the error, and a `ValidationFailed` event is recorded

```bash
# Verify filter configuration
kubectl get namespacesync <name> -o jsonpath='{.spec.resourceFilters}'
```

<br/>

### A namespace is listed in `failedNamespaces` with `sync conflict`

Another NamespaceSync owns the object: it reads that object as its source, or it still syncs that copy from a different source namespace. The message names the other NamespaceSync, and the README section "Overlapping NamespaceSyncs" has the rules. To resolve it, list the namespace in `targetNamespaces` of the NamespaceSync that should own the copy, narrow the other one with `targetNamespaces` or `exclude`, or delete one of them. A change to the other NamespaceSync does not retrigger this one, so the conflict can take up to the reconcile interval to clear.

```bash
# See which namespaces conflict and why
kubectl get namespacesync <name> -o jsonpath='{.status.failedNamespaces}'
kubectl get events --field-selector reason=SyncConflict
```

<br/>

### `sourceNamespace is immutable` on apply

`spec.sourceNamespace` cannot change after creation. Delete the NamespaceSync, whose finalizer removes the copies it made, and create a new one with the new source.

<br/>

### Copies left behind by an earlier `sourceNamespace` change

Before v0.5.0 the source could be changed in place, which left the copies from the old source behind; cleanup never matches them. Find them by their source annotation and delete the ones you no longer need:

```bash
kubectl get secrets,configmaps -A -o json | jq -r '.items[]
  | select(.metadata.annotations["namespacesync.nsync.dev/source-namespace"] == "<old-source>")
  | "\(.kind) \(.metadata.namespace)/\(.metadata.name)"'
```

<br/>

## CI/CD Issues

<br/>

### `git push` rejected (remote ahead)

Workflow-generated commits (CHANGELOG.md) can make remote ahead.

```bash
git pull --rebase origin main
git push origin main
```

<br/>

### Release workflow: `GITHUB_TOKEN` doesn't trigger other workflows

This is expected. Use `PAT_TOKEN` for operations that need to trigger downstream workflows.

<br/>

### Dependabot PR merge fails: OAuth token lacks `workflow` scope

Dependabot PRs that modify `.github/workflows/` files need the `workflow` scope. Merge these manually via GitHub web UI.

<br/>

## Build Issues

<br/>

### `make manifests generate` shows diff in CI

Generated files are out of date. Run locally and commit:

```bash
make manifests generate
git add config/ api/
git commit -m "chore: update generated manifests"
```
