# Task 021: Add Proxy Configuration DELETE Endpoint

## Description
Implement REST API endpoint for removing proxy configuration from a specific token. This endpoint allows clients to disable proxy for a token by removing its proxy configuration.

## Technical Context
This task implements the DELETE endpoint for proxy configuration as defined in proxy-per-token architecture.

**Endpoint Specification**:
```
DELETE /api/credentials/{provider}/{tokenID}/proxy
```

**Response Schema** (Success):
```json
{
  "success": true,
  "message": "Proxy configuration removed"
}
```

**Response Schema** (Error):
```json
{
  "error": "not_found",
  "message": "Token not found",
  "details": {
    "provider": "qwen",
    "token_id": "uuid"
  }
}
```

**Related Components**:
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Updates token to remove proxy config
- [`restapi/rest_api.go`](restapi/rest_api.go) - REST API server

**Behavior**:
- Sets token's Proxy field to nil
- Token will use direct connection (no proxy) after deletion
- Proxy health score reset to 1.0 (healthy)
- Client cache cleared to force new client creation

**Validation Rules**:
- Provider must be valid (qwen, gemini, iflow, kiro, antigravity)
- Token ID must exist for the provider
- Return 404 if token not found
- Return 400 if token has no proxy configured (optional: return success anyway)

**Security Considerations**:
- Log all proxy deletion requests (audit trail)
- Clear client cache on proxy config change
- Return success even if no proxy configured (idempotent)

**Architectural Principles**:
- **RESTful Design**: DELETE endpoint for removing resource
- **Idempotency**: Multiple deletes have same effect
- **Security**: Audit logging for all changes

## Subtasks
1. Modify [`restapi/rest_api.go`](restapi/rest_api.go) Server struct:
   - Add handler method for DELETE /api/credentials/{provider}/{tokenID}/proxy
2. Implement deleteProxyConfigHandler() method:
   - Parse provider and tokenID from URL path
   - Validate provider type
   - Get token from MultiTokenStore
   - Check if token has proxy configured
   - Set token.Proxy to nil
   - Reset token.ProxyHealthScore to 1.0
   - Save updated token
   - Clear HTTP client cache (to force new client creation)
   - Return success response
   - Return 404 if token not found
3. Add route registration:
   - Register DELETE route for /api/credentials/{provider}/{tokenID}/proxy
   - Ensure route is properly ordered
4. Add error handling:
   - Handle token not found (404)
   - Handle invalid provider type (400)
   - Handle store errors (500)
5. Add security measures:
   - Clear client cache on proxy config change
   - Log proxy deletion requests (audit)
6. Add logging:
   - Log proxy config deletion requests
   - Log successful deletions
   - Log errors (token not found, store errors)
7. Add unit tests for:
   - Successful proxy config deletion
   - Token not found (404)
   - Invalid provider type (400)
   - Idempotency (delete already deleted proxy)
   - Client cache clearing
   - Proxy health score reset
   - Error handling

## Dependencies
- Task 004: Extend Token Metadata with Proxy Configuration (updates ProviderToken.Proxy)

## Files to Modify
- `restapi/rest_api.go` (add DELETE endpoint handler)
- `restapi/rest_api_test.go` (add tests)

## Related Tasks
- Task 019: Add Proxy Configuration GET Endpoint (read endpoint)
- Task 020: Add Proxy Configuration PUT Endpoint (update endpoint)
