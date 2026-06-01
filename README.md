# PrivatePaste

A zero-knowledge encrypted text vault. The server stores only ciphertext. It has no access to paste content.

## How it works

Encryption happens in the browser using the Web Crypto API (AES-256-GCM). The encryption key is generated client-side, appended to the share URL as a fragment (`#key=...`), and never transmitted to the server. The server receives and stores only the encrypted payload and IV. Decryption happens client-side on retrieval.

Pastes support configurable expiry (burn after read, 1h, 24h, 7d, or no expiry).

## Architecture

**Go + standard library** `net/http` with Go 1.22+ method-based routing covers everything.

**DynamoDB** The access pattern is key-value by paste ID. DynamoDB's TTL attribute handles paste expiration.

**JS embedded in the Go binary** Frontend files are embedded via `go:embed` and served directly by the Go server. 

**ECS Fargate + ALB** The ALB is included even for low traffic, HTTPS terminated at the ALB.

**Terraform** Complete infra packaged in the repo. State is stored in S3.

## Design decisions

**The store is a dumb data layer.** `store.Store` methods only read or write to DynamoDB. 

**Owner tokens are generated server-side.** At paste creation, the server generates a cryptographically random owner token, returns it once in the creation response, and stores only its SHA-256 hash alongside the paste. The raw token is never persisted. On delete, the handler fetches the paste, hashes the provided token, compares it to the stored hash, and only proceeds if they match.

**`DeletePaste` takes only an ID.** It does not know about tokens. Token verification is the handler's responsibility.

**`burn_after_read` is handled by the handler.** `GetPaste` retrieves and returns a paste unconditionally. If the paste has `BurnAfterRead: true`, the handler calls `DeletePaste` after a successful retrieval. This keeps the store's behavior predictable and makes the burn logic easy to reason about in isolation.

**Request body size is capped.** CreatePaste wraps r.Body with http.MaxBytesReader before decoding, limiting payloads to 512KB. Oversized requests are rejected at the HTTP layer before touching DynamoDB.

## Deployment

Built and pushed to ECR manually or via GitHub Actions. The ECS service runs task definition on Fargate (see Terraform ECS module).

To deploy a new image manually:

1. Build and push to ECR with a new tag (tags are immutable, `latest` cannot be overwritten)
2. Register a new task definition revision pointing at the new image tag
3. Update the service: `aws ecs update-service --cluster <project_name>-cluster --service <project_name>-service --task-definition <project_name>-task:<revision> --force-new-deployment`

## Data model

DynamoDB table: `pastes`

| Attribute | Type | Notes |
|---|---|---|
| `id` | String (PK) | nanoid- short, URL-safe |
| `ciphertext` | String | AES-256-GCM encrypted content, base64 |
| `iv` | String | AES-GCM nonce, base64 |
| `owner_token_hash` | String | SHA-256 of owner token |
| `burn_after_read` | Boolean | Delete on first retrieval |
| `ttl` | Number | Unix timestamp |
| `created_at` | Number | Unix timestamp |

## Structure

```
privatepaste/
├── app/
│   ├── assets.go
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── model/paste.go
│   │   ├── store/store.go
│   │   ├── store/dynamo.go
│   │   └── handler/paste.go
│   ├── frontend/
│   │   ├── index.html
│   │   ├── app.js
│   │   └── style.css
│   ├── Dockerfile
│   ├── Makefile
│   └── go.mod
├── infra/
    ├── modules/
│   ├── main.tf
│   ├── variables.tf
│   └── outputs.tf
└── .github/workflows/app.yml
```
