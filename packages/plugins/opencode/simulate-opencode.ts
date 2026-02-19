// Simulate OpenCode executing the AgentQA plugin
// Run this with: cd agentqa && bun install && bun run simulate

import { createOpencodeClient } from "@opencode-ai/sdk/v2"

const EXTERNAL_SERVICE_URL = "http://localhost:3456"

console.log("=== Simulating OpenCode Plugin Execution ===\n")

// Create a mock OpenCode client (same as what OpenCode provides to plugins)
console.log("Creating mock OpenCode client...")
const mockClient = createOpencodeClient({
  baseUrl: "http://localhost:4096", // Default OpenCode server port
})

// Check what the client looks like
console.log("\n--- Client Structure ---")
console.log("Client type:", typeof mockClient)
console.log("Client constructor name:", mockClient.constructor?.name)
console.log("Client has 'question' property:", 'question' in mockClient)
console.log("Client.question value:", mockClient.question)
console.log("Client.question type:", typeof mockClient.question)

// Try to enumerate all properties
console.log("\n--- All Client Properties ---")
const descriptors = Object.getOwnPropertyNames(Object.getPrototypeOf(mockClient))
console.log("Property names:", descriptors.filter(name => name !== 'constructor'))

// Check if question is a getter
const questionDescriptor = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(mockClient), 'question')
console.log("Question is getter:", questionDescriptor?.get !== undefined)

// Try to access question.reply
console.log("\n--- Testing question.reply ---")
try {
  const questionObj = mockClient.question
  console.log("question object:", questionObj)
  console.log("question has 'reply' method:", typeof questionObj?.reply === 'function')
  if (questionObj?.reply) {
    console.log("reply method found ✓")
  } else {
    console.log("reply method NOT found ✗")
  }
} catch (e) {
  console.log("Error accessing question:", e)
}

console.log("\n--- Creating Mock Plugin Context ---")

// Create mock plugin input (same as PluginInput)
const mockCtx = {
  client: mockClient,
  project: { id: "test-project", name: "Test" },
  directory: "/tmp/test",
  worktree: "/tmp/test",
  serverUrl: new URL("http://localhost:4096"),
  $: {
    // Mock Bun shell
    raw: async (strings: TemplateStringsArray, ...values: any[]) => {
      console.log("[Shell] Would execute:", strings.join("${...}"))
      return { text: () => "" }
    }
  } as any
}

console.log("Mock context created")
console.log("ctx.client exists:", !!mockCtx.client)
console.log("ctx.client.question exists:", !!(mockCtx.client && mockCtx.client.question))

console.log("\n--- Importing Plugin ---")

// Import the actual plugin
try {
  const pluginModule = await import("./agentqa-plugin.ts")
  const pluginFn = pluginModule.AgentQAPlugin
  
  console.log("Plugin module imported:", !!pluginModule)
  console.log("Plugin function found:", typeof pluginFn === 'function')
  
  console.log("\n--- Initializing Plugin ---")
  const hooks = await pluginFn(mockCtx)
  console.log("✓ Plugin initialized successfully")
  console.log("Available hooks:", Object.keys(hooks))
  
  if (hooks["tool.execute.before"]) {
    console.log("\n--- Testing tool.execute.before hook ---")
    
    const mockInput = {
      tool: "question",
      sessionID: "session_test123",
      callID: "tool_test456"
    }
    
    const mockOutput = {
      args: {
        questions: [{
          header: "Test Question",
          question: "What is your favorite programming language?",
          options: [
            { label: "JavaScript", description: "Web development" },
            { label: "Python", description: "Data science" },
            { label: "Rust", description: "Systems programming" }
          ],
          multiple: false,
          custom: true
        }]
      }
    }
    
    console.log("Calling hook with mock question...")
    await hooks["tool.execute.before"](mockInput, mockOutput)
    console.log("✓ Hook executed (check if server received the question)")
  }
  
} catch (error) {
  console.error("\n✗ Error:", error)
  console.error("Stack:", (error as Error).stack)
}

console.log("\n=== Simulation Complete ===")
console.log("If you see 'ctx.client.question is undefined' above, that's the bug!")
console.log("The plugin should have ctx.client.question defined but doesn't.")
