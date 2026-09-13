const codedError = (code) => Object.assign(new Error(code.toLowerCase().replaceAll("_", " ")), { code })

const user = (row) => ({ id: row.id, email: row.email, name: row.name, authProvider: row.auth_provider, authIssuer: row.auth_issuer, authSubject: row.auth_subject, createdAt: row.created_at, updatedAt: row.updated_at })
const apiKey = (row) => ({ id: row.id, userId: row.user_id, keyHash: row.key_hash, keyCiphertext: row.key_ciphertext, keyNonce: row.key_nonce, keyPrefix: row.key_prefix, name: row.name, scopes: row.scopes, createdAt: row.created_at, revokedAt: row.revoked_at, lastUsedAt: row.last_used_at })
const install = (row) => ({ installId: row.install_id, userId: row.user_id, name: row.name, keyId: row.key_id, active: row.active, paired: row.paired, pairingRequestedAt: row.pairing_requested_at, pairingDeniedAt: row.pairing_denied_at, pairedAt: row.paired_at, createdAt: row.created_at, lastSeenAt: row.last_seen_at })
const question = (row) => ({ id: row.id, userId: row.user_id, installId: row.install_id || "", sessionId: row.session_id, payload: row.payload, status: row.status, createdAt: row.created_at })

async function one(result, map = (row) => row) {
  if (!result.rows.length) throw codedError("NOT_FOUND")
  return map(result.rows[0])
}

export async function migrate(pool, config = {}) {
  await pool.query(`
    CREATE EXTENSION IF NOT EXISTS pgcrypto;
    CREATE TABLE IF NOT EXISTS users (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL UNIQUE, name text NOT NULL DEFAULT '', auth_provider text NOT NULL,
      auth_issuer text, auth_subject text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE UNIQUE INDEX IF NOT EXISTS users_auth_issuer_subject ON users(auth_issuer, auth_subject);
    ALTER TABLE users ALTER COLUMN auth_provider SET DEFAULT 'better-auth';
    ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
    ALTER TABLE users ADD COLUMN IF NOT EXISTS image text;
    UPDATE users SET email_verified=true WHERE auth_subject IS NOT NULL AND email_verified=false;
    CREATE TABLE IF NOT EXISTS auth_sessions (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, token text NOT NULL UNIQUE,
      expires_at timestamptz NOT NULL, ip_address text, user_agent text, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
    );
    CREATE INDEX IF NOT EXISTS auth_sessions_user_id ON auth_sessions(user_id);
    CREATE TABLE IF NOT EXISTS auth_accounts (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, account_id text NOT NULL, provider_id text NOT NULL,
      access_token text, refresh_token text, id_token text, access_token_expires_at timestamptz, refresh_token_expires_at timestamptz,
      scope text, password text, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
    );
    CREATE INDEX IF NOT EXISTS auth_accounts_user_id ON auth_accounts(user_id);
    CREATE UNIQUE INDEX IF NOT EXISTS auth_accounts_provider_account ON auth_accounts(provider_id,account_id);
    CREATE TABLE IF NOT EXISTS auth_verifications (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), identifier text NOT NULL, value text NOT NULL, expires_at timestamptz NOT NULL,
      created_at timestamptz, updated_at timestamptz
    );
    CREATE INDEX IF NOT EXISTS auth_verifications_identifier ON auth_verifications(identifier);
    ALTER TABLE auth_sessions ALTER COLUMN id SET DEFAULT gen_random_uuid();
    ALTER TABLE auth_accounts ALTER COLUMN id SET DEFAULT gen_random_uuid();
    ALTER TABLE auth_verifications ALTER COLUMN id SET DEFAULT gen_random_uuid();
    CREATE TABLE IF NOT EXISTS api_keys (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, key_hash text NOT NULL UNIQUE,
      key_ciphertext text, key_nonce text, key_prefix text NOT NULL, name text NOT NULL DEFAULT '', scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
      created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz, last_used_at timestamptz
    );
    CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
    CREATE UNIQUE INDEX IF NOT EXISTS api_keys_user_active_unique ON api_keys(user_id) WHERE revoked_at IS NULL;
    CREATE TABLE IF NOT EXISTS plugin_installs (
      install_id text PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, name text NOT NULL DEFAULT '',
      key_id uuid REFERENCES api_keys(id) ON DELETE SET NULL, active boolean NOT NULL DEFAULT false, paired boolean NOT NULL DEFAULT false,
      pairing_requested_at timestamptz, pairing_denied_at timestamptz, paired_at timestamptz,
      created_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz
    );
    CREATE INDEX IF NOT EXISTS idx_plugin_installs_user_id ON plugin_installs(user_id);
    CREATE INDEX IF NOT EXISTS idx_plugin_installs_key_id ON plugin_installs(key_id);
    CREATE TABLE IF NOT EXISTS devices (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, platform text NOT NULL,
      push_token text NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz
    );
    CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
    CREATE TABLE IF NOT EXISTS questions (
      id text PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, install_id text, session_id text NOT NULL DEFAULT '',
      payload jsonb NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','answered','rejected')),
      created_at timestamptz NOT NULL DEFAULT now(), answered_at timestamptz, rejected_at timestamptz
    );
    CREATE INDEX IF NOT EXISTS idx_questions_user_id ON questions(user_id);
    CREATE TABLE IF NOT EXISTS answers (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), question_id text NOT NULL UNIQUE REFERENCES questions(id) ON DELETE CASCADE,
      user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, body text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE INDEX IF NOT EXISTS idx_answers_user_id ON answers(user_id);
    CREATE TABLE IF NOT EXISTS ui_preferences (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
      data jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE TABLE IF NOT EXISTS billing_entitlements (
      user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, pro_active boolean NOT NULL DEFAULT false,
      pro_expires_at timestamptz, trial_used_at timestamptz, updated_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE TABLE IF NOT EXISTS billing_usage_monthly (
      user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, period_start timestamptz NOT NULL, question_requests integer NOT NULL DEFAULT 0,
      UNIQUE(user_id, period_start)
    );
    CREATE TABLE IF NOT EXISTS superwall_webhook_events (
      event_id text PRIMARY KEY, payload jsonb NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
    );
  `)
  if (config.authLegacyProviderId) await pool.query(`INSERT INTO auth_accounts(id,user_id,account_id,provider_id,created_at,updated_at)
    SELECT gen_random_uuid(),id,auth_subject,$1,now(),now() FROM users WHERE auth_subject IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM auth_accounts WHERE provider_id=$1 AND account_id=users.auth_subject)`, [config.authLegacyProviderId])
  await pool.query(`
    DO $$ BEGIN
      IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='api_keys' AND column_name='scopes' AND data_type='ARRAY') THEN
        ALTER TABLE api_keys ALTER COLUMN scopes TYPE jsonb USING to_jsonb(scopes), ALTER COLUMN scopes SET DEFAULT '[]'::jsonb;
      END IF;
    END $$;
  `)
}

export class PostgresStore {
  constructor(pool) { this.pool = pool }

  async upsertUser(value) {
    return one(await this.pool.query(`
      INSERT INTO users(email,name,auth_provider,auth_issuer,auth_subject) VALUES($1,$2,$3,$4,$5)
      ON CONFLICT(email) DO UPDATE SET name=EXCLUDED.name,auth_provider=EXCLUDED.auth_provider,updated_at=now() RETURNING *
    `, [value.email, value.name, value.authProvider, value.authIssuer, value.authSubject]), user)
  }

  async getUser(id) { return one(await this.pool.query("SELECT * FROM users WHERE id=$1", [id]), user) }
  async getUserByOIDC(issuer, subject) { return one(await this.pool.query("SELECT * FROM users WHERE auth_issuer=$1 AND auth_subject=$2", [issuer.trim(), subject.trim()]), user) }

  async attachOIDCToUser(id, issuer, subject) {
    const updated = await this.pool.query("UPDATE users SET auth_issuer=$2,auth_subject=$3,updated_at=now() WHERE id=$1 AND auth_issuer IS NULL AND auth_subject IS NULL", [id, issuer.trim(), subject.trim()])
    if (updated.rowCount) return
    const existing = await this.getUser(id)
    if (existing.authIssuer !== issuer.trim() || existing.authSubject !== subject.trim()) throw new Error("user already linked to different OIDC identity")
  }

  async createAPIKey(value) {
    return one(await this.pool.query(`INSERT INTO api_keys(user_id,name,key_hash,key_prefix,key_ciphertext,key_nonce,scopes)
      VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *`, [value.userId, value.name, value.keyHash, value.keyPrefix, value.keyCiphertext || null, value.keyNonce || null, JSON.stringify(value.scopes || [])]), apiKey)
  }

  async listAPIKeys(userId) { return (await this.pool.query("SELECT * FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC", [userId])).rows.map(apiKey) }
  async getActiveAPIKey(userId) { return one(await this.pool.query("SELECT * FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1", [userId]), apiKey) }
  async lookupAPIKey(hash) { return one(await this.pool.query("SELECT * FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL", [hash]), apiKey) }
  async updateAPIKeyLastUsed(id) { await this.pool.query("UPDATE api_keys SET last_used_at=now() WHERE id=$1", [id]) }
  async revokeActiveAPIKeys(userId) { await this.pool.query("UPDATE api_keys SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL", [userId]) }
  async revokeAPIKey(userId, id) { if (!(await this.pool.query("UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL", [id, userId])).rowCount) throw codedError("NOT_FOUND") }

  async upsertPluginInstall(value) {
    const result = await this.pool.query(`INSERT INTO plugin_installs(install_id,user_id,name,key_id,active) VALUES($1,$2,$3,$4,$5)
      ON CONFLICT(install_id) DO UPDATE SET name=EXCLUDED.name,key_id=EXCLUDED.key_id,last_seen_at=now()
      WHERE plugin_installs.user_id=EXCLUDED.user_id`,
    [value.installId, value.userId, value.name, value.keyId || null, value.active])
    if (!result.rowCount) throw codedError("FORBIDDEN")
  }

  async getPluginInstall(id) { return one(await this.pool.query("SELECT * FROM plugin_installs WHERE install_id=$1", [id]), install) }
  async listPluginInstalls(userId) { return (await this.pool.query("SELECT * FROM plugin_installs WHERE user_id=$1 ORDER BY last_seen_at DESC NULLS LAST,created_at DESC", [userId])).rows.map(install) }
  async updatePluginInstallActive(userId, id, active) { if (!(await this.pool.query("UPDATE plugin_installs SET active=$3 WHERE install_id=$1 AND user_id=$2", [id, userId, active])).rowCount) throw codedError("NOT_FOUND") }
  async deletePluginInstall(userId, id) { if (!(await this.pool.query("DELETE FROM plugin_installs WHERE install_id=$1 AND user_id=$2", [id, userId])).rowCount) throw codedError("NOT_FOUND") }
  async requestPluginInstallPairing(userId, id) { return Boolean((await this.pool.query("UPDATE plugin_installs SET pairing_requested_at=now() WHERE install_id=$1 AND user_id=$2 AND paired=false AND pairing_requested_at IS NULL", [id, userId])).rowCount) }

  async pairPluginInstall(userId, id) {
    if (!(await this.pool.query("UPDATE plugin_installs SET paired=true,paired_at=COALESCE(paired_at,now()),pairing_requested_at=NULL,pairing_denied_at=NULL WHERE install_id=$1 AND user_id=$2", [id, userId])).rowCount) throw codedError("NOT_FOUND")
  }

  async denyPluginInstallPairing(userId, id) {
    if (!(await this.pool.query("UPDATE plugin_installs SET paired=false,pairing_denied_at=now(),pairing_requested_at=NULL WHERE install_id=$1 AND user_id=$2", [id, userId])).rowCount) throw codedError("NOT_FOUND")
  }

  async activatePluginInstallExclusive(userId, id) {
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      if (!(await client.query("SELECT 1 FROM plugin_installs WHERE install_id=$1 AND user_id=$2 FOR UPDATE", [id, userId])).rowCount) throw codedError("NOT_FOUND")
      await client.query("UPDATE plugin_installs SET active=false WHERE user_id=$1", [userId])
      await client.query("UPDATE plugin_installs SET active=true WHERE install_id=$1 AND user_id=$2", [id, userId])
      await client.query("COMMIT")
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async enforceSingleActivePluginInstall(userId) {
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      const result = await client.query("SELECT install_id FROM plugin_installs WHERE user_id=$1 AND active=true ORDER BY last_seen_at DESC NULLS LAST,created_at DESC LIMIT 1 FOR UPDATE", [userId])
      if (!result.rowCount) { await client.query("COMMIT"); return "" }
      const id = result.rows[0].install_id
      await client.query("UPDATE plugin_installs SET active=false WHERE user_id=$1 AND active=true AND install_id<>$2", [userId, id])
      await client.query("COMMIT")
      return id
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async upsertDevice(value) {
    await this.pool.query(`INSERT INTO devices(user_id,platform,push_token) VALUES($1,$2,$3)
      ON CONFLICT(push_token) DO UPDATE SET user_id=EXCLUDED.user_id,platform=EXCLUDED.platform,last_seen_at=now()`, [value.userId, value.platform, value.pushToken])
  }

  async listDevices(userId) { return (await this.pool.query("SELECT * FROM devices WHERE user_id=$1 ORDER BY last_seen_at DESC NULLS LAST,created_at DESC", [userId])).rows.map((row) => ({ id: row.id, userId: row.user_id, platform: row.platform, pushToken: row.push_token, createdAt: row.created_at, lastSeenAt: row.last_seen_at })) }
  async countPendingQuestions(userId) { return Number((await this.pool.query("SELECT count(*) FROM questions WHERE user_id=$1 AND status='pending'", [userId])).rows[0].count) }

  async createQuestion(value, periodStart, monthlyLimit) {
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      const inserted = await client.query(`INSERT INTO questions(id,user_id,install_id,session_id,payload,status) VALUES($1,$2,$3,$4,$5,$6)
        ON CONFLICT(id) DO NOTHING`, [value.id, value.userId, value.installId || null, value.sessionId, JSON.stringify(value.payload), value.status || "pending"])
      if (!inserted.rowCount) { await client.query("COMMIT"); return false }
      if (monthlyLimit > 0) {
        const usage = await client.query(`INSERT INTO billing_usage_monthly(user_id,period_start,question_requests) VALUES($1,$2,1)
          ON CONFLICT(user_id,period_start) DO UPDATE SET question_requests=billing_usage_monthly.question_requests+1
          WHERE billing_usage_monthly.question_requests<$3 RETURNING question_requests`, [value.userId, periodStart, monthlyLimit])
        if (!usage.rowCount) throw codedError("QUOTA_EXCEEDED")
      }
      await client.query("COMMIT")
      return true
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async listQuestions(userId) { return (await this.pool.query("SELECT * FROM questions WHERE user_id=$1 ORDER BY created_at DESC", [userId])).rows.map(question) }
  async getQuestion(userId, id) { return one(await this.pool.query("SELECT * FROM questions WHERE id=$1 AND user_id=$2", [id, userId]), question) }
  async getQuestionStatus(userId, id) { return (await one(await this.pool.query("SELECT status FROM questions WHERE id=$1 AND user_id=$2", [id, userId]))).status }
  async getAnswer(id) { return one(await this.pool.query("SELECT * FROM answers WHERE question_id=$1", [id]), (row) => ({ id: row.id, questionId: row.question_id, userId: row.user_id, body: JSON.parse(row.body), createdAt: row.created_at })) }

  async answerQuestion(id, userId, answers) {
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      const existing = await client.query("SELECT status FROM questions WHERE id=$1 AND user_id=$2 FOR UPDATE", [id, userId])
      if (!existing.rowCount) throw codedError("NOT_FOUND")
      if (existing.rows[0].status !== "pending") throw codedError("ALREADY_RESOLVED")
      const result = await client.query("INSERT INTO answers(question_id,user_id,body) VALUES($1,$2,$3) RETURNING *", [id, userId, JSON.stringify(answers)])
      await client.query("UPDATE questions SET status='answered',answered_at=now() WHERE id=$1 AND user_id=$2", [id, userId])
      await client.query("COMMIT")
      const row = result.rows[0]
      return { id: row.id, questionId: row.question_id, userId: row.user_id, body: answers, createdAt: row.created_at }
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async rejectQuestion(id, userId) {
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      const existing = await client.query("SELECT status FROM questions WHERE id=$1 AND user_id=$2 FOR UPDATE", [id, userId])
      if (!existing.rowCount) throw codedError("NOT_FOUND")
      if (existing.rows[0].status !== "pending") throw codedError("ALREADY_RESOLVED")
      await client.query("UPDATE questions SET status='rejected',rejected_at=now() WHERE id=$1 AND user_id=$2", [id, userId])
      await client.query("COMMIT")
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async markQuestionsAnswered(userId, ids) {
    if (!ids.length) return 0
    const client = await this.pool.connect()
    try {
      await client.query("BEGIN")
      const pending = await client.query("SELECT id FROM questions WHERE user_id=$1 AND id=ANY($2) AND status='pending' FOR UPDATE", [userId, ids])
      for (const row of pending.rows) await client.query("INSERT INTO answers(question_id,user_id,body) VALUES($1,$2,'[]')", [row.id, userId])
      if (pending.rowCount) await client.query("UPDATE questions SET status='answered',answered_at=now() WHERE id=ANY($1)", [pending.rows.map((row) => row.id)])
      await client.query("COMMIT")
      return pending.rowCount
    } catch (error) { await client.query("ROLLBACK"); throw error } finally { client.release() }
  }

  async deleteQuestions(userId, ids) { return (await this.pool.query("DELETE FROM questions WHERE user_id=$1 AND id=ANY($2)", [userId, ids])).rowCount }

  async getUIPreferences(userId) {
    return one(await this.pool.query("SELECT * FROM ui_preferences WHERE user_id=$1", [userId]), (row) => ({ id: row.id, userId: row.user_id, data: row.data, createdAt: row.created_at, updatedAt: row.updated_at }))
  }

  async upsertUIPreferences(userId, data) {
    return one(await this.pool.query(`INSERT INTO ui_preferences(user_id,data) VALUES($1,$2)
      ON CONFLICT(user_id) DO UPDATE SET data=EXCLUDED.data,updated_at=now() RETURNING *`, [userId, JSON.stringify(data)]),
    (row) => ({ id: row.id, userId: row.user_id, data: row.data, createdAt: row.created_at, updatedAt: row.updated_at }))
  }

  async storeSuperwallWebhookEvent(id, payload) { return Boolean((await this.pool.query("INSERT INTO superwall_webhook_events(event_id,payload) VALUES($1,$2) ON CONFLICT(event_id) DO NOTHING", [id, payload])).rowCount) }

  async getBillingEntitlements(userId) {
    return one(await this.pool.query("SELECT * FROM billing_entitlements WHERE user_id=$1", [userId]), (row) => ({ userId: row.user_id, proActive: row.pro_active, proExpiresAt: row.pro_expires_at, trialUsedAt: row.trial_used_at, updatedAt: row.updated_at }))
  }

  async upsertBillingEntitlements(userId, active, expiresAt, markTrial) {
    return one(await this.pool.query(`INSERT INTO billing_entitlements(user_id,pro_active,pro_expires_at,trial_used_at) VALUES($1,$2,$3,CASE WHEN $4 THEN now() END)
      ON CONFLICT(user_id) DO UPDATE SET pro_active=EXCLUDED.pro_active,pro_expires_at=EXCLUDED.pro_expires_at,
      trial_used_at=COALESCE(billing_entitlements.trial_used_at,EXCLUDED.trial_used_at),updated_at=now() RETURNING *`, [userId, active, expiresAt, markTrial]),
    (row) => ({ userId: row.user_id, proActive: row.pro_active, proExpiresAt: row.pro_expires_at, trialUsedAt: row.trial_used_at, updatedAt: row.updated_at }))
  }

  async getBillingUsageMonthly(userId, periodStart) {
    return one(await this.pool.query("SELECT * FROM billing_usage_monthly WHERE user_id=$1 AND period_start=$2", [userId, periodStart]), (row) => ({ userId: row.user_id, periodStart: row.period_start, questionRequests: row.question_requests }))
  }

  async countActivePluginInstalls(userId) { return Number((await this.pool.query("SELECT count(*) FROM plugin_installs WHERE user_id=$1 AND active=true AND paired=true", [userId])).rows[0].count) }
}
