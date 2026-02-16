# Authentication

Guide to configuring and managing authentication for Qwencoder Proxy providers.

---

## Table of Contents

- [Overview](#overview)
- [Provider Authentication](#provider-authentication)
- [Web Dashboard](#web-dashboard)
- [Credential Storage](#credential-storage)
- [OAuth2 Flows](#oauth2-flows)
- [API Authentication](#api-authentication)

---

## Overview

Qwencoder Proxy supports multiple authentication methods depending on the provider:

| Provider | Authentication Method | OAuth2 Support |
|----------|----------------------|----------------|
| Qwen | Device Code Flow | Yes |
| Gemini | API Key | No |
| Kiro (Claude) | API Key | No |
| Antigravity | API Key | No |
| iFlow | API Key | No |

The proxy provides a web dashboard for easy credential management and OAuth2 authentication flows.

---

## Provider Authentication

### Qwen Authentication

Qwen uses OAuth2 Device Code Flow for authentication.

#### Method 1: Web Dashboard (Recommended)

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the Qwen provider section
3. Click "Authenticate" to start the OAuth2 flow
4. Follow the instructions to complete authentication

#### Method 2: Manual Device Code Flow

```bash
# Start the device code flow
curl -X POST http://localhost:8080/api/qwen/device-code

# Response:
{
  "device_code": "abc123...",
  "user_code": "ABCD-EFGH",
  "verification_url": "https://qwen.com/device",
  "expires_in": 900,
  "interval": 5
}

# Visit the verification_url and enter the user_code
# Then poll for token completion
curl -X POST http://localhost:8080/api/qwen/token \
  -H "Content-Type: application/json" \
  -d '{"device_code": "abc123..."}'
```

#### Method 3: Direct API Key

If you have a Qwen API key, you can set it directly:

```bash
curl -X POST http://localhost:8080/api/qwen/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-qwen-api-key"
  }'
```

### Gemini Authentication

Gemini uses API key authentication.

#### Method 1: Web Dashboard

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the Gemini provider section
3. Enter your Gemini API key
4. Click "Save"

#### Method 2: API Endpoint

```bash
curl -X POST http://localhost:8080/api/gemini/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-gemini-api-key"
  }'
```

### Kiro (Claude) Authentication

Kiro uses Anthropic API key authentication.

#### Method 1: Web Dashboard

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the Kiro provider section
3. Enter your Anthropic API key
4. Click "Save"

#### Method 2: API Endpoint

```bash
curl -X POST http://localhost:8080/api/kiro/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-anthropic-api-key"
  }'
```

### Antigravity Authentication

Antigravity uses API key authentication.

#### Method 1: Web Dashboard

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the Antigravity provider section
3. Enter your API key
4. Click "Save"

#### Method 2: API Endpoint

```bash
curl -X POST http://localhost:8080/api/antigravity/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-antigravity-api-key"
  }'
```

### iFlow Authentication

iFlow uses API key authentication.

#### Method 1: Web Dashboard

1. Open the web dashboard at `http://localhost:8080`
2. Navigate to the iFlow provider section
3. Enter your API key
4. Click "Save"

#### Method 2: API Endpoint

```bash
curl -X POST http://localhost:8080/api/iflow/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-iflow-api-key"
  }'
```

---

## Web Dashboard

The Qwencoder Proxy includes a web dashboard for easy credential management.

### Accessing the Dashboard

1. Start the proxy server
2. Open your browser and navigate to `http://localhost:8080`
3. You'll see the dashboard with provider tabs

### Dashboard Features

- **Provider Tabs**: Switch between different providers
- **Authentication Status**: View current authentication status for each provider
- **OAuth2 Flow**: Complete OAuth2 authentication flows directly in the browser
- **API Key Input**: Enter and save API keys
- **Token Management**: View and manage multiple tokens per provider
- **Health Status**: Monitor provider health and token status

### Managing Credentials

#### Adding New Credentials

1. Select the provider tab
2. Click "Add Credentials" or "Authenticate"
3. Follow the authentication flow
4. Credentials are automatically saved

#### Viewing Credentials

1. Select the provider tab
2. View the list of configured tokens
3. See token details including email, expiry, and health status

#### Removing Credentials

1. Select the provider tab
2. Find the token you want to remove
3. Click "Delete" or "Remove"

---

## Credential Storage

### Storage Location

Credentials are stored in the `~/.qwencoder-proxy/` directory (or the directory specified by your configuration).

### Storage Format

Credentials are stored as JSON files:

```json
{
  "tokens": [
    {
      "id": "token-id-1",
      "email": "user@example.com",
      "access_token": "encrypted-token",
      "refresh_token": "encrypted-refresh-token",
      "expiry_date": 1234567890,
      "created_at": 1234567890,
      "last_used": 1234567890,
      "error_count": 0
    }
  ],
  "settings": {
    "proxy_url": "",
    "timeout": 30
  }
}
```

### Security

- Access tokens are encrypted at rest
- Refresh tokens are encrypted at rest
- Credentials are never logged
- Tokens are automatically refreshed when expired

### Multi-Token Support

The proxy supports multiple tokens per provider:

- Load balancing across multiple tokens
- Automatic token rotation
- Health tracking for each token
- Automatic failover to healthy tokens

---

## OAuth2 Flows

### Device Code Flow

The Device Code Flow is used for providers that don't support traditional OAuth2 redirect flows (like Qwen).

#### Flow Steps

1. **Request Device Code**

```bash
curl -X POST http://localhost:8080/api/qwen/device-code
```

Response:
```json
{
  "device_code": "abc123...",
  "user_code": "ABCD-EFGH",
  "verification_url": "https://qwen.com/device",
  "expires_in": 900,
  "interval": 5
}
```

2. **User Authorization**

- Direct the user to `verification_url`
- User enters `user_code`
- User completes authorization

3. **Poll for Token**

```bash
# Poll every 5 seconds (as specified by interval)
curl -X POST http://localhost:8080/api/qwen/token \
  -H "Content-Type: application/json" \
  -d '{"device_code": "abc123..."}'
```

Response (pending):
```json
{
  "error": "authorization_pending"
}
```

Response (success):
```json
{
  "access_token": "ya29...",
  "refresh_token": "1/...",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

4. **Token Storage**

The proxy automatically stores the token and manages refresh cycles.

### Authorization Code Flow

Some providers may support the Authorization Code Flow.

#### Flow Steps

1. **Request Authorization**

```bash
curl -X GET "http://localhost:8080/api/provider/auth?redirect_uri=http://localhost:8080/api/callback"
```

2. **User Authorization**

- User is redirected to provider's authorization page
- User grants permission

3. **Callback Handling**

The proxy handles the callback automatically and exchanges the authorization code for tokens.

---

## API Authentication

### Proxy API Authentication

The proxy's management API (for credential management) does not require authentication by default when accessed locally. You can configure authentication in your configuration file.

### LLM API Authentication

The LLM API endpoints (`/v1/chat/completions`, `/v1/models`) do not require API keys. The proxy uses the configured provider credentials internally.

### Adding API Key Authentication (Optional)

If you want to add API key authentication to the LLM API endpoints:

1. Add middleware to your configuration
2. Configure the allowed API keys
3. Clients must include the API key in the `Authorization` header:

```bash
curl -X POST http://localhost:8143/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "qwen-max",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## Token Management API

### List Tokens

Get all tokens for a provider:

```bash
curl -X GET http://localhost:8080/api/qwen/tokens
```

Response:
```json
{
  "tokens": [
    {
      "id": "token-id-1",
      "email": "user@example.com",
      "expiry_date": 1234567890,
      "expires_in": 3600,
      "token_type": "Bearer",
      "healthy": true,
      "health_score": 1.0,
      "last_used": 1234567890,
      "created_at": 1234567890,
      "error_count": 0
    }
  ]
}
```

### Get Token Info

Get information about a specific token:

```bash
curl -X GET http://localhost:8080/api/qwen/tokens/token-id-1
```

### Delete Token

Delete a specific token:

```bash
curl -X DELETE http://localhost:8080/api/qwen/tokens/token-id-1
```

### Clear All Tokens

Clear all tokens for a provider:

```bash
curl -X DELETE http://localhost:8080/api/qwen/tokens
```

---

## Proxy Configuration

### Per-Token Proxy Settings

Each token can have its own proxy configuration:

```bash
curl -X POST http://localhost:8080/api/qwen/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "api_key": "your-api-key",
    "proxy_url": "http://proxy.example.com:8080",
    "timeout": 30
  }'
```

### Provider-Level Proxy Settings

Set proxy settings for all tokens of a provider:

```bash
curl -X POST http://localhost:8080/api/qwen/settings \
  -H "Content-Type: application/json" \
  -d '{
    "proxy_url": "http://proxy.example.com:8080",
    "timeout": 30
  }'
```

---

## Troubleshooting Authentication

### Common Issues

#### 1. Token Expired

**Symptom:** Requests fail with authentication errors.

**Solution:** The proxy automatically refreshes expired tokens. If automatic refresh fails, re-authenticate through the web dashboard.

#### 2. Invalid API Key

**Symptom:** 401 Unauthorized errors.

**Solution:** Verify your API key is correct and has the necessary permissions.

#### 3. OAuth2 Flow Fails

**Symptom:** Device code flow doesn't complete.

**Solution:**
- Ensure you're using the correct verification URL
- Make sure you enter the user code correctly
- Check that the device code hasn't expired (typically 15 minutes)

#### 4. Proxy Connection Issues

**Symptom:** Requests timeout or fail to connect.

**Solution:**
- Verify proxy URL is correct
- Check proxy server is accessible
- Ensure proxy supports HTTPS if required

#### 5. Multi-Token Load Balancing Issues

**Symptom:** Requests are not distributed across tokens.

**Solution:**
- Ensure all tokens are healthy
- Check token health scores in the dashboard
- Remove unhealthy tokens

### Debug Mode

Enable debug logging to troubleshoot authentication issues:

```bash
# Set environment variable
export QWENCODER_DEBUG=true

# Or in your config file
debug: true
```

### Checking Token Health

Monitor token health through the dashboard or API:

```bash
curl -X GET http://localhost:8080/api/qwen/tokens
```

Look for:
- `healthy`: true/false
- `health_score`: 0.0 - 1.0
- `error_count`: number of recent errors

---

## Security Best Practices

1. **Never commit credentials to version control**
   - Add `~/.qwencoder-proxy/` to `.gitignore`
   - Use environment variables for sensitive data

2. **Use strong API keys**
   - Generate long, random API keys
   - Rotate API keys regularly

3. **Limit token permissions**
   - Use tokens with minimal required permissions
   - Create separate tokens for different environments

4. **Monitor token usage**
   - Regularly check token health and usage
   - Set up alerts for unusual activity

5. **Secure the dashboard**
   - Use authentication for the management API in production
   - Restrict dashboard access to trusted networks

6. **Encrypt credentials at rest**
   - The proxy encrypts stored credentials by default
   - Ensure file system permissions are correct

---

## Configuration Examples

### Environment Variables

```bash
# Qwen credentials
export QWEN_API_KEY="your-qwen-api-key"

# Gemini credentials
export GEMINI_API_KEY="your-gemini-api-key"

# Kiro credentials
export KIRO_API_KEY="your-anthropic-api-key"

# Antigravity credentials
export ANTIGRAVITY_API_KEY="your-antigravity-api-key"

# iFlow credentials
export IFLOW_API_KEY="your-iflow-api-key"
```

### Configuration File

```yaml
providers:
  qwen:
    api_key: "your-qwen-api-key"
    proxy_url: ""
    timeout: 30
  
  gemini:
    api_key: "your-gemini-api-key"
    proxy_url: ""
    timeout: 30
  
  kiro:
    api_key: "your-anthropic-api-key"
    proxy_url: ""
    timeout: 30
  
  antigravity:
    api_key: "your-antigravity-api-key"
    proxy_url: ""
    timeout: 30
  
  iflow:
    api_key: "your-iflow-api-key"
    proxy_url: ""
    timeout: 30
```
