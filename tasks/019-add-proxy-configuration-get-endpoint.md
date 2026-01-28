# Task 019: Add Proxy Configuration GET Endpoint

## Description
Implement REST API endpoint for retrieving proxy configuration for a specific token. This endpoint allows clients to view the proxy settings and health status for any token.

## Technical Context
This task implements the GET endpoint for proxy configuration as defined in proxy-per-token architecture.

**Endpoint Specification**:
```
GET /api/credentials/{provider}/{tokenID}/proxy
```

**Response Schema**:
```json
{
  "token_id": "uuid",
  "proxy": {
    "type": "socks5",
    "host": "proxy.example.com",
    "port": 1080,
    "username": "user",
    "password": "***",  // Masked in responses
    "enabled": true
  },
  "health_status": {
    "is_healthy": true,
    "last_check": 1234567890000,
    "average_latency_ms": 150
  }
}
```

**Related Components**:
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Provides token with proxy config
- [`auth/proxy_health_tracker.go`](auth/proxy_health_tracker.go) - Provides health status
- [`restapi/rest_api.go`](restapi/rest_api.go) - REST API server

**Validation Rules**:
- Provider must be valid (qwen, gemini, iflow, kiro, antigravity)
- Token ID must exist for the provider
- Return 404 if token not found
- Mask proxy passwords in responses

**Architectural Principles**:
- **RESTful Design**: GET endpoint for retrieving resource
- **Security**: Passwords masked in responses
- **Error Handling**: Clear error messages for invalid requests

## Subtasks
1. Modify [`restapi/rest_api.go`](restapi/rest_api.go) Server struct:
   - Add handler method for GET /api/credentials/{provider}/{tokenID}/proxy
2. Implement getProxyConfigHandler() method:
   - Parse provider and tokenID from URL path
   - Validate provider type
   - Get token from MultiTokenStore
   - Get proxy health from ProxyHealthTracker
   - Format response with proxy config and health status
   - Mask proxy password (replace with "***")
   - Return 404 if token not found
3. Add route registration:
   - Register GET route for /api/credentials/{provider}/{tokenID}/proxy
   - Ensure route is properly ordered (before catch-all routes)
4. Add error handling:
   - Handle token not found (404)
   - Handle invalid provider type (400)
   - Handle store errors (500)
   - Handle health tracker errors (500)
5. Add logging:
   - Log proxy config retrieval requests
   - Log successful responses
   - Log errors (token not found, store errors)
6. Add unit tests for:
   - Successful proxy config retrieval
   - Token not found (404)
   - Invalid provider type (400)
   - Password masking in response
   - Health status inclusion
   - Error handling

## Dependencies
- Task 004: Extend Token Metadata with Proxy Configuration (uses ProviderToken.Proxy)
- Task 005: Add Proxy Health Tracking to Token Metadata (uses ProxyHealthTracker)

## Files to Modify
- `restapi/rest_api.go` (add GET endpoint handler)
- `restapi/rest_api_test.go` (add tests)

## Related Tasks
- Task 020: Add Proxy Configuration PUT Endpoint (update endpoint)
- Task 021: Add Proxy Configuration DELETE Endpoint (delete endpoint)
