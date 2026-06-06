# Kiropi

**MCP Server + OpenAI-Compatible REST API Bridge**

Kiropi adalah server Golang yang menjembatani aplikasi (NSA) dengan Kiro AI. Aplikasi mengirim prompt via REST API format OpenAI, Kiro mengambil dan menjawab prompt tersebut via MCP protocol.

## Architecture

```
┌───────┐  POST /v1/chat/completions  ┌──────────────────────┐  MCP SSE (tools)  ┌────────┐
│  NSA  │ ──────────────────────────► │       Kiropi         │ ◄────────────────── │  Kiro  │
│ (App) │ ◄────────────────────────── │  REST API + MCP Srv  │ ──────────────────► │  (AI)  │
└───────┘     OpenAI JSON response     └──────────────────────┘   tool results      └────────┘
                                              :50403                   :50404
```

### Flow

1. **NSA** sends `POST /v1/chat/completions` to Kiropi (port 50403)
2. **Kiropi** queues the prompt and waits
3. **Kiro** (connected via MCP SSE on port 50404) calls `get_pending_prompt` tool
4. **Kiro** processes the prompt with its AI capabilities
5. **Kiro** calls `submit_response` tool with the answer
6. **Kiropi** receives the response, packages it as OpenAI JSON, returns to NSA

## Features

- OpenAI-compatible REST API (`/v1/chat/completions`)
- SSE Streaming support (`"stream": true`)
- MCP Server (SSE transport) for Kiro to connect
- Thread-safe prompt queue with configurable timeout
- Bearer token authentication (optional)
- CORS support
- Cloudflare Tunnel ready (expose to internet without public IP)
- Single binary, cross-platform (Linux, macOS, Windows)

## Quick Start

### Prerequisites

- Go 1.22+

### Build & Run

```bash
git clone https://github.com/ifcoid/kiropi.git
cd kiropi

# Build
go build -o kiropi ./cmd/server

# Run
./kiropi
```

Output:
```
[MCP] Starting MCP SSE Server on port 50404
[MCP] Kiro connects to: http://localhost:50404/sse
[MCP] Tools available: get_pending_prompt, submit_response, queue_status
[API] Starting REST server on port 50403
[API] Endpoints:
[API]   POST /v1/chat/completions  - OpenAI-compatible chat
[API]   POST /api/ask              - Alias for chat completions
[API]   GET  /v1/models            - List available models
[API]   GET  /health               - Health check
```

### Cross-compile

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o kiropi-linux-amd64 ./cmd/server

# Linux ARM64 (Raspberry Pi, etc)
GOOS=linux GOARCH=arm64 go build -o kiropi-linux-arm64 ./cmd/server

# macOS
GOOS=darwin GOARCH=arm64 go build -o kiropi-darwin-arm64 ./cmd/server

# Windows
GOOS=windows GOARCH=amd64 go build -o kiropi.exe ./cmd/server
```

## Configuration

All configuration via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `KIROPI_PORT` | REST API port (for NSA/apps) | `50403` |
| `KIROPI_MCP_PORT` | MCP SSE port (for Kiro) | `50404` |
| `KIROPI_API_KEY` | API key for REST auth | (disabled) |
| `KIROPI_MODEL` | Default model name | `kiropi-1` |
| `KIROPI_MAX_CONCURRENT` | Max concurrent requests | `10` |
| `KIROPI_PROMPT_TIMEOUT` | Seconds to wait for Kiro | `120` |

## MCP Tools (for Kiro)

Kiro connects to Kiropi as an MCP client and uses these tools:

### `get_pending_prompt`

Picks up the next prompt from the queue.

**Input:** none

**Output:**
```json
{
  "status": "pending",
  "prompt_id": "uuid-here",
  "model": "kiropi-1",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Explain microservices"}
  ]
}
```

Or if empty:
```json
{"status": "empty", "message": "No pending prompts in queue"}
```

### `submit_response`

Submits Kiro's answer for a prompt.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt_id` | string | yes | The ID from `get_pending_prompt` |
| `response` | string | yes | The AI response text |

**Output:**
```json
{"status": "submitted", "prompt_id": "uuid-here"}
```

### `queue_status`

Check queue status.

**Output:**
```json
{"pending": 3, "total": 5}
```

## How Kiro Connects

Kiro connects to Kiropi's MCP SSE endpoint. The typical flow:

```
Kiro (MCP Client) ──── SSE ────► Kiropi (MCP Server :50404)
                                       │
                                       ├─ tool: get_pending_prompt → returns prompt
                                       ├─ (Kiro thinks...)
                                       └─ tool: submit_response(prompt_id, response)
```

### Kiro MCP Config

Add Kiropi as an MCP server in Kiro's configuration:

```json
{
  "mcpServers": {
    "kiropi": {
      "url": "http://localhost:50404/sse"
    }
  }
}
```

Or via Cloudflare Tunnel:
```json
{
  "mcpServers": {
    "kiropi": {
      "url": "https://kiropi.yourdomain.com/sse"
    }
  }
}
```

## REST API Endpoints (for NSA/Apps)

### POST `/v1/chat/completions`

OpenAI-compatible chat completion. Blocks until Kiro responds or timeout.

```bash
curl -X POST http://localhost:50403/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "kiropi-1",
    "messages": [
      {"role": "user", "content": "Hello, explain Docker in 3 sentences"}
    ]
  }'
```

**Response:**
```json
{
  "id": "chatcmpl-abc12345",
  "object": "chat.completion",
  "created": 1717689600,
  "model": "kiropi-1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Docker is a containerization platform..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 25,
    "total_tokens": 35
  }
}
```

### POST `/v1/chat/completions` (streaming)

```bash
curl -N -X POST http://localhost:50403/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kiropi-1",
    "stream": true,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### POST `/api/ask`

Alias for `/v1/chat/completions`.

### GET `/v1/models`

List available models.

### GET `/health`

```json
{
  "status": "ok",
  "queue_pending": 0,
  "queue_total": 0,
  "timestamp": "2025-01-01T00:00:00Z"
}
```

## Client Examples

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:50403/v1",
    api_key="your-api-key"
)

response = client.chat.completions.create(
    model="kiropi-1",
    messages=[{"role": "user", "content": "Hello Kiro!"}]
)

print(response.choices[0].message.content)
```

### Python (streaming)

```python
stream = client.chat.completions.create(
    model="kiropi-1",
    messages=[{"role": "user", "content": "Explain Go concurrency"}],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

### JavaScript/TypeScript

```typescript
const response = await fetch('http://localhost:50403/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer your-api-key'
  },
  body: JSON.stringify({
    model: 'kiropi-1',
    messages: [{ role: 'user', content: 'Hello!' }]
  })
});

const data = await response.json();
console.log(data.choices[0].message.content);
```

## Deployment with Cloudflare Tunnel

Expose Kiropi to the internet so Kiro can reach it from anywhere:

```
┌────────┐         ┌──────────────────┐         ┌──────────────────┐
│  Kiro  │ ◄─HTTPS─► Cloudflare Edge  │ ◄─────► │  cloudflared     │
│(remote)│         │  (global CDN)    │         │  + Kiropi        │
└────────┘         └──────────────────┘         │  (your laptop)   │
                                                 └──────────────────┘
```

### Quick Tunnel (no account needed)

```bash
# Terminal 1: Run Kiropi
./kiropi

# Terminal 2: Expose MCP port via Cloudflare
cloudflared tunnel --url http://localhost:50404
# → https://random-words.trycloudflare.com
```

Then configure Kiro MCP:
```json
{
  "mcpServers": {
    "kiropi": {
      "url": "https://random-words.trycloudflare.com/sse"
    }
  }
}
```

### Named Tunnel (persistent)

```bash
cloudflared tunnel login
cloudflared tunnel create kiropi
cloudflared tunnel route dns kiropi kiropi.yourdomain.com

cat > ~/.cloudflared/config.yml << EOF
tunnel: <TUNNEL_ID>
credentials-file: ~/.cloudflared/<TUNNEL_ID>.json

ingress:
  - hostname: kiropi.yourdomain.com
    service: http://localhost:50404
  - service: http_status:404
EOF

cloudflared tunnel run kiropi
```

### Security Notes

- **Always set `KIROPI_API_KEY`** for the REST API when exposed to the internet
- MCP port (50404) should only be exposed to Kiro (via tunnel)
- Cloudflare provides DDoS protection and TLS automatically

## License

MIT
