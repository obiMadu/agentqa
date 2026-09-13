import assert from "node:assert/strict"
import { createHmac } from "node:crypto"
import test from "node:test"

import { createServer } from "../src/server.js"

const now = new Date("2026-07-17T12:00:00Z")
const user = { id: "user-1", email: "obi@example.com", name: "Obi" }
const apiKey = { id: "key-1", userId: user.id, keyPrefix: "aq_12345", createdAt: now }
const notFound = () => Object.assign(new Error("not found"), { code: "NOT_FOUND" })

function fixture(overrides = {}) {
  const store = {
    getUser: async () => user,
    lookupAPIKey: async () => apiKey,
    updateAPIKeyLastUsed: async () => {},
    getActiveAPIKey: async () => { throw notFound() },
    revokeActiveAPIKeys: async () => {},
    createAPIKey: async (record) => ({ ...record, id: apiKey.id, createdAt: now }),
    getBillingEntitlements: async () => { throw notFound() },
    getBillingUsageMonthly: async () => { throw notFound() },
    countActivePluginInstalls: async () => 0,
    getUIPreferences: async () => { throw notFound() },
    upsertUIPreferences: async (userId, data) => ({ userId, data, updatedAt: now }),
    upsertDevice: async () => {},
    listPluginInstalls: async () => [],
    getPluginInstall: async () => { throw notFound() },
    upsertPluginInstall: async () => {},
    requestPluginInstallPairing: async () => false,
    updatePluginInstallActive: async () => {},
    activatePluginInstallExclusive: async () => {},
    deletePluginInstall: async () => {},
    pairPluginInstall: async () => {},
    denyPluginInstallPairing: async () => {},
    enforceSingleActivePluginInstall: async () => "",
    createQuestion: async () => true,
    listQuestions: async () => [],
    getQuestion: async () => { throw notFound() },
    getAnswer: async () => { throw notFound() },
    answerQuestion: async () => ({}),
    rejectQuestion: async () => {},
    markQuestionsAnswered: async () => 0,
    deleteQuestions: async () => 0,
    storeSuperwallWebhookEvent: async () => true,
    upsertBillingEntitlements: async () => ({}),
    ...overrides.store,
  }
  const auth = {
    api: { getSession: async ({ headers }) => headers.get("cookie") ? { user, session: { id: "session-1" } } : null },
    handler: async () => new Response(null, { status: 404 }),
    ...overrides.auth,
  }
  if (overrides.auth?.api) auth.api = { ...auth.api, ...overrides.auth.api }
  const push = {
    sendQuestion: async () => {},
    sendPairingRequested: async () => {},
    sendPairingPaired: async () => {},
    ...overrides.push,
  }
  const server = createServer({
    store,
    auth,
    push,
    now: () => now,
    encryptionKey: Buffer.alloc(32, 7),
    config: { questionTTL: 7 * 24 * 60 * 60 * 1000, ...overrides.config },
  })
  return { server, store, auth, push }
}

async function request(server, path, { method = "GET", auth = "user-token", body, headers } = {}) {
  return server.fetch(new Request(`http://localhost${path}`, {
    method,
    headers: {
      ...(auth === null ? {} : auth === "user-token" ? { cookie: `agentqa.session_token=${auth}` } : { authorization: `Bearer ${auth}` }),
      ...(body === undefined ? {} : { "content-type": "application/json" }),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  }))
}

async function expectJSON(response, status, body) {
  assert.equal(response.status, status)
  assert.equal(response.headers.get("content-type"), "application/json")
  assert.deepEqual(await response.json(), body)
}

test("serves health, CORS, preflight, and the Go-compatible fallback", async () => {
  const { server } = fixture()
  const health = await request(server, "/healthz", { auth: null })
  await expectJSON(health, 200, { ok: true })
  assert.equal(health.headers.get("access-control-allow-origin"), "*")

  const options = await request(server, "/unknown", { method: "OPTIONS", auth: null })
  assert.equal(options.status, 204)
  assert.equal(options.headers.get("access-control-allow-headers"), "Authorization, Content-Type")

  const missing = await request(server, "/unknown", { auth: null })
  assert.equal(missing.status, 404)
  assert.equal(await missing.text(), "404 page not found\n")

  const wrongMethod = await request(server, "/healthz", { method: "POST", auth: null })
  assert.equal(wrongMethod.status, 405)
  assert.equal(await wrongMethod.text(), "Method Not Allowed\n")
})

test("enforces each route's authentication mode", async () => {
  const { server } = fixture()
  await expectJSON(await request(server, "/me", { auth: null }), 401, { error: "invalid session" })
  await expectJSON(await request(server, "/plugin/register", { method: "POST", auth: null, body: {} }), 401, { error: "missing api key" })
  await expectJSON(await request(server, "/questions/id/reject", { method: "POST", auth: null }), 401, { error: "invalid session" })
  const tabBearer = await server.fetch(new Request("http://localhost/me", { headers: { authorization: "Bearer\tuser-token" } }))
  await expectJSON(tabBearer, 401, { error: "invalid session" })

  const invalidKey = fixture({ store: { lookupAPIKey: async () => { throw notFound() } } })
  await expectJSON(await request(invalidKey.server, "/plugin/register", { method: "POST", auth: "not-a-key", body: {} }), 401, { error: "invalid api key" })
})

test("applies credentialed CORS only to trusted origins", async () => {
  const trusted = fixture({
    auth: { handler: async () => new Response("auth") },
    config: { authTrustedOrigins: ["agentqa://"] },
  })
  const trustedResponse = await trusted.server.fetch(new Request("http://localhost/api/auth/session", { headers: { origin: "agentqa://" } }))
  assert.equal(await trustedResponse.text(), "auth")
  assert.equal(trustedResponse.headers.get("access-control-allow-origin"), "agentqa://")
  assert.equal(trustedResponse.headers.get("access-control-allow-credentials"), "true")

  const untrustedResponse = await trusted.server.fetch(new Request("http://localhost/api/auth/session", { headers: { origin: "https://evil.example" } }))
  assert.equal(untrustedResponse.headers.get("access-control-allow-origin"), "*")
  assert.equal(untrustedResponse.headers.get("access-control-allow-credentials"), null)
})

test("returns the Better Auth user and rejects invalid sessions", async () => {
  const { server } = fixture()
  await expectJSON(await request(server, "/me"), 200, user)

  const invalid = fixture({ auth: { api: { getSession: async () => null } } })
  await expectJSON(await request(invalid.server, "/me"), 401, { error: "invalid session" })
})

test("reads, decrypts, and resets API keys", async () => {
  let activeKey = { ...apiKey, lastUsedAt: null }
  const { server } = fixture({ store: {
    getActiveAPIKey: async () => activeKey,
    createAPIKey: async (record) => { activeKey = { ...record, id: apiKey.id, createdAt: now }; return activeKey },
  } })
  await expectJSON(await request(server, "/api-key"), 200, { key_prefix: "aq_12345", created_at: now.toISOString().replace(".000", "") })

  const reset = await request(server, "/api-key/reset", { method: "POST" })
  assert.equal(reset.status, 201)
  const resetBody = await reset.json()
  assert.match(resetBody.api_key, /^aq_[A-Za-z0-9_-]{43}$/)
  assert.equal(resetBody.key_prefix, resetBody.api_key.slice(0, 8))

  assert.equal("_ciphertext" in resetBody, false)
  await expectJSON(await request(server, "/api-key/raw"), 200, { api_key: resetBody.api_key })
})

test("returns raw API-key availability errors", async () => {
  const absent = fixture()
  await expectJSON(await request(absent.server, "/api-key/raw"), 404, { error: "api key not found" })

  const legacy = fixture({ store: { getActiveAPIKey: async () => apiKey } })
  await expectJSON(await request(legacy.server, "/api-key/raw"), 404, { error: "raw api key not available" })
})

test("creates and validates UI preferences", async () => {
  const { server } = fixture()
  const defaults = {
    background: { mode: "shuffle" },
    theme: { accent: "green", mode: "system" },
    questions: { display_mode: "stacked" },
  }
  await expectJSON(await request(server, "/ui-preferences"), 200, {
    preferences: defaults,
    updated_at: now.toISOString().replace(".000", ""),
  })

  await expectJSON(await request(server, "/ui-preferences", { method: "PUT", body: { preferences: { background: { mode: "fixed" }, theme: { accent: "green", mode: "dark" } } } }), 400, {
    error: "background.pattern_id is required when mode is fixed",
  })
  await expectJSON(await request(server, "/ui-preferences", { method: "PUT", body: { preferences: { background: { mode: "shuffle" }, theme: { accent: "blue", mode: "dark" }, questions: { display_mode: "pager" } } } }), 200, {
    preferences: { background: { mode: "shuffle" }, theme: { accent: "blue", mode: "dark" }, questions: { display_mode: "pager" } },
    updated_at: now.toISOString().replace(".000", ""),
  })
})

test("registers devices with Go-compatible validation", async () => {
  let saved
  const { server } = fixture({ store: { upsertDevice: async (value) => { saved = value } } })
  await expectJSON(await request(server, "/devices", { method: "POST", body: { platform: "ios", push_token: "ExpoPushToken[x]" } }), 200, { registered: true })
  assert.deepEqual(saved, { userId: user.id, platform: "ios", pushToken: "ExpoPushToken[x]" })
  await expectJSON(await request(server, "/devices", { method: "POST", body: {} }), 400, { error: "platform and push_token are required" })
})

test("registers a plugin through API-key auth and requests pairing once", async () => {
  let install
  let pairingPush
  const { server } = fixture({
    store: {
      upsertPluginInstall: async (value) => { install = value },
      requestPluginInstallPairing: async () => true,
    },
    push: { sendPairingRequested: async (value) => { pairingPush = value } },
  })
  await expectJSON(await request(server, "/plugin/register", { method: "POST", auth: "any-api-key", body: { install_id: "install-1", name: "  " } }), 200, { linked: true })
  assert.equal(install.name, "OpenCode")
  assert.equal(install.active, false)
  assert.deepEqual(pairingPush, { userId: user.id, installId: "install-1", installName: "OpenCode" })
})

test("lists and manages plugin installs", async () => {
  const install = { installId: "install-1", name: "Agent", active: false, paired: false, createdAt: now, lastSeenAt: null }
  let active
  const { server } = fixture({ store: {
    listPluginInstalls: async () => [install],
    updatePluginInstallActive: async (_userId, _installId, value) => { active = value },
  } })
  await expectJSON(await request(server, "/plugins"), 200, { installs: [{
    install_id: "install-1", name: "Agent", active: false, paired: false,
    created_at: now.toISOString().replace(".000", ""), last_seen_at: null,
  }] })
  await expectJSON(await request(server, "/plugins/install-1", { method: "PATCH", body: { active: false } }), 200, { updated: true, install_id: "install-1", active: false })
  assert.equal(active, false)
  await expectJSON(await request(server, "/plugins/install-1", { method: "DELETE" }), 200, { deleted: true, install_id: "install-1" })
})

test("free activation is exclusive while pro activation is not", async () => {
  let operation
  const free = fixture({ store: { activatePluginInstallExclusive: async () => { operation = "exclusive" } } })
  await request(free.server, "/plugins/install-1", { method: "PATCH", body: { active: true } })
  assert.equal(operation, "exclusive")

  const pro = fixture({
    store: {
      getBillingEntitlements: async () => ({ proActive: true, proExpiresAt: null, trialUsedAt: null }),
      updatePluginInstallActive: async () => { operation = "ordinary" },
    },
  })
  await request(pro.server, "/plugins/install-1", { method: "PATCH", body: { active: true } })
  assert.equal(operation, "ordinary")
})

test("pairs and denies plugin installs", async () => {
  const { server } = fixture({ store: { getPluginInstall: async () => ({ name: "Agent" }) } })
  await expectJSON(await request(server, "/plugins/install-1/pair", { method: "POST" }), 200, { paired: true, install_id: "install-1" })
  await expectJSON(await request(server, "/plugins/install-1/deny", { method: "POST" }), 200, { denied: true, install_id: "install-1" })
})

test("creates idempotent questions after install, pairing, and quota checks", async () => {
  let question
  let monthlyLimit
  const { server } = fixture({ store: {
    getPluginInstall: async () => ({ installId: "install-1", userId: user.id, name: "Agent", active: true, paired: true }),
    createQuestion: async (value, _periodStart, limit) => { question = value; monthlyLimit = limit; return true },
  } })
  const body = { request_id: "question-1", session_id: "session", install_id: "install-1", questions: [{ question: "Choose", options: [{ label: "A" }] }] }
  await expectJSON(await request(server, "/questions", { method: "POST", auth: "api-key", body }), 201, { created: true, question_id: "question-1" })
  assert.equal(question.status, "pending")
  assert.equal(monthlyLimit, 10)

  const duplicate = fixture({ store: {
    getPluginInstall: async () => ({ userId: user.id, active: true, paired: true }),
    createQuestion: async () => false,
  } })
  await expectJSON(await request(duplicate.server, "/questions", { method: "POST", auth: "api-key", body }), 200, { created: false, question_id: "question-1" })
})

test("rejects question creation for disabled, unpaired, foreign, and over-quota installs", async () => {
  const body = { request_id: "question-1", install_id: "install-1", questions: [{}] }
  const disabled = fixture({ store: { getPluginInstall: async () => ({ userId: user.id, active: false, paired: false }) } })
  await expectJSON(await request(disabled.server, "/questions", { method: "POST", auth: "key", body }), 403, { error: "install disabled" })
  const unpaired = fixture({ store: { getPluginInstall: async () => ({ userId: user.id, active: true, paired: false }), requestPluginInstallPairing: async () => false } })
  await expectJSON(await request(unpaired.server, "/questions", { method: "POST", auth: "key", body }), 428, { error: "pairing required" })
  const foreign = fixture({ store: { getPluginInstall: async () => ({ userId: "other", active: true, paired: true }) } })
  await expectJSON(await request(foreign.server, "/questions", { method: "POST", auth: "key", body }), 403, { error: "install_id not registered" })
  const quota = fixture({ store: {
    getPluginInstall: async () => ({ userId: user.id, active: true, paired: true }),
    createQuestion: async () => { throw Object.assign(new Error("quota"), { code: "QUOTA_EXCEEDED" }) },
  } })
  await expectJSON(await request(quota.server, "/questions", { method: "POST", auth: "key", body }), 402, { error: "quota exceeded" })
})

test("lists and fetches questions with previews, source, and answers", async () => {
  const question = {
    id: "question-1", userId: user.id, installId: "install-1", sessionId: "session", status: "answered", createdAt: now,
    payload: { questions: [{ question: "", header: "Header" }, { question: "Second" }] },
  }
  const { server } = fixture({ store: {
    listQuestions: async () => [question],
    getPluginInstall: async () => ({ name: "Agent" }),
    getQuestion: async () => question,
    getAnswer: async () => ({ body: [["A"], ["B"]] }),
  } })
  await expectJSON(await request(server, "/questions"), 200, { questions: [{
    id: "question-1", status: "answered", session_id: "session", install_id: "install-1", preview: "Header", question_count: 2, source: "Agent", created_at: now.toISOString().replace(".000", ""),
  }] })
  await expectJSON(await request(server, "/questions/question-1"), 200, {
    id: "question-1", status: "answered", session_id: "session", install_id: "install-1", questions: question.payload.questions, answers: [["A"], ["B"]], created_at: now.toISOString().replace(".000", ""),
  })
})

test("bulk marks or deletes trimmed, deduplicated question IDs", async () => {
  let ids
  const { server } = fixture({ store: {
    markQuestionsAnswered: async (_userId, values) => { ids = values; return 1 },
    deleteQuestions: async (_userId, values) => { ids = values; return 2 },
  } })
  const question_ids = [" one ", "one", "two", ""]
  await expectJSON(await request(server, "/questions/bulk", { method: "POST", body: { action: " MARK_ANSWERED ", question_ids } }), 200, { action: "mark_answered", updated: 1, deleted: 0, skipped: 1 })
  assert.deepEqual(ids, ["one", "two"])
  await expectJSON(await request(server, "/questions/bulk", { method: "POST", body: { action: "delete", question_ids } }), 200, { action: "delete", updated: 0, deleted: 2, skipped: 0 })
})

test("answers and rejects pending questions through either auth mode", async () => {
  let answers
  const { server } = fixture({ store: { answerQuestion: async (_id, _userId, value) => { answers = value } } })
  await expectJSON(await request(server, "/questions/question-1/answers", { method: "POST", auth: "aq_key", body: { answers: [["A"]], source: "mobile" } }), 200, { answered: true, question_id: "question-1" })
  assert.deepEqual(answers, [["A"]])
  await expectJSON(await request(server, "/questions/question-2/reject", { method: "POST" }), 200, { rejected: true, question_id: "question-2" })
})

test("wait returns resolved questions, expiry, and notifications", async () => {
  let question = { id: "question-1", status: "answered", createdAt: now }
  const { server } = fixture({ store: {
    getPluginInstall: async () => ({ userId: user.id }),
    getQuestion: async () => question,
    getAnswer: async () => ({ body: [["A"]] }),
  } })
  await expectJSON(await request(server, "/questions/question-1/wait?install_id=install-1", { auth: "key" }), 200, { status: "answered", answers: [["A"]], question_id: "question-1" })
  question = { ...question, status: "pending", createdAt: new Date("2026-07-01T00:00:00Z") }
  assert.equal((await request(server, "/questions/question-1/wait?install_id=install-1", { auth: "key" })).status, 410)
})

test("wait omits empty answers and receives in-process resolutions", async () => {
  const question = { id: "question-1", status: "answered", createdAt: now }
  const { server } = fixture({ store: {
    getPluginInstall: async () => ({ userId: user.id }),
    getQuestion: async () => question,
    getAnswer: async () => ({ body: [] }),
  } })
  await expectJSON(await request(server, "/questions/question-1/wait?install_id=install-1", { auth: "key" }), 200, { status: "answered", question_id: "question-1" })

  question.status = "pending"
  const waiting = request(server, "/questions/question-1/wait?install_id=install-1", { auth: "key" })
  await new Promise((resolve) => setTimeout(resolve, 0))
  await request(server, "/questions/question-1/answers", { method: "POST", auth: "aq_key", body: { answers: [["A"]] } })
  await expectJSON(await waiting, 200, { status: "answered", answers: [["A"]], question_id: "question-1" })
})

test("reports free and pro billing status", async () => {
  const free = fixture()
  await expectJSON(await request(free.server, "/billing/status"), 200, {
    plan: "free", pro_active: false, trial_used: false, month_period_start: "2026-07-01T00:00:00Z", month_limit: 10, month_used: 0, agent_limit: 1, active_agents: 0,
  })
  const pro = fixture({ store: {
    getBillingEntitlements: async () => ({ proActive: true, proExpiresAt: null, trialUsedAt: now }),
    getBillingUsageMonthly: async () => ({ questionRequests: 4 }),
    countActivePluginInstalls: async () => 2,
  } })
  await expectJSON(await request(pro.server, "/billing/status"), 200, {
    plan: "pro", pro_active: true, trial_used: true, month_period_start: "2026-07-01T00:00:00Z", month_limit: 0, month_used: 4, agent_limit: 0, active_agents: 2,
  })
})

test("applies scoped subscription overrides", async () => {
  const { server } = fixture({ config: { subscriptionOverridePlan: "pro", subscriptionOverrideUserIds: [user.id] } })
  const response = await request(server, "/billing/status")
  const body = await response.json()
  assert.equal(body.plan, "pro")
  assert.deepEqual(body.override, { plan: "pro" })
})

test("rejects malformed and unknown JSON fields", async () => {
  const { server } = fixture()
  await expectJSON(await request(server, "/devices", { method: "POST", body: { platform: "ios", push_token: "x", extra: true } }), 400, { error: "invalid request body" })
  const malformed = await server.fetch(new Request("http://localhost/devices", { method: "POST", headers: { cookie: "agentqa.session_token=user-token" }, body: "{" }))
  await expectJSON(malformed, 400, { error: "invalid request body" })

  const wrongType = await request(server, "/devices", { method: "POST", body: { platform: 1, push_token: "x" } })
  await expectJSON(wrongType, 400, { error: "invalid request body" })

  const nestedUnknown = await request(server, "/questions", { method: "POST", auth: "key", body: {
    request_id: "question-1", install_id: "install-1", questions: [{ question: "Choose", unexpected: true }],
  } })
  await expectJSON(nestedUnknown, 400, { error: "invalid request body" })

  const trailing = await server.fetch(new Request("http://localhost/devices", {
    method: "POST",
    headers: { cookie: "agentqa.session_token=user-token" },
    body: '{"platform":"ios","push_token":"x"}{"ignored":true}',
  }))
  await expectJSON(trailing, 200, { registered: true })
})

test("verifies and applies Superwall webhook events idempotently", async () => {
  const secretBytes = Buffer.from("webhook-secret")
  const secret = `whsec_${secretBytes.toString("base64")}`
  let entitlement
  let eventId
  const { server } = fixture({
    config: { superwallWebhookSecret: secret },
    store: {
      storeSuperwallWebhookEvent: async (id) => { eventId = id; return true },
      upsertBillingEntitlements: async (...args) => { entitlement = args },
    },
  })
  const payload = JSON.stringify({ data: {
    id: "event-1",
    originalAppUserId: `$SuperwallAlias:${user.id}`,
    expirationAt: "2026-08-01T00:00:00Z",
    periodType: "TRIAL",
    name: "initial_purchase",
  } })
  const timestamp = String(Math.floor(now.getTime() / 1000))
  const signature = createHmac("sha256", secretBytes).update(`message-1.${timestamp}.${payload}`).digest("base64")
  const response = await server.fetch(new Request("http://localhost/billing/superwall/webhook", {
    method: "POST",
    headers: { "svix-id": "message-1", "svix-timestamp": timestamp, "svix-signature": `v1,${signature}` },
    body: payload,
  }))
  assert.equal(response.status, 200)
  assert.deepEqual(entitlement, [user.id, true, new Date("2026-08-01T00:00:00Z"), true])
  assert.equal(eventId, "event-1")

  const invalid = await server.fetch(new Request("http://localhost/billing/superwall/webhook", { method: "POST", body: payload }))
  await expectJSON(invalid, 400, { error: "invalid webhook signature" })
})

test("rejects typed and non-RFC3339 Superwall fields", async () => {
  const secretBytes = Buffer.from("webhook-secret")
  const secret = `whsec_${secretBytes.toString("base64")}`
  const { server } = fixture({ config: { superwallWebhookSecret: secret } })
  const signed = async (data) => {
    const body = JSON.stringify({ data })
    const timestamp = String(Math.floor(now.getTime() / 1000))
    const signature = createHmac("sha256", secretBytes).update(`message.${timestamp}.${body}`).digest("base64")
    return server.fetch(new Request("http://localhost/billing/superwall/webhook", {
      method: "POST", headers: { "svix-id": "message", "svix-timestamp": timestamp, "svix-signature": `v1,${signature}` }, body,
    }))
  }
  await expectJSON(await signed({ id: 1 }), 400, { error: "invalid webhook payload" })
  await expectJSON(await signed({ id: "event", originalAppUserId: user.id, expirationAt: "2026-08-01" }), 400, { error: "invalid expirationAt" })
})
