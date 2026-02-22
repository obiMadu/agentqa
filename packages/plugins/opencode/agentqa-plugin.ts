import type { Plugin } from "@opencode-ai/plugin"
import { mkdir } from "fs/promises"
import os from "os"
import path from "path"
import { randomUUID } from "crypto"

const EXTERNAL_SERVICE_URL = "http://localhost:8080"
const SERVICE = "agentqa"
const PROVIDER_ID = "agentqa"
const STATE_FILE = path.join(dataDir(), "agentqa.json")

const inflight = new Set<string>()
const answeredExternally = new Set<string>()
let cached: { installId: string; apiKey?: string; installName: string } | undefined

function dataDir() {
  const base = process.env.XDG_DATA_HOME ?? path.join(os.homedir(), ".local", "share")
  return path.join(base, "opencode")
}

async function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

async function log(ctx: any, level: "debug" | "info" | "warn" | "error", message: string, extra?: any) {
  try {
    await ctx.client.app.log({
      body: {
        service: SERVICE,
        level,
        message,
        extra,
      },
    })
  } catch {
    // Ignore logging failures to avoid breaking the plugin
  }
}

async function loadState() {
  if (cached) return cached
  await mkdir(dataDir(), { recursive: true })
  const file = Bun.file(STATE_FILE)
  const data = await file.json().catch(() => ({})) as {
    installId?: string
    apiKey?: string
    installName?: string
  }
  const installId = data.installId ?? `agentqa_${randomUUID()}`
  const installName = (data.installName ?? "").trim() || "OpenCode"
  cached = { installId, apiKey: data.apiKey, installName }
  if (!data.installId || !data.installName) await saveState(cached)
  return cached
}

async function saveState(state: { installId: string; apiKey?: string; installName: string }) {
  await Bun.write(STATE_FILE, JSON.stringify(state, null, 2), { mode: 0o600 })
  cached = state
}

function authHeader(state: { apiKey?: string }) {
  if (!state.apiKey) return {}
  return { Authorization: `Bearer ${state.apiKey}` }
}

async function pollForExternalAnswer(requestID: string, ctx: any) {
  const state = await loadState()
  let retries = 0
  const maxRetries = 10

  while (true) {
    try {
      const waitUrl = new URL(`${EXTERNAL_SERVICE_URL}/questions/${requestID}/wait`)
      waitUrl.searchParams.set("install_id", state.installId)
      const res = await fetch(waitUrl.toString(), {
        headers: authHeader(state),
      })

      if (res.status === 204) continue
      if (res.status === 410) return

      if (res.ok) {
        const data = await res.json()
        
        if (data.status === "rejected") {
          await ctx.client.question.reject({ requestID })
          await log(ctx, "info", "External question rejected", { requestID })
          return
        }
        
        if (data.status === "answered") {
          await ctx.client.question.reply({ requestID, answers: data.answers })
          answeredExternally.add(requestID)
          await log(ctx, "info", "External answer submitted", { requestID })
          return
        }
      }
      
      // Reset retries on successful connection
      retries = 0
    } catch (err) {
      // Any error - retry with backoff
      retries++
      if (retries > maxRetries) {
        throw new Error(`Max retries (${maxRetries}) exceeded for question ${requestID}`)
      }
      const backoffMs = Math.min(1000 * Math.pow(2, retries - 1), 30000)
      await log(ctx, "warn", "Polling error, retrying", { 
        requestID, 
        retry: retries, 
        maxRetries,
        backoffMs
      })
      await sleep(backoffMs)
    }
  }
}

export const AgentQAPlugin: Plugin = async (ctx) => {
  await log(ctx, "info", "Plugin initialized")

  return {
    auth: {
      provider: PROVIDER_ID,
      methods: [
        {
          type: "api",
          label: "API key",
          prompts: [
            {
              type: "text",
              key: "api_key",
              message: "AgentQA API key",
              validate: (value) => (value && value.length > 0 ? undefined : "Required"),
            },
            {
              type: "text",
              key: "install_name",
              message: "OpenCode instance name (default: OpenCode)",
            },
          ],
          authorize: async (inputs) => {
            const key = inputs.api_key
            if (!key) return { type: "failed" }
            const state = await loadState()
            const installNameInput =
              typeof inputs.install_name === "string" ? inputs.install_name.trim() : ""
            const installName = installNameInput || state.installName || "OpenCode"
            await saveState({ ...state, apiKey: key, installName })
            try {
              const res = await fetch(`${EXTERNAL_SERVICE_URL}/plugin/register`, {
                method: "POST",
                headers: {
                  "Content-Type": "application/json",
                  Authorization: `Bearer ${key}`,
                },
                body: JSON.stringify({ install_id: state.installId, name: installName }),
              })
              if (!res.ok) {
                await log(ctx, "error", "Failed to register plugin", {
                  status: res.status,
                })
                return { type: "failed" }
              }
            } catch (e) {
              await log(ctx, "error", "Failed to register plugin", { error: String(e) })
              return { type: "failed" }
            }
            return { type: "success", key }
          },
        },
      ],
    },
    event: async ({ event }) => {
      if (event.type === "question.asked") {
        const requestID = event.properties.id
        if (inflight.has(requestID)) return
        inflight.add(requestID)

        try {
          const state = await loadState()
			const res = await fetch(`${EXTERNAL_SERVICE_URL}/questions`, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              ...authHeader(state),
            },
			body: JSON.stringify({
				request_id: requestID,
				session_id: event.properties.sessionID,
				questions: event.properties.questions,
				install_id: state.installId,
			}),
		})

          if (!res.ok) {
            if (res.status === 428) {
              await log(ctx, "warn", "Pairing required - approve this agent in the AgentQA mobile app.", {
                requestID,
              })
              inflight.delete(requestID)
              return
            }
            if (res.status === 402) {
              await log(ctx, "warn", "Quota exceeded - upgrade to Pro to continue receiving questions.", {
                requestID,
              })
              inflight.delete(requestID)
              return
            }
            await log(ctx, "error", "Failed to send question", {
              requestID,
              status: res.status,
            })
            inflight.delete(requestID)
            return
          }

          pollForExternalAnswer(requestID, ctx).finally(() => inflight.delete(requestID))
        } catch (e) {
          inflight.delete(requestID)
          await log(ctx, "error", "Failed to send question", { requestID, error: String(e) })
        }
      }

      if (event.type === "question.replied") {
        const requestID = event.properties.requestID
        if (answeredExternally.has(requestID)) {
          answeredExternally.delete(requestID)
          return
        }

        try {
          const state = await loadState()
			await fetch(`${EXTERNAL_SERVICE_URL}/questions/${requestID}/answers`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authHeader(state),
				},
				body: JSON.stringify({
					answers: event.properties.answers,
					source: "opencode",
				}),
			})
        } catch (e) {
          await log(ctx, "error", "Failed to forward answer", { requestID, error: String(e) })
        }
      }

      if (event.type === "question.rejected") {
        const requestID = event.properties.requestID
        try {
          const state = await loadState()
			await fetch(`${EXTERNAL_SERVICE_URL}/questions/${requestID}/reject`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authHeader(state),
				},
				body: JSON.stringify({
					source: "opencode",
				}),
			})
        } catch (e) {
          await log(ctx, "error", "Failed to forward rejection", { requestID, error: String(e) })
        }
      }
    },
  }
}
