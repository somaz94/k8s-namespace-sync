#!/usr/bin/env bash
# Runs the README demo for real on a throwaway kind cluster, with
# k8s-namespace-sync installed from helm/k8s-namespace-sync and its published
# image, and saves each command's output under examples/demo/captured/.
# Usage: examples/demo/capture.sh   (needs kind, kubectl and helm; pulls the image)
set -euo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT_DIR="$(cd "${DEMO_DIR}/../.." && pwd)"
OUT="${DEMO_DIR}/captured"
CLUSTER="nsync-demo"

for tool in kind kubectl helm; do
    command -v "${tool}" >/dev/null 2>&1 || { echo "${tool} not found" >&2; exit 1; }
done
if kind get clusters 2>/dev/null | grep -qx "${CLUSTER}"; then
    echo "kind cluster ${CLUSTER} already exists; delete it first" >&2
    exit 1
fi

WORK="$(mktemp -d)"
# kind would otherwise switch the current context of the user's own kubeconfig.
export KUBECONFIG="${WORK}/kubeconfig"
cleanup() {
    kind delete cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG}" >/dev/null 2>&1 || true
    rm -rf "${WORK:?}"
}
trap cleanup EXIT

copies() { kubectl get secret -A -l app=billing --no-headers 2>/dev/null | wc -l | tr -d ' '; }
wait_copies() {
    for _ in $(seq 1 60); do
        [ "$(copies)" = "$1" ] && return 0
        sleep 1
    done
    echo "timed out waiting for $1 copies" >&2
    exit 1
}

kind create cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG}" --wait 60s
helm install k8s-namespace-sync "${ROOT_DIR}/helm/k8s-namespace-sync" --wait --timeout 3m

cd "${DEMO_DIR}"
kubectl apply -f source.yaml

rm -rf "${OUT}"
mkdir -p "${OUT}"
kubectl get secret -A -l app=billing > "${OUT}/01-kubectl" 2>&1
kubectl apply -f namespacesync.yaml > "${OUT}/02-kubectl" 2>&1
wait_copies 3
kubectl get secret -A -l app=billing > "${OUT}/03-kubectl" 2>&1
kubectl create namespace team-c > "${OUT}/04-kubectl" 2>&1
wait_copies 4
kubectl get secret -A -l app=billing > "${OUT}/05-kubectl" 2>&1

echo "captured into ${OUT}"
