#!/usr/bin/env bash
# Create a local kind cluster for PrivatePaste. Idempotent.
#
# Uses its own cluster name so it never touches any other kind cluster you
# have running.
set -euo pipefail

CLUSTER="${1:-privatepaste}"
# Same minor version as the EKS default (infra/eks/variables.tf).
NODE_IMAGE="kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed"

if kind get clusters | grep -qx "$CLUSTER"; then
  echo "kind cluster '$CLUSTER' already exists"
else
  kind create cluster --name "$CLUSTER" --image "$NODE_IMAGE" --wait 120s
fi

kubectl --context "kind-$CLUSTER" get nodes
