# Qwencoder Proxy API Documentation

Welcome to the comprehensive API documentation for Qwencoder Proxy. This proxy provides a unified, OpenAI-compatible interface for multiple LLM providers.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Documentation](#documentation)

## Overview

Qwencoder Proxy is a unified interface for multiple LLM providers that offers:

- **OpenAI-Compatible API**: Full compatibility with OpenAI's `/v1/chat/completions` and `/v1/models` endpoints
- **Multi-Provider Support**: Integrate with Qwen, Gemini, Claude (Kiro), Antigravity, and iFlow
- **Smart Routing**: Automatically routes requests to the appropriate provider based on the model name
- **Protocol Conversion**: Seamlessly converts between OpenAI format and provider-specific formats
- **Streaming Support**: Full support for streaming responses using Server-Sent Events (SSE)
- **Provider-Specific Routes**: Force requests to specific providers using dedicated endpoints

## Quick Start

### Installation

```bash
git clone https://github.com/sunbankio/qwencoder-proxy.git
cd qwencoder-proxy
go build -o qwencoder-proxy cmd/qwencoder-proxy/main.go
```

### Running the Server

```bash
./qwencoder-proxy
```

The server listens on port defined in your config (default: `8143`).

### Basic Usage

```bash
# List all available models
curl http://localhost:8143/v1/models

# Send a chat completion request
curl http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Documentation

### Core Documentation

| Document | Description |
|----------|-------------|
| [API Reference](./api-reference.md) | Complete API reference with all endpoints, parameters, and curl examples |
| [Request/Response Formats](./request-response-formats.md) | Detailed specification of request and response formats |
| [Usage Examples](./usage-examples.md) | Practical examples and integration guides |
| [Authentication](./authentication.md) | Authentication and credential management |
| [Troubleshooting](./troubleshooting.md) | Common issues and solutions |

### API Endpoints

#### OpenAI-Compatible Endpoints

- `GET /v1/models` - List all available models
- `POST /v1/chat/completions` - Create chat completions

#### Provider-Specific Endpoints

- `GET /qwen/v1/models` - List Qwen models only
- `POST /qwen/v1/chat/completions` - Force Qwen provider
- `GET /gemini/v1/models` - List Gemini models only
- `POST /gemini/v1/chat/completions` - Force Gemini provider
- `GET /kiro/v1/models` - List Kiro (Claude) models only
- `POST /kiro/v1/chat/completions` - Force Kiro provider
- `GET /antigravity/v1/models` - List Antigravity models only
- `POST /antigravity/v1/chat/completions` - Force Antigravity provider
- `GET /iflow/v1/models` - List iFlow models only
- `POST /iflow/v1/chat/completions` - Force iFlow provider

## Supported Providers

| Provider | Protocol | Models |
|----------|----------|--------|
| Qwen | OpenAI-compatible | qwen-max, qwen-plus, qwen-turbo |
| Gemini | Gemini-native | gemini-1.5-pro, gemini-1.5-flash |
| Kiro (Claude) | Claude-native | claude-3-opus, claude-3-sonnet |
| Antigravity | Gemini-native | Various Gemini models |
| iFlow | OpenAI-compatible | Various open-source models |

## Key Features

### Smart Routing

When using the general `/v1/*` endpoints, the proxy automatically routes requests to the appropriate provider based on the `model` parameter:

```bash
# Automatically routed to Qwen provider
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "qwen-max", "messages": [...]}'

# Automatically routed to Gemini provider
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-1.5-pro", "messages": [...]}'
```

### Provider-Specific Routing

Force requests to use a specific provider by using provider-specific prefixes:

```bash
# Force using Qwen provider
curl -X POST http://localhost:8143/qwen/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "qwen-max", "messages": [...]}'

# Force using Gemini provider
curl -X POST http://localhost:8143/gemini/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gemini-1.5-pro", "messages": [...]}'
```

### Streaming Responses

Full streaming support using Server-Sent Events:

```bash
curl http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

## Configuration

The proxy can be configured through environment variables or a configuration file. See the main [README.md](../README.md) for detailed configuration options.

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/sunbankio/qwencoder-proxy).
