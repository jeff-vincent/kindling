# Kindling — Roadmap

## Generate: Helm-native deploy

When scanning a repo, detect existing Helm charts (`Chart.yaml`). Default
behavior: apply the chart directly to the cluster via `helm install/upgrade` in
the deploy step of the generated workflow — this avoids env drift by using the
chart as the source of truth at deploy time.

Run `helm template` during scanning to extract env vars, ports, service names,
and dependencies, then pass that rendered context to the LLM so it can:

1. Populate build steps with template-derived env vars
2. Generate a deploy step that runs `helm upgrade --install` instead of raw
   `kubectl apply`

Add a `--no-helm` override flag to fall back to DSE-based generation even when a
chart is present.

Also detect kustomize configs — same pattern: `kustomize build` to extract
context, deploy via `kubectl apply -k` by default.

---

## Generate: smarter ingress heuristics

If no existing manifests are found, proceed with current AI-based generation but
improve heuristics for identifying which services should get ingress routes:

- **Frontends**: React, Next.js, Vue, Angular, static file servers
- **SSR frameworks**: Rails views, Django templates, PHP
- **API gateways**: services named `gateway`, `api-gateway`, `bff`, etc.

Only services identified as user-facing get ingress entries by default.

---

## Generate: --ingress-all flag

Add a `--ingress-all` flag (or similar) to `kindling generate` that wires up
every service with a default ingress route including health endpoints (e.g.
`/healthz`, `/actuator/health`). Without the flag, only heuristically-identified
user-facing services get routes. This gives users an easy override when the
heuristics miss something.

---

## ~~CLI: kindling secrets subcommand~~ ✅

Implemented `kindling secrets` with four subcommands:

- `kindling secrets set <NAME> <VALUE>` — creates K8s Secret + local backup
- `kindling secrets list` — shows managed secret names and keys
- `kindling secrets delete <NAME>` — removes from cluster + local backup
- `kindling secrets restore` — re-creates K8s Secrets from local backup after
  cluster rebuild

---

## Generate: detect external credentials

During `kindling generate` repo scanning, detect references to external
credentials — env vars matching patterns like `*_API_KEY`, `*_SECRET`,
`*_TOKEN`, `*_DSN`, `*_CONNECTION_STRING` in source code, Dockerfiles,
docker-compose, and `.env` files.

For each detected external secret:

1. Emit a `# TODO: run kindling secrets set <name>` comment in the generated
   workflow
2. In interactive mode, prompt the user to provide the value immediately
3. Wire the secret ref into the generated K8s manifests as a `secretKeyRef`

---

## ~~Config: .kindling/secrets.yaml~~ ✅

Implemented as part of `kindling secrets`. File is stored at
`.kindling/secrets.yaml` (auto-gitignored). Values are base64-encoded.
`kindling secrets restore` reads this file and re-creates K8s Secrets.

---

## TLS + public exposure for OAuth

Support TLS with a publicly accessible IP/hostname for local dev environments so
external identity providers (Auth0, Okta, Firebase Auth, etc.) can call back into
the cluster.

1. `kindling expose` sets up a tunnel (cloudflared, ngrok, or similar) from a
   public HTTPS URL to the Kind cluster's ingress — the tunnel provider handles
   TLS termination
2. For direct IP exposure, deploy cert-manager with Let's Encrypt into the
   cluster
3. The generated workflow and DSE spec accept an optional `publicHost` field so
   ingress rules use the real hostname instead of `*.localhost`
4. `kindling generate` detects OAuth callback URLs, Auth0 config, OIDC discovery
   endpoints in source code and flags that TLS/public exposure is required,
   prompting the user to run `kindling expose`

---

## OSS infrastructure (deprioritized)

Low priority — do when there's actual community interest:

- `CONTRIBUTING.md` with dev setup, test instructions, PR expectations, DCO signoff
- `CODE_OF_CONDUCT.md` (Contributor Covenant v2.1)
- Issue & PR templates (`.github/ISSUE_TEMPLATE/`, PR template)
- Dynamic README badges (CI status, release, Go Report Card, coverage)
- Homebrew tap (`brews:` section in `.goreleaser.yml`)
- MkDocs Material docs site + GitHub Pages deploy workflow
