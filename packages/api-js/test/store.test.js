import assert from "node:assert/strict"
import { Pool } from "pg"
import test from "node:test"

const databaseURL = process.env.TEST_DATABASE_URL

test("Postgres store preserves the Go API persistence contract", { skip: !databaseURL }, async () => {
  let storeModule
  await assert.doesNotReject(async () => { storeModule = await import("../src/store.js") })
  const pool = new Pool({ connectionString: databaseURL })
  try {
    await storeModule.migrate(pool)
    await pool.query("TRUNCATE superwall_webhook_events, billing_usage_monthly, billing_entitlements, ui_preferences, answers, questions, devices, plugin_installs, api_keys, users CASCADE")
    const store = new storeModule.PostgresStore(pool)

    const createdUser = await store.upsertUser({ email: "obi@example.com", name: "Obi", authProvider: "oidc", authIssuer: "issuer", authSubject: "subject" })
    assert.equal((await store.getUser(createdUser.id)).email, "obi@example.com")
    assert.equal((await store.getUserByOIDC("issuer", "subject")).id, createdUser.id)
    await store.attachOIDCToUser(createdUser.id, "issuer", "subject")

    const key = await store.createAPIKey({ userId: createdUser.id, name: "Primary", keyHash: "hash", keyPrefix: "aq_hash", keyCiphertext: "ciphertext", keyNonce: "nonce", scopes: ["questions:write"] })
    assert.equal((await store.lookupAPIKey("hash")).id, key.id)
    await store.updateAPIKeyLastUsed(key.id)
    assert.ok((await store.getActiveAPIKey(createdUser.id)).lastUsedAt)

    await store.upsertPluginInstall({ installId: "install-1", userId: createdUser.id, name: "Agent", keyId: key.id, active: false })
    assert.equal(await store.requestPluginInstallPairing(createdUser.id, "install-1"), true)
    assert.equal(await store.requestPluginInstallPairing(createdUser.id, "install-1"), false)
    await store.pairPluginInstall(createdUser.id, "install-1")
    await store.activatePluginInstallExclusive(createdUser.id, "install-1")
    assert.equal(await store.countActivePluginInstalls(createdUser.id), 1)
    assert.equal((await store.listPluginInstalls(createdUser.id))[0].installId, "install-1")

    await store.upsertDevice({ userId: createdUser.id, platform: "ios", pushToken: "ExpoPushToken[x]" })
    assert.equal((await store.listDevices(createdUser.id))[0].platform, "ios")

    const period = new Date("2026-07-01T00:00:00Z")
    const question = { id: "question-1", userId: createdUser.id, installId: "install-1", sessionId: "session", payload: { questions: [{ question: "Choose" }] }, status: "pending" }
    assert.equal(await store.createQuestion(question, period, 1), true)
    assert.equal(await store.createQuestion(question, period, 1), false)
    await assert.rejects(() => store.createQuestion({ ...question, id: "question-2" }, period, 1), { code: "QUOTA_EXCEEDED" })
    assert.equal((await store.getBillingUsageMonthly(createdUser.id, period)).questionRequests, 1)
    assert.equal((await store.listQuestions(createdUser.id))[0].id, "question-1")
    await store.answerQuestion("question-1", createdUser.id, [["A"]])
    assert.deepEqual((await store.getAnswer("question-1")).body, [["A"]])
    await assert.rejects(() => store.answerQuestion("question-1", createdUser.id, [["B"]]), { code: "ALREADY_RESOLVED" })

    await store.createQuestion({ ...question, id: "question-3" }, period, 0)
    await store.rejectQuestion("question-3", createdUser.id)
    assert.equal((await store.getQuestion(createdUser.id, "question-3")).status, "rejected")
    await store.createQuestion({ ...question, id: "question-4" }, period, 0)
    await store.createQuestion({ ...question, id: "question-5" }, period, 0)
    assert.equal(await store.markQuestionsAnswered(createdUser.id, ["question-4"]), 1)
    assert.equal(await store.deleteQuestions(createdUser.id, ["question-4", "question-5"]), 2)

    const preferences = await store.upsertUIPreferences(createdUser.id, { background: { mode: "shuffle" } })
    assert.equal((await store.getUIPreferences(createdUser.id)).id, preferences.id)
    await store.upsertBillingEntitlements(createdUser.id, true, new Date("2026-08-01T00:00:00Z"), true)
    assert.equal((await store.getBillingEntitlements(createdUser.id)).proActive, true)
    assert.equal(await store.storeSuperwallWebhookEvent("event-1", "{}"), true)
    assert.equal(await store.storeSuperwallWebhookEvent("event-1", "{}"), false)

    await store.revokeActiveAPIKeys(createdUser.id)
    await assert.rejects(() => store.lookupAPIKey("hash"), { code: "NOT_FOUND" })
  } finally {
    await pool.end()
  }
})
