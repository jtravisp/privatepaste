# PrivatePaste

A zero-knowledge encrypted text vault. The server stores only ciphertext — it has no access to paste content, ever.

## How it works

Encryption happens in the browser using the Web Crypto API (AES-256-GCM). The encryption key is generated client-side, appended to the share URL as a fragment (`#key=...`), and never transmitted to the server. The server receives and stores only the encrypted payload and IV. Decryption happens client-side on retrieval.

Pastes support configurable expiry (burn after read, 1h, 24h, 7d, or no expiry) and optional password protection as a second encryption layer. No accounts, no login.

## Architecture

**Go + standard library** — no web framework. `net/http` with Go 1.22+ method-based routing covers everything needed, and keeping dependencies minimal reduces attack surface and build complexity.

**DynamoDB** — the access pattern is purely key-value by paste ID. There are no relational queries, no joins, no need for a connection pool, and no migrations. DynamoDB's native TTL attribute handles paste expiry without a background job. PAY_PER_REQUEST billing fits low and bursty traffic.

**Vanilla JS embedded in the Go binary** — frontend files are embedded via `go:embed` and served directly by the Go server. No CDN, no S3 bucket, no separate deployment. A single container is the whole application.

**ECS Fargate + ALB** — containerized from the start. The ALB is included even for low traffic; it's the right pattern for a Fargate-hosted service and makes HTTPS termination straightforward.

**Terraform** — all infrastructure is code. State is stored in S3 with DynamoDB locking.

## Design decisions

**The store is a dumb data layer.** `store.Store` methods do exactly one thing: read or write to DynamoDB. Auth logic, business rules, and conditional behavior live in the handlers, not the store.

**Owner tokens are generated server-side.** At paste creation, the server generates a cryptographically random owner token, returns it once in the creation response, and stores only its SHA-256 hash alongside the paste. The raw token is never persisted. On delete, the handler fetches the paste, hashes the provided token, compares it to the stored hash, and only proceeds if they match.

**`DeletePaste` takes only an ID.** It does not know about tokens. Token verification is the handler's responsibility.

**`burn_after_read` is handled by the handler.** `GetPaste` retrieves and returns a paste unconditionally. If the paste has `BurnAfterRead: true`, the handler calls `DeletePaste` after a successful retrieval. This keeps the store's behavior predictable and makes the burn logic easy to reason about in isolation.

## Data model

DynamoDB table: `pastes`

| Attribute | Type | Notes |
|---|---|---|
| `ID` | String (PK) | nanoid — short, URL-safe |
| `Ciphertext` | String | AES-256-GCM encrypted content, base64 |
| `IV` | String | AES-GCM nonce, base64 |
| `OwnerTokenHash` | String | SHA-256 of owner token |
| `BurnAfterRead` | Boolean | Delete on first retrieval |
| `TTL` | Number | Unix timestamp — DynamoDB native TTL |
| `CreatedAt` | Number | Unix timestamp |

## Structure

```
privatepaste/
├── app/
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
│   ├── main.tf
│   ├── variables.tf
│   └── outputs.tf
└── .github/workflows/app.yml
```
