# API Reference

Complete API reference for Qwencoder Proxy's OpenAI-compatible endpoints.

## Base URL

```
http://localhost:8143
```

## Authentication

The proxy uses provider-specific authentication. See the [Authentication Guide](./authentication.md) for details on configuring credentials for each provider.

---

## Endpoints

### List Models

Lists all available models from all registered providers.

#### Endpoint

```
GET /v1/models
```

#### Request

```bash
curl -X GET http://localhost:8143/v1/models
```

#### Response

```json
{
  "object": "list",
  "data": [
    {
      "id": "qwen-max",
      "object": "model",
      "created": 1234567890,
      "owned_by": "qwen"
    },
    {
      "id": "qwen-plus",
      "object": "model",
      "created": 1234567890,
      "owned_by": "qwen"
    },
    {
      "id": "gemini-1.5-pro",
      "object": "model",
      "created": 1234567890,
      "owned_by": "gemini-cli"
    },
    {
      "id": "gemini-1.5-flash",
      "object": "model",
      "created": 1234567890,
      "owned_by": "gemini-cli"
    },
    {
      "id": "claude-3-opus",
      "object": "model",
      "created": 1234567890,
      "owned_by": "kiro"
    },
    {
      "id": "claude-3-sonnet",
      "object": "model",
      "created": 1234567890,
      "owned_by": "kiro"
    }
  ]
}
```

#### Provider-Specific Models

List models from a specific provider only:

```bash
# Qwen models only
curl -X GET http://localhost:8143/qwen/v1/models

# Gemini models only
curl -X GET http://localhost:8143/gemini/v1/models

# Kiro (Claude) models only
curl -X GET http://localhost:8143/kiro/v1/models

# Antigravity models only
curl -X GET http://localhost:8143/antigravity/v1/models

# iFlow models only
curl -X GET http://localhost:8143/iflow/v1/models
```

---

### Create Chat Completion

Creates a model response for the given chat conversation.

#### Endpoint

```
POST /v1/chat/completions
```

#### Request Headers

```
Content-Type: application/json
```

#### Request Body

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | ID of the model to use |
| `messages` | array | Yes | Array of message objects |
| `stream` | boolean | No | Enable streaming responses (default: false) |
| `temperature` | number | No | Sampling temperature (0-2) |
| `max_tokens` | integer | No | Maximum tokens to generate |
| `top_p` | number | No | Nucleus sampling threshold |
| `frequency_penalty` | number | No | Frequency penalty (-2.0 to 2.0) |
| `presence_penalty` | number | No | Presence penalty (-2.0 to 2.0) |

#### Message Object

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `role` | string | Yes | Role of the message sender (`system`, `user`, `assistant`) |
| `content` | string | Yes | Content of the message |

#### Basic Example

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

#### Response

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1699012345,
  "model": "qwen-max",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm doing well, thank you for asking. How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}
```

#### Multi-turn Conversation

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"},
      {"role": "assistant", "content": "The capital of France is Paris."},
      {"role": "user", "content": "And what about Germany?"}
    ]
  }'
```

#### With Parameters

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Write a short poem about technology"}
    ],
    "temperature": 0.7,
    "max_tokens": 100,
    "top_p": 0.9
  }'
```

---

### Streaming Chat Completion

Creates a streaming chat completion response using Server-Sent Events (SSE).

#### Endpoint

```
POST /v1/chat/completions
```

#### Request

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Tell me a joke"}
    ],
    "stream": true
  }'
```

#### Streaming Response

The response is sent as a stream of Server-Sent Events:

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":"Why"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" don"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":"'t"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" scientists"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" trust"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" atoms?"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" Because"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" they"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" make"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" up"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" everything"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

#### Streaming with Python

```python
import requests
import json

response = requests.post(
    "http://localhost:8143/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={
        "model": "qwen-max",
        "messages": [{"role": "user", "content": "Tell me a joke"}],
        "stream": True
    },
    stream=True
)

for line in response.iter_lines():
    if line:
        line = line.decode('utf-8')
        if line.startswith('data: '):
            data = line[6:]  # Remove 'data: ' prefix
            if data == '[DONE]':
                break
            chunk = json.loads(data)
            if 'choices' in chunk and len(chunk['choices']) > 0:
                delta = chunk['choices'][0].get('delta', {})
                content = delta.get('content', '')
                if content:
                    print(content, end='', flush=True)
```

---

## Provider-Specific Endpoints

Force requests to use a specific provider by using provider-specific prefixes.

### Qwen Provider

#### List Qwen Models

```bash
curl -X GET http://localhost:8143/qwen/v1/models
```

#### Chat Completion with Qwen

```bash
curl -X POST http://localhost:8143/qwen/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### Gemini Provider

#### List Gemini Models

```bash
curl -X GET http://localhost:8143/gemini/v1/models
```

#### Chat Completion with Gemini

```bash
curl -X POST http://localhost:8143/gemini/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Explain quantum computing"}
    ]
  }'
```

### Kiro (Claude) Provider

#### List Kiro Models

```bash
curl -X GET http://localhost:8143/kiro/v1/models
```

#### Chat Completion with Kiro

```bash
curl -X POST http://localhost:8143/kiro/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-opus",
    "messages": [
      {"role": "user", "content": "Write a haiku"}
    ]
  }'
```

### Antigravity Provider

#### List Antigravity Models

```bash
curl -X GET http://localhost:8143/antigravity/v1/models
```

#### Chat Completion with Antigravity

```bash
curl -X POST http://localhost:8143/antigravity/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

### iFlow Provider

#### List iFlow Models

```bash
curl -X GET http://localhost:8143/iflow/v1/models
```

#### Chat Completion with iFlow

```bash
curl -X POST http://localhost:8143/iflow/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

---

## Error Responses

The API returns standard HTTP status codes and error messages:

### 400 Bad Request

```json
{
  "error": {
    "message": "Model is required",
    "type": "invalid_request_error"
  }
}
```

### 404 Not Found

```json
{
  "error": {
    "message": "Provider not found: unknown-model",
    "type": "invalid_request_error"
  }
}
```

### 500 Internal Server Error

```json
{
  "error": {
    "message": "Failed to convert request",
    "type": "server_error"
  }
}
```

---

## CORS Support

The proxy supports Cross-Origin Resource Sharing (CORS). Configure CORS settings in your configuration file.

---

## Rate Limiting

Rate limiting is handled by the individual providers. The proxy does not impose additional rate limits.
