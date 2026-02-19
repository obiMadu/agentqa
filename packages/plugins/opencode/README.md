# AgentQA Plugin

OpenCode plugin for AgentQA. AgentQA lets you unblock your agents anywhere, anytime. Never leave your agents waiting again. OpenCode is the only supported client today.

## Files

- `agentqa-plugin.ts` - OpenCode plugin
- `simulate-opencode.ts` - Local simulation helper

## Install (project scope)

```bash
cp agentqa-plugin.ts /path/to/your/project/.opencode/plugins/
```

## Configure

Set the backend URL and API key in the plugin file if needed (legacy Bun server):

```ts
const EXTERNAL_SERVICE_URL = "http://localhost:3456"
```

You will be prompted for the API key the first time the plugin runs (copy it from Settings → API settings).
