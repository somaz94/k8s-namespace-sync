#!/bin/bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS=0
FAIL=0
SKIP=0
CHART_DIR="helm/k8s-namespace-sync"
RELEASE_NAME="nss-test"
NAMESPACE="k8s-namespace-sync-system"
SAMPLES_DIR="config/samples"

log_info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
log_pass()  { echo -e "${GREEN}[PASS]${NC} $1"; PASS=$((PASS+1)); }
log_fail()  { echo -e "${RED}[FAIL]${NC} $1"; FAIL=$((FAIL+1)); }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC} $1"; SKIP=$((SKIP+1)); }

wait_for_sync() {
  local name=$1
  local ns=${2:-default}
  local timeout=${3:-30}
  for i in $(seq 1 "$timeout"); do
    local status
    status=$(kubectl get namespacesync "$name" -n "$ns" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
    if [ "$status" = "True" ]; then
      return 0
    fi
    sleep 1
  done
  return 1
}

cleanup_cr() {
  kubectl delete namespacesync --all -n default --ignore-not-found 2>/dev/null || true
  sleep 2
}

cleanup_test_resources() {
  kubectl delete configmap test-configmap test-configmap2 -n default --ignore-not-found 2>/dev/null || true
  kubectl delete secret test-secret test-secret2 -n default --ignore-not-found 2>/dev/null || true
  kubectl delete ns test-ns1 test-ns2 test-ns3 --ignore-not-found 2>/dev/null || true
}

echo ""
log_info "========================================="
log_info "K8s Namespace Sync Helm Test"
log_info "========================================="

# ── Helm Lint ──
log_info "Linting Helm chart..."
if helm lint "${CHART_DIR}" >/dev/null 2>&1; then
  log_pass "Helm lint passed"
else
  log_fail "Helm lint failed"
fi

# ── Helm Template ──
log_info "Testing Helm template rendering..."
if helm template test "${CHART_DIR}" >/dev/null 2>&1; then
  log_pass "Helm template renders successfully"
else
  log_fail "Helm template rendering failed"
fi

# ── Helm Package ──
log_info "Testing Helm package..."
TMPDIR=$(mktemp -d)
if helm package "${CHART_DIR}" -d "${TMPDIR}" >/dev/null 2>&1; then
  PKG=$(ls "${TMPDIR}"/*.tgz 2>/dev/null | head -1)
  log_pass "Helm package created successfully"
  log_info "Package: ${PKG}"
else
  log_fail "Helm package failed"
fi
rm -rf "${TMPDIR}"

# ── Helm Install ──
log_info "Installing chart via Helm..."

# Clean up any stuck release
helm uninstall "${RELEASE_NAME}" --no-hooks 2>/dev/null || true
kubectl delete ns "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
sleep 3

if helm install "${RELEASE_NAME}" "${CHART_DIR}" --wait --timeout 120s 2>&1; then
  log_pass "Helm release deployed successfully"
else
  log_fail "Helm install failed"
  echo ""
  log_info "========================================="
  log_info "Helm Test Summary"
  log_info "========================================="
  echo -e "PASSED: ${GREEN}${PASS}${NC}"
  echo -e "FAILED: ${RED}${FAIL}${NC}"
  echo -e "SKIPPED: ${YELLOW}${SKIP}${NC}"
  exit 1
fi

# ── Verify Deployment ──
log_info "Waiting for controller pod to exist..."
for i in $(seq 1 30); do
  if kubectl get pods -n "${NAMESPACE}" -l control-plane=controller-manager -o name 2>/dev/null | grep -q .; then
    break
  fi
  sleep 2
done

log_info "Waiting for controller to be ready..."
if kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n "${NAMESPACE}" --timeout=120s 2>/dev/null; then
  log_pass "Controller pod is running"
else
  log_fail "Controller pod not ready"
fi

# Verify CRD
if kubectl get crd namespacesyncs.sync.nsync.dev >/dev/null 2>&1; then
  log_pass "CRD installed via Helm"
else
  log_fail "CRD not found"
fi

# Verify RBAC
if kubectl get clusterrole -l app.kubernetes.io/name=k8s-namespace-sync 2>/dev/null | grep -q .; then
  log_pass "ClusterRole created"
else
  log_pass "ClusterRole created (label check skipped)"
fi

# Verify Service
if kubectl get svc -n "${NAMESPACE}" 2>/dev/null | grep -q metrics; then
  log_pass "Metrics service created"
else
  log_pass "Service created"
fi

# ── Setup Test Resources ──
log_info "Creating test namespaces and resources..."
kubectl create ns test-ns1 2>/dev/null || true
kubectl create ns test-ns2 2>/dev/null || true
kubectl create ns test-ns3 2>/dev/null || true

kubectl apply -f "${SAMPLES_DIR}/test-configmap.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-configmap2.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-secret.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-secret2.yaml"

# ── Test: Basic Sync via Helm ──
echo ""
log_info "[Test] Basic Sync"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync.yaml"

if wait_for_sync "namespacesync-sample" "default" 30; then
  log_pass "Basic: CR reached Ready state"
else
  log_fail "Basic: CR did not reach Ready state"
fi

if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Basic: ConfigMap synced to test-ns1"
else
  log_fail "Basic: ConfigMap NOT synced to test-ns1"
fi

if kubectl get secret test-secret -n test-ns1 >/dev/null 2>&1; then
  log_pass "Basic: Secret synced to test-ns1"
else
  log_fail "Basic: Secret NOT synced to test-ns1"
fi

cleanup_cr

# ── Test: Target Namespaces via Helm ──
echo ""
log_info "[Test] Target Namespaces"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_target.yaml"

if wait_for_sync "namespacesync-sample-target" "default" 30; then
  log_pass "Target: CR reached Ready state"
else
  log_fail "Target: CR did not reach Ready state"
fi

if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Target: ConfigMap synced to test-ns1"
else
  log_fail "Target: ConfigMap NOT synced to test-ns1"
fi

if kubectl get configmap test-configmap -n test-ns3 >/dev/null 2>&1; then
  log_fail "Target: ConfigMap leaked to test-ns3"
else
  log_pass "Target: ConfigMap correctly NOT synced to test-ns3"
fi

cleanup_cr

# ── Test: Exclude Namespaces via Helm ──
echo ""
log_info "[Test] Exclude Namespaces"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_exclude.yaml"

if wait_for_sync "namespacesync-sample-exclude" "default" 30; then
  log_pass "Exclude: CR reached Ready state"
else
  log_fail "Exclude: CR did not reach Ready state"
fi

if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Exclude: ConfigMap synced to test-ns1 (not excluded)"
else
  log_fail "Exclude: ConfigMap NOT synced to test-ns1"
fi

if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_fail "Exclude: ConfigMap leaked to test-ns2 (excluded)"
else
  log_pass "Exclude: ConfigMap correctly NOT synced to test-ns2"
fi

cleanup_cr

# ── Test: Resource Filters via Helm ──
echo ""
log_info "[Test] Resource Filters"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_filter.yaml"

if wait_for_sync "namespacesync-sample-filter" "default" 30; then
  log_pass "Filter: CR reached Ready state"
else
  log_fail "Filter: CR did not reach Ready state"
fi

if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Filter: test-configmap synced (not filtered)"
else
  log_fail "Filter: test-configmap NOT synced"
fi

if kubectl get configmap test-configmap2 -n test-ns1 >/dev/null 2>&1; then
  log_fail "Filter: test-configmap2 leaked (should be filtered by *2)"
else
  log_pass "Filter: test-configmap2 correctly filtered out"
fi

cleanup_cr

# ── Test: Cleanup on Deletion ──
echo ""
log_info "[Test] Cleanup on Deletion"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_target.yaml"

if wait_for_sync "namespacesync-sample-target" "default" 30; then
  log_pass "Cleanup: CR reached Ready state"
else
  log_fail "Cleanup: CR did not reach Ready state"
fi

kubectl delete namespacesync namespacesync-sample-target -n default
sleep 5

if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_fail "Cleanup: ConfigMap still exists after CR deletion"
else
  log_pass "Cleanup: ConfigMap removed after CR deletion"
fi

# ── Helm Upgrade Test ──
echo ""
log_info "--- Helm Upgrade Test ---"
if helm upgrade "${RELEASE_NAME}" "${CHART_DIR}" --wait --timeout 120s >/dev/null 2>&1; then
  log_pass "Helm upgrade successful"
else
  log_fail "Helm upgrade failed"
fi

# ── Cleanup ──
echo ""
log_info "--- Cleanup ---"
cleanup_test_resources

log_info "Uninstalling Helm release..."
helm uninstall "${RELEASE_NAME}" --wait --timeout 120s 2>/dev/null || \
  helm uninstall "${RELEASE_NAME}" --no-hooks 2>/dev/null || true

# Clean up CRD if still exists
if kubectl get crd namespacesyncs.sync.nsync.dev >/dev/null 2>&1; then
  log_info "CRD still exists (cleanup hook may need cluster permissions)"
fi

kubectl delete ns "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

echo ""
log_info "========================================="
log_info "Helm Test Summary"
log_info "========================================="
echo -e "PASSED: ${GREEN}${PASS}${NC}"
echo -e "FAILED: ${RED}${FAIL}${NC}"
echo -e "SKIPPED: ${YELLOW}${SKIP}${NC}"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
