# sample-app

A tiny Go web server that demonstrates the full **kindling** developer
loop in about 100 lines of code. It connects to Postgres and Redis
(auto-provisioned by the operator) and exposes a few HTTP endpoints.

The goal is to show the shortest path from `git push` to a working app
with real dependencies, running on your local Kind cluster.

## What it does

```
 ┌──────────────┐       ┌───────────────┐       ┌──────────┐
 │  sample-app  │──────▶│  PostgreSQL 16 │       │  Redis   │
 │  :8080       │──────▶│  (auto)        │       │  (auto)  │
 └──────┬───────┘       └───────────────┘       └──────────┘
        │
        ▼
  GET /          → Hello message
  GET /healthz   → Liveness probe
  GET /status    → Postgres + Redis connectivity report
```

| Endpoint | Description |
|---|---|
| `GET /` | Friendly hello from the cluster |
| `GET /healthz` | Always returns `{"status":"ok"}` |
| `GET /status` | Pings Postgres and Redis, reports connectivity |

## Files

```
sample-app/
├── main.go                  # The app — ~100 lines of Go
├── Dockerfile               # Two-stage build (golang:1.20 → alpine:3.19)
├── go.mod                   # Go module
├── dev-environment.yaml     # DevStagingEnvironment CR
└── README.md                # ← you are here
```

## Quick-start

### Prerequisites

- Local Kind cluster created with `kind-config.yaml`
- **kindling** operator deployed ([Getting Started](../../README.md#getting-started))
- `setup-ingress.sh` run (deploys registry + ingress-nginx)

### Option A — Push to GitHub (CI flow)

If you have a `GithubActionRunnerPool` running, copy this example into
your target repo and add a workflow that builds via the sidecar:

```yaml
# .github/workflows/dev-deploy.yml (simplified)
name: Deploy sample-app
on: push

jobs:
  deploy:
    runs-on: [self-hosted]
    steps:
      # 1. Clean stale signal files
      - run: rm -f /builds/*

      # 2. Build image via Kaniko sidecar
      - uses: actions/checkout@v4
      - name: Build image
        run: |
          TAG="${{ github.sha }}"
          cd ${{ github.workspace }}
          tar czf /builds/sample-app.tar.gz -C . .
          echo "registry:5000/sample-app:${TAG}" > /builds/sample-app.dest
          touch /builds/sample-app.request
          echo "Waiting for Kaniko build..."
          while [ ! -f /builds/sample-app.done ]; do sleep 2; done
          echo "Build complete (exit code: $(cat /builds/sample-app.done))"

      # 3. Deploy DSE CR via sidecar
      - name: Deploy
        run: |
          ACTOR="${{ github.actor }}"
          TAG="${{ github.sha }}"
          cat > /builds/sample-app-dse.yaml <<EOF
          apiVersion: apps.example.com/v1alpha1
          kind: DevStagingEnvironment
          metadata:
            name: ${ACTOR}-sample-app
          spec:
            deployment:
              image: registry:5000/sample-app:${TAG}
              replicas: 1
              port: 8080
              healthCheck:
                path: /healthz
            service:
              port: 8080
              type: ClusterIP
            ingress:
              enabled: true
              host: ${ACTOR}-sample-app.localhost
              ingressClassName: nginx
            dependencies:
              - type: postgres
                version: "16"
              - type: redis
          EOF
          touch /builds/sample-app-dse.apply
          while [ ! -f /builds/sample-app-dse.apply-done ]; do sleep 2; done
          echo "Deploy complete"
```

Push your code and the runner handles everything — Kaniko builds the
image, pushes to `registry:5000`, and the operator provisions Postgres,
Redis, Deployment, Service, and Ingress.

### Option B — Deploy manually (no GitHub)

```bash
# 1. Build and load image into Kind
docker build -t sample-app:dev examples/sample-app/
kind load docker-image sample-app:dev --name dev

# 2. Apply the DevStagingEnvironment CR
kubectl apply -f examples/sample-app/dev-environment.yaml

# 3. Wait for rollout
kubectl rollout status deployment/sample-app-dev --timeout=120s
```

### Try it out

With ingress-nginx running:

```bash
# Via Ingress (no port-forward needed)
curl http://sample-app.localhost/
curl http://sample-app.localhost/healthz
curl http://sample-app.localhost/status | jq .
```

<details>
<summary><strong>Without Ingress (port-forward fallback)</strong></summary>

```bash
kubectl port-forward svc/sample-app-dev 8080:8080
curl localhost:8080/status | jq .
```

</details>

Expected `/status` output:

```json
{
  "app": "sample-app",
  "time": "2026-02-14T12:00:00Z",
  "postgres": { "status": "connected" },
  "redis": { "status": "connected" }
}
```

## What the operator creates for you

When you apply the `DevStagingEnvironment` CR, the kindling operator
auto-provisions:

| Resource | Description |
|---|---|
| **Deployment** | Your app container, configured with health checks |
| **Service** (ClusterIP) | Internal routing to your app |
| **Ingress** | `sample-app.localhost` → your app (via ingress-nginx) |
| **Postgres 16** | Pod + Service, `DATABASE_URL` injected into your app |
| **Redis** | Pod + Service, `REDIS_URL` injected into your app |

You write zero infrastructure YAML for the backing services — just
declare `dependencies: [{type: postgres}, {type: redis}]` and the
operator handles the rest.

## Cleaning up

```bash
kubectl delete devstagingenvironment sample-app-dev
```

The operator garbage-collects all owned resources (Deployment, Service,
Ingress, Postgres pod, Redis pod) automatically.
