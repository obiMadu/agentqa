const validToken = (token) => token.startsWith("ExponentPushToken[") || token.startsWith("ExpoPushToken[")
const agentName = (name) => name?.trim() || "Agent"

function questionTitle(count, name) {
  if (count <= 0) return `New questions from ${agentName(name)}`
  if (count === 1) return `1 new question from ${agentName(name)}`
  return `${count} new questions from ${agentName(name)}`
}

export function createPushSender({ endpoint, store, fetchImpl = fetch }) {
  async function send(userId, title, body, data) {
    if (!endpoint) return
    const devices = await store.listDevices(userId)
    const tokens = devices.map((device) => device.pushToken.trim()).filter(validToken)
    if (!tokens.length) return
    const badge = await store.countPendingQuestions(userId)
    const messages = tokens.map((to) => ({ to, title, body, badge, data }))
    for (let index = 0; index < messages.length; index += 100) {
      const response = await fetchImpl(endpoint, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(messages.slice(index, index + 100)),
        signal: AbortSignal.timeout(10_000),
      })
      const text = await response.text()
      if (!response.ok) throw new Error(`expo push request failed: ${text.trim() || response.statusText}`)
      if (!text) continue
      const result = JSON.parse(text)
      if (result.errors?.length) throw new Error(`expo push error: ${result.errors[0].message}`)
      const failed = result.data?.find((ticket) => ticket.status === "error")
      if (failed) throw new Error(`expo push error${failed.message ? `: ${failed.message}` : ""}`)
    }
  }

  return {
    sendQuestion(message) {
      return send(message.userId, questionTitle(message.questionCount, message.installName), message.preview, { question_id: message.questionId })
    },
    sendPairingRequested(message) {
      return send(message.userId, "Pairing request", `${agentName(message.installName)} wants to pair.`, { event: "pairing_requested", install_id: message.installId })
    },
    sendPairingPaired(message) {
      return send(message.userId, "Paired", `${agentName(message.installName)} is now paired.`, { event: "paired", install_id: message.installId })
    },
  }
}
