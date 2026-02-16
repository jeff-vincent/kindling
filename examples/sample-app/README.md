# sample-app

A tiny Go web server that demonstrates the full **kindling** developer
loop in about 100 lines of code. It connects to Postgres and Redis
(auto-provisioned by the operator) and exposes a few HTTP endpoints.

The goal is to show the shortest path from `git push` to a working app
with real dependencies, running on your local Kind cluster.

## Architecture

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

## Files

```
sample-app/
├── .github/workflows/
│   └── dev-deploy.yml       # GitHub Actions workflow (uses kindling actions)
├── main.go                  # The app — ~100 lines of Go
├── Dockerfile               # Two-stage build (golang:1.20 → alpine:3.19)
├── go.mod
├── dev-environment.yaml     # DevStagingEnvironment CR (for manual deploy)
└── README.md                # ← you are here
```

## Quick-start

### Prerequisites

- Local Kind cluster with **kindling** operator deployed ([Getting Started](../../README.md#getting-started))
- `GithubActionRunnerPool` CR applied with your GitHub username

### Option A — Push to GitHub (CI flow)

Copy this example into a repo that your runner pool targets:

```bash
mkdir my-app && cd my-app && git init
cp -r /path/to/kindling/examples/sample-app/* .
cp -r /path/to/kindling/examples/sample-app/.github .

git remote add origin git@github.com:you/my-app.git
git add -A && git commit -m "initial commit" && git push -u origin main
```

The included workflow uses the **reusable kindling actions** — no raw
signal-file scripting:

```yaml
# .github/workflows/dev-deploy.yml (simplified)
steps:
  - uses: actions/checkout@v4

  - name: Clean builds directory
    run: rm -f /builds/*

  - name: Build image
    uses: jeff-vincent/kindling/.github/actions/kindling-build@main
    with:
      name: sample-app
      context: ${{ github.workspace }}
      image: "registry:5000/sample-app:${{ env.TAG }}"

  - name: Deploy
    uses: jeff-vincent/kindling/.github/actions/kindling-deploy@main
    with:
      name: "${{ github.actor }}-sample-app"
      image: "registry:5000/sample-app:${{ env.TAG }}"
      port: "8080"
      ingress-host: "${{ github.actor }}-sample-app.localhost"
      dependencies: |
        - type: postgres
          version: "16"
        - type: redis
```

### Option B — Deploy manually (no GitHub)

```bash
docker build -t sample-app:dev examples/sample-app/
kind load docker-image sample-app:dev --name dev
kubectl apply -f examples/sample-app/dev-environment.yaml
kubectl rollout status deployment/sample-app-dev --timeout=120s
```

### Try it out

```bash
curl http://<username>-sample-app.localhost/
curl http://<username>-sample-app.localhost/healthz
curl http://<username>-sample-app.localhost/status | jq .
```

Expected `/status` output:

```json
{
  "app": "sample-app",
  "time": "2026-02-15T12:00:00Z",
  "postgres": { "status": "connected" },
  "redis": { "status": "connected" }
}
```

## What the operator creates

| Resource | Description |
|---|---|
| **Deployment** | Your app container with health checks |
| **Service** (ClusterIP) | Internal routing |
| **Ingress** | `<user>-sample-app.localhost` → your app |
| **Postgres 16** | Pod + Service, `DATABASE_URL` injected |
| **Redis** | Pod + Service, `REDIS_URL` injected |

You write zero infrastructure YAML for the backing services — just
declare `dependencies: [{type: postgres}, {type: redis}]` and the
operator handles the rest.

## Cleaning up

```bash
kubectl delete devstagingenvironment sample-app-dev
```

The operator garbage-collects all owned resources automatically.
