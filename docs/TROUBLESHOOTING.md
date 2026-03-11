# Troubleshooting

<br/>

## Helm Test Issues

### `UPGRADE FAILED: "release-name" has no deployed releases`

A previous failed install/uninstall left a stuck release.

```bash
# Check stuck releases
helm list -a --all-namespaces | grep <release-name>

# Force cleanup
helm uninstall <release-name> --no-hooks
kubectl delete ns k8s-namespace-sync-system --ignore-not-found
```

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

### Helm uninstall hangs

The pre-delete hook job is failing repeatedly.

```bash
# Cancel and force uninstall
helm uninstall <release-name> --no-hooks

# Clean up namespace
kubectl delete ns k8s-namespace-sync-system --ignore-not-found
```

<br/>

## Controller Issues

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

### CRD not found

```bash
# Verify CRD exists
kubectl get crd namespacesyncs.sync.nsync.dev

# Reinstall CRDs
make install
```

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

### Synced resources not cleaned up after CR deletion

```bash
# Check if controller is running
kubectl get pods -n k8s-namespace-sync-system

# If controller is down, manually remove finalizer
kubectl patch namespacesync <name> -p '{"metadata":{"finalizers":null}}' --type=merge

# Manually clean up orphaned resources
kubectl delete configmap <name> -n <target-ns>
kubectl delete secret <name> -n <target-ns>
```

### Resource filter not working as expected

- Patterns use glob matching (e.g., `*2` matches `test-configmap2`)
- Exclude takes precedence over include
- Namespace exclude and resource filter are independent

```bash
# Verify filter configuration
kubectl get namespacesync <name> -o jsonpath='{.spec.resourceFilters}'
```

<br/>

## CI/CD Issues

### `git push` rejected (remote ahead)

Workflow-generated commits (CHANGELOG.md) can make remote ahead.

```bash
git pull --rebase origin main
git push origin main
```

### Release workflow: `GITHUB_TOKEN` doesn't trigger other workflows

This is expected. Use `PAT_TOKEN` for operations that need to trigger downstream workflows.

### Dependabot PR merge fails: OAuth token lacks `workflow` scope

Dependabot PRs that modify `.github/workflows/` files need the `workflow` scope. Merge these manually via GitHub web UI.

<br/>

## Build Issues

### `make manifests generate` shows diff in CI

Generated files are out of date. Run locally and commit:

```bash
make manifests generate
git add config/ api/
git commit -m "chore: update generated manifests"
```
