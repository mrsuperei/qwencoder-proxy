# Usage Examples

Practical examples and integration guides for using Qwencoder Proxy with various programming languages and frameworks.

---

## Table of Contents

- [cURL Examples](#curl-examples)
- [Python Examples](#python-examples)
- [JavaScript/Node.js Examples](#javascriptnodejs-examples)
- [Go Examples](#go-examples)
- [OpenAI SDK Integration](#openai-sdk-integration)
- [LangChain Integration](#langchain-integration)

---

## cURL Examples

### Basic Chat Completion

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

### Streaming Response

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

### Multi-turn Conversation

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is Python?"},
      {"role": "assistant", "content": "Python is a high-level programming language."},
      {"role": "user", "content": "What are its main features?"}
    ]
  }'
```

### With Temperature and Max Tokens

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-pro",
    "messages": [
      {"role": "user", "content": "Write a short story"}
    ],
    "temperature": 0.8,
    "max_tokens": 200
  }'
```

### List All Models

```bash
curl -X GET http://localhost:8143/v1/models
```

### Provider-Specific Request

```bash
# Force using Qwen provider
curl -X POST http://localhost:8143/qwen/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-max",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

---

## Python Examples

### Using requests Library

```python
import requests
import json

# Basic chat completion
response = requests.post(
    "http://localhost:8143/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={
        "model": "qwen-max",
        "messages": [
            {"role": "user", "content": "Hello, how are you?"}
        ]
    }
)

result = response.json()
print(result["choices"][0]["message"]["content"])
```

### Streaming with Python

```python
import requests
import json

response = requests.post(
    "http://localhost:8143/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={
        "model": "qwen-max",
        "messages": [
            {"role": "user", "content": "Tell me a story"}
        ],
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

### Using OpenAI Python SDK

```python
from openai import OpenAI

# Configure the client to use the proxy
client = OpenAI(
    base_url="http://localhost:8143/v1",
    api_key="dummy-key"  # Required by SDK but not used by proxy
)

# Basic chat completion
response = client.chat.completions.create(
    model="qwen-max",
    messages=[
        {"role": "user", "content": "Hello, how are you?"}
    ]
)

print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="qwen-max",
    messages=[
        {"role": "user", "content": "Tell me a story"}
    ],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end='', flush=True)
```

### Multi-turn Conversation with Context

```python
import requests

class ChatSession:
    def __init__(self, model="qwen-max"):
        self.model = model
        self.messages = []
    
    def add_system_message(self, content):
        self.messages.insert(0, {"role": "system", "content": content})
    
    def add_user_message(self, content):
        self.messages.append({"role": "user", "content": content})
    
    def add_assistant_message(self, content):
        self.messages.append({"role": "assistant", "content": content})
    
    def send(self):
        response = requests.post(
            "http://localhost:8143/v1/chat/completions",
            headers={"Content-Type": "application/json"},
            json={
                "model": self.model,
                "messages": self.messages
            }
        )
        result = response.json()
        assistant_message = result["choices"][0]["message"]["content"]
        self.add_assistant_message(assistant_message)
        return assistant_message

# Usage
session = ChatSession(model="qwen-max")
session.add_system_message("You are a helpful coding assistant.")

response1 = session.send()
print(f"Assistant: {response1}")

session.add_user_message("Can you help me write a Python function?")
response2 = session.send()
print(f"Assistant: {response2}")
```

---

## JavaScript/Node.js Examples

### Using fetch API

```javascript
// Basic chat completion
async function chatCompletion() {
  const response = await fetch('http://localhost:8143/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: 'qwen-max',
      messages: [
        { role: 'user', content: 'Hello, how are you?' }
      ]
    })
  });

  const data = await response.json();
  console.log(data.choices[0].message.content);
}

chatCompletion();
```

### Streaming with fetch API

```javascript
async function streamingChatCompletion() {
  const response = await fetch('http://localhost:8143/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: 'qwen-max',
      messages: [
        { role: 'user', content: 'Tell me a story' }
      ],
      stream: true
    })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const chunk = decoder.decode(value);
    const lines = chunk.split('\n');

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const data = line.slice(6);
        if (data === '[DONE]') continue;
        
        try {
          const parsed = JSON.parse(data);
          const content = parsed.choices[0]?.delta?.content;
          if (content) {
            process.stdout.write(content);
          }
        } catch (e) {
          // Skip invalid JSON
        }
      }
    }
  }
}

streamingChatCompletion();
```

### Using OpenAI Node.js SDK

```javascript
import OpenAI from 'openai';

// Configure the client to use the proxy
const client = new OpenAI({
  baseURL: 'http://localhost:8143/v1',
  apiKey: 'dummy-key' // Required by SDK but not used by proxy
});

// Basic chat completion
async function chatCompletion() {
  const response = await client.chat.completions.create({
    model: 'qwen-max',
    messages: [
      { role: 'user', content: 'Hello, how are you?' }
    ]
  });

  console.log(response.choices[0].message.content);
}

chatCompletion();

// Streaming
async function streamingChatCompletion() {
  const stream = await client.chat.completions.create({
    model: 'qwen-max',
    messages: [
      { role: 'user', content: 'Tell me a story' }
    ],
    stream: true
  });

  for await (const chunk of stream) {
    const content = chunk.choices[0]?.delta?.content;
    if (content) {
      process.stdout.write(content);
    }
  }
}

streamingChatCompletion();
```

### Multi-turn Conversation Class

```javascript
class ChatSession {
  constructor(model = 'qwen-max') {
    this.model = model;
    this.messages = [];
  }

  addSystemMessage(content) {
    this.messages.unshift({ role: 'system', content });
  }

  addUserMessage(content) {
    this.messages.push({ role: 'user', content });
  }

  async send() {
    const response = await fetch('http://localhost:8143/v1/chat/completions', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: this.model,
        messages: this.messages
      })
    });

    const data = await response.json();
    const assistantMessage = data.choices[0].message.content;
    this.messages.push({ role: 'assistant', content: assistantMessage });
    return assistantMessage;
  }
}

// Usage
async function main() {
  const session = new ChatSession('qwen-max');
  session.addSystemMessage('You are a helpful coding assistant.');

  const response1 = await session.send();
  console.log(`Assistant: ${response1}`);

  session.addUserMessage('Can you help me write a JavaScript function?');
  const response2 = await session.send();
  console.log(`Assistant: ${response2}`);
}

main();
```

---

## Go Examples

### Using net/http

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Choice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

func chatCompletion() error {
	reqBody := ChatRequest{
		Model: "qwen-max",
		Messages: []Message{
			{Role: "user", Content: "Hello, how are you?"},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		"http://localhost:8143/v1/chat/completions",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result ChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	fmt.Println(result.Choices[0].Message.Content)
	return nil
}

func main() {
	if err := chatCompletion(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
```

### Streaming in Go

```go
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type Choice struct {
	Index   int   `json:"index"`
	Delta   Delta `json:"delta"`
}

type StreamChunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

func streamingChatCompletion() error {
	reqBody := map[string]interface{}{
		"model": "qwen-max",
		"messages": []Message{
			{Role: "user", Content: "Tell me a story"},
		},
		"stream": true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		"http://localhost:8143/v1/chat/completions",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 6 && line[:6] == "data: " {
			data := line[6:]
			if data == "[DONE]" {
				break
			}

			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					fmt.Print(content)
				}
			}
		}
	}

	fmt.Println()
	return scanner.Err()
}

func main() {
	if err := streamingChatCompletion(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
```

---

## OpenAI SDK Integration

The Qwencoder Proxy is fully compatible with the official OpenAI SDKs. Simply change the `base_url` to point to the proxy.

### Python (openai library)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8143/v1",
    api_key="dummy-key"  # Required but not used by proxy
)

# Use exactly like OpenAI API
response = client.chat.completions.create(
    model="qwen-max",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Explain quantum computing"}
    ],
    temperature=0.7,
    max_tokens=500
)

print(response.choices[0].message.content)
```

### JavaScript (openai npm package)

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:8143/v1',
  apiKey: 'dummy-key' // Required but not used by proxy
});

const response = await client.chat.completions.create({
  model: 'qwen-max',
  messages: [
    { role: 'system', content: 'You are a helpful assistant.' },
    { role: 'user', content: 'Explain quantum computing' }
  ],
  temperature: 0.7,
  max_tokens: 500
});

console.log(response.choices[0].message.content);
```

### Go (github.com/sashabaranov/go-openai)

```go
package main

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

func main() {
	config := openai.DefaultConfig("dummy-key")
	config.BaseURL = "http://localhost:8143/v1"

	client := openai.NewClientWithConfig(config)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a helpful assistant.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Explain quantum computing",
				},
			},
			Temperature: 0.7,
			MaxTokens:   500,
		},
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(resp.Choices[0].Message.Content)
}
```

---

## LangChain Integration

The proxy works seamlessly with LangChain by configuring the OpenAI-compatible endpoint.

### Python

```python
from langchain_openai import ChatOpenAI
from langchain.schema import HumanMessage, SystemMessage

# Configure LangChain to use the proxy
llm = ChatOpenAI(
    base_url="http://localhost:8143/v1",
    api_key="dummy-key",  # Required but not used by proxy
    model="qwen-max",
    temperature=0.7
)

# Basic usage
response = llm.invoke([HumanMessage(content="Hello, how are you?")])
print(response.content)

# With system message
messages = [
    SystemMessage(content="You are a helpful coding assistant."),
    HumanMessage(content="Write a Python function to calculate fibonacci numbers.")
]
response = llm.invoke(messages)
print(response.content)

# Streaming
for chunk in llm.stream("Tell me a story"):
    print(chunk.content, end="", flush=True)
```

### JavaScript

```javascript
import { ChatOpenAI } from "@langchain/openai";

// Configure LangChain to use the proxy
const llm = new ChatOpenAI({
  configuration: {
    baseURL: "http://localhost:8143/v1",
    apiKey: "dummy-key", // Required but not used by proxy
  },
  model: "qwen-max",
  temperature: 0.7,
});

// Basic usage
const response = await llm.invoke("Hello, how are you?");
console.log(response.content);

// Streaming
const stream = await llm.stream("Tell me a story");
for await (const chunk of stream) {
  process.stdout.write(chunk.content);
}
```

---

## Best Practices

### 1. Error Handling

Always implement proper error handling:

```python
import requests
from requests.exceptions import RequestException

try:
    response = requests.post(
        "http://localhost:8143/v1/chat/completions",
        headers={"Content-Type": "application/json"},
        json={"model": "qwen-max", "messages": [...]}
    )
    response.raise_for_status()
    result = response.json()
except RequestException as e:
    print(f"Request failed: {e}")
except KeyError as e:
    print(f"Unexpected response format: {e}")
```

### 2. Timeout Management

Set appropriate timeouts:

```python
response = requests.post(
    "http://localhost:8143/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={...},
    timeout=30  # 30 second timeout
)
```

### 3. Streaming for Long Responses

Use streaming for long responses to improve user experience:

```python
# For long-form content generation
response = requests.post(
    "http://localhost:8143/v1/chat/completions",
    headers={"Content-Type": "application/json"},
    json={
        "model": "qwen-max",
        "messages": [{"role": "user", "content": "Write a long article"}],
        "stream": True
    },
    stream=True
)
```

### 4. Context Management

Manage conversation context efficiently:

```python
# Keep only recent messages to stay within token limits
MAX_CONTEXT_LENGTH = 10

class ChatSession:
    def __init__(self):
        self.messages = []
    
    def add_message(self, role, content):
        self.messages.append({"role": role, "content": content})
        # Keep only recent messages
        if len(self.messages) > MAX_CONTEXT_LENGTH:
            # Keep system message if exists
            if self.messages[0]["role"] == "system":
                self.messages = [self.messages[0]] + self.messages[-(MAX_CONTEXT_LENGTH-1):]
            else:
                self.messages = self.messages[-MAX_CONTEXT_LENGTH:]
```

### 5. Provider Selection

Choose the right provider for your use case:

```python
# For coding tasks
model = "qwen-max"  # Good for coding

# For creative writing
model = "claude-3-opus"  # Excellent for creative tasks

# For fast responses
model = "gemini-1.5-flash"  # Fast and cost-effective

# Force specific provider
response = requests.post(
    "http://localhost:8143/qwen/v1/chat/completions",  # Force Qwen
    headers={"Content-Type": "application/json"},
    json={"model": "qwen-max", "messages": [...]}
)
```
