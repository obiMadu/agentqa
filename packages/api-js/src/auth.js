import { betterAuth } from "better-auth"
import { expo } from "@better-auth/expo"
import { genericOAuth } from "better-auth/plugins"

export function createAuth(pool, config) {
  const socialProviders = {}
  if (config.googleClientId && config.googleClientSecret) socialProviders.google = { clientId: config.googleClientId, clientSecret: config.googleClientSecret }
  if (config.githubClientId && config.githubClientSecret) socialProviders.github = { clientId: config.githubClientId, clientSecret: config.githubClientSecret }
  const plugins = [expo()]
  if (config.oauthProviderId && config.oauthIssuer && config.oauthClientId) plugins.push(genericOAuth({ config: [{
    providerId: config.oauthProviderId,
    discoveryUrl: `${config.oauthIssuer.replace(/\/$/, "")}/.well-known/openid-configuration`,
    clientId: config.oauthClientId,
    clientSecret: config.oauthClientSecret,
    scopes: ["openid", "profile", "email"],
    pkce: true,
  }] }))
  const trustedProviders = [...Object.keys(socialProviders), ...(config.oauthProviderId ? [config.oauthProviderId] : [])]

  return betterAuth({
    appName: "AgentQA",
    baseURL: config.authURL,
    secret: config.authSecret,
    trustedOrigins: config.authTrustedOrigins,
    database: pool,
    user: {
      modelName: "users",
      fields: { emailVerified: "email_verified", image: "image", createdAt: "created_at", updatedAt: "updated_at" },
    },
    session: {
      modelName: "auth_sessions",
      fields: { userId: "user_id", expiresAt: "expires_at", ipAddress: "ip_address", userAgent: "user_agent", createdAt: "created_at", updatedAt: "updated_at" },
      expiresIn: 60 * 60 * 24 * 30,
    },
    account: {
      modelName: "auth_accounts",
      fields: {
        userId: "user_id", accountId: "account_id", providerId: "provider_id", accessToken: "access_token",
        refreshToken: "refresh_token", idToken: "id_token", accessTokenExpiresAt: "access_token_expires_at",
        refreshTokenExpiresAt: "refresh_token_expires_at", createdAt: "created_at", updatedAt: "updated_at",
      },
      accountLinking: { enabled: true, trustedProviders },
    },
    verification: {
      modelName: "auth_verifications",
      fields: { expiresAt: "expires_at", createdAt: "created_at", updatedAt: "updated_at" },
    },
    advanced: { database: { generateId: "uuid" }, cookiePrefix: "agentqa" },
    emailAndPassword: { enabled: false },
    socialProviders,
    plugins,
  })
}
