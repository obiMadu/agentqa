# AgentQA API (JavaScript)

Complete Node.js port of the adjacent Go API. It uses the same HTTP contract, environment variables, and PostgreSQL schema, so it can run against an existing AgentQA database.

## Requirements

- Node.js 24+
- PostgreSQL 14+

## Run

```bash
npm install
export DATABASE_URL="postgres://agentqa:agentqa@localhost:5432/agentqa?sslmode=disable"
export API_KEY_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export BETTER_AUTH_URL="http://localhost:8080"
export BETTER_AUTH_SECRET="$(openssl rand -base64 32)"
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
export SUPERWALL_WEBHOOK_SECRET="whsec_..."
npm start
```

The server listens on `ADDR` (default `:8080`). See `.env.example` for every supported setting.

Better Auth is mounted at `/api/auth/*`. Superwall webhooks use `/billing/superwall/webhook`.

Configure OAuth callbacks as `${BETTER_AUTH_URL}/api/auth/callback/google` for built-in providers or `${BETTER_AUTH_URL}/api/auth/oauth2/callback/<AUTH_OAUTH_PROVIDER_ID>` for generic OIDC.

For an existing AgentQA database, set `AUTH_LEGACY_PROVIDER_ID` to the Better Auth provider ID replacing the old OIDC issuer. Startup then links the existing OIDC subjects without changing AgentQA user UUIDs.

## Test

Unit and HTTP contract tests:

```bash
npm test
```

Include the PostgreSQL integration contract:

```bash
TEST_DATABASE_URL="postgres://agentqa:agentqa@localhost:5432/agentqa_test" npm test
```

The integration test truncates all AgentQA tables in `TEST_DATABASE_URL`; use a dedicated test database.

## Docker

```bash
docker build -t agentqa-api-js .
docker run --env-file .env -p 8080:8080 agentqa-api-js
```
