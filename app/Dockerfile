# Stage 1: Build
FROM golang:1.26.10-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 ensures a static binary
# -trimpath makes builds reproducible
# -s -w strips debug info to reduce size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o server ./cmd/server

# Stage 2: Run
# Using distroless/static for CA certs, tzdata, and nonroot user
FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

WORKDIR /app
COPY --from=builder /app/server .

# Use numeric UID/GID for Kubernetes runAsNonRoot compatibility (already set by :nonroot, kept here as reminder and to survive image changes)
USER 65532:65532

EXPOSE 8080
CMD ["./server"]
