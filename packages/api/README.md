# AgentQA API (Go)

Go API for AgentQA. AgentQA lets you unblock your agents anywhere, anytime. Never leave your agents waiting again. OpenCode support is available today.

## Requirements

- Go 1.24+
- Postgres 14+

## Local run

```bash
cd packages/api
export DATABASE_URL="postgres://user:pass@localhost:5432/agentqa?sslmode=disable"
export DB_CONNECT_RETRIES=12
export DB_CONNECT_DELAY=2s
export JWT_SECRET="dev-secret"
export API_KEY_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export QUESTION_TTL="168h"
export GOOGLE_CLIENT_ID="your-google-client-id"
go run ./cmd/api
```

`API_KEY_ENCRYPTION_KEY` must be a base64-encoded 32-byte value.

`QUESTION_TTL` controls how long pending questions remain pollable (default `168h`).

The server listens on `ADDR` (default `:8080`).

## Schema

The API uses GORM AutoMigrate on startup. It also ensures `pgcrypto` is available and will
convert `api_keys.scopes` from a legacy `text[]` column to `jsonb` when needed.

## Dev auth

Set `DEV_AUTH=1` to skip Google validation. The API will accept any `id_token` and treat it as the user email.
