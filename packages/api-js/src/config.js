function duration(value, fallback) {
  if (!value) return fallback
  if (value === "0") return 0
  const units = { ns: 1e-6, us: 1e-3, "µs": 1e-3, ms: 1, s: 1000, m: 60_000, h: 3_600_000 }
  let total = 0
  let consumed = ""
  for (const match of value.matchAll(/(-?(?:\d+(?:\.\d*)?|\.\d+))(ns|us|µs|ms|s|m|h)/g)) {
    total += Number(match[1]) * units[match[2]]
    consumed += match[0]
  }
  return consumed === value ? total : fallback
}

const csv = (value = "") => value.split(",").map((part) => part.trim()).filter(Boolean)

export function loadConfig(env = process.env) {
  const addr = env.ADDR || ":8080"
  const retriesValue = env.DB_CONNECT_RETRIES || "12"
  const retries = /^-?\d+$/.test(retriesValue) ? Number(retriesValue) : 12
  return {
    addr,
    port: Number(addr.match(/:(\d+)$/)?.[1] || addr),
    databaseURL: env.DATABASE_URL || "",
    dbConnectRetries: retries,
    dbConnectDelay: duration(env.DB_CONNECT_DELAY, 2000),
    apiKeyEncryptionKey: env.API_KEY_ENCRYPTION_KEY || "",
    superwallWebhookSecret: env.SUPERWALL_WEBHOOK_SECRET || "",
    subscriptionOverridePlan: (env.SUBSCRIPTION_OVERRIDE_PLAN || "").trim(),
    subscriptionOverrideUserIds: csv(env.SUBSCRIPTION_OVERRIDE_USER_IDS),
    expoPushURL: (env.EXPO_PUSH_URL || "").trim(),
    questionTTL: duration(env.QUESTION_TTL, 7 * 24 * 60 * 60 * 1000),
    authURL: env.BETTER_AUTH_URL || "",
    authSecret: env.BETTER_AUTH_SECRET || "",
    authTrustedOrigins: csv(env.AUTH_TRUSTED_ORIGINS || "agentqa://"),
    googleClientId: env.GOOGLE_CLIENT_ID || "",
    googleClientSecret: env.GOOGLE_CLIENT_SECRET || "",
    githubClientId: env.GITHUB_CLIENT_ID || "",
    githubClientSecret: env.GITHUB_CLIENT_SECRET || "",
    oauthProviderId: env.AUTH_OAUTH_PROVIDER_ID || "",
    oauthIssuer: env.AUTH_OAUTH_ISSUER || "",
    oauthClientId: env.AUTH_OAUTH_CLIENT_ID || "",
    oauthClientSecret: env.AUTH_OAUTH_CLIENT_SECRET || "",
    authLegacyProviderId: env.AUTH_LEGACY_PROVIDER_ID || "",
  }
}

export function parseEncryptionKey(value) {
  const decoded = Buffer.from(value, "base64")
  if (!value || decoded.length !== 32 || decoded.toString("base64") !== value) throw new Error("API_KEY_ENCRYPTION_KEY must be base64-encoded 32 bytes")
  return decoded
}
