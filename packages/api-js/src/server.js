import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto"

const corsHeaders = {
  "access-control-allow-headers": "Authorization, Content-Type",
  "access-control-allow-methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
}

function applyCors(response, request, trustedOrigins) {
  const origin = request.headers.get("origin")
  if (origin && trustedOrigins.includes(origin)) {
    response.headers.set("access-control-allow-origin", origin)
    response.headers.set("access-control-allow-credentials", "true")
    response.headers.append("vary", "Origin")
  } else response.headers.set("access-control-allow-origin", "*")
  for (const [name, value] of Object.entries(corsHeaders)) response.headers.set(name, value)
  return response
}

class HttpError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

const fail = (status, message) => { throw new HttpError(status, message) }
const isNotFound = (error) => error?.code === "NOT_FOUND"
const formatTime = (value) => new Date(value).toISOString().replace(".000Z", "Z")
const monthStart = (date) => new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), 1))
const parseStored = (value) => typeof value === "string" || Buffer.isBuffer(value) ? JSON.parse(value.toString()) : value

function json(status, body) {
  return new Response(`${JSON.stringify(body)}\n`, { status, headers: { "content-type": "application/json" } })
}

function parseBearer(header) {
  const match = /^bearer (.+)$/i.exec(header || "")
  return match?.[1].trim() || ""
}

async function decodeJSON(request, allowedKeys) {
  let body
  try {
    const text = (await request.text()).trimStart()
    if (text.startsWith("null")) body = null
    else {
      if (!text.startsWith("{")) throw new Error("expected object")
      let depth = 0
      let quoted = false
      let escaped = false
      let end = -1
      for (let index = 0; index < text.length; index++) {
        const character = text[index]
        if (quoted) {
          if (escaped) escaped = false
          else if (character === "\\") escaped = true
          else if (character === '"') quoted = false
        } else if (character === '"') quoted = true
        else if (character === "{") depth++
        else if (character === "}" && --depth === 0) { end = index + 1; break }
      }
      if (end < 0) throw new Error("incomplete JSON")
      body = JSON.parse(text.slice(0, end))
    }
  } catch {
    fail(400, "invalid request body")
  }
  if (body === null) body = {}
  if (typeof body !== "object" || Array.isArray(body) || Object.keys(body).some((key) => !allowedKeys.includes(key))) {
    fail(400, "invalid request body")
  }
  return body
}

function validateObject(value, allowedKeys) {
  if (value == null) return {}
  if (typeof value !== "object" || Array.isArray(value) || Object.keys(value).some((key) => !allowedKeys.includes(key))) fail(400, "invalid request body")
  return value
}

function validateStringFields(value, fields) {
  for (const field of fields) if (value[field] != null && typeof value[field] !== "string") fail(400, "invalid request body")
}

function validateUIPreferencesBody(preferences) {
  preferences = validateObject(preferences, ["background", "theme", "questions"])
  const background = validateObject(preferences.background, ["mode", "pattern_id"])
  validateStringFields(background, ["mode", "pattern_id"])
  if (preferences.theme != null) {
    const theme = validateObject(preferences.theme, ["accent", "mode"])
    validateStringFields(theme, ["accent", "mode"])
  }
  if (preferences.questions != null) {
    const questions = validateObject(preferences.questions, ["display_mode"])
    validateStringFields(questions, ["display_mode"])
  }
  return preferences
}

function validateQuestionBody(body) {
  validateStringFields(body, ["request_id", "session_id", "install_id"])
  if (body.tool != null) {
    const tool = validateObject(body.tool, ["messageID", "callID"])
    validateStringFields(tool, ["messageID", "callID"])
  }
  if (body.questions != null && !Array.isArray(body.questions)) fail(400, "invalid request body")
  for (const rawPrompt of body.questions || []) {
    const prompt = validateObject(rawPrompt, ["question", "header", "options", "multiple", "custom"])
    validateStringFields(prompt, ["question", "header"])
    if (prompt.multiple != null && typeof prompt.multiple !== "boolean") fail(400, "invalid request body")
    if (prompt.custom != null && typeof prompt.custom !== "boolean") fail(400, "invalid request body")
    if (prompt.options != null && !Array.isArray(prompt.options)) fail(400, "invalid request body")
    for (const rawOption of prompt.options || []) {
      const option = validateObject(rawOption, ["label", "description"])
      validateStringFields(option, ["label", "description"])
    }
  }
}

function hashAPIKey(value) {
  return createHash("sha256").update(value).digest("hex")
}

function generateAPIKey() {
  const plain = `aq_${randomBytes(32).toString("base64url")}`
  return { plain, keyHash: hashAPIKey(plain), keyPrefix: plain.slice(0, 8) }
}

function encryptAPIKey(plain, key) {
  const nonce = randomBytes(12)
  const cipher = createCipheriv("aes-256-gcm", key, nonce)
  const ciphertext = Buffer.concat([cipher.update(plain), cipher.final(), cipher.getAuthTag()])
  return { keyCiphertext: ciphertext.toString("base64"), keyNonce: nonce.toString("base64") }
}

function decryptAPIKey(ciphertext, nonce, key) {
  const encrypted = Buffer.from(ciphertext, "base64")
  const tag = encrypted.subarray(-16)
  const decipher = createDecipheriv("aes-256-gcm", key, Buffer.from(nonce, "base64"))
  decipher.setAuthTag(tag)
  return Buffer.concat([decipher.update(encrypted.subarray(0, -16)), decipher.final()]).toString()
}

function defaultPreferences() {
  return {
    background: { mode: "shuffle" },
    theme: { accent: "green", mode: "system" },
    questions: { display_mode: "stacked" },
  }
}

function applyPreferenceDefaults(input) {
  const data = structuredClone(input || {})
  let changed = false
  data.background ||= {}
  if (!(data.background.mode || "").trim()) { data.background.mode = "shuffle"; changed = true }
  if (!data.theme) { data.theme = { accent: "green", mode: "system" }; changed = true }
  if (!(data.theme.accent || "").trim()) { data.theme.accent = "green"; changed = true }
  if (!["system", "light", "dark"].includes((data.theme.mode || "").trim())) { data.theme.mode = "system"; changed = true }
  if (!data.questions) { data.questions = { display_mode: "stacked" }; changed = true }
  if (!["stacked", "pager", "accordion"].includes((data.questions.display_mode || "").trim())) { data.questions.display_mode = "stacked"; changed = true }
  return { data, changed }
}

function validatePreferences(preferences) {
  const background = preferences?.background || {}
  const mode = (background.mode || "").trim()
  if (!mode) fail(400, "background.mode is required")
  if (!["shuffle", "fixed"].includes(mode)) fail(400, "background.mode must be shuffle or fixed")
  if (mode === "fixed" && !(background.pattern_id || "").trim()) fail(400, "background.pattern_id is required when mode is fixed")
  if (!preferences.theme) fail(400, "theme is required")
  const accent = (preferences.theme.accent || "").trim()
  if (!accent) fail(400, "theme.accent is required")
  if (!["green", "blue", "orange", "red", "teal"].includes(accent)) fail(400, "theme.accent is invalid")
  const themeMode = (preferences.theme.mode || "").trim()
  if (!themeMode) fail(400, "theme.mode is required")
  if (!["system", "light", "dark"].includes(themeMode)) fail(400, "theme.mode is invalid")
  const displayMode = (preferences.questions?.display_mode || "").trim()
  if (displayMode && !["stacked", "pager", "accordion"].includes(displayMode)) fail(400, "questions.display_mode is invalid")
}

class WaitHub {
  constructor() { this.waiters = new Map() }

  wait(questionId, signal) {
    return new Promise((resolve) => {
      const timeout = setTimeout(() => finish(null), 50_000)
      const finish = (result) => {
        clearTimeout(timeout)
        signal?.removeEventListener("abort", aborted)
        const waiters = this.waiters.get(questionId)?.filter((waiter) => waiter !== finish) || []
        if (waiters.length) this.waiters.set(questionId, waiters)
        else this.waiters.delete(questionId)
        resolve(result)
      }
      const aborted = () => finish(null)
      signal?.addEventListener("abort", aborted, { once: true })
      this.waiters.set(questionId, [...(this.waiters.get(questionId) || []), finish])
    })
  }

  notify(questionId, result) {
    for (const finish of this.waiters.get(questionId) || []) finish(result)
  }
}

export function createServer({ store, auth: authService, push, config = {}, encryptionKey, now = () => new Date(), waitHub = new WaitHub() } = {}) {
  const cfg = {
    questionTTL: 7 * 24 * 60 * 60 * 1000,
    subscriptionOverridePlan: "",
    subscriptionOverrideUserIds: [],
    superwallWebhookSecret: "",
    ...config,
  }

  async function authenticateUser(request) {
    try {
      const session = await authService.api.getSession({ headers: request.headers })
      if (!session) fail(401, "invalid session")
      return session.user
    } catch (error) {
      if (error instanceof HttpError) throw error
      fail(500, "user auth failed")
    }
  }

  async function authenticateAPIKey(request, missingMessage = "missing api key") {
    const raw = parseBearer(request.headers.get("authorization"))
    if (!raw) fail(401, missingMessage)
    try {
      const key = await store.lookupAPIKey(hashAPIKey(raw))
      store.updateAPIKeyLastUsed(key.id).catch(() => {})
      return { user: { id: key.userId }, key }
    } catch (error) {
      if (isNotFound(error)) fail(401, "invalid api key")
      fail(500, "api key lookup failed")
    }
  }

  async function authenticate(request, mode) {
    if (mode === "user") return { user: await authenticateUser(request) }
    if (mode === "api") return authenticateAPIKey(request)
    const raw = parseBearer(request.headers.get("authorization"))
    return raw.startsWith("aq_") ? authenticateAPIKey(request, "missing access token") : { user: await authenticateUser(request) }
  }

  async function resolvePlan(userId) {
    let entitlements
    try { entitlements = await store.getBillingEntitlements(userId) } catch (error) {
      if (!isNotFound(error)) throw error
      entitlements = { userId, proActive: false, proExpiresAt: null, trialUsedAt: null }
    }
    const overridePlan = (cfg.subscriptionOverridePlan || "").trim().toLowerCase()
    if (overridePlan && !["free", "pro"].includes(overridePlan)) throw new Error("invalid SUBSCRIPTION_OVERRIDE_PLAN")
    const overrideApplies = overridePlan && (!cfg.subscriptionOverrideUserIds?.length || cfg.subscriptionOverrideUserIds.some((id) => id.trim() === userId))
    if (overrideApplies) return { plan: overridePlan, entitlements, override: { plan: overridePlan } }
    const plan = entitlements.proExpiresAt ? (new Date(entitlements.proExpiresAt) > now() ? "pro" : "free") : (entitlements.proActive ? "pro" : "free")
    return { plan, entitlements }
  }

  async function handle(request, route, match, auth) {
    const userId = auth?.user.id
    const id = match?.groups?.id

    switch (route) {
      case "health": return json(200, { ok: true })
      case "me": {
        try {
          const current = await store.getUser(userId)
          return json(200, { id: current.id, email: current.email, name: current.name })
        } catch (error) { if (isNotFound(error)) fail(404, "user not found"); fail(500, "user lookup failed") }
        break
      }
      case "get-key": {
        try {
          const key = await store.getActiveAPIKey(userId)
          return json(200, {
            key_prefix: key.keyPrefix,
            created_at: formatTime(key.createdAt),
            ...(key.lastUsedAt ? { last_used_at: formatTime(key.lastUsedAt) } : {}),
          })
        } catch (error) { if (isNotFound(error)) fail(404, "api key not found"); fail(500, "api key lookup failed") }
        break
      }
      case "raw-key": {
        let key
        try { key = await store.getActiveAPIKey(userId) } catch (error) { if (isNotFound(error)) fail(404, "api key not found"); fail(500, "api key lookup failed") }
        if (!key.keyCiphertext || !key.keyNonce) fail(404, "raw api key not available")
        try { return json(200, { api_key: decryptAPIKey(key.keyCiphertext, key.keyNonce, encryptionKey) }) } catch { fail(500, "api key decrypt failed") }
        break
      }
      case "reset-key": {
        try { await store.revokeActiveAPIKeys(userId) } catch { fail(500, "api key revoke failed") }
        const generated = generateAPIKey()
        let encrypted
        try { encrypted = encryptAPIKey(generated.plain, encryptionKey) } catch { fail(500, "api key encryption failed") }
        try {
          const key = await store.createAPIKey({
            userId, name: "Primary", keyHash: generated.keyHash, keyPrefix: generated.keyPrefix,
            ...encrypted, scopes: ["questions:write", "questions:wait"],
          })
          return json(201, { api_key: generated.plain, key_prefix: key.keyPrefix, created_at: formatTime(key.createdAt) })
        } catch { fail(500, "api key store failed") }
        break
      }
      case "billing-status": {
        let resolved
        try { resolved = await resolvePlan(userId) } catch { fail(500, "plan resolve failed") }
        const periodStart = monthStart(now())
        let usage
        try { usage = await store.getBillingUsageMonthly(userId, periodStart) } catch (error) { if (!isNotFound(error)) fail(500, "usage lookup failed"); usage = { questionRequests: 0 } }
        let activeAgents
        try { activeAgents = await store.countActivePluginInstalls(userId) } catch { fail(500, "active agent count failed") }
        return json(200, {
          plan: resolved.plan,
          pro_active: resolved.plan === "pro",
          ...(resolved.entitlements.proExpiresAt ? { pro_expires_at: formatTime(resolved.entitlements.proExpiresAt) } : {}),
          trial_used: Boolean(resolved.entitlements.trialUsedAt),
          month_period_start: formatTime(periodStart),
          month_limit: resolved.plan === "free" ? 10 : 0,
          month_used: usage.questionRequests,
          agent_limit: resolved.plan === "free" ? 1 : 0,
          active_agents: activeAgents,
          ...(resolved.override ? { override: resolved.override } : {}),
        })
      }
      case "get-preferences": {
        let preferences
        try { preferences = await store.getUIPreferences(userId) } catch (error) {
          if (!isNotFound(error)) fail(500, "ui preferences lookup failed")
          try { preferences = await store.upsertUIPreferences(userId, defaultPreferences()) } catch { fail(500, "ui preferences create failed") }
        }
        const applied = applyPreferenceDefaults(preferences.data)
        if (applied.changed) {
          try { preferences = await store.upsertUIPreferences(userId, applied.data) } catch { fail(500, "ui preferences update failed") }
        }
        return json(200, { preferences: preferences.data, updated_at: formatTime(preferences.updatedAt) })
      }
      case "put-preferences": {
        const body = await decodeJSON(request, ["preferences"])
        body.preferences = validateUIPreferencesBody(body.preferences)
        validatePreferences(body.preferences)
        try {
          const preferences = await store.upsertUIPreferences(userId, body.preferences)
          return json(200, { preferences: preferences.data, updated_at: formatTime(preferences.updatedAt) })
        } catch { fail(500, "ui preferences update failed") }
        break
      }
      case "device": {
        const body = await decodeJSON(request, ["platform", "push_token"])
        validateStringFields(body, ["platform", "push_token"])
        if (!body.platform || !body.push_token) fail(400, "platform and push_token are required")
        try { await store.upsertDevice({ userId, platform: body.platform, pushToken: body.push_token }) } catch { fail(500, "device register failed") }
        return json(200, { registered: true })
      }
      case "register-plugin": {
        const body = await decodeJSON(request, ["install_id", "name"])
        validateStringFields(body, ["install_id", "name"])
        if (!body.install_id) fail(400, "install_id is required")
        const name = (body.name || "").trim() || "OpenCode"
        try { await resolvePlan(userId) } catch { fail(500, "plan resolve failed") }
        try { await store.upsertPluginInstall({ installId: body.install_id, userId, name, keyId: auth.key.id, active: false }) }
        catch (error) { if (error?.code === "FORBIDDEN") fail(403, "install_id already registered"); fail(500, "install link failed") }
        let requested
        try { requested = await store.requestPluginInstallPairing(userId, body.install_id) } catch { fail(500, "pairing request failed") }
        if (requested) push.sendPairingRequested({ userId, installId: body.install_id, installName: name }).catch(() => {})
        return json(200, { linked: true })
      }
      case "list-plugins": {
        let installs
        try { installs = await store.listPluginInstalls(userId) } catch { fail(500, "plugin list failed") }
        return json(200, { installs: installs.map((install) => ({
          install_id: install.installId, name: install.name, active: install.active, paired: install.paired,
          ...(install.pairingRequestedAt ? { pairing_requested_at: formatTime(install.pairingRequestedAt) } : {}),
          ...(install.pairingDeniedAt ? { pairing_denied_at: formatTime(install.pairingDeniedAt) } : {}),
          ...(install.pairedAt ? { paired_at: formatTime(install.pairedAt) } : {}),
          created_at: formatTime(install.createdAt), last_seen_at: install.lastSeenAt ? formatTime(install.lastSeenAt) : null,
        })) })
      }
      case "patch-plugin": {
        const body = await decodeJSON(request, ["active"])
        if (typeof body.active !== "boolean") fail(400, body.active == null ? "active is required" : "invalid request body")
        try {
          if (!body.active) await store.updatePluginInstallActive(userId, id, false)
          else if ((await resolvePlan(userId)).plan === "free") await store.activatePluginInstallExclusive(userId, id)
          else await store.updatePluginInstallActive(userId, id, true)
        } catch (error) { if (isNotFound(error)) fail(404, "install not found"); fail(500, error.message === "invalid SUBSCRIPTION_OVERRIDE_PLAN" ? "plan resolve failed" : "install update failed") }
        return json(200, { updated: true, install_id: id, active: body.active })
      }
      case "delete-plugin": {
        try { await store.deletePluginInstall(userId, id) } catch (error) { if (isNotFound(error)) fail(404, "install not found"); fail(500, "install delete failed") }
        return json(200, { deleted: true, install_id: id })
      }
      case "pair-plugin": {
        try { await store.pairPluginInstall(userId, id) } catch (error) { if (isNotFound(error)) fail(404, "install not found"); fail(500, "install pair failed") }
        try {
          const resolved = await resolvePlan(userId)
          if (resolved.plan === "free") await store.activatePluginInstallExclusive(userId, id)
          else await store.updatePluginInstallActive(userId, id, true)
        } catch {}
        let installName = ""
        try { installName = (await store.getPluginInstall(id)).name } catch {}
        push.sendPairingPaired({ userId, installId: id, installName }).catch(() => {})
        return json(200, { paired: true, install_id: id })
      }
      case "deny-plugin": {
        try { await store.denyPluginInstallPairing(userId, id) } catch (error) { if (isNotFound(error)) fail(404, "install not found"); fail(500, "install deny failed") }
        return json(200, { denied: true, install_id: id })
      }
      case "create-question": {
        const body = await decodeJSON(request, ["request_id", "session_id", "install_id", "questions", "tool"])
        validateQuestionBody(body)
        if (!body.request_id || !Array.isArray(body.questions) || !body.questions.length) fail(400, "request_id and questions are required")
        if (!body.install_id) fail(400, "install_id is required")
        let install
        try { install = await store.getPluginInstall(body.install_id) } catch (error) { if (isNotFound(error)) fail(403, "install_id not registered"); fail(500, "install lookup failed") }
        if (install.userId !== userId) fail(403, "install_id not registered")
        if (!install.active) fail(403, "install disabled")
        if (!install.paired) {
          let requested
          try { requested = await store.requestPluginInstallPairing(userId, body.install_id) } catch { fail(500, "pairing request failed") }
          if (requested) push.sendPairingRequested({ userId, installId: body.install_id, installName: install.name }).catch(() => {})
          fail(428, "pairing required")
        }
        store.upsertPluginInstall({ installId: body.install_id, userId, name: install.name, keyId: install.keyId, active: install.active }).catch(() => {})
        let resolved
        try { resolved = await resolvePlan(userId) } catch { fail(500, "plan resolve failed") }
        const question = {
          id: body.request_id, userId, installId: body.install_id, sessionId: body.session_id || "",
          payload: { questions: body.questions, ...(body.tool ? { tool: body.tool } : {}) }, status: "pending",
        }
        let created
        try { created = await store.createQuestion(question, monthStart(now()), resolved.plan === "free" ? 10 : 0) } catch (error) { if (error?.code === "QUOTA_EXCEEDED") fail(402, "quota exceeded"); fail(500, "question create failed") }
        if (!created) return json(200, { created: false, question_id: body.request_id })
        const first = body.questions[0] || {}
        push.sendQuestion({ userId, questionId: body.request_id, installName: install.name, questionCount: body.questions.length, preview: first.question || first.header || "" }).catch(() => {})
        return json(201, { created: true, question_id: body.request_id })
      }
      case "list-questions": {
        let questions
        try { questions = await store.listQuestions(userId) } catch { fail(500, "question list failed") }
        const names = new Map()
        const items = []
        for (const question of questions) {
          let payload
          try { payload = parseStored(question.payload) } catch { fail(500, "payload decode failed") }
          if (!question.installId) fail(500, "install_id missing for question")
          if (!names.has(question.installId)) {
            try { names.set(question.installId, ((await store.getPluginInstall(question.installId)).name || "").trim() || "OpenCode") }
            catch (error) { if (isNotFound(error)) fail(403, "install_id not registered"); fail(500, "install lookup failed") }
          }
          const prompts = payload.questions || []
          items.push({ id: question.id, status: question.status, session_id: question.sessionId, install_id: question.installId, preview: prompts[0]?.question || prompts[0]?.header || "", question_count: prompts.length, source: names.get(question.installId), created_at: formatTime(question.createdAt) })
        }
        return json(200, { questions: items })
      }
      case "bulk-questions": {
        const body = await decodeJSON(request, ["action", "question_ids"])
        if (body.action != null && typeof body.action !== "string") fail(400, "invalid request body")
        if (body.question_ids != null && (!Array.isArray(body.question_ids) || body.question_ids.some((id) => typeof id !== "string"))) fail(400, "invalid request body")
        const action = typeof body.action === "string" ? body.action.trim().toLowerCase() : ""
        if (!action) fail(400, "action is required")
        if (!Array.isArray(body.question_ids) || !body.question_ids.length) fail(400, "question_ids are required")
        const ids = [...new Set(body.question_ids.map((value) => typeof value === "string" ? value.trim() : "").filter(Boolean))]
        if (!ids.length) fail(400, "question_ids are required")
        if (action === "mark_answered") {
          let updated
          try { updated = await store.markQuestionsAnswered(userId, ids) } catch { fail(500, "bulk answer failed") }
          return json(200, { action, updated, deleted: 0, skipped: ids.length - updated })
        }
        if (action === "delete") {
          let deleted
          try { deleted = await store.deleteQuestions(userId, ids) } catch { fail(500, "bulk delete failed") }
          return json(200, { action, updated: 0, deleted, skipped: ids.length - deleted })
        }
        fail(400, "unsupported action")
        break
      }
      case "get-question": {
        let question
        try { question = await store.getQuestion(userId, id) } catch (error) { if (isNotFound(error)) fail(404, "question not found"); fail(500, "question fetch failed") }
        let payload
        try { payload = parseStored(question.payload) } catch { fail(500, "payload decode failed") }
        const response = { id: question.id, status: question.status, session_id: question.sessionId, install_id: question.installId, questions: payload.questions || [], created_at: formatTime(question.createdAt) }
        if (question.status === "answered") {
          let answer
          try { answer = await store.getAnswer(id) } catch { fail(500, "answer lookup failed") }
          try {
            const answers = parseStored(answer.body)
            if (answers?.length) response.answers = answers
          } catch { fail(500, "answer decode failed") }
        }
        return json(200, response)
      }
      case "answer-question": {
        const body = await decodeJSON(request, ["answers", "source"])
        if (body.source != null && typeof body.source !== "string") fail(400, "invalid request body")
        if (body.answers != null && (!Array.isArray(body.answers) || body.answers.some((answer) => !Array.isArray(answer) || answer.some((value) => typeof value !== "string")))) fail(400, "invalid request body")
        if (!Array.isArray(body.answers) || !body.answers.length) fail(400, "answers are required")
        try { await store.answerQuestion(id, userId, body.answers) } catch (error) { if (isNotFound(error)) fail(404, "question not found"); if (error?.code === "ALREADY_RESOLVED") fail(409, "question already resolved"); fail(500, "answer failed") }
        waitHub.notify(id, { status: "answered", answers: body.answers })
        return json(200, { answered: true, question_id: id })
      }
      case "reject-question": {
        try { await store.rejectQuestion(id, userId || null) } catch (error) { if (isNotFound(error)) fail(404, "question not found"); if (error?.code === "ALREADY_RESOLVED") fail(409, "question already resolved"); fail(500, "reject failed") }
        waitHub.notify(id, { status: "rejected" })
        return json(200, { rejected: true, question_id: id })
      }
      case "wait-question": {
        const installId = new URL(request.url).searchParams.get("install_id") || ""
        if (!installId) fail(400, "install_id is required")
        let install
        try { install = await store.getPluginInstall(installId) } catch (error) { if (isNotFound(error)) fail(403, "install_id not registered"); fail(500, "install lookup failed") }
        if (install.userId !== userId) fail(403, "install_id not registered")
        let question
        try { question = await store.getQuestion(userId, id) } catch (error) { if (isNotFound(error)) fail(404, "question not found"); fail(500, "question fetch failed") }
        if (question.status === "answered") {
          let answer
          try { answer = await store.getAnswer(id) } catch { fail(500, "answer lookup failed") }
          try {
            const answers = parseStored(answer.body)
            return json(200, { status: "answered", ...(answers?.length ? { answers } : {}), question_id: id })
          } catch { fail(500, "answer decode failed") }
        }
        if (question.status === "rejected") return json(200, { status: "rejected", question_id: id })
        if (cfg.questionTTL > 0 && now() - new Date(question.createdAt) > cfg.questionTTL) return new Response(null, { status: 410 })
        const result = await waitHub.wait(id, request.signal)
        return result ? json(200, { status: result.status, ...(result.answers?.length ? { answers: result.answers } : {}), question_id: id }) : new Response(null, { status: 204 })
      }
      case "webhook": return handleWebhook(request)
      default: return new Response("404 page not found\n", { status: 404 })
    }
  }

  async function handleWebhook(request) {
    const secret = (cfg.superwallWebhookSecret || "").trim()
    if (!secret) fail(500, "webhook not configured")
    const body = await request.text()
    if (!verifySvix(secret, request.headers, body, now())) fail(400, "invalid webhook signature")
    let event
    try { event = JSON.parse(body) } catch { fail(400, "invalid webhook payload") }
    const data = event?.data || {}
    if (typeof data !== "object" || Array.isArray(data)) fail(400, "invalid webhook payload")
    for (const field of ["id", "originalAppUserId", "expirationAt", "periodType", "name"]) {
      if (data[field] != null && typeof data[field] !== "string") fail(400, "invalid webhook payload")
    }
    if (data.userAttributes != null) {
      if (typeof data.userAttributes !== "object" || Array.isArray(data.userAttributes)) fail(400, "invalid webhook payload")
      if (data.userAttributes.agentqa_user_id != null && typeof data.userAttributes.agentqa_user_id !== "string") fail(400, "invalid webhook payload")
    }
    const eventId = (data.id || "").trim()
    if (!eventId) fail(400, "missing event id")
    const userId = (String(data.originalAppUserId || "").trim().replace(/^\$SuperwallAlias:/, "").trim() || String(data.userAttributes?.agentqa_user_id || "").trim())
    if (!userId) fail(400, "missing user id")
    let expiration = null
    let updatePro = false
    let proActive = false
    if (data.expirationAt != null && String(data.expirationAt).trim()) {
      const value = String(data.expirationAt).trim()
      if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value)) fail(400, "invalid expirationAt")
      expiration = new Date(value)
      if (Number.isNaN(expiration.getTime())) fail(400, "invalid expirationAt")
      updatePro = true
      proActive = expiration > now()
    }
    const markTrial = String(data.periodType || "").trim().toUpperCase() === "TRIAL" && String(data.name || "").trim() === "initial_purchase"
    if (updatePro || markTrial) {
      if (!updatePro) {
        try {
          const existing = await store.getBillingEntitlements(userId)
          proActive = existing.proActive
          expiration = existing.proExpiresAt
        } catch (error) { if (!isNotFound(error)) fail(500, "entitlements lookup failed") }
      }
      try { await store.upsertBillingEntitlements(userId, proActive, expiration, markTrial) }
      catch { fail(500, "entitlements update failed") }
    }
    let inserted
    try { inserted = await store.storeSuperwallWebhookEvent(eventId, body) }
    catch { fail(500, "webhook event store failed") }
    if (!inserted) return new Response(null, { status: 200 })
    let resolved
    try { resolved = await resolvePlan(userId) } catch { fail(500, "plan resolve failed") }
    if (resolved.plan === "free") {
      try { await store.enforceSingleActivePluginInstall(userId) }
      catch { fail(500, "agent limit enforcement failed") }
    }
    return new Response(null, { status: 200 })
  }

  const routes = [
    ["GET", /^\/healthz$/, null, "health"],
    ["POST", /^\/billing\/superwall\/webhook$/, null, "webhook"],
    ["GET", /^\/me$/, "user", "me"],
    ["GET", /^\/api-key$/, "user", "get-key"],
    ["GET", /^\/api-key\/raw$/, "user", "raw-key"],
    ["POST", /^\/api-key\/reset$/, "user", "reset-key"],
    ["GET", /^\/billing\/status$/, "user", "billing-status"],
    ["GET", /^\/ui-preferences$/, "user", "get-preferences"],
    ["PUT", /^\/ui-preferences$/, "user", "put-preferences"],
    ["POST", /^\/devices$/, "user", "device"],
    ["GET", /^\/plugins$/, "user", "list-plugins"],
    ["PATCH", /^\/plugins\/(?<id>[^/]+)$/, "user", "patch-plugin"],
    ["DELETE", /^\/plugins\/(?<id>[^/]+)$/, "user", "delete-plugin"],
    ["POST", /^\/plugins\/(?<id>[^/]+)\/pair$/, "user", "pair-plugin"],
    ["POST", /^\/plugins\/(?<id>[^/]+)\/deny$/, "user", "deny-plugin"],
    ["GET", /^\/questions$/, "user", "list-questions"],
    ["POST", /^\/questions\/bulk$/, "user", "bulk-questions"],
    ["GET", /^\/questions\/(?<id>[^/]+)$/, "user", "get-question"],
    ["POST", /^\/plugin\/register$/, "api", "register-plugin"],
    ["POST", /^\/questions$/, "api", "create-question"],
    ["GET", /^\/questions\/(?<id>[^/]+)\/wait$/, "api", "wait-question"],
    ["POST", /^\/questions\/(?<id>[^/]+)\/answers$/, "mixed", "answer-question"],
    ["POST", /^\/questions\/(?<id>[^/]+)\/reject$/, "mixed", "reject-question"],
  ]

  return {
    async fetch(request) {
      let response
      try {
        const pathname = new URL(request.url).pathname
        if (pathname === "/api/auth" || pathname.startsWith("/api/auth/")) {
          response = request.method === "OPTIONS" ? new Response(null, { status: 204 }) : await authService.handler(request)
          return applyCors(response, request, cfg.authTrustedOrigins || [])
        }
        if (request.method === "OPTIONS") response = new Response(null, { status: 204 })
        else {
          const found = routes.map((route) => ({ route, match: route[1].exec(pathname) })).find(({ route, match }) => route[0] === request.method && match)
          if (!found) {
            const pathExists = routes.some((route) => route[1].test(pathname))
            response = pathExists ? new Response("Method Not Allowed\n", { status: 405 }) : new Response("404 page not found\n", { status: 404 })
          }
          else {
            const auth = found.route[2] ? await authenticate(request, found.route[2]) : null
            response = await handle(request, found.route[3], found.match, auth)
          }
        }
      } catch (error) {
        response = error instanceof HttpError ? json(error.status, { error: error.message }) : json(500, { error: "internal server error" })
      }
      return applyCors(response, request, cfg.authTrustedOrigins || [])
    },
  }
}

function verifySvix(secret, headers, body, currentTime) {
  const id = (headers.get("svix-id") || "").trim()
  const timestamp = (headers.get("svix-timestamp") || "").trim()
  const signatures = (headers.get("svix-signature") || "").trim()
  if (!id || !timestamp || !signatures || !/^\d+$/.test(timestamp)) return false
  if (Math.abs(Math.floor(currentTime.getTime() / 1000) - Number(timestamp)) > 300) return false
  if (!secret.startsWith("whsec_")) return false
  let key
  try { key = Buffer.from(secret.slice(6), "base64") } catch { return false }
  const expected = createHmac("sha256", key).update(`${id}.${timestamp}.${body}`).digest()
  return signatures.split(/\s+/).some((part) => {
    if (!part.startsWith("v1,")) return false
    try {
      const candidate = Buffer.from(part.slice(3), "base64")
      return candidate.length === expected.length && timingSafeEqual(candidate, expected)
    } catch { return false }
  })
}

export { WaitHub, decryptAPIKey, encryptAPIKey, generateAPIKey, hashAPIKey }
