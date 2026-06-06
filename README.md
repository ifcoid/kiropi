# Kiropi

**MCP Bridge + OpenAI-Compatible REST API**

Kiropi is a Golang server that acts as a bridge between any application and an MCP (Model Context Protocol) server like Kiro. It exposes an OpenAI-compatible REST API, so any app that speaks OpenAI format can communicate with your MCP server transparently.

## Architecture

```
┌─────────────┐       MCP Protocol        ┌──────────────────────┐       REST API        ┌─────────────────┐
│    Kiro      │ ◄────────────────────────► │      Kiropi          │ ◄───────────────────► │  Your App       │
│  (MCP Server)│    (stdio/SSE)             │  (Bridge + REST)     │    (OpenAI format)    │  (Mobile/Web)   │
└─────────────┘                            └──────────────────────┘                       └─────────────────┘
```

## Features

- OpenAI-compatible REST API (`/v1/chat/completions`)
- **SSE Streaming** support (`"stream": true`)
- MCP client that connects to any MCP server via stdio or SSE
- Bearer token authentication (optional)
- CORS support
- Graceful shutdown
- Health check endpoint
- Cloudflare Tunnel ready (expose to internet without public IP)
- Single binary, runs anywhere (Linux, macOS, Windows)

## Quick Start

### Prerequisites

- Go 1.22+
- An MCP server to connect to

### Run locally

```bash
# Clone
git clone https://github.com/ifcoid/kiropi.git
cd kiropi

# Configure
cp .env.example .env
# Edit .env with your MCP server settings

# Build and run
go build -o kiropi ./cmd/server
./kiropi
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

All configuration is done via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `KIROPI_PORT` | REST API server port | `8080` |
| `KIROPI_MCP_COMMAND` | Command to start MCP server | (required) |
| `KIROPI_MCP_ARGS` | Arguments for MCP server command | |
| `KIROPI_MCP_TRANSPORT` | Transport type: `stdio` or `sse` | `stdio` |
| `KIROPI_MCP_SSE_URL` | SSE URL (if using SSE transport) | |
| `KIROPI_API_KEY` | API key for authentication | (disabled) |
| `KIROPI_MODEL` | Default model name in responses | `kiropi-1` |
| `KIROPI_MAX_CONCURRENT` | Max concurrent requests | `10` |

## API Endpoints

### POST `/v1/chat/completions`

OpenAI-compatible chat completion endpoint.

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "kiropi-1",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

**Response:**

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1717689600,
  "model": "kiropi-1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm doing well. How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 12,
    "total_tokens": 32
  }
}
```

### POST `/api/ask`

Alias for `/v1/chat/completions` (same request/response format).

### GET `/v1/models`

List available models.

### GET `/v1/tools`

List available MCP tools from the connected server.

### GET `/health`

Health check (no auth required).

```json
{
  "status": "ok",
  "mcp_bridge": "connected",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## MCP Server Setup

Kiropi connects to an MCP server via stdio. Your MCP server needs to expose tools that Kiropi can call. At minimum, define an `ask` or `chat` tool:

**Example MCP tool (the server Kiropi connects to should expose):**

```json
{
  "name": "ask",
  "description": "Ask a question and get a response",
  "inputSchema": {
    "type": "object",
    "properties": {
      "prompt": { "type": "string" },
      "context": { "type": "string" }
    },
    "required": ["prompt"]
  }
}
```

## Integration Examples

### Python (using openai library)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-api-key"
)

response = client.chat.completions.create(
    model="kiropi-1",
    messages=[
        {"role": "user", "content": "Explain microservices architecture"}
    ]
)

print(response.choices[0].message.content)
```

### Python (streaming)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-api-key"
)

stream = client.chat.completions.create(
    model="kiropi-1",
    messages=[{"role": "user", "content": "Explain Golang concurrency"}],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()
```

### JavaScript/TypeScript

```typescript
const response = await fetch('http://localhost:8080/v1/chat/completions', {
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

### cURL (streaming)

```bash
curl -N -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "kiropi-1",
    "stream": true,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Deployment with Cloudflare Tunnel

Cloudflare Tunnel lets you expose Kiropi to the internet **without a public IP or port forwarding**. This is the recommended way to let Kiro (or any remote MCP client) connect to your Kiropi instance.

### Architecture

```
┌───────────────┐         ┌──────────────────┐         ┌──────────────────┐
│  Kiro / Apps  │ ◄─HTTPS─► Cloudflare Edge  │ ◄─────► │  cloudflared     │
│  (anywhere)   │         │  (global CDN)    │         │  + Kiropi        │
└───────────────┘         └──────────────────┘         │  (your machine)  │
                                                        └──────────────────┘
```

### Option 1: Quick Tunnel (no account needed)

Perfect for testing. Gets a random `*.trycloudflare.com` URL:

```bash
# Terminal 1: Run Kiropi
export KIROPI_MCP_COMMAND=your-mcp-server
export KIROPI_API_KEY=mysecretkey
./kiropi

# Terminal 2: Expose via Cloudflare
cloudflared tunnel --url http://localhost:8080
```

Output:
```
Your quick Tunnel has been created! Visit it at:
https://random-words-here.trycloudflare.com
```

Now any app can call:
```bash
curl -X POST https://random-words-here.trycloudflare.com/v1/chat/completions \
  -H "Authorization: Bearer mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{"messages": [{"role": "user", "content": "Hello from the internet!"}]}'
```

### Option 2: Named Tunnel (persistent URL)

For production — requires a free Cloudflare account and a domain:

```bash
# 1. Login to Cloudflare
cloudflared tunnel login

# 2. Create a named tunnel
cloudflared tunnel create kiropi

# 3. Route DNS (use your domain)
cloudflared tunnel route dns kiropi kiropi.yourdomain.com

# 4. Create config ~/.cloudflared/config.yml
cat > ~/.cloudflared/config.yml << EOF
tunnel: <TUNNEL_ID>
credentials-file: /root/.cloudflared/<TUNNEL_ID>.json

ingress:
  - hostname: kiropi.yourdomain.com
    service: http://localhost:8080
  - service: http_status:404
EOF

# 5. Run the tunnel
cloudflared tunnel run kiropi
```

Now Kiropi is permanently available at `https://kiropi.yourdomain.com`.

### Option 3: Use the setup script

```bash
./scripts/setup-tunnel.sh
```

Interactive script that handles installation and configuration.

### Run as System Services

```bash
# Kiropi as systemd service
sudo tee /etc/systemd/system/kiropi.service << EOF
[Unit]
Description=Kiropi MCP Bridge
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/kiropi
EnvironmentFile=/etc/kiropi/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Cloudflared as systemd service
sudo cloudflared service install

# Enable and start
sudo systemctl enable --now kiropi
sudo systemctl enable --now cloudflared
```

### Security Notes

- **Always set `KIROPI_API_KEY`** when exposing to the internet
- Cloudflare provides DDoS protection and TLS termination automatically
- Consider using [Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/policies/access/) for additional auth layer
- The `X-Accel-Buffering: no` header is set for SSE compatibility through Cloudflare

## License

MIT
