# PrivatePaste

A zero-knowledge encrypted text vault. The browser encrypts every paste with AES-256-GCM before it leaves the page; the key lives in the URL fragment (`#key=...`), which browsers never send to a server. The server stores ciphertext it cannot read.

It runs on an **on-demand Amazon EKS cluster that destroys itself**. A GitHub Actions workflow creates the cluster and deploys in about 15 minutes; a reaper workflow tears it down when its time is up (2.5 hours by default). Between sessions the only things left in AWS are a DynamoDB table and some IAM roles, which cost close to nothing.

```
browser ──HTTPS──▶ Cloudflare edge ──Tunnel──▶ cloudflared pods ──▶ Service ──▶ app pods ──▶ DynamoDB
                   privatepasteapp.com          (outbound-only)     ClusterIP     Go          (Pod Identity)
                                       └──────────────── ephemeral EKS cluster ─────────────┘   persistent
```

| | |
|---|---|
| [docs/architecture.md](docs/architecture.md) | How every piece works and why it is built that way |
| [docs/runbook.md](docs/runbook.md) | One-time setup, starting and stopping sessions, local development, troubleshooting |

## Quick reference

**Start a session** (about 15 min, about $0.35 for 2.5 hours):
GitHub → Actions → **cluster-up** → Run workflow. Running it again extends the session.

**End it early:** Actions → **cluster-down**. Otherwise the **cluster-reaper** destroys it within 15 minutes of expiry.

**kubectl access while it is up:**

```bash
aws eks update-kubeconfig --name privatepaste --region us-east-1 --profile privatepaste
kubectl -n privatepaste get pods
```

**Run locally, no AWS:**

```bash
make -C app kind-up kind-deploy
kubectl --context kind-privatepaste -n privatepaste port-forward svc/privatepaste 8080:80
# http://localhost:8080
```

## Repository layout

```
app/                         Go server + embedded frontend
  cmd/server/main.go           config, AWS client, graceful shutdown
  internal/handler/            paste API: create, read (with burn), delete
  internal/httpserver/         routes, security headers, health probes
  internal/store/              DynamoDB access (+ in-memory store for tests)
  frontend/                    index.html, app.js (Web Crypto), style.css
  Dockerfile
deploy/k8s/                  Kustomize manifests
  base/                        Deployment, Service, ServiceAccount, PDB
  components/tunnel/           cloudflared connector
  overlays/eks/                GHCR image, real table, tunnel
  overlays/kind/               local image, DynamoDB Local
infra/
  persistent/                  long-lived: DynamoDB, IAM, GitHub OIDC, Cloudflare tunnel, budget
  eks/                         ephemeral: VPC, EKS cluster, Spot nodes, add-ons, access
.github/workflows/
  build.yml                    test + image on every PR/push; push + deploy on main
  deploy.yml                   reusable: roll an image out to the cluster
  cluster-up.yml               create cluster, deploy
  cluster-down.yml             destroy cluster
  cluster-reaper.yml           every 15 min: destroy if expired
scripts/kind-up.sh           local cluster
```

## API

| Method | Path | |
|---|---|---|
| `POST` | `/pastes` | `{ciphertext, iv, expiry, burn_after_read}` → `201 {id, owner_token}` |
| `GET` | `/pastes/{id}` | `200 {ciphertext, iv, burn_after_read}`; a burn paste is deleted by this read |
| `DELETE` | `/pastes/{id}` | `Authorization: Bearer <owner_token>` → `204` |
| `GET` | `/healthz` | liveness: the process answers |
| `GET` | `/readyz` | readiness: DynamoDB reachable and not shutting down |

`expiry` is one of `burn`, `1h`, `24h`, `7d`, `never`.
