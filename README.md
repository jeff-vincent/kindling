<p align="center">
  <img src="https://img.shields.io/badge/Kubernetes-Operator-326CE5?logo=kubernetes&logoColor=white" alt="Kubernetes Operator" />
  <img src="https://img.shields.io/badge/Go-1.20-00ADD8?logo=go&logoColor=white" alt="Go 1.20" />
  <img src="https://img.shields.io/badge/kubebuilder-v4-blueviolet" alt="Kubebuilder v4" />
  <img src="https://img.shields.io/badge/License-MIT-green" alt="MIT" />
</p>

# <img src="assets/logo.svg" width="36" height="36" alt="kindling" style="vertical-align: middle;" /> kindling

**Push code. Your laptop builds it. Your laptop runs it. Zero cloud CI minutes.**

`kindling` is a Kubernetes operator that turns every developer's local [Kind](https://kind.sigs.k8s.io/) cluster into a personal, self-hosted GitHub Actions CI/CD environment. When you push to GitHub, the CI job is routed *back to your own machine*, where it builds the app container and deploys an ephemeral staging environment — all in seconds, all on localhost.

---

## Why?

Most teams pay for cloud CI runners that:

- 🐢 Queue behind other jobs
- 💸 Burn money on compute you already have under your desk
- 🔄 Build artifacts you then have to pull back down to test locally

**What if the runner was already on your laptop?**

`kindling` flips the model. Each developer runs a lightweight Kind cluster. Inside it, a self-hosted GitHub Actions runner polls for CI jobs triggered by *their* pushes. When a job arrives, the runner builds containers using **Kaniko** (no Docker daemon required) and pushes them to an **in-cluster registry**. The operator then deploys the result as a full staging environment — Deployment, Service, and optional Ingress — right on localhost.

```mermaid
flowchart TB
    dev("👩‍💻 Developer\n<i>git push</i>")
    gh("🐙 GitHub.com\nActions")

    dev -- "git push" --> gh
    gh -- "runs-on: [self-hosted, ‹username›]" --> runner

    subgraph machine["🖥️  Developer's Machine"]
        subgraph kind["☸  Kind Cluster"]
            controller("🔥 kindling\ncontroller")
            runner("🏃 Runner Pod\n<i>runner + build-agent sidecar</i>")
            registry("📦 registry:5000\n<i>in-cluster</i>")
            staging("📦 DevStagingEnv\n<i>Deployment + Service + Ingress</i>")

            runner -- "Kaniko builds\n→ registry:5000" --> registry
            registry -- "image pull" --> staging
            controller -- "creates &\nreconciles" --> staging
            runner -- "applies CR" --> controller
        end
    end

    staging -. "localhost:80\n(via Ingress)" .-> dev

    style machine fill:#1a1a2e,stroke:#16213e,color:#e0e0e0
    style kind fill:#0f3460,stroke:#326CE5,color:#e0e0e0,stroke-width:2px
    style controller fill:#FF6B35,stroke:#FF6B35,color:#fff
    style runner fill:#2ea043,stroke:#2ea043,color:#fff
    style registry fill:#0db7ed,stroke:#0db7ed,color:#fff
    style staging fill:#326CE5,stroke:#326CE5,color:#fff
    style dev fill:#6e40c9,stroke:#6e40c9,color:#fff
    style gh fill:#24292f,stroke:#24292f,color:#fff
```

---

## Custom Resources

The operator manages two CRDs in the `apps.example.com/v1alpha1` group:

### `GithubActionRunnerPool`

Declares a self-hosted runner pool bound to a **single developer** and a **single GitHub repository**. The operator creates a Deployment with two containers:

1. **Runner** — the [official GitHub Actions runner image](https://github.com/actions/runner). Exchanges the stored PAT for a short-lived registration token at startup, registers with GitHub using the developer's username as a label, and cleanly de-registers on shutdown.
2. **Build-agent sidecar** — a `bitnami/kubectl` container that watches a shared `/builds` volume for build and deploy requests. It runs Kaniko pods for image builds and `kubectl apply` for deployments, keeping the runner container stock and unprivileged.

The operator also **auto-provisions RBAC** (ServiceAccount, ClusterRole, ClusterRoleBinding) for each runner pool — no manual SA setup required.

```yaml
apiVersion: apps.example.com/v1alpha1
kind: GithubActionRunnerPool
metadata:
  name: jeff-runner-pool
spec:
  githubUsername: "jeff-vincent"              # routes jobs to this dev's cluster
  repository: "jeff-vincent/demo-kindling"   # repo to poll for workflow runs
  tokenSecretRef:
    name: github-runner-token                # Secret holding a GitHub PAT (repo scope)
  replicas: 1                                # one runner per developer
  labels: [linux]                            # extra runner labels
  resources:
    cpuRequest: "500m"
    memoryLimit: "4Gi"
  # githubURL: "https://github.example.com"  # uncomment for GitHub Enterprise Server
```

Your GitHub Actions workflow routes jobs to a specific developer's machine:

```yaml
# In your app repo: .github/workflows/dev-deploy.yml
on: push

jobs:
  build-and-deploy:
    runs-on: [self-hosted, "${{ github.actor }}"]

    env:
      TAG: "${{ github.actor }}-${{ github.sha }}"
      REGISTRY: "registry:5000"

    steps:
      - uses: actions/checkout@v4

      # Clean stale signal files from previous runs
      - name: Clean builds directory
        run: rm -f /builds/*

      # Build via the sidecar — write tarball + trigger, wait for done
      - name: Build container image
        run: |
          tar -czf /builds/myapp.tar.gz -C . .
          echo "$REGISTRY/myapp:$TAG" > /builds/myapp.dest
          touch /builds/myapp.request
          while [ ! -f /builds/myapp.done ]; do sleep 2; done

      # Deploy via the sidecar — write YAML + trigger, wait for done
      - name: Deploy to local Kind cluster
        run: |
          cat > /builds/myapp.yaml <<EOF
          apiVersion: apps.example.com/v1alpha1
          kind: DevStagingEnvironment
          metadata:
            name: ${{ github.actor }}-dev
          spec:
            deployment:
              image: $REGISTRY/myapp:$TAG
              port: 8080
            service:
              port: 8080
          EOF
          touch /builds/myapp.apply
          while [ ! -f /builds/myapp.apply-done ]; do sleep 1; done
```

<details>
<summary><strong>Full GithubActionRunnerPool spec reference</strong></summary>

| Field | Default | Description |
|---|---|---|
| `githubUsername` | *(required)* | Developer's GitHub handle — auto-added as a runner label |
| `repository` | *(required)* | GitHub repo slug (`org/repo`) |
| `tokenSecretRef` | *(required)* | Reference to a Secret holding a GitHub PAT (`repo` scope). The runner auto-exchanges it for a short-lived registration token at startup. |
| `replicas` | `1` | Number of runner pods |
| `runnerImage` | `ghcr.io/actions/actions-runner:latest` | Runner container image |
| `labels` | `[]` | Extra runner labels (`self-hosted` + username always added) |
| `runnerGroup` | `"Default"` | GitHub runner group |
| `resources` | `nil` | CPU/memory requests and limits |
| `serviceAccountName` | `""` | SA for the runner pod (auto-created if empty) |
| `workDir` | `/home/runner/_work` | Runner working directory |
| `githubURL` | `https://github.com` | Override for GitHub Enterprise Server |
| `env` | `[]` | Extra environment variables |
| `volumeMounts` | `[]` | Additional volume mounts |
| `volumes` | `[]` | Additional volumes |

</details>

### How the build-agent sidecar works

The runner pod contains two containers sharing a `/builds` emptyDir volume:

```
┌─────────────────────────────────────────────────────────┐
│ Runner Pod                                              │
│                                                         │
│  ┌─────────────────┐   /builds   ┌───────────────────┐  │
│  │ runner           │◄──────────►│ build-agent        │  │
│  │ (GH Actions)     │  emptyDir  │ (bitnami/kubectl)  │  │
│  │                  │            │                    │  │
│  │ writes:          │            │ watches for:       │  │
│  │  *.tar.gz + .dest│            │  .request → Kaniko │  │
│  │  *.yaml          │            │  .apply   → apply  │  │
│  │  *.sh            │            │  .kubectl → exec   │  │
│  └─────────────────┘            └───────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

| Trigger file | What the sidecar does | Signal when done |
|---|---|---|
| `<name>.request` | Reads `<name>.tar.gz` + `<name>.dest`, pipes into a one-shot Kaniko pod | `<name>.done` |
| `<name>.apply` | Runs `kubectl apply -f <name>.yaml` | `<name>.apply-done` |
| `<name>.kubectl` | Runs `bash <name>.sh` (arbitrary kubectl scripts) | `<name>.kubectl-done` |

> **Important:** The `/builds` emptyDir persists across workflow runs (same pod). Always clean it at the start of each workflow to avoid stale signal files from a previous run.

### `DevStagingEnvironment`

Declares a complete ephemeral staging environment: a **Deployment**, a **Service**, an optional **Ingress**, and zero or more **Dependencies** (databases, caches, queues). The operator auto-provisions each dependency and injects connection env vars (e.g. `DATABASE_URL`, `REDIS_URL`) into the app container — no manual wiring required.

Typically created by the CI workflow step *inside* the runner pod after building the container image.

```yaml
apiVersion: apps.example.com/v1alpha1
kind: DevStagingEnvironment
metadata:
  name: jeff-dev
spec:
  deployment:
    image: registry:5000/myapp:jeff-abc123
    replicas: 1
    port: 8080
    healthCheck:
      path: /healthz
  service:
    port: 8080
    type: ClusterIP
  ingress:
    enabled: true
    host: jeff-dev.localhost
    ingressClassName: nginx
  dependencies:
    - type: postgres
      version: "16"
    - type: redis
```

With the above, the operator creates:
- A **Deployment + Service** for PostgreSQL 16 — injects `DATABASE_URL=postgres://devuser:devpass@jeff-dev-postgres:5432/devdb?sslmode=disable`
- A **Deployment + Service** for Redis — injects `REDIS_URL=redis://jeff-dev-redis:6379/0`
- A **credentials Secret** per dependency
- All connection env vars merged into the app container automatically

#### Supported dependency types

| Type | Default Image | Default Port | Injected Env Var | Notes |
|---|---|---|---|---|
| `postgres` | `postgres:16` | 5432 | `DATABASE_URL` | Auto-creates `devdb` with user `devuser` |
| `redis` | `redis:latest` | 6379 | `REDIS_URL` | Stateless, no persistence |
| `mysql` | `mysql:latest` | 3306 | `DATABASE_URL` | Auto-creates `devdb` with user `devuser` |
| `mongodb` | `mongo:latest` | 27017 | `MONGO_URL` | Root user `devuser` |
| `rabbitmq` | `rabbitmq:3-management` | 5672 | `AMQP_URL` | Includes management UI |
| `minio` | `minio/minio:latest` | 9000 | `S3_ENDPOINT` | Also injects `S3_ACCESS_KEY` + `S3_SECRET_KEY` |

<details>
<summary><strong>Full DevStagingEnvironment spec reference</strong></summary>

| Field | Default | Description |
|---|---|---|
| `deployment.image` | *(required)* | Container image to run |
| `deployment.port` | *(required)* | Container port |
| `deployment.replicas` | `1` | Pod replica count |
| `deployment.command` | `[]` | Override container entrypoint |
| `deployment.args` | `[]` | Entrypoint arguments |
| `deployment.env` | `[]` | Environment variables |
| `deployment.resources` | `nil` | CPU/memory requests and limits |
| `deployment.healthCheck.path` | `/healthz` | HTTP health check path |
| `deployment.healthCheck.initialDelaySeconds` | `5` | Delay before first probe |
| `deployment.healthCheck.periodSeconds` | `10` | Probe interval |
| `service.port` | *(required)* | Service port |
| `service.targetPort` | container port | Target port on the pod |
| `service.type` | `ClusterIP` | `ClusterIP` / `NodePort` / `LoadBalancer` |
| `ingress.enabled` | `false` | Create an Ingress resource |
| `ingress.host` | `""` | Hostname for the Ingress rule |
| `ingress.path` | `/` | URL path prefix |
| `ingress.pathType` | `Prefix` | `Prefix` / `Exact` / `ImplementationSpecific` |
| `ingress.ingressClassName` | `nil` | Ingress class (e.g. `nginx`) |
| `ingress.tls` | `nil` | TLS termination config |
| `ingress.annotations` | `{}` | Extra Ingress annotations |
| `dependencies[].type` | *(required)* | `postgres`, `redis`, `mysql`, `mongodb`, `rabbitmq`, or `minio` |
| `dependencies[].version` | latest | Image tag (e.g. `"16"`, `"7.2"`) |
| `dependencies[].image` | per-type default | Override the container image entirely |
| `dependencies[].port` | per-type default | Override the service port |
| `dependencies[].env` | `[]` | Extra/override env vars for the dependency container |
| `dependencies[].envVarName` | per-type default | Override the injected env var name (e.g. `MY_DB_URL`) |
| `dependencies[].storageSize` | `1Gi` | PVC size for stateful dependencies |
| `dependencies[].resources` | `nil` | CPU/memory requests and limits |

</details>

---

## Getting Started

### Prerequisites

| Tool | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.20+ |
| [Kind](https://kind.sigs.k8s.io/) | 0.20+ |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | 1.28+ |
| [Docker](https://docs.docker.com/get-docker/) | 24+ (for building the operator image only — app images use Kaniko) |

### Option A: Use the CLI (recommended)

The `kindling` CLI wraps the entire bootstrap flow into simple commands:

```bash
# Build the CLI
make cli

# Bootstrap everything: Kind cluster + ingress + registry + operator
./bin/kindling init

# Register a self-hosted GitHub Actions runner
./bin/kindling quickstart -u <github-user> -r <owner/repo> -t <pat>

# Deploy a dev staging environment
./bin/kindling deploy -f examples/sample-app/dev-environment.yaml

# View cluster status at a glance
./bin/kindling status

# Tail the operator logs
./bin/kindling logs

# Tear everything down
./bin/kindling destroy
```

<details>
<summary><strong>CLI command reference</strong></summary>

| Command | Description |
|---|---|
| `kindling init` | Create Kind cluster, install ingress + registry, build & deploy operator |
| `kindling quickstart` | Create GitHub PAT secret + runner pool CR (interactive prompts for missing values) |
| `kindling deploy -f <file>` | Apply a DevStagingEnvironment from a YAML file |
| `kindling status` | Dashboard view of cluster, operator, runners, environments, and ingress routes |
| `kindling logs` | Tail the kindling controller logs (`-f` for follow, `--all` for all containers) |
| `kindling destroy` | Delete the Kind cluster (with confirmation prompt, or `-y` to skip) |
| `kindling version` | Print CLI version |

Global flags: `-c <name>` (cluster name, default `dev`), `-p <path>` (project directory).

</details>

### Option B: Manual setup

<details>
<summary><strong>Step-by-step without the CLI</strong></summary>

#### 1. Create a local Kind cluster

Use the included config to enable Ingress support (maps ports 80/443 to localhost) and configure the containerd registry mirror for the in-cluster registry:

```bash
kind create cluster --name dev --config kind-config.yaml
```

Then install the ingress-nginx controller and the in-cluster image registry:

```bash
chmod +x setup-ingress.sh && ./setup-ingress.sh
```

This deploys:
- **registry:5000** — an in-cluster container registry (Kaniko pushes images here, containerd pulls from here)
- **ingress-nginx** — routes `*.localhost` hostnames to Services inside the cluster

> **Note:** If you skip this step and use a plain `kind create cluster`, you'll need to use `kubectl port-forward` instead of `.localhost` hostnames, and image builds won't work without a registry.

#### 2. Build and deploy the operator

```bash
# Build the operator image
make docker-build IMG=controller:latest

# Load it into Kind (no remote registry needed)
kind load docker-image controller:latest --name dev

# Install CRDs and deploy the operator
make install
make deploy IMG=controller:latest
```

#### 3. Create the GitHub token Secret and runner pool

Generate a [GitHub Personal Access Token](https://github.com/settings/tokens) with **`repo`** scope, then use the quickstart target:

```bash
make quickstart \
  GITHUB_USERNAME=your-github-username \
  GITHUB_REPO=your-org/your-repo \
  GITHUB_PAT=ghp_YOUR_TOKEN_HERE
```

</details>

This creates the Secret and `GithubActionRunnerPool` CR in one command. The operator will:
1. Auto-provision a **ServiceAccount + ClusterRole + ClusterRoleBinding** for the runner
2. Create a runner **Deployment** with two containers (runner + build-agent sidecar)
3. Register the runner with GitHub using labels `[self-hosted, <username>]`

> **How token exchange works:** You provide a long-lived Personal Access Token (PAT) in the Secret. When the runner pod starts, it **automatically exchanges** the PAT for a short-lived GitHub runner registration token via the GitHub API (`POST /repos/{owner}/{repo}/actions/runners/registration-token`). The registration token is used once to register the runner, and on shutdown a removal token cleanly de-registers it. Your PAT never leaves the cluster.
>
> **GitHub Enterprise Server** is supported: set `spec.githubURL` in your `GithubActionRunnerPool` CR.

### 4. Verify everything is running

```bash
# Operator
kubectl get pods -n kindling-system

# Runner (should show 2/2 — runner + build-agent)
kubectl get pods

# Runner pool status
kubectl get githubactionrunnerpools
```

```
NAME               USER           REPOSITORY                    REPLICAS   READY
jeff-runner-pool   jeff-vincent   jeff-vincent/demo-kindling    1          1
```

The runner will also appear on your repository's **Settings → Actions → Runners** page on GitHub.

### 5. Push code and watch it deploy

Push a commit to your repo. The GitHub Actions workflow routes the job to your local runner, which builds the container via Kaniko, pushes it to `registry:5000`, and applies a `DevStagingEnvironment` CR. The operator takes it from there:

```bash
kubectl get devstagingenvironments
kubectl get pods
```

Access your app via Ingress:

```
http://<username>-<app>.localhost
```

Or fall back to port-forwarding:

```bash
kubectl port-forward svc/<username>-<app> 8080:8080
```

### Try the microservices demo (multi-service + Redis queue)

For a realistic example, the repo includes a [microservices demo](examples/microservices/) with four services:

| Service | Database | Role |
|---|---|---|
| **ui** | — | React + TypeScript dashboard — place orders, view inventory, watch activity |
| **gateway** | — | Public API, reverse-proxies to backend services |
| **orders** | Postgres 16 | Manages orders, publishes `order.created` events to a Redis queue |
| **inventory** | MongoDB | Manages stock levels, consumes events from the Redis queue |

See the full walkthrough in the [microservices README](examples/microservices/README.md).

### Try the sample app (single-service)

A simpler [sample app](examples/sample-app/) demonstrates the full loop with a single Go service, Postgres, and Redis. See the [sample app README](examples/sample-app/README.md).

---

## Project Layout

```
.
├── api/v1alpha1/                        # CRD type definitions
│   ├── devstagingenvironment_types.go   #   DevStagingEnvironment spec & status
│   ├── githubactionrunnerpool_types.go  #   GithubActionRunnerPool spec & status
│   ├── groupversion_info.go             #   apps.example.com/v1alpha1 registration
│   └── zz_generated.deepcopy.go         #   auto-generated DeepCopy methods
├── cli/                                 # kindling CLI tool
│   ├── main.go                          #   CLI entrypoint
│   ├── cmd/                             #   Cobra command implementations
│   │   ├── root.go                      #     Root command + global flags
│   │   ├── init.go                      #     Bootstrap cluster + operator
│   │   ├── quickstart.go                #     Register GitHub runner
│   │   ├── deploy.go                    #     Apply DevStagingEnvironment
│   │   ├── status.go                    #     Cluster dashboard
│   │   ├── logs.go                      #     Tail controller logs
│   │   ├── destroy.go                   #     Tear down cluster
│   │   └── version.go                   #     Print version
│   └── go.mod                           #   Separate Go module (cobra)
├── cmd/main.go                          # Operator entrypoint & controller wiring
├── examples/
│   ├── sample-app/                      # Single-service reference app
│   │   ├── .github/workflows/           #   GH Actions workflow: build + deploy
│   │   ├── main.go                      #   Go web server (Postgres + Redis)
│   │   ├── Dockerfile                   #   Multi-stage build
│   │   ├── dev-environment.yaml         #   DevStagingEnvironment CR
│   │   └── README.md                    #   Full walkthrough guide
│   └── microservices/                   # Multi-service demo (4 services + queue)
│       ├── gateway/                     #   API gateway — reverse proxy
│       ├── orders/                      #   Orders API — Postgres + Redis publisher
│       ├── inventory/                   #   Inventory API — MongoDB + Redis consumer
│       ├── ui/                          #   React + TypeScript dashboard (Vite → nginx)
│       ├── deploy/                      #   DevStagingEnvironment CRs for all services
│       ├── .github/workflows/           #   GH Actions: build all 4 images + deploy
│       └── README.md                    #   Architecture & walkthrough
├── internal/controller/
│   ├── devstagingenvironment_controller.go   # Reconciler → Deployment + Service + Ingress + Deps
│   └── githubactionrunnerpool_controller.go  # Reconciler → Runner Deployment + RBAC + Sidecar
├── config/
│   ├── crd/bases/                       # Generated CRD manifests
│   ├── rbac/                            # ClusterRoles, bindings, service accounts
│   ├── manager/                         # Operator Deployment manifest
│   ├── registry/                        # In-cluster image registry (registry:2)
│   ├── samples/                         # Example CRs to get started
│   └── default/                         # Kustomize overlay for full deployment
├── Dockerfile                           # Multi-stage build for the operator image
├── Makefile                             # Build, test, generate, deploy, quickstart targets
├── kind-config.yaml                     # Kind cluster config (Ingress + registry mirror)
├── setup-ingress.sh                     # Deploys registry + ingress-nginx
└── PROJECT                              # Kubebuilder project metadata
```

---

## How It All Fits Together

```mermaid
flowchart LR
    push("🚀 git push") --> gh("⚡ GitHub\nActions")
    gh -- "runs-on:\n[self-hosted, user]" --> runner

    subgraph cluster["☸  Kind Cluster — Developer's Laptop"]
        runner("🏃 Runner Pod\n<i>runner + build-agent</i>")
        kaniko["🔨 Kaniko Pod\n<i>(one-shot)</i>"]
        registry[("📦 registry:5000")]
        cr("📋 DevStagingEnvironment CR")
        operator("🔥 kindling\noperator")
        resources("📦 Deployment\n📡 Service\n🌐 Ingress")

        runner -- "sidecar pipes\ntarball" --> kaniko
        kaniko -- "pushes image" --> registry
        runner -- "sidecar runs\nkubectl apply" --> cr
        cr -- "watches" --> operator
        operator -- "reconciles" --> resources
        registry -- "image pull" --> resources
    end

    resources -. "http://app.localhost" .-> user("👩‍💻 Developer")

    style cluster fill:#0f3460,stroke:#326CE5,color:#e0e0e0,stroke-width:2px
    style push fill:#6e40c9,stroke:#6e40c9,color:#fff
    style gh fill:#24292f,stroke:#24292f,color:#fff
    style runner fill:#2ea043,stroke:#2ea043,color:#fff
    style kaniko fill:#f0883e,stroke:#f0883e,color:#fff
    style registry fill:#0db7ed,stroke:#0db7ed,color:#fff
    style cr fill:#f0883e,stroke:#f0883e,color:#fff
    style operator fill:#FF6B35,stroke:#FF6B35,color:#fff
    style resources fill:#326CE5,stroke:#326CE5,color:#fff
    style user fill:#6e40c9,stroke:#6e40c9,color:#fff
```

1. **Developer creates a Kind cluster** on their laptop and deploys the operator + a `GithubActionRunnerPool` CR with their GitHub username.

2. **The operator spins up a runner Deployment** with two containers: the GitHub Actions runner and a build-agent sidecar. It also auto-provisions RBAC (ServiceAccount, ClusterRole, ClusterRoleBinding). On startup the runner **exchanges the PAT for a short-lived registration token** and self-registers with GitHub using labels `[self-hosted, <username>]`.

3. **Developer pushes code.** The GitHub Actions workflow fires with `runs-on: [self-hosted, <username>]`, routing the job to *that developer's laptop*.

4. **The runner executes the job**: checks out the code, writes build artifacts to the shared `/builds` volume. The build-agent sidecar detects trigger files and launches **Kaniko pods** to build container images, pushing them to the **in-cluster registry** at `registry:5000`.

5. **The runner applies a `DevStagingEnvironment` CR** (via the sidecar's `kubectl apply`). The operator reconciles it: creates a Deployment pulling the image from the in-cluster registry, a Service, optional Ingress, and auto-provisions any declared dependencies (Postgres, Redis, MongoDB, etc.).

6. **The developer hits localhost** via Ingress (`http://app.localhost`) or `kubectl port-forward`, seeing their changes running in a real Kubernetes environment instantly.

---

## Development

```bash
# Regenerate deepcopy methods and CRD/RBAC manifests after editing types
make generate manifests

# Run the full test suite
make test

# Format and vet
make fmt vet

# Build the operator binary
make build

# Build the Docker image
make docker-build IMG=controller:dev
```

### Modifying the API

If you edit files under `api/v1alpha1/`, always regenerate:

```bash
make generate manifests
```

This updates `zz_generated.deepcopy.go`, the CRD YAML in `config/crd/bases/`, and RBAC roles in `config/rbac/`.

---

## Uninstalling

```bash
# Remove the operator and all managed resources
make undeploy

# Remove only the CRDs (and all CRs)
make uninstall

# Delete the Kind cluster entirely
kind delete cluster --name dev
```

---

## Roadmap

- [ ] Automatic TTL-based cleanup of stale `DevStagingEnvironment` CRs
- [ ] Live status integration — update `GithubActionRunnerPool.status.activeJob` via the GitHub API
- [x] Multi-service environments — deploy databases, caches, and queues alongside the app
- [x] Automatic PAT → registration token exchange — no manual token juggling
- [x] GitHub Enterprise Server support via `spec.githubURL`
- [x] In-cluster image builds via Kaniko (no Docker daemon required)
- [x] Build-agent sidecar architecture — runner image stays stock/unprivileged
- [x] Auto-provisioned RBAC per runner pool
- [x] In-cluster container registry for Kaniko → containerd image flow
- [x] CLI bootstrap tool — `kindling init` + `kindling quickstart` + `kindling deploy` + `kindling status`
- [ ] Webhook receiver for GitHub push events as an alternative to long-polling

---

## Contributing

Contributions are welcome! Please open an issue to discuss your idea before submitting a PR.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and run `make test`
4. Commit (`git commit -m 'Add amazing feature'`)
5. Push (`git push origin feature/amazing-feature`)
6. Open a Pull Request

---

## License

MIT License — Copyright 2024.

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

