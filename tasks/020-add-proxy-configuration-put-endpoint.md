# Task 020: Add Proxy Configuration PUT Endpoint

## Description
Implement REST API endpoint for updating proxy configuration for a specific token. This endpoint allows clients to set or modify proxy settings for any token.

## Technical Context
This task implements the PUT endpoint for proxy configuration as defined in proxy-per-token architecture.

**Endpoint Specification**:
```
PUT /api/credentials/{provider}/{tokenID}/proxy
```

**Request Schema**:
```json
{
  "type": "socks5",
  "host": "proxy.example.com",
  "port": 1080,
  "username": "user",
  "password": "pass",
  "enabled": true
}
```

**Response Schema** (Success):
```json
{
  "success": true,
  "message": "Proxy configuration updated"
}
```

**Response Schema** (Error):
```json
{
  "error": "validation_error",
  "message": "Invalid proxy configuration",
  "details": {
    "field": "port",
    "issue": "must be between 1 and 65535"
  }
}
```

**Related Components**:
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Updates token with proxy config
- [`auth/proxy_config.go`](auth/proxy_config.go) - Validates proxy config
- [`restapi/rest_api.go`](restapi/rest_api.go) - REST API server

**Validation Rules**:
- `Type` must be one of: none, http, https, socks5
- `Host` required when `Type` is not none
- `Port` must be between 1 and 65535
- `Username` and `Password` must both be provided or both empty
- `Enabled` defaults to true if not specified
- Return 400 on validation errors

**Security Considerations**:
- Encrypt proxy passwords before storing
- Validate proxy configuration before applying
- Clear client cache on proxy config change
- Log all proxy configuration changes (audit trail)

**Architectural Principles**:
- **RESTful Design**: PUT endpoint for updating resource
- **Validation**: Request validation before applying changes
- **Security**: Password encryption and audit logging

## Subtasks
1. Modify [`restapi/rest_api.go`](restapi/rest_api.go) Server struct:
   - Add handler method for PUT /api/credentials/{provider}/{tokenID}/proxy
2. Implement updateProxyConfigHandler() method:
   - Parse provider and tokenID from URL path
   - Parse request body into ProxyConfig struct
   - Validate proxy configuration using ProxyConfig.Validate()
   - Validate provider type
   - Get token from MultiTokenStore
   - Encrypt proxy password if provided
   - Update token with new proxy config
   - Clear HTTP client cache (to force new client creation)
   - Return success response
   - Return 400 on validation errors
   - Return 404 if token not found
3. Add route registration:
   - Register PUT route for /api/credentials/{provider}/{tokenID}/proxy
   - Ensure route is properly ordered
4. Add error handling:
   - Handle validation errors (400)
   - Handle token not found (404)
   - Handle store errors (500)
   - Handle encryption errors (500)
5. Add security measures:
   - Encrypt proxy password before storing
   - Clear client cache on proxy config change
   - Log proxy configuration changes (audit)
6. Add logging:
   - Log proxy config update requests
   - Log successful updates
   - Log validation errors
   - Log errors (token not found, store errors)
7. Add unit tests for:
   - Successful proxy config update
   - Validation errors (400)
   - Token not found (404)
   - Invalid provider type (400)
   - Password encryption
   - Client cache clearing
   - Error handling

## Dependencies
- Task 001: Create Proxy Configuration Data Structures (uses ProxyConfig)
- Task 004: Extend Token Metadata with Proxy Configuration (updates ProviderToken)

## Files to Modify
- `restapi/rest_api.go` (add PUT endpoint handler)
- `restapi/rest_api_test.go` (add tests)

## Related Tasks
- Task 019: Add Proxy Configuration GET Endpoint (read endpoint)
- Task 021: Add Proxy Configuration DELETE Endpoint (delete endpoint)
