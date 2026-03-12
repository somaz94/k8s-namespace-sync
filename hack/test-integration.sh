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
NAMESPACE="k8s-namespace-sync-system"
SAMPLES_DIR="config/samples"

log_info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
log_pass()  { echo -e "${GREEN}[PASS]${NC} $1"; PASS=$((PASS+1)); }
log_fail()  { echo -e "${RED}[FAIL]${NC} $1"; FAIL=$((FAIL+1)); }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC} $1"; SKIP=$((SKIP+1)); }

wait_for_pods() {
  local ns=$1
  local timeout=${2:-60}
  log_info "Waiting for pods in ${ns} to be ready (timeout: ${timeout}s)..."
  kubectl wait --for=condition=ready pod --all -n "$ns" --timeout="${timeout}s" 2>/dev/null || true
}

wait_for_resource() {
  local resource=$1
  local name=$2
  local ns=$3
  local timeout=${4:-30}
  for i in $(seq 1 "$timeout"); do
    if kubectl get "$resource" "$name" -n "$ns" 2>/dev/null | grep -q .; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_sync() {
  local name=$1
  local ns=${2:-default}
  local timeout=${3:-30}
  log_info "Waiting for NamespaceSync '${name}' to sync (timeout: ${timeout}s)..."
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
  log_info "Cleaning up test resources..."
  kubectl delete configmap test-configmap test-configmap2 -n default --ignore-not-found 2>/dev/null || true
  kubectl delete secret test-secret test-secret2 -n default --ignore-not-found 2>/dev/null || true
  kubectl delete ns test-ns1 test-ns2 test-ns3 --ignore-not-found 2>/dev/null || true
}

final_cleanup() {
  echo ""
  log_info "--- Final Cleanup (trap) ---"
  cleanup_test_resources
  cleanup_cr
  make undeploy || true
}
trap final_cleanup EXIT

echo ""
log_info "========================================="
log_info "K8s Namespace Sync Integration Test"
log_info "========================================="

# ── Check Prerequisites ──
log_info "Checking prerequisites..."

if ! kubectl cluster-info >/dev/null 2>&1; then
  log_fail "Cannot connect to Kubernetes cluster"
  exit 1
fi
log_pass "Kubernetes cluster is accessible"

# Auto-install CRD if not found
if ! kubectl get crd namespacesyncs.sync.nsync.dev >/dev/null 2>&1; then
  log_info "NamespaceSync CRD not found. Installing with 'make install'..."
  make install
  if ! kubectl get crd namespacesyncs.sync.nsync.dev >/dev/null 2>&1; then
    log_fail "Failed to install NamespaceSync CRD"
    exit 1
  fi
fi
log_pass "NamespaceSync CRD is installed"

# Auto-deploy controller if not running
CONTROLLER_POD=$(kubectl get pods -n "$NAMESPACE" -l control-plane=controller-manager -o name 2>/dev/null | head -1)
if [ -n "$CONTROLLER_POD" ]; then
  # Ensure imagePullPolicy is Always for testing
  kubectl patch deployment k8s-namespace-sync-controller-manager -n "$NAMESPACE" \
    -p '{"spec":{"template":{"spec":{"containers":[{"name":"manager","imagePullPolicy":"Always"}]}}}}' 2>/dev/null || true
  kubectl rollout status deployment/k8s-namespace-sync-controller-manager -n "$NAMESPACE" --timeout=120s 2>/dev/null || true
elif [ -z "$CONTROLLER_POD" ]; then
  log_info "Controller not found. Deploying with 'make deploy'..."
  make deploy IMG="$(grep '^IMG ?=' Makefile | awk -F'= ' '{print $2}')"
  # Force image pull to ensure latest image is used during testing
  kubectl patch deployment k8s-namespace-sync-controller-manager -n "$NAMESPACE" \
    -p '{"spec":{"template":{"spec":{"containers":[{"name":"manager","imagePullPolicy":"Always"}]}}}}' 2>/dev/null || true
  log_info "Waiting for controller pod to be created..."
  for i in $(seq 1 60); do
    if kubectl get pods -n "$NAMESPACE" -l control-plane=controller-manager -o name 2>/dev/null | grep -q .; then
      break
    fi
    sleep 2
  done
  log_info "Waiting for controller to be ready..."
  if ! kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n "$NAMESPACE" --timeout=180s; then
    log_fail "Failed to deploy controller in ${NAMESPACE}"
    exit 1
  fi
fi
log_pass "Controller pod is running"

# ── Setup Test Environment ──
log_info "Setting up test environment..."

kubectl create ns test-ns1 2>/dev/null || true
kubectl create ns test-ns2 2>/dev/null || true
kubectl create ns test-ns3 2>/dev/null || true
log_pass "Test namespaces created"

kubectl apply -f "${SAMPLES_DIR}/test-configmap.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-configmap2.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-secret.yaml"
kubectl apply -f "${SAMPLES_DIR}/test-secret2.yaml"
log_pass "Test resources created in default namespace"

# ── Test A: Basic Sync (all namespaces) ──
echo ""
log_info "--- Test A: Basic Sync (all namespaces) ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync.yaml"

if wait_for_sync "namespacesync-sample" "default" 30; then
  log_pass "Basic Sync: CR reached Ready state"
else
  log_fail "Basic Sync: CR did not reach Ready state"
fi

# Verify configmap synced to test-ns1
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Basic Sync: ConfigMap synced to test-ns1"
else
  log_fail "Basic Sync: ConfigMap NOT synced to test-ns1"
fi

# Verify secret synced to test-ns1
if kubectl get secret test-secret -n test-ns1 >/dev/null 2>&1; then
  log_pass "Basic Sync: Secret synced to test-ns1"
else
  log_fail "Basic Sync: Secret NOT synced to test-ns1"
fi

# Verify synced to test-ns2
if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_pass "Basic Sync: ConfigMap synced to test-ns2"
else
  log_fail "Basic Sync: ConfigMap NOT synced to test-ns2"
fi

# Verify synced to test-ns3
if kubectl get configmap test-configmap -n test-ns3 >/dev/null 2>&1; then
  log_pass "Basic Sync: ConfigMap synced to test-ns3"
else
  log_fail "Basic Sync: ConfigMap NOT synced to test-ns3"
fi

# Verify data integrity
SYNCED_DATA=$(kubectl get configmap test-configmap -n test-ns1 -o jsonpath='{.data.app\.properties}' 2>/dev/null || echo "")
if echo "$SYNCED_DATA" | grep -q "environment=development"; then
  log_pass "Basic Sync: Data integrity verified"
else
  log_fail "Basic Sync: Data integrity check failed"
fi

cleanup_cr

# ── Test B: Target Namespaces ──
echo ""
log_info "--- Test B: Target Namespaces ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_target.yaml"

if wait_for_sync "namespacesync-sample-target" "default" 30; then
  log_pass "Target: CR reached Ready state"
else
  log_fail "Target: CR did not reach Ready state"
fi

# Should sync to test-ns1 (target)
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Target: ConfigMap synced to test-ns1 (target)"
else
  log_fail "Target: ConfigMap NOT synced to test-ns1 (target)"
fi

# Should sync to test-ns2 (target)
if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_pass "Target: ConfigMap synced to test-ns2 (target)"
else
  log_fail "Target: ConfigMap NOT synced to test-ns2 (target)"
fi

# Should NOT sync to test-ns3 (not in target list)
if kubectl get configmap test-configmap -n test-ns3 >/dev/null 2>&1; then
  log_fail "Target: ConfigMap leaked to test-ns3 (not target)"
else
  log_pass "Target: ConfigMap correctly NOT synced to test-ns3"
fi

# Verify multiple resources: configmap2 and secret2
if kubectl get configmap test-configmap2 -n test-ns1 >/dev/null 2>&1; then
  log_pass "Target: ConfigMap2 synced to test-ns1"
else
  log_fail "Target: ConfigMap2 NOT synced to test-ns1"
fi

if kubectl get secret test-secret2 -n test-ns2 >/dev/null 2>&1; then
  log_pass "Target: Secret2 synced to test-ns2"
else
  log_fail "Target: Secret2 NOT synced to test-ns2"
fi

cleanup_cr

# ── Test C: Exclude Namespaces ──
echo ""
log_info "--- Test C: Exclude Namespaces ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_exclude.yaml"

if wait_for_sync "namespacesync-sample-exclude" "default" 30; then
  log_pass "Exclude: CR reached Ready state"
else
  log_fail "Exclude: CR did not reach Ready state"
fi

# Should sync to test-ns1 (not excluded)
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Exclude: ConfigMap synced to test-ns1 (not excluded)"
else
  log_fail "Exclude: ConfigMap NOT synced to test-ns1"
fi

# Should NOT sync to test-ns2 (excluded)
if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_fail "Exclude: ConfigMap leaked to test-ns2 (excluded)"
else
  log_pass "Exclude: ConfigMap correctly NOT synced to test-ns2"
fi

# Should NOT sync to test-ns3 (excluded)
if kubectl get configmap test-configmap -n test-ns3 >/dev/null 2>&1; then
  log_fail "Exclude: ConfigMap leaked to test-ns3 (excluded)"
else
  log_pass "Exclude: ConfigMap correctly NOT synced to test-ns3"
fi

cleanup_cr

# ── Test D: Resource Filters ──
echo ""
log_info "--- Test D: Resource Filters ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_filter.yaml"

if wait_for_sync "namespacesync-sample-filter" "default" 30; then
  log_pass "Filter: CR reached Ready state"
else
  log_fail "Filter: CR did not reach Ready state"
fi

# test-configmap should sync (not excluded by *2 pattern)
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Filter: test-configmap synced (not filtered)"
else
  log_fail "Filter: test-configmap NOT synced"
fi

# test-configmap2 should NOT sync (excluded by *2 pattern)
if kubectl get configmap test-configmap2 -n test-ns1 >/dev/null 2>&1; then
  log_fail "Filter: test-configmap2 leaked (should be filtered by *2)"
else
  log_pass "Filter: test-configmap2 correctly filtered out"
fi

# test-secret should sync (not excluded by *2 pattern)
if kubectl get secret test-secret -n test-ns1 >/dev/null 2>&1; then
  log_pass "Filter: test-secret synced (not filtered)"
else
  log_fail "Filter: test-secret NOT synced"
fi

# test-secret2 should NOT sync (excluded by *2 pattern)
if kubectl get secret test-secret2 -n test-ns1 >/dev/null 2>&1; then
  log_fail "Filter: test-secret2 leaked (should be filtered by *2)"
else
  log_pass "Filter: test-secret2 correctly filtered out"
fi

# test-ns2, test-ns3 excluded by namespace exclude
if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_fail "Filter: ConfigMap leaked to test-ns2 (ns excluded)"
else
  log_pass "Filter: ConfigMap correctly NOT synced to test-ns2 (ns excluded)"
fi

cleanup_cr

# ── Test E: Cleanup on Deletion ──
echo ""
log_info "--- Test E: Cleanup on Deletion ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync_target.yaml"

if wait_for_sync "namespacesync-sample-target" "default" 30; then
  log_pass "Cleanup: CR reached Ready state"
else
  log_fail "Cleanup: CR did not reach Ready state"
fi

# Verify resources exist before deletion
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_pass "Cleanup: Resources exist before CR deletion"
else
  log_fail "Cleanup: Resources not found before CR deletion"
fi

# Delete the CR
kubectl delete namespacesync namespacesync-sample-target -n default
sleep 5

# Verify resources are cleaned up
if kubectl get configmap test-configmap -n test-ns1 >/dev/null 2>&1; then
  log_fail "Cleanup: ConfigMap still exists after CR deletion"
else
  log_pass "Cleanup: ConfigMap removed from test-ns1"
fi

if kubectl get secret test-secret -n test-ns1 >/dev/null 2>&1; then
  log_fail "Cleanup: Secret still exists after CR deletion"
else
  log_pass "Cleanup: Secret removed from test-ns1"
fi

if kubectl get configmap test-configmap -n test-ns2 >/dev/null 2>&1; then
  log_fail "Cleanup: ConfigMap still exists in test-ns2"
else
  log_pass "Cleanup: ConfigMap removed from test-ns2"
fi

# ── Test F: Dynamic Namespace Detection ──
echo ""
log_info "--- Test F: Dynamic Namespace Detection ---"
kubectl apply -f "${SAMPLES_DIR}/sync_v1_namespacesync.yaml"

if wait_for_sync "namespacesync-sample" "default" 30; then
  log_pass "Dynamic NS: CR reached Ready state"
else
  log_fail "Dynamic NS: CR did not reach Ready state"
fi

# Create a new namespace after CR is active
kubectl create ns test-ns-dynamic 2>/dev/null || true
sleep 5

# Verify resources synced to the new namespace
if kubectl get configmap test-configmap -n test-ns-dynamic >/dev/null 2>&1; then
  log_pass "Dynamic NS: ConfigMap synced to newly created namespace"
else
  log_fail "Dynamic NS: ConfigMap NOT synced to newly created namespace"
fi

if kubectl get secret test-secret -n test-ns-dynamic >/dev/null 2>&1; then
  log_pass "Dynamic NS: Secret synced to newly created namespace"
else
  log_fail "Dynamic NS: Secret NOT synced to newly created namespace"
fi

cleanup_cr
kubectl delete ns test-ns-dynamic --ignore-not-found 2>/dev/null || true

# ── Summary ──
echo ""
log_info "========================================="
log_info "Integration Test Summary"
log_info "========================================="
echo -e "PASSED: ${GREEN}${PASS}${NC}"
echo -e "FAILED: ${RED}${FAIL}${NC}"
echo -e "SKIPPED: ${YELLOW}${SKIP}${NC}"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
