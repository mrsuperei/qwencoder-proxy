# cURL Examples

Pure cURL examples for using the Qwencoder Proxy chat endpoint.

---

## Table of Contents

- [Basic Chat Completion](#basic-chat-completion)
- [Multi-turn Conversation](#multi-turn-conversation)
- [Streaming Response](#streaming-response)
- [Parameters](#parameters)
  - [Temperature](#temperature)
  - [Max Tokens](#max-tokens)
  - [Top P](#top-p)
  - [Frequency Penalty](#frequency-penalty)
  - [Presence Penalty](#presence-penalty)
  - [Multiple Parameters](#multiple-parameters)
- [Provider-Specific Requests](#provider-specific-requests)
- [List Models](#list-models)

---

## Basic Chat Completion

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

---

## Multi-turn Conversation

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

### Multi-turn with System Message

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "system", "content": "You are a helpful coding assistant. Provide clear and concise code examples."},
      {"role": "user", "content": "How do I create a simple HTTP server in Python?"},
      {"role": "assistant", "content": "You can use the built-in http.server module. Here is a simple example:\n\n```python\nimport http.server\nimport socketserver\n\nPORT = 8000\n\nHandler = http.server.SimpleHTTPRequestHandler\n\nwith socketserver.TCPServer(("", PORT), Handler) as httpd:\n    print(f"Serving at port {PORT}")\n    httpd.serve_forever()\n```\n\nThis creates a basic server that serves files from the current directory."},
      {"role": "user", "content": "Can you explain what each part does?"}
    ]
  }'
```

---

## Streaming Response

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Tell me a short story about a robot learning to paint."}
    ],
    "stream": true
  }'
```

### Streaming with Multi-turn Conversation

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Explain quantum computing in simple terms."},
      {"role": "assistant", "content": "Quantum computing is like having a magical coin that can be heads and tails at the same time. Regular computers use bits (0 or 1), but quantum computers use qubits that can be 0, 1, or both simultaneously."},
      {"role": "user", "content": "Can you give me a practical example of how it could be used?"}
    ],
    "stream": true
  }'
```

---

## Parameters

### Temperature

Controls randomness in the response. Lower values (0.0-0.3) make the output more focused and deterministic, while higher values (0.7-2.0) make it more random and creative.

```bash
# Low temperature (more focused)
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Write a technical definition of REST API."}
    ],
    "temperature": 0.2
  }'
```

```bash
# High temperature (more creative)
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Write a creative story about a cloud."}
    ],
    "temperature": 1.5
  }'
```

### Max Tokens

Limits the maximum number of tokens in the response.

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Explain the history of the internet."}
    ],
    "max_tokens": 100
  }'
```

### Top P

Alternative to temperature, controls diversity via nucleus sampling.

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Generate a unique business idea."}
    ],
    "top_p": 0.9
  }'
```

### Frequency Penalty

Penalizes new tokens based on their existing frequency in the text. Positive values decrease the likelihood of repeating the same content.

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Write about the importance of diversity in teams."}
    ],
    "frequency_penalty": 0.8
  }'
```

### Presence Penalty

Penalizes new tokens based on whether they appear in the text so far. Positive values encourage talking about new topics.

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Discuss various aspects of artificial intelligence."}
    ],
    "presence_penalty": 0.6
  }'
```

### Multiple Parameters

Combining multiple parameters for fine-tuned control.

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Write a comprehensive overview of machine learning."}
    ],
    "temperature": 0.7,
    "max_tokens": 500,
    "top_p": 0.95,
    "frequency_penalty": 0.3,
    "presence_penalty": 0.4
  }'
```

---

## Provider-Specific Requests

Force the request to use a specific provider by including the provider name in the URL path.

### Qwen Provider

```bash
curl -X POST http://localhost:8143/qwen/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Hello from Qwen!"}
    ]
  }'
```

### Gemini Provider

```bash
curl -X POST http://localhost:8143/gemini/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Hello from Gemini!"}
    ]
  }'
```

### Kiro Provider (Claude)

```bash
curl -X POST http://localhost:8143/kiro/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-opus",
    "messages": [
      {"role": "user", "content": "Hello from Claude!"}
    ]
  }'
```

### Antigravity Provider

```bash
curl -X POST http://localhost:8143/antigravity/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello from Antigravity!"}
    ]
  }'
```

### iFlow Provider

```bash
curl -X POST http://localhost:8143/iflow/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello from iFlow!"}
    ]
  }'
```

---

## List Models

Get a list of all available models from all registered providers.

```bash
curl -X GET http://localhost:8143/v1/models
```

### List Models from Specific Provider

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

## Advanced Examples

### Streaming with Low Temperature

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "system", "content": "You are a technical documentation writer."},
      {"role": "user", "content": "Explain the concept of microservices architecture."}
    ],
    "stream": true,
    "temperature": 0.3,
    "max_tokens": 300
  }'
```

### Multi-turn Conversation with Multiple Parameters

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "system", "content": "You are an expert software architect."},
      {"role": "user", "content": "What are the key principles of clean code?"},
      {"role": "assistant", "content": "Clean code principles include:\n1. Meaningful names\n2. Small functions\n3. Single Responsibility Principle\n4. DRY (Don't Repeat Yourself)\n5. Comments should explain why, not what"},
      {"role": "user", "content": "Can you elaborate on the Single Responsibility Principle?"}
    ],
    "temperature": 0.5,
    "max_tokens": 400,
    "top_p": 0.9
  }'
```

### Provider-Specific Streaming with Custom Parameters

```bash
curl -X POST http://localhost:8143/qwen/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-plus",
    "messages": [
      {"role": "user", "content": "Write a poem about programming."}
    ],
    "stream": true,
    "temperature": 0.9,
    "frequency_penalty": 0.5,
    "presence_penalty": 0.3
  }'
```
