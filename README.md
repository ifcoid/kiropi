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
- MCP client that connects to any MCP server via stdio or SSE
- Bearer token authentication (optional)
- CORS support
- Graceful shutdown
- Health check endpoint
- Docker support

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

### Run with Docker

```bash
docker build -t kiropi .
docker run -p 8080:8080 --env-file .env kiropi
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

## License

MIT
