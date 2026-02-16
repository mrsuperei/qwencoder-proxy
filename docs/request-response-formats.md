# Request and Response Formats

Detailed specification of request and response formats for the Qwencoder Proxy OpenAI-compatible API.

---

## Chat Completion Request

### Request Structure

```json
{
  "model": "string",
  "messages": [
    {
      "role": "system|user|assistant",
      "content": "string"
    }
  ],
  "stream": boolean,
  "temperature": number,
  "max_tokens": integer,
  "top_p": number,
  "frequency_penalty": number,
  "presence_penalty": number
}
```

### Parameters

#### model (required)

The ID of the model to use for the chat completion.

**Type:** `string`

**Examples:**
- `"qwen-max"`
- `"qwen-plus"`
- `"gemini-1.5-pro"`
- `"gemini-1.5-flash"`
- `"claude-3-opus"`
- `"claude-3-sonnet"`

#### messages (required)

An array of message objects representing the conversation history.

**Type:** `array of Message objects`

Each message object has the following structure:

```json
{
  "role": "system|user|assistant",
  "content": "string"
}
```

**Message Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `role` | string | Yes | The role of the message sender. Possible values: `system`, `user`, `assistant` |
| `content` | string | Yes | The content of the message |

**Roles:**

- **system**: Sets the behavior of the assistant. Typically appears at the beginning of the conversation.
- **user**: Messages from the user.
- **assistant**: Messages from the assistant (previous responses).

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful assistant specialized in technical documentation."
    },
    {
      "role": "user",
      "content": "What is REST API?"
    },
    {
      "role": "assistant",
      "content": "REST (Representational State Transfer) is an architectural style for designing networked applications."
    },
    {
      "role": "user",
      "content": "Can you give me an example?"
    }
  ]
}
```

#### stream (optional)

Whether to stream back partial progress. If set, tokens will be sent as data-only [server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events).

**Type:** `boolean`

**Default:** `false`

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Tell me a story"}
  ],
  "stream": true
}
```

#### temperature (optional)

What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.

**Type:** `number`

**Default:** `1.0`

**Range:** `0.0 - 2.0`

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Write a creative story"}
  ],
  "temperature": 0.8
}
```

#### max_tokens (optional)

The maximum number of tokens to generate in the chat completion.

**Type:** `integer`

**Default:** Varies by model

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Summarize this article"}
  ],
  "max_tokens": 100
}
```

#### top_p (optional)

An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

**Type:** `number`

**Default:** `1.0`

**Range:** `0.0 - 1.0`

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Generate text"}
  ],
  "top_p": 0.9
}
```

#### frequency_penalty (optional)

Number between -2.0 and 2.0. Positive values penalize new tokens based on their existing frequency in the text so far, decreasing the model's likelihood to repeat the same line verbatim.

**Type:** `number`

**Default:** `0.0`

**Range:** `-2.0 - 2.0`

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Write about technology"}
  ],
  "frequency_penalty": 0.5
}
```

#### presence_penalty (optional)

Number between -2.0 and 2.0. Positive values penalize new tokens based on whether they appear in the text so far, increasing the model's likelihood to talk about new topics.

**Type:** `number`

**Default:** `0.0`

**Range:** `-2.0 - 2.0`

**Example:**

```json
{
  "model": "qwen-max",
  "messages": [
    {"role": "user", "content": "Write about various topics"}
  ],
  "presence_penalty": 0.5
}
```

---

## Chat Completion Response

### Non-Streaming Response Structure

```json
{
  "id": "string",
  "object": "chat.completion",
  "created": integer,
  "model": "string",
  "choices": [
    {
      "index": integer,
      "message": {
        "role": "assistant",
        "content": "string"
      },
      "finish_reason": "string"
    }
  ],
  "usage": {
    "prompt_tokens": integer,
    "completion_tokens": integer,
    "total_tokens": integer
  }
}
```

### Response Parameters

#### id

A unique identifier for the chat completion.

**Type:** `string`

**Example:** `"chatcmpl-abc123xyz"`

#### object

The object type, always `"chat.completion"`.

**Type:** `string`

#### created

The Unix timestamp (in seconds) of when the chat completion was created.

**Type:** `integer`

**Example:** `1699012345`

#### model

The model used for the chat completion.

**Type:** `string`

**Example:** `"qwen-max"`

#### choices

An array of choice objects. Currently only one choice is supported.

**Type:** `array of Choice objects`

**Choice Object:**

```json
{
  "index": 0,
  "message": {
    "role": "assistant",
    "content": "string"
  },
  "finish_reason": "string"
}
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `index` | integer | The index of the choice in the choices array |
| `message` | object | The message object generated by the model |
| `message.role` | string | Always `"assistant"` |
| `message.content` | string | The content of the assistant's message |
| `finish_reason` | string | The reason the model stopped generating tokens |

**Finish Reasons:**

- `"stop"`: The model hit a natural stop point or a provided stop sequence
- `"length"`: The maximum number of tokens specified in the request was reached
- `"content_filter"`: Omitted content due to a flag from content filters
- `"tool_calls"`: The model called a tool (not currently supported)

#### usage

Usage statistics for the completion request.

**Type:** `Usage object`

```json
{
  "prompt_tokens": integer,
  "completion_tokens": integer,
  "total_tokens": integer
}
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `prompt_tokens` | integer | Number of tokens in the prompt |
| `completion_tokens` | integer | Number of tokens in the generated completion |
| `total_tokens` | integer | Total number of tokens used (prompt + completion) |

### Example Response

```json
{
  "id": "chatcmpl-abc123xyz",
  "object": "chat.completion",
  "created": 1699012345,
  "model": "qwen-max",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris. It's known for its iconic Eiffel Tower, world-class museums like the Louvre, and rich cultural heritage."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 25,
    "total_tokens": 40
  }
}
```

---

## Streaming Response Format

When `stream` is set to `true`, the API sends Server-Sent Events (SSE) with partial results.

### Chunk Structure

Each chunk is a JSON object preceded by `data: `:

```
data: {"id":"...","object":"chat.completion.chunk",...}
```

### Chunk Object

```json
{
  "id": "string",
  "object": "chat.completion.chunk",
  "created": integer,
  "model": "string",
  "choices": [
    {
      "index": integer,
      "delta": {
        "role": "assistant",
        "content": "string"
      },
      "finish_reason": null | "string"
    }
  ]
}
```

### Delta Object

The `delta` object contains only the new content for this chunk:

| Parameter | Type | Description |
|-----------|------|-------------|
| `role` | string | Present only in the first chunk, always `"assistant"` |
| `content` | string | The new text content for this chunk |

### Stream Termination

The stream ends with a final message:

```
data: [DONE]
```

### Example Stream

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" How"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" can"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" I"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" help"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" you"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":" today"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{"content":"?"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1699012345,"model":"qwen-max","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

---

## List Models Response

### Response Structure

```json
{
  "object": "list",
  "data": [
    {
      "id": "string",
      "object": "model",
      "created": integer,
      "owned_by": "string"
    }
  ]
}
```

### Model Object

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | The unique identifier of the model |
| `object` | string | Always `"model"` |
| `created` | integer | The Unix timestamp when the model was created |
| `owned_by` | string | The provider that owns the model |

### Example Response

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

---

## Error Response Format

### Error Object

```json
{
  "error": {
    "message": "string",
    "type": "string"
  }
}
```

### Error Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `error.message` | string | A human-readable error message |
| `error.type` | string | The type of error |

### Error Types

| Type | HTTP Status | Description |
|------|-------------|-------------|
| `invalid_request_error` | 400 | The request was malformed or missing required parameters |
| `authentication_error` | 401 | Authentication failed or credentials are invalid |
| `permission_error` | 403 | The user doesn't have permission to access the resource |
| `not_found_error` | 404 | The requested resource was not found |
| `rate_limit_error` | 429 | Rate limit exceeded |
| `server_error` | 500 | An internal server error occurred |

### Example Error Response

```json
{
  "error": {
    "message": "Model is required",
    "type": "invalid_request_error"
  }
}
```

---

## Protocol Conversion

The proxy automatically handles protocol conversion between OpenAI format and provider-specific formats:

### Supported Protocols

| Provider | Native Protocol | Conversion |
|----------|-----------------|------------|
| Qwen | OpenAI-compatible | Pass-through (no conversion needed) |
| iFlow | OpenAI | Pass-through (no conversion needed) |
| Gemini | Gemini | Automatic conversion to/from OpenAI format |
| Kiro (Claude) | Claude | Automatic conversion to/from OpenAI format |
| Antigravity | Gemini | Automatic conversion to/from OpenAI format |

This means you can use the same OpenAI-compatible request format for all providers, and the proxy handles the necessary conversions transparently.
