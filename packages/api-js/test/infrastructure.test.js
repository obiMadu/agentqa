import assert from "node:assert/strict"
import { createServer as createHTTPServer } from "node:http"
import test from "node:test"

test("loads the AgentQA API environment contract", async () => {
  let configModule
  await assert.doesNotReject(async () => { configModule = await import("../src/config.js") })
  const config = configModule.loadConfig({
    ADDR: ":9090",
    DATABASE_URL: "postgres://db",
    DB_CONNECT_RETRIES: "3",
    DB_CONNECT_DELAY: "250ms",
    API_KEY_ENCRYPTION_KEY: Buffer.alloc(32, 1).toString("base64"),
    BETTER_AUTH_URL: "https://api.example.com",
    AUTH_TRUSTED_ORIGINS: "agentqa://, https://app.example.com",
    SUBSCRIPTION_OVERRIDE_USER_IDS: " a, ,b ",
    QUESTION_TTL: "168h",
  })
  assert.equal(config.port, 9090)
  assert.equal(config.dbConnectDelay, 250)
  assert.equal(config.questionTTL, 604_800_000)
  assert.equal(config.authURL, "https://api.example.com")
  assert.deepEqual(config.authTrustedOrigins, ["agentqa://", "https://app.example.com"])
  assert.deepEqual(config.subscriptionOverrideUserIds, ["a", "b"])
  assert.equal(configModule.parseEncryptionKey(config.apiKeyEncryptionKey).length, 32)
  assert.throws(() => configModule.parseEncryptionKey("bad"), /base64-encoded 32 bytes/)

  const edgeCases = configModule.loadConfig({ DB_CONNECT_RETRIES: "3x", QUESTION_TTL: "0" })
  assert.equal(edgeCases.dbConnectRetries, 12)
  assert.equal(edgeCases.questionTTL, 0)
})

test("sends Expo pushes in batches with pending badges", async () => {
  let pushModule
  await assert.doesNotReject(async () => { pushModule = await import("../src/push.js") })
  const batches = []
  const sender = pushModule.createPushSender({
    endpoint: "https://expo.test/send",
    store: {
      listDevices: async () => [
        ...Array.from({ length: 101 }, (_, index) => ({ pushToken: `ExpoPushToken[${index}]` })),
        { pushToken: "not-expo" },
      ],
      countPendingQuestions: async () => 3,
    },
    fetchImpl: async (_url, options) => { batches.push(JSON.parse(options.body)); return new Response(JSON.stringify({ data: [] }), { status: 200 }) },
  })
  await sender.sendQuestion({ userId: "user-1", questionId: "question-1", installName: "Agent", questionCount: 2, preview: "Choose" })
  assert.deepEqual(batches.map((batch) => batch.length), [100, 1])
  assert.deepEqual(batches[0][0], {
    to: "ExpoPushToken[0]", title: "2 new questions from Agent", body: "Choose", badge: 3, data: { question_id: "question-1" },
  })
})

test("adapts Fetch requests and responses to Node HTTP", async (context) => {
  let indexModule
  await assert.doesNotReject(async () => { indexModule = await import("../src/index.js") })
  const listener = indexModule.createNodeListener({
    fetch: async (request) => new Response(`${request.method} ${new URL(request.url).pathname}`, { status: 201, headers: { "x-test": "yes" } }),
  })
  await new Promise((resolve) => listener.listen(0, "127.0.0.1", resolve))
  context.after(() => listener.close())
  const response = await fetch(`http://127.0.0.1:${listener.address().port}/healthz`, { method: "POST" })
  assert.equal(response.status, 201)
  assert.equal(response.headers.get("x-test"), "yes")
  assert.equal(await response.text(), "POST /healthz")
})
