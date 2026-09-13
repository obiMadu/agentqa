import { createServer as createHTTPServer } from "node:http"
import { pathToFileURL } from "node:url"
import { Pool } from "pg"

import { loadConfig, parseEncryptionKey } from "./config.js"
import { createAuth } from "./auth.js"
import { createPushSender } from "./push.js"
import { createServer } from "./server.js"
import { migrate, PostgresStore } from "./store.js"

export function createNodeListener(app) {
  return createHTTPServer(async (incoming, outgoing) => {
    try {
      const chunks = []
      let size = 0
      for await (const chunk of incoming) {
        size += chunk.length
        if (size > 1_048_576) {
          outgoing.writeHead(413)
          outgoing.end()
          incoming.destroy()
          return
        }
        chunks.push(chunk)
      }
      const body = Buffer.concat(chunks)
      const request = new Request(`http://${incoming.headers.host}${incoming.url}`, {
        method: incoming.method,
        headers: incoming.headers,
        body: body.length ? body : undefined,
      })
      const response = await app.fetch(request)
      const headers = Object.fromEntries(response.headers)
      const cookies = response.headers.getSetCookie()
      if (cookies.length) headers["set-cookie"] = cookies
      outgoing.writeHead(response.status, headers)
      outgoing.end(Buffer.from(await response.arrayBuffer()))
    } catch (error) {
      console.error(error)
      if (!outgoing.headersSent) outgoing.writeHead(500)
      outgoing.end()
    }
  })
}

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds))

async function connect(pool, retries, delay) {
  const attempts = retries > 0 ? retries : 1
  const wait = delay > 0 ? delay : 2000
  let lastError
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try { await pool.query("SELECT 1"); return } catch (error) {
      lastError = error
      if (attempt < attempts) await sleep(wait)
    }
  }
  throw lastError
}

export async function start(env = process.env) {
  const config = loadConfig(env)
  if (!config.databaseURL) throw new Error("DATABASE_URL is required")
  const encryptionKey = parseEncryptionKey(config.apiKeyEncryptionKey)
  if (!config.authURL) throw new Error("BETTER_AUTH_URL is required")
  if (!config.authSecret) throw new Error("BETTER_AUTH_SECRET is required")
  const pool = new Pool({ connectionString: config.databaseURL, max: 10, maxLifetimeSeconds: 1800 })
  await connect(pool, config.dbConnectRetries, config.dbConnectDelay)
  await migrate(pool, config)
  const store = new PostgresStore(pool)
  const auth = createAuth(pool, config)
  const push = createPushSender({ endpoint: config.expoPushURL, store })
  const app = createServer({ store, auth, push, config, encryptionKey })
  const listener = createNodeListener(app)
  const host = config.addr.startsWith(":") ? undefined : config.addr.slice(0, config.addr.lastIndexOf(":"))
  await new Promise((resolve, reject) => listener.once("error", reject).listen(config.port, host, resolve))
  console.log(`agentqa-api listening on ${config.addr}`)
  return { listener, pool }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  start().catch((error) => { console.error(error); process.exit(1) })
}
