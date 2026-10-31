# Runbook

- [One-time setup](#one-time-setup)
- [Sessions](#sessions)
- [Local development](#local-development)
- [Troubleshooting](#troubleshooting)

All AWS commands assume `export AWS_PROFILE=privatepaste` (account `612529366989`, us-east-1). The state bucket `privatepaste-terraform-state` already exists and is outside Terraform: it's the one thing that has to exist before Terraform can store state.

## One-time setup

Order matters: the old single-stack setup owns the GitHub OIDC provider and role names the new stacks create, so it goes first.

### 1. Destroy the old stack

The old configuration lives on `main` before this rewrite. Run its destroy from a temporary worktree:

```bash
git worktree add ../privatepaste-old 5413951   # last commit with the old ECS stack
cp infra/terraform.tfvars ../privatepaste-old/infra/
cd ../privatepaste-old/infra
terraform init
terraform destroy      # review: ECR + images, ECS, VPC, IAM, OIDC provider, cert, empty table
cd - && git worktree remove ../privatepaste-old --force
```

The old ALB was deleted outside Terraform; `destroy` refreshes, notices, and skips it.

### 2. Cloudflare API token

The tunnel needs permissions the old token didn't. Create a token (Cloudflare dashboard → My Profile → API Tokens) with:

- Account › **Cloudflare Tunnel** › Edit
- Zone › **DNS** › Edit (zone `privatepasteapp.com`)
- Zone › **Zone** › Read (zone `privatepasteapp.com`)

Revoke the old token.

### 3. Apply the persistent stack

```bash
cd infra/persistent
cp terraform.tfvars.example terraform.tfvars   # token + budget email
terraform init
terraform apply
```

This creates the table, roles, OIDC trust, tunnel, DNS record (overwriting the old ALB CNAME) and the budget. AWS Budgets doesn't send a confirmation email; alerts just arrive.

Optional cleanup in Cloudflare: delete the old ACM validation CNAME (`_xxxx.privatepasteapp.com`). Nothing uses it anymore.

### 4. First image

Merge this branch to `main`. `build.yml` pushes `ghcr.io/jtravisp/privatepaste:sha-<commit>`.

Then make the package public, once: GitHub → your profile → Packages → `privatepaste` → Package settings → Change visibility → Public. Nodes pull without credentials, so a private package fails with `ImagePullBackOff`.

### 5. First session

Actions → **cluster-up** → Run workflow. See [Sessions](#sessions).

## Sessions

| To | Do |
|---|---|
| Start | Actions → **cluster-up** → Run (optional `ttl_minutes`, default 150, max 480) |
| Extend | Run **cluster-up** again. It moves `expires-at` forward and redeploys |
| Stop now | Actions → **cluster-down** |
| See time left | `aws eks describe-cluster --name privatepaste --query 'cluster.tags."expires-at"'` |
| Deploy a change | Merge to `main`; `build.yml` deploys automatically while a cluster is up |
| Roll back | Re-run a previous successful `build` run on `main`, or locally: `kubectl -n privatepaste set image deploy/privatepaste app=ghcr.io/jtravisp/privatepaste:sha-<older>` |

**kubectl from your laptop:**

```bash
aws sso login --profile privatepaste          # if the session expired
aws eks update-kubeconfig --name privatepaste --region us-east-1 --profile privatepaste
kubectl -n privatepaste get pods,svc
kubectl -n privatepaste logs deploy/privatepaste
kubectl -n privatepaste logs deploy/cloudflared
```

**Running Terraform for the eks stack locally** (debugging only; normally the pipeline owns it):

```bash
cd infra/eks
terraform init
terraform plan -var expires_at=$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)
```

Note: if you create the cluster locally, *your* SSO role becomes the cluster creator. The pipeline's role then has no kubectl access and `deploy.yml` fails. Create clusters through the workflow.

## Local development

**Plain Go against DynamoDB Local** (fastest loop):

```bash
docker run -d --name ddb -p 8000:8000 amazon/dynamodb-local:3.3.2 -jar DynamoDBLocal.jar -inMemory -sharedDb
cd app
cp .env.example .env
make run                 # http://localhost:8080, table created automatically
make test
```

**In kind** (same manifests as EKS):

```bash
make -C app kind-up      # creates kind cluster "privatepaste" (Kubernetes 1.36)
make -C app kind-deploy  # build image, kind load, apply overlays/kind, wait
kubectl --context kind-privatepaste -n privatepaste port-forward svc/privatepaste 8080:80
make -C app kind-down
```

To expose the local cluster publicly through the real tunnel (while no EKS cluster is up), add `components: [../../components/tunnel]` to `overlays/kind` and create the `cloudflared-token` Secret from SSM, as `deploy.yml` does.

## Troubleshooting

| Symptom | Likely cause | Check / fix |
|---|---|---|
| Site shows Cloudflare **1033 / 530** | No cluster running, or `cloudflared` not connected | `kubectl -n privatepaste logs deploy/cloudflared` |
| Site shows **502** | Tunnel up, no Ready app pods | `kubectl -n privatepaste describe pods`; usually `/readyz` failing |
| `/readyz` 503, logs say `AccessDenied` | Pod Identity not applied | Pod must use service account `privatepaste` in namespace `privatepaste`; `aws eks list-pod-identity-associations --cluster-name privatepaste` |
| `ImagePullBackOff` | GHCR package is private, or the tag doesn't exist | Make the package public; check that `build` ran for that commit |
| Pods `Pending` | Nodes not Ready or not yet joined | `kubectl get nodes`; `aws eks describe-nodegroup --cluster-name privatepaste --nodegroup-name default` (Spot capacity issues show here) |
| kubectl `Unauthorized` from laptop | SSO session expired, or no access entry | `aws sso login`; `aws eks list-access-entries --cluster-name privatepaste` |
| Workflow: `Not authorized to perform sts:AssumeRoleWithWebIdentity` | Run from a branch other than `main` | The trust policy only accepts `main` |
| Workflow: `AccessDenied` on an EC2/EKS call | The deploy role's policy is missing an action | CloudTrail shows the exact action; add it in `infra/persistent/iam-github.tf` and re-apply |
| `cluster-up` waits on state lock | Another apply/destroy is running | Concurrency group plus `-lock-timeout=20m`; wait, or check for a stuck run |
| Reaper hasn't run | Scheduled workflows delayed or disabled (60 days of inactivity) | Run **cluster-reaper** or **cluster-down** manually; re-enable in the Actions tab |
