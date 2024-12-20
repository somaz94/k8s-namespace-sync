# K8s Namespace Sync Helm Chart

## Introduction
This Helm chart installs K8s Namespace Sync Controller on your Kubernetes cluster. The controller automatically synchronizes ConfigMaps and Secrets across multiple namespaces.

## Prerequisites
- Kubernetes 1.16+
- Helm 3.0+

## Installing the Chart

Add the Helm repository:
```bash
helm repo add k8s-namespace-sync https://somaz94.github.io/k8s-namespace-sync/helm-repo
helm repo update
```

Install the chart:
```bash
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync
```

To install with custom values:
```bash
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync -f values.yaml
```

## Configuration

The following table lists the configurable parameters of the k8s-namespace-sync chart and their default values:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `namespace` | Namespace where the controller will be installed | `k8s-namespace-sync-system` |
| `nameOverride` | Override the name of the chart | `""` |
| `fullnameOverride` | Override the full name of the chart | `""` |
| `image.repository` | Controller image repository | `somaz940/k8s-namespace-sync` |
| `image.tag` | Controller image tag | `v0.1.6` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `serviceAccount.create` | Create ServiceAccount | `true` |
| `serviceAccount.name` | ServiceAccount name | `k8s-namespace-sync-controller-manager` |
| `serviceAccount.annotations` | ServiceAccount annotations | `{}` |
| `podAnnotations` | Pod annotations | `{}` |
| `podLabels` | Pod labels | `{}` |
| `podSecurityContext.runAsNonRoot` | Run container as non-root | `true` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `securityContext.capabilities.drop` | Drop capabilities | `["ALL"]` |
| `resources.limits.cpu` | CPU resource limits | `500m` |
| `resources.limits.memory` | Memory resource limits | `128Mi` |
| `resources.requests.cpu` | CPU resource requests | `10m` |
| `resources.requests.memory` | Memory resource requests | `64Mi` |
| `controller.metrics.bindAddress` | Metrics bind address | `:8443` |
| `controller.health.bindAddress` | Health probe bind address | `:8081` |
| `controller.leaderElection.enabled` | Enable leader election | `true` |
| `controller.logging.development` | Enable development logging | `true` |
| `controller.logging.level` | Log level | `debug` |
| `controller.logging.encoder` | Log encoder | `console` |
| `controller.logging.stacktraceLevel` | Stack trace log level | `error` |
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `8443` |
| `probes.liveness.initialDelaySeconds` | Liveness probe initial delay | `15` |
| `probes.liveness.periodSeconds` | Liveness probe period | `20` |
| `probes.liveness.port` | Liveness probe port | `8081` |
| `probes.liveness.path` | Liveness probe path | `/healthz` |
| `probes.readiness.initialDelaySeconds` | Readiness probe initial delay | `5` |
| `probes.readiness.periodSeconds` | Readiness probe period | `10` |
| `probes.readiness.port` | Readiness probe port | `8081` |
| `probes.readiness.path` | Readiness probe path | `/readyz` |
| `rbac.create` | Create RBAC resources | `true` |
| `crds.create` | Create CRDs | `true` |
| `crds.remove` | Remove CRDs on uninstall | `true` |
| `metrics.enabled` | Enable metrics | `true` |
| `metrics.service.port` | Metrics service port | `8443` |
| `metrics.service.annotations` | Metrics service annotations | `{}` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity rules | `{}` |
| `customresource.basic.enabled` | Enable basic sync configuration | `false` |
| `customresource.basic.name` | Basic sync configuration name | `namespacesync-basic` |
| `customresource.basic.sourceNamespace` | Source namespace for basic sync | `default` |
| `customresource.basic.configMapName` | ConfigMaps to sync for basic config | `[]` |
| `customresource.basic.secretName` | Secrets to sync for basic config | `[]` |
| `customresource.exclude.enabled` | Enable exclude sync configuration | `false` |
| `customresource.exclude.name` | Exclude sync configuration name | `namespacesync-exclude` |
| `customresource.exclude.sourceNamespace` | Source namespace for exclude sync | `default` |
| `customresource.exclude.configMapName` | ConfigMaps to sync for exclude config | `[]` |
| `customresource.exclude.secretName` | Secrets to sync for exclude config | `[]` |
| `customresource.exclude.namespaces` | Namespaces to exclude | `[]` |
| `customresource.filter.enabled` | Enable filter sync configuration | `false` |
| `customresource.filter.name` | Filter sync configuration name | `namespacesync-filter` |
| `customresource.filter.sourceNamespace` | Source namespace for filter sync | `default` |
| `customresource.filter.configMapName` | ConfigMaps to sync for filter config | `[]` |
| `customresource.filter.secretName` | Secrets to sync for filter config | `[]` |
| `customresource.filter.configMaps.include` | ConfigMap include patterns | `[]` |
| `customresource.filter.configMaps.exclude` | ConfigMap exclude patterns | `[]` |
| `customresource.filter.secrets.include` | Secret include patterns | `[]` |
| `customresource.filter.secrets.exclude` | Secret exclude patterns | `[]` |
| `customresource.filter.exclude` | Namespaces to exclude for filter config | `[]` |
| `customresource.target.enabled` | Enable target sync configuration | `false` |
| `customresource.target.name` | Target sync configuration name | `namespacesync-target` |
| `customresource.target.sourceNamespace` | Source namespace for target sync | `default` |
| `customresource.target.namespaces` | Target namespaces | `[]` |
| `customresource.target.configMapName` | ConfigMaps to sync for target config | `[]` |
| `customresource.target.secretName` | Secrets to sync for target config | `[]` |

## Namespace Configuration

### Changing Installation Namespace

By default, the controller is installed in the `k8s-namespace-sync-system` namespace. If you need to install it in a different namespace, you can use the `namespace` parameter:

```bash
# Using default
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync

# Using --set
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync --set namespace=custom-namespace

# Or using values file
# In your values.yaml:
# namespace: custom-namespace
helm install k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync -f values.yaml
```

**Important Note**: If you change the installation namespace, you must exclude this namespace in your NamespaceSync CR to prevent recursive synchronization. The controller already excludes the following system namespaces by default:
- kube-system
- kube-public
- kube-node-lease
- default

When using a custom installation namespace, add it to the exclude list:

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
  exclude:
    - custom-namespace  # Add your custom installation namespace here
```

This prevents the controller from attempting to synchronize resources in its own namespace, which could cause unexpected behavior. The system namespaces are automatically excluded, so you only need to add your custom installation namespace to the exclude list.

## Custom Resource Configuration

The chart supports creating different types of NamespaceSync resources during installation. You can enable and configure them in your values file:

### Basic Sync
```yaml
customresource:
  basic:
    enabled: true
    sourceNamespace: "default"
    configMapName:
      - test-configmap
    secretName:
      - test-secret
```

Local install Method 
```bash
git clone https://github.com/somaz94/k8s-namespace-sync.git
cd k8s-namespace-sync
helm install k8s-namespace-sync ./helm/k8s-namespace-sync -f ./helm/k8s-namespace-sync/values/basic-values.yaml
```

### Exclude Sync
```yaml
customresource:
  exclude:
    enabled: true
    sourceNamespace: "default"
    configMapName:
      - test-configmap
      - test-configmap2
    secretName:
      - test-secret
      - test-secret2
    namespaces:
      - test-ns2
      - test-ns3
```

### Filter Sync
```yaml
customresource:
  filter:
    enabled: true
    sourceNamespace: "default"
    configMapName:
      - test-configmap
      - test-configmap2
    secretName:
      - test-secret
      - test-secret2
    configMaps:
      exclude:
        - "*2"
    secrets:
      exclude:
        - "*2"
    exclude:
      - test-ns2
      - test-ns3
```

### Target Sync
```yaml
customresource:
  target:
    enabled: true
    sourceNamespace: "default"
    namespaces:
      - test-ns1
      - test-ns2
    configMapName:
      - test-configmap
      - test-configmap2
    secretName:
      - test-secret
      - test-secret2
```

You can enable multiple types of sync configurations simultaneously by setting their respective `enabled` flags to `true`.

## Usage

After installing the chart, you can create a NamespaceSync resource to start syncing:

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

## Uninstalling the Chart

To properly uninstall the chart and its resources:

1. First, delete all NamespaceSync resources:
```bash
kubectl delete namespacesync --all
```

2. Then, uninstall the Helm chart:
```bash
helm delete k8s-namespace-sync
```

## Upgrading the Chart

To upgrade the chart:
```bash
helm upgrade k8s-namespace-sync k8s-namespace-sync/k8s-namespace-sync
```

## Troubleshooting

### Verify Installation
```bash
# Check if pods are running
kubectl get pods -n k8s-namespace-sync-system

# Check controller logs
kubectl logs -n k8s-namespace-sync-system -l control-plane=controller-manager -f
```

### Common Issues

1. **CRD not installed**
   - Ensure CRDs are installed:
     ```bash
     kubectl get crd namespacesyncs.sync.nsync.dev
     ```

2. **Permission Issues**
   - Verify RBAC settings:
     ```bash
     kubectl get clusterrole,clusterrolebinding -l app.kubernetes.io/name=k8s-namespace-sync
     ```

3. **Pod not starting**
   - Check pod events:
     ```bash
     kubectl describe pod -n k8s-namespace-sync-system -l control-plane=controller-manager
     ```

## Support

For support, please check:
- [Documentation](https://github.com/somaz94/k8s-namespace-sync)
- [Issues](https://github.com/somaz94/k8s-namespace-sync/issues)
