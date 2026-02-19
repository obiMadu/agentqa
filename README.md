# Agents QA Core

Open-source core backend + integrations for AgentQA.

## Structure

- `packages/api` - Go backend (REST API)
- `packages/plugins/opencode` - OpenCode plugin

## Quick start

### 1) Go API

Requirements:

- Go 1.24+
- Postgres 14+

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

Notes:

- `API_KEY_ENCRYPTION_KEY` must be a base64-encoded 32-byte value.
- `QUESTION_TTL` controls how long pending questions remain pollable (default `168h`).
- The server listens on `ADDR` (default `:8080`).
- Set `DEV_AUTH=1` to skip Google validation (the API will accept any `id_token` and treat it as the user email).

### 2) OpenCode plugin

```bash
cp packages/plugins/opencode/agentqa-plugin.ts /path/to/your/project/.opencode/plugins/
```

Authenticate via OpenCode:

```bash
opencode auth login
```

Choose "Other", enter `agentqa`, then paste the API key from Settings > API settings.
