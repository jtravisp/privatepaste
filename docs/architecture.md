# Architecture

This document explains how PrivatePaste works from the browser down to the AWS account, and why each choice was made. The "Why" notes and the trade-offs section at the end are the parts worth being able to say out loud.

- [1. The short version](#1-the-short-version)
- [2. The life of a paste](#2-the-life-of-a-paste)
- [3. The application](#3-the-application)
- [4. The container image](#4-the-container-image)
- [5. Kubernetes objects](#5-kubernetes-objects)
- [6. The EKS cluster](#6-the-eks-cluster)
- [7. Networking and the Cloudflare Tunnel](#7-networking-and-the-cloudflare-tunnel)
- [8. Identity and permissions](#8-identity-and-permissions)
- [9. Terraform: two stacks](#9-terraform-two-stacks)
- [10. CI/CD](#10-cicd)
- [11. The life of a session](#11-the-life-of-a-session)
- [12. Cost](#12-cost)
- [13. Failure modes](#13-failure-modes)
- [14. Trade-offs and decisions](#14-trade-offs-and-decisions)
- [15. Security controls](#15-security-controls)

---

## 1. The short version

PrivatePaste is a Go web server that stores encrypted pastes in DynamoDB. It runs on Amazon EKS, but the cluster only exists while someone is using it. A GitHub Actions workflow builds it from Terraform in about 15 minutes, deploys the latest image, and stamps an expiry time on it. Every 15 minutes, another workflow checks the stamp and destroys the cluster once it has passed.

Everything is split by lifetime:

| Lifetime | What | Where it's defined | Applied by |
|---|---|---|---|
| Permanent | DynamoDB table, IAM roles, GitHub OIDC trust, Cloudflare tunnel and DNS, budget alert | `infra/persistent` | A human, rarely |
| Per session | VPC, EKS cluster, Spot nodes, add-ons, access entries | `infra/eks` | The pipeline |
| Per deploy | Pods, Services, the tunnel token Secret | `deploy/k8s` | The pipeline |

The data outlives the cluster, so destroying the cluster loses nothing.

## 2. The life of a paste

**Creating:**

1. The user types text and picks an expiry. `app.js` generates a random 256-bit AES key and a 12-byte IV with the browser's Web Crypto API and encrypts the text with AES-GCM.
2. The browser `POST`s `{ciphertext, iv, expiry}` to `/pastes`. **The key is not in the request.**
3. DNS for `privatepasteapp.com` points at Cloudflare. Cloudflare terminates TLS and sends the request down the **tunnel**: a connection the `cloudflared` pods opened *outbound* from the cluster.
4. `cloudflared` forwards it to `http://privatepaste.privatepaste.svc.cluster.local`, the app's ClusterIP **Service**, which load-balances across the two app **pods**.
5. The Go handler validates the shape (the IV must decode to 12 bytes, the ciphertext must be base64, the expiry must be a known value), generates a random ID and a random 32-byte **owner token**, and writes the item to DynamoDB with **only the SHA-256 of the token**.
6. The AWS SDK signs that DynamoDB request with temporary credentials that **EKS Pod Identity** provided to the pod. No access keys exist anywhere.
7. The response carries the ID and the owner token, once. The browser builds the share link `https://privatepasteapp.com/<id>#key=<base64 key>`.

**Reading:** the browser loads `/<id>`, gets `index.html`, and `app.js` reads the ID from the path and the key from the fragment. It `GET`s `/pastes/<id>`, receives ciphertext and IV, and decrypts locally. The server never had the key.

**Burn after read:** the read does a single conditional `DeleteItem ... ReturnValues=ALL_OLD`. DynamoDB applies conditional writes atomically, so if two people open the link at the same instant, exactly one delete succeeds and the other gets a 404. The paste can never be served twice.

**Deleting:** `DELETE /pastes/<id>` with `Authorization: Bearer <owner token>`. The server hashes the token and compares the hashes in constant time.

## 3. The application

`app/`: Go standard library `net/http` with Go 1.22+ method routing, the frontend embedded with `go:embed`, and one external runtime dependency besides the AWS SDK (nanoid).

| Package | Responsibility |
|---|---|
| `cmd/server` | Load config, build the AWS client, start the server, shut down gracefully |
| `internal/httpserver` | Routes, middleware (security headers, request logging, panic recovery), health probes |
| `internal/handler` | The rules: validation, expiry, burn-after-read, owner tokens |
| `internal/store` | DynamoDB reads and writes; no business logic. Plus an in-memory store for tests |
| `internal/config` | Environment variables with local defaults |

**Design rules:**

- **The store is a dumb data layer.** It doesn't know what an owner token is. `ConsumePaste` (delete and return) is the one place it encodes behavior, because atomicity can only be provided by the database.
- **Owner tokens are stored as hashes.** A leaked table doesn't let anyone delete pastes.
- **Expiry is enforced on read.** DynamoDB's TTL deletion is a background sweep that can lag by days, so the handler treats a past-TTL item as missing.
- **Unread burn pastes still expire** (after 7 days), and `never` stores no TTL attribute at all rather than a `0`.
- **Malformed IDs and expired items return the same 404 as missing ones**, so responses reveal nothing.

**Health probes.** These are deliberately different:

- `/healthz` (liveness) only proves the process answers HTTP. If it checked DynamoDB, a database outage would make Kubernetes restart every pod in a loop, which can't help and makes recovery slower.
- `/readyz` (readiness) checks the table is reachable (`DescribeTable`, 2-second timeout). A failing pod is taken out of the Service but not restarted.

**Graceful shutdown.** When Kubernetes deletes a pod, removing it from the Service's endpoints and sending it SIGTERM happen *in parallel*. If the server exited immediately, requests already routed to it would fail. So on SIGTERM the app:

1. fails `/readyz`
2. keeps serving for `SHUTDOWN_DELAY` (5s) while endpoints update
3. calls `http.Server.Shutdown`, which stops accepting and drains in-flight requests (up to 15s)

`terminationGracePeriodSeconds: 30` covers both. You can watch this in the pod logs: "shutdown signal received", then five seconds later "shutdown complete".

**Security headers** on every response:

- **CSP:** a strict Content-Security-Policy: same-origin scripts, styles and fetches only, no inline code, no framing. This matters more than usual here: any injected script could read the key out of the URL fragment.
- **`Referrer-Policy: no-referrer`:** the paste ID in the path never leaks to another site.
- **Caching:** `Cache-Control: no-store` on API responses, so no browser or edge cache can replay a burned paste.

**Logging** is JSON (`log/slog`) and records the *route pattern* (`GET /pastes/{id}`), never the path, so paste IDs don't end up in logs.

**Tests** (`go test ./...`) run against the in-memory store: validation, the 512KB body limit, owner-token hashing, delete authorization, expiry, and a burn-after-read race with 20 concurrent readers that must produce exactly one success. CI runs them with `-race`.

## 4. The container image

`app/Dockerfile` is a two-stage build:

1. **Build stage** (`golang:1.26.10-alpine`, pinned by digest): download and verify modules against `go.sum`, then compile a static binary (`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`).
2. **Runtime stage** (`gcr.io/distroless/static:nonroot`, pinned by digest): copy in only the binary and run it as UID 65532.

Only the last stage ships. The Go toolchain, Alpine and the source code stay in the build stage. The base OS of the two stages doesn't matter because the binary is static: it links nothing from the OS and talks straight to the host's kernel. The hardening choices and measured before/after are in §15.1.

Images go to **GHCR** (`ghcr.io/jtravisp/privatepaste`), tagged `sha-<commit>` (immutable by convention: one tag per commit) and `main` (floating, for people). Deploys always use the SHA tag, so the cluster runs exactly the commit that was tested, and rolling back means deploying an older SHA.

## 5. Kubernetes objects

Manifests are [Kustomize](https://kustomize.io): a `base` that's the same everywhere, plus `overlays` per environment. Kustomize is built into `kubectl` (`kubectl apply -k`).

| Object | File | Why it's there |
|---|---|---|
| Namespace `privatepaste` | `base/namespace.yaml` | One boundary for everything the app owns; `kubectl delete namespace` removes it all |
| ServiceAccount `privatepaste` | `base/serviceaccount.yaml` | The pod's identity. EKS maps it to an IAM role (§8); the manifest itself has no AWS references |
| Deployment `privatepaste` | `base/deployment.yaml` | 2 replicas; rolling update with `maxUnavailable: 0`; probes; resource requests; spread across nodes |
| Service `privatepaste` | `base/service.yaml` | ClusterIP: a stable internal name and virtual IP; never reachable from outside |
| PodDisruptionBudget | `base/pdb.yaml` | Voluntary evictions (node drains) keep at least one pod running |
| Deployment `cloudflared` | `components/tunnel/` | The tunnel connector, 2 replicas (§7) |
| Secret `cloudflared-token` | created by `deploy.yml` | The connector's credential, copied from SSM at deploy time, never stored in git |

**Deployment details worth knowing:**

- **`maxSurge: 1, maxUnavailable: 0`:** a new pod must pass readiness before an old one is removed, so a deploy never drops below two serving pods.
- **`topologySpreadConstraints` on hostname:** one replica per node when possible. When AWS reclaims a Spot node you lose one pod, not both. `ScheduleAnyway` (not `DoNotSchedule`) keeps it working on single-node kind.
- **Resources:** requests (50m CPU, 64Mi) are what the scheduler reserves. A memory *limit* (128Mi) caps leaks. There's no CPU limit, on purpose: CPU is compressible, and a CPU limit throttles the process even when the node has idle CPU, which shows up as latency.
- **`securityContext`:** non-root, no capabilities, no privilege escalation, seccomp, read-only root filesystem, and no Kubernetes API token. Details and verification in §15.2.

**Overlays:**

- `overlays/eks`: the image becomes `ghcr.io/jtravisp/privatepaste:<tag>` (the workflow sets the tag), sets `TABLE_NAME=privatepaste-prod`, and includes the tunnel component.
- `overlays/kind`: the image becomes `privatepaste:dev` with `imagePullPolicy: Never` (loaded with `kind load`, never pulled), 1 replica, and a DynamoDB Local pod. `DYNAMODB_ENDPOINT` points the SDK at it, which also makes the app create the table on start (a local-only convenience; the production role has no `CreateTable` permission).

## 6. The EKS cluster

`infra/eks/cluster.tf`.

**Control plane.** AWS runs the Kubernetes API server and etcd across three availability zones, patches them, and backs them up, for $0.10/hour. That charge accrues whether or not anything runs, and a cluster can't be paused, which is the whole reason this cluster is ephemeral.

- **Version 1.36**, with `upgrade_policy { support_type = "STANDARD" }`. When a version leaves standard support, EKS would otherwise move it into extended support at $0.60/hour. This makes that a deliberate choice.
- **`authentication_mode = "API"`**: kubectl access is granted with **access entries** (EKS API objects managed by Terraform), not the legacy `aws-auth` ConfigMap. Who has access is visible in AWS and in code.
- **`bootstrap_cluster_creator_admin_permissions = true`**: the identity that creates the cluster (the pipeline's role) becomes cluster admin, which is how the same workflow can deploy to it right after.
- **`bootstrap_self_managed_addons = false`**: EKS doesn't install its own unmanaged copies of the networking components; Terraform installs them as **managed add-ons** instead (versioned, upgradeable through the EKS API).

**Add-ons:**

| Add-on | What it does |
|---|---|
| `vpc-cni` | Gives every pod a real IP address from the VPC subnet (no overlay network). Nodes aren't Ready without it |
| `kube-proxy` | Programs each node so traffic to a Service's virtual IP reaches a backing pod |
| `coredns` | Cluster DNS, the reason `privatepaste.privatepaste.svc.cluster.local` resolves. Created after the nodes, since its pods need somewhere to run |
| `eks-pod-identity-agent` | A DaemonSet that hands IAM credentials to pods (§8) |

**Nodes:** a **managed node group** of two **Spot** instances (`t3.medium` or `t3a.medium`, whichever has capacity).

- **Managed:** EKS creates the Auto Scaling group and launch template, uses the EKS-optimized Amazon Linux 2023 AMI, and drains nodes gracefully during updates.
- **Spot:** about 70% cheaper. AWS can reclaim an instance with two minutes' notice; the replica spread and the PDB turn that into a brief reschedule. Listing two instance types means a shortage of one doesn't block the node group.
- **Two nodes** so the two replicas can be on different machines. Max size is three, to leave room for a surge node during node group updates.

**Who can run kubectl:**

- **The pipeline's role**, as cluster creator.
- **Your SSO AdministratorAccess role**, via an access entry plus the `AmazonEKSClusterAdminPolicy` association. Terraform finds that role by name pattern, since its suffix is generated.
- **One gotcha:** SSO role ARNs contain the path `/aws-reserved/sso.amazonaws.com/`, and access entries must use the ARN *without* it, or kubectl returns `Unauthorized`.

## 7. Networking and the Cloudflare Tunnel

**VPC** (`infra/eks/network.tf`): one `/16`, two public `/20` subnets in two AZs (EKS requires two), an internet gateway, and one route table. **No NAT gateway.**

The textbook layout puts nodes in private subnets behind a NAT gateway. A NAT gateway costs about $0.045/hour plus data processing, which would be the largest line item in this project. Instead, nodes get public IPs and reach ECR, GHCR, DynamoDB and Cloudflare directly.

That's safe because **nothing listens publicly**:

- The node security group (created by EKS) admits traffic only from within the cluster.
- There's no load balancer and no NodePort.
- The only inbound path is the tunnel, which is outbound-initiated.

A public IP with no open ports reaches nothing.

**The tunnel** (`infra/persistent/cloudflare.tf`, `deploy/k8s/components/tunnel`):

- **Terraform creates** a Cloudflare Tunnel, its routing rule (`privatepasteapp.com` → the in-cluster Service URL), and a proxied CNAME `privatepasteapp.com → <tunnel-id>.cfargotunnel.com`. All of these are permanent and free.
- **The `cloudflared` pods** start with the tunnel's token and open outbound connections to Cloudflare's edge.
- **Requests** arrive at Cloudflare (which terminates TLS) and are sent down those connections to `cloudflared`, which forwards them to the Service.

Why a tunnel instead of the AWS Load Balancer Controller and an ALB:

- **Cost:** an ALB is about $0.0225/hour plus public IPs; the tunnel is free.
- **Teardown safety:** an ALB created by a Kubernetes controller is invisible to Terraform. If the cluster is destroyed before the Ingress is deleted, the ALB is orphaned and keeps billing. With the tunnel, Kubernetes provisions nothing in AWS.
- **Attack surface:** no inbound ports at all.
- **Portability:** the same connector works from a local kind cluster.

What you give up: an AWS-native ingress story, and a dependency on Cloudflare. When no cluster is running, Cloudflare returns its error 1033 page (HTTP 530).

## 8. Identity and permissions

There are no long-lived credentials anywhere: no access keys in GitHub, none in the cluster, none on laptops (SSO).

| Identity | Assumed by | Can do | Defined in |
|---|---|---|---|
| `privatepaste-gha-deploy` | GitHub Actions, `main` branch only | Create/destroy *this* cluster and its VPC; read/write *eks/* state; pass three specific roles; read the tunnel token | `persistent/iam-github.tf` |
| `privatepaste-eks-cluster` | EKS control plane | Manage cluster networking on your behalf | `persistent/iam-eks.tf` |
| `privatepaste-eks-node` | Worker EC2 instances | Join the cluster, assign pod IPs, pull add-on images from ECR | `persistent/iam-eks.tf` |
| `privatepaste-app` | App pods, via Pod Identity | `GetItem`/`PutItem`/`DeleteItem`/`DescribeTable` on one table | `persistent/iam-eks.tf` |
| SSO AdministratorAccess | You | Everything; kubectl admin via access entry | IAM Identity Center |

**GitHub → AWS (OIDC):**

1. GitHub mints a signed OIDC token for each workflow run. Its `sub` claim names the repo and ref, for example `repo:jtravisp/privatepaste:ref:refs/heads/main`.
2. AWS trusts GitHub's issuer, and the role's trust policy requires that exact `sub`. A workflow on another branch, or a pull request from a fork, presents a different `sub` and is refused.
3. `configure-aws-credentials` exchanges the token for about an hour of credentials.

**Why the cluster's roles live in the persistent stack:**

- **No role creation:** the pipeline never needs `iam:CreateRole` or `iam:AttachRolePolicy`. It can only `iam:PassRole` the three roles a human already created.
- **Bounded blast radius:** if a workflow were compromised, it could create and delete one EKS cluster. It couldn't mint itself an admin role.
- **Service-linked roles:** for the same reason, the roles EKS normally creates on first use (`AWSServiceRoleForAmazonEKS` and friends) are pre-created in the persistent stack.

**Pod Identity (pods → AWS):**

1. Terraform creates an association: *service account `privatepaste` in namespace `privatepaste` → role `privatepaste-app`*.
2. When such a pod starts, EKS injects `AWS_CONTAINER_CREDENTIALS_FULL_URI` and a projected service account token.
3. The AWS SDK's default credential chain calls the Pod Identity agent on the node, which exchanges the token for role credentials.

Other pods on the same node, running as other service accounts, get nothing. The nodes' own role has no DynamoDB access.

Pod Identity versus IRSA (the older mechanism): IRSA needs an IAM OIDC provider per cluster, and the role's trust policy names that cluster's OIDC issuer URL. Since this cluster gets a new issuer every session, IRSA would mean rewriting trust policies on every create. With Pod Identity the trust policy just names `pods.eks.amazonaws.com`, and the association lives with the cluster.

**State isolation:** the pipeline's S3 permissions cover `eks/*` only. It can't read the persistent state, which contains the tunnel token, or change it.

## 9. Terraform: two stacks

Both stacks use the same S3 bucket (`privatepaste-terraform-state`, versioned) with different keys and S3-native locking (`use_lockfile = true`, Terraform ≥ 1.10: no DynamoDB lock table).

**`infra/persistent`.** Applied by a human with `AWS_PROFILE=privatepaste`. Changes rarely.

| File | Resources |
|---|---|
| `dynamodb.tf` | `pastes` table: on-demand, TTL on `ttl`, deletion protection |
| `iam-github.tf` | GitHub OIDC provider, deploy role and its policy |
| `iam-eks.tf` | Cluster, node and app roles; service-linked roles |
| `cloudflare.tf` | Tunnel, tunnel routing, DNS CNAME, tunnel token → SSM SecureString |
| `budget.tf` | Monthly budget, emails at 50% actual and 100% forecast |

**`infra/eks`.** Applied and destroyed by the pipeline. It takes one required variable, `expires_at`, which is stamped on every resource through the provider's `default_tags`.

| File | Resources |
|---|---|
| `network.tf` | VPC, two public subnets, internet gateway, route table |
| `cluster.tf` | Cluster, Spot node group, four managed add-ons |
| `access.tf` | Admin access entry (SSO role lookup), Pod Identity association |

The ephemeral stack finds persistent resources **by name** with data sources (`data "aws_iam_role"`), not `terraform_remote_state`. Reading remote state would require read access to the persistent state file; looking up a role by name only requires `iam:GetRole` on that role.

**Dependency ordering inside `infra/eks`:**

1. VPC and subnets.
2. The cluster.
3. The `vpc-cni`, `kube-proxy` and Pod Identity add-ons (their DaemonSets wait for nodes).
4. The node group (nodes become Ready because the CNI exists).
5. `coredns` (its pods need nodes).

Destroy runs in reverse.

## 10. CI/CD

| Workflow | Trigger | Does |
|---|---|---|
| `build.yml` | every PR, every push to `main` | `go vet`, `go test -race`, build image. On `main`: push `sha-<commit>` and `main` tags to GHCR; if the cluster is ACTIVE, call `deploy.yml` |
| `deploy.yml` | called by `build` and `cluster-up` | Wait for the image, set the tunnel Secret from SSM, `kustomize edit set image`, apply, wait for rollout, smoke-test `https://privatepasteapp.com/readyz` |
| `cluster-up.yml` | manual, input `ttl_minutes` (default 150) | Compute `expires_at`, `terraform apply` the eks stack, call `deploy.yml` |
| `cluster-down.yml` | manual, or called by the reaper | Delete any Kubernetes-provisioned AWS resources, then `terraform destroy` |
| `cluster-reaper.yml` | every 15 minutes | Read the cluster's `expires-at` tag; call `cluster-down` if it passed or is missing |

**Points worth explaining:**

- **Commit to production:** a merge to `main` runs tests, builds and pushes `sha-<commit>`. If a cluster is up, it deploys that exact tag and verifies it end to end through the public URL. If no cluster is up, the image waits in GHCR, and the next `cluster-up` deploys the head of `main`.
- **Concurrency groups:** `eks-terraform` serializes apply and destroy (the reaper can't destroy mid-apply), and `eks-deploy` serializes rollouts. `-lock-timeout=20m` covers the state lock as a second layer.
- **Extending a session:** re-running `cluster-up` re-applies with a later `expires_at`, which only changes tags, and redeploys.
- **Input handling:** `ttl_minutes` reaches the shell through an environment variable, never interpolated into the script text, and is validated as an integer from 30 to 480.
- **Least-privilege tokens:** the workflow default is `contents: read`. Only jobs that need it get `packages: write` (push) or `id-token: write` (AWS).
- **The smoke test** exercises the full path: DNS, the Cloudflare edge, the tunnel, `cloudflared`, the Service, the pod, Pod Identity and DynamoDB. A green deploy means the public site works, not just that pods started.

## 11. The life of a session

| Time | Event |
|---|---|
| T+0 | You run **cluster-up** (ttl 150). Expiry is T+150 |
| T+1 to T+11 | Terraform creates the VPC and the control plane (the slow part) |
| T+11 to T+14 | Add-ons, then Spot nodes join, then CoreDNS |
| T+14 to T+16 | `deploy.yml`: tunnel Secret, manifests, rollout. `cloudflared` connects; the smoke test passes |
| | The site is live. Pushes to `main` deploy automatically |
| T+150 | Expiry passes |
| ≤ T+165 | The reaper's next run sees it, then `cluster-down` deletes the namespace and runs `terraform destroy` (about 10 minutes) |
| After | DynamoDB, IAM, the tunnel and the DNS remain. The site shows Cloudflare 1033 |

## 12. Cost

| While a cluster is up | Per hour |
|---|---|
| EKS control plane | $0.10 |
| 2 × t3.medium Spot (varies) | ~$0.025 |
| 2 public IPv4 addresses | $0.01 |
| EBS (2 × 20 GB gp3) | ~$0.004 |
| **Total** | **~$0.14/hour, about $0.35 per 2.5-hour session** |

| Always | Per month |
|---|---|
| DynamoDB on-demand (idle) | cents |
| S3 state, SSM parameter, IAM, OIDC | ~$0 |
| Cloudflare tunnel and DNS | $0 |
| GHCR (public), GitHub Actions (public repo) | $0 |

Avoided on purpose: NAT gateway (~$33/month), ALB (~$16/month + IPs), always-on control plane ($73/month), extended-support pricing ($438/month), CloudWatch control-plane logs.

The **budget alert** ($10/month: email at 50% actual, 100% forecast) catches the failure that matters, a cluster that never got destroyed.

## 13. Failure modes

| What breaks | What happens | Why that's acceptable or how it's handled |
|---|---|---|
| A Spot node is reclaimed | One app pod and one `cloudflared` pod go away; the Service and tunnel use the survivors; the node group launches a replacement | Two replicas spread across nodes |
| DynamoDB is unreachable | `/readyz` fails, pods leave the Service, Cloudflare returns 502; pods are *not* restarted | Liveness doesn't depend on the database |
| A bad image is deployed | New pods fail readiness; with `maxUnavailable: 0` the old pods keep serving; `rollout status` fails the workflow | Roll back by deploying the previous SHA |
| A deploy races a fresh build | `deploy.yml` waits up to 10 minutes for the image tag | |
| The reaper misses runs | The cluster lives longer than intended | Untagged clusters are treated as expired; the budget email is the backstop |
| `cluster-up` fails halfway | Partial resources in the eks state | Run `cluster-down`; destroy is idempotent. A leftover VPC costs nothing |
| Something Kubernetes provisioned in AWS (an ELB, an EBS volume) | Would be orphaned by `terraform destroy` | `cluster-down` deletes LoadBalancer Services and PVCs first; this project creates neither |
| Cloudflare outage | The site is down | Accepted dependency |
| Tunnel token leaks | Someone could run a connector and receive a share of traffic | Rotate by re-creating the tunnel in Terraform; the token is never in git or GitHub |

## 14. Trade-offs and decisions

**Why EKS for an app this small?** It isn't the cheapest way to host this app. Lambda would be about $0 (see the git history for that analysis). The point is to run a real workload on the platform that employers run, with the operational concerns that come with it: identity, ingress, rollouts, cost control. The ephemeral pattern makes that affordable.

**Why not keep the cluster running and scale nodes to zero?** The $73/month is the control plane, which can't scale to zero. Only deleting the cluster stops the charge.

**Why not k3s on one EC2 instance?** Cheaper, but you'd run the control plane yourself, and you'd miss the EKS-specific parts (access entries, Pod Identity, managed add-ons and node groups) that are most of what's worth learning here.

**Why public subnets?** NAT gateway cost. The safety argument is in §7: no listeners, so public addresses expose nothing. In a production account you'd use private subnets with VPC endpoints or NAT, and you'd likely restrict the API endpoint's `public_access_cidrs`.

**Why is the API endpoint public?** GitHub-hosted runners have no fixed IPs. Authentication is IAM plus access entries either way; a public endpoint is reachable, not open. The production alternative is a private endpoint with self-hosted runners inside the VPC.

**Why Pod Identity over IRSA?** No per-cluster OIDC provider, and trust policies don't change when the cluster is recreated (§8).

**Why a Cloudflare Tunnel over an ALB?** Cost, teardown safety, zero inbound ports (§7).

**Why two Terraform stacks?** Different lifetimes and different trust: the human-applied stack holds data and permissions; the pipeline-applied stack holds only disposable compute. CI can't touch the first.

**Why GHCR over ECR?** Free, no AWS credentials needed to build, and it pairs with keyless signing later. The cost is that the package must be public for nodes to pull it without credentials.

## 15. Security controls

The first version of this stack shipped deliberately unhardened (an Alpine image running as root, no `securityContext`, no scanning) so that hardening it would have a real "before". This section records what changed, what was measured, and the rules the pipeline enforces.

### 15.1 The image

| | Before | After |
|---|---|---|
| Runtime base | `alpine:3.21` (tag) | `distroless/static:nonroot` (digest) |
| Builder base | `golang:1.26.3-alpine` (tag) | `golang:1.26.10-alpine` (digest) |
| Size (disk / compressed) | 35 MB / 11.5 MB | 21.3 MB / 5.0 MB |
| OS packages | 15 | 6 |
| Runs as | root (UID 0) | UID/GID 65532 |
| Shell | yes | none |
| Trivy, OS packages | 0 | 0 |
| Trivy, Go binary | 27 (15 HIGH), all in `stdlib v1.26.3` | 0 |

**What fixed what.** The base-image swap fixed **zero** CVEs: Alpine 3.21.9 was already clean. All 27 findings were in Go's standard library, which is compiled *into* the binary, so no base image could remove them. A Go patch bump (1.26.3 to 1.26.10) cleared all of them. What distroless changed is **attack surface**: no shell, no package manager, no busybox, fewer packages, non-root by default. A CVE count is a snapshot of today's disclosures. Attack surface is what's still true when the next CVE is published.

**Choices worth explaining:**

- **`CGO_ENABLED=0`:** a static binary with no libc dependency, so it runs in an image that has no libc. With cgo on, the container would fail at start with a misleading `no such file or directory`.
- **`distroless/static` over `scratch`:** the app calls DynamoDB over HTTPS and needs a CA bundle. Distroless provides CA certificates, tzdata, `/tmp` and a `nonroot` passwd entry; scratch provides nothing.
- **`USER 65532:65532`, numeric:** Kubernetes `runAsNonRoot` can only verify a numeric user. The kubelet reads the image config, not the image's `/etc/passwd`, so it rejects a named user it can't resolve.
- **Exec-form `CMD ["./server"]`:** shell form would need `/bin/sh` (absent), and would make the shell PID 1. The shell doesn't forward SIGTERM, so graceful shutdown would never run.
- **Digest pins:** a tag is a mutable pointer in the registry; a digest is a hash of the content, and the client checks it on pull. The cost is that patched base images don't arrive until someone bumps the digest. The plan is for Dependabot to open those bumps as PRs, so updates go through review and the scanners.
- **The binary is owned by root, the process runs as 65532:** the running app can't modify its own executable.
- **`.dockerignore` is an allowlist:** only `go.mod`, `go.sum`, `cmd/`, `internal/`, `assets.go` and `frontend/` enter the build context. A new file, such as a stray `.env`, is excluded by default.
- **One Go version, set in `go.mod`:** CI's `setup-go` reads it, and the Dockerfile's builder tag must match. If they differ, CI tests a different standard library from the one that ships.

### 15.2 The pod

`deploy/k8s/base/deployment.yaml`:

| Setting | Level | Prevents |
|---|---|---|
| `runAsNonRoot: true` | pod | Starting any container as UID 0 |
| `runAsUser` / `runAsGroup: 65532` | pod | Running as whatever user a future base image defaults to, or with root as the primary group |
| `seccompProfile: RuntimeDefault` | pod | Rarely used, dangerous syscalls (many clusters default to Unconfined) |
| `allowPrivilegeEscalation: false` | container | Gaining privilege through setuid binaries (sets `no_new_privs`) |
| `capabilities.drop: [ALL]` | container | Using any Linux capability, such as raw sockets or `chown` |
| `readOnlyRootFilesystem: true` | container | Writing tools or modifying files inside the container |
| `automountServiceAccountToken: false` | pod spec | Getting a Kubernetes API credential from a compromised pod. EKS Pod Identity mounts its own token, so AWS access is unaffected |

**Verified on a running pod** (`kubectl debug --target=app`, then `cat /proc/1/status`), not just read from the manifest:

| Field | Value | Meaning |
|---|---|---|
| `Uid` / `Gid` | `65532` ×4 | Real, effective, saved and filesystem IDs are all non-root, and the group isn't root |
| `CapEff` / `CapBnd` | `0000000000000000` | No effective capabilities, and an empty bounding set, so none can ever be gained |
| `NoNewPrivs` | `1` | Privilege escalation is blocked |
| `Seccomp` | `2` | A seccomp filter is active (0 = off, 1 = strict, 2 = filter) |

**Defense in depth:** because the pod sets `runAsUser: 65532`, even the old root-default Alpine image would start as UID 65532 rather than being rejected. The pod spec protects you from a bad image. That's also why admission policy (§15.4) checks the pod spec, not just the image.

**Pod Security Admission:** the namespace has `pod-security.kubernetes.io/warn: restricted`. It never blocks; it prints violations on every `kubectl apply`. The app passes `restricted`. `dynamodb-local` (a third-party image, kind only) does not yet.

### 15.3 Scanning policy

**Principles:**

1. **A gate fails only on something the PR author can fix.** A check that blocks with no remedy teaches people to bypass checks.
2. **Every exception lives in the repo, with a reason, and an expiry where the tool supports one.** Accepted risk is a reviewed decision, not a silenced alert.
3. **Each scanner answers one question no other scanner is answering.** Overlapping scanners mean triaging the same finding twice.
4. **Scan the artifact that ships.** On `main`, the image is built once, scanned, pushed and signed by **digest**. What was scanned is byte-for-byte what runs.
5. **Pin scanner versions, not vulnerability databases.** A new scanner release can add rules and break unrelated PRs, so upgrade it deliberately. The vulnerability database must stay fresh, or the scan stops being useful.

| Scanner | Question it answers | Scans | **Fails the build** | Reports only | Why |
|---|---|---|---|---|---|
| **Gitleaks** | Was a secret committed? | Git history (the PR's commits; full history on a schedule) | Any finding | — | A pushed secret is compromised, especially in a public repo, and the only fix is to rotate it. False positives are suppressed per finding in `.gitleaksignore` |
| **govulncheck** | Does our code *call* a vulnerable function? | Go source, with the toolchain from `go.mod` | Any reachable vulnerability | Vulnerable modules that are imported but not called | Reachability analysis has very few false positives. A reachable vulnerability is a real exposure, and the fix is almost always a version bump |
| **Trivy** (image, `--scanners vuln`) | Does the shipped image contain known CVEs? | The built image: OS packages and the Go binary | HIGH or CRITICAL **with a fix available** (`--ignore-unfixed`) | Everything else: lower severities, and HIGH/CRITICAL with no fix yet | A fixable HIGH is cheap to fix and dangerous to ship. An unfixed one has no action the author can take; it's tracked instead, and an accepted one goes in `.trivyignore` with a reason and expiry |
| **Checkov** | Is the infrastructure configured insecurely? | `infra/**/*.tf` | Nothing yet (soft-fail) | All findings | The first run on existing Terraform produces a backlog, much of it deliberate (public subnets, public EKS endpoint; see §14). Each finding gets fixed or gets an inline `#checkov:skip=<ID>:<reason>` next to the resource. Once the baseline is clean, switch to hard-fail so only *new* findings block |

**Known gaps, and what covers them:**

- **A CI secret scan detects; it doesn't prevent.** By the time a PR runs, the branch is already pushed to a public repo. Prevention happens earlier: GitHub secret scanning **push protection** (free on public repos) rejects the push, and a local Gitleaks pre-commit hook stops the commit.
- **CVEs are disclosed against code that hasn't changed.** The PR gate stops new risk going out; it can't see new disclosures against what's already deployed. A scheduled weekly Trivy scan of the current `main` image covers that (planned).
- **Overlap with Trivy.** Trivy can also scan for secrets and IaC problems. Those are left to Gitleaks and Checkov, so each finding has one owner.
- **gosec (Go static analysis)** isn't adopted yet. It flags *patterns*, not known CVEs, and needs tuning to keep noise down. It's a candidate once the gates above are routine.

### 15.4 Still planned

| Area | Current state | Planned |
|---|---|---|
| Scanning in CI | Run locally (§15.3) | The PR workflow enforces the table above |
| Signing | Images unsigned | cosign keyless signing on `main`, using the workflow's GitHub OIDC identity, by digest |
| Admission control | PSA `warn` only | Kyverno in kind: Audit first, then Enforce; reject pods that can run as root. Stretch: `verifyImages` rejects unsigned images |
| Supply chain | Actions pinned to major tags | Pin actions by commit SHA; Dependabot for actions, Go modules and base-image digests |
| Network | No NetworkPolicy | Allow app ingress only from `cloudflared` (VPC CNI network policy) |
