#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────
# setup-ingress.sh — Install ingress-nginx and the in-cluster
# image registry on a Kind cluster.
#
# This script:
#   1. Deploys the ingress-nginx controller with Kind-specific
#      patches so it binds to host ports 80/443.
#   2. Deploys a registry:2 pod with hostNetwork so containerd
#      (via the mirror in kind-config.yaml) and Kaniko pods can
#      both reach it.
#
# Usage:
#   ./setup-ingress.sh
#
# Prerequisites:
#   - Kind cluster created with kind-config.yaml
#   - kubectl configured to talk to the Kind cluster
# ─────────────────────────────────────────────────────────────────
set -euo pipefail

# ── In-cluster image registry ──────────────────────────────────────
echo "📦 Deploying in-cluster image registry..."
kubectl apply -f config/registry/registry.yaml

echo "⏳ Waiting for registry to be ready..."
kubectl wait --for=condition=available deployment/registry --timeout=60s
echo "✅ Registry is ready at registry:5000 (in-cluster)"

# ── Ingress controller ────────────────────────────────────────────
echo ""
echo "📦 Installing ingress-nginx for Kind..."

kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

echo "⏳ Waiting for ingress-nginx controller to be ready..."

kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s

echo "✅ ingress-nginx is ready!"
echo ""
echo "Your Kind cluster now routes:"
echo "  http://<host>.localhost  →  Ingress → Service → Pod"
echo ""
echo "Image builds use Kaniko → registry:5000 (no Docker daemon needed)"
