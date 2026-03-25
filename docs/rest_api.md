# REST API Reference

This document describes all dashboard-related REST API endpoints for the qwencoder-proxy. These endpoints are used by the web dashboard for managing tokens, rate limits, settings, and proxy configurations.

**Base URL**: `http://localhost:8080` (default)

## Table of Contents

- [Provider Discovery](#provider-discovery)
- [Token Management](#token-management)
- [Credentials Management](#credentials-management)
- [Proxy Configuration](#proxy-configuration)
- [Rate Limit Configuration](#rate-limit-configuration)
- [Usage Tracking](#usage-tracking)
- [Model Usage Tracking](#model-usage-tracking)
- [Error Tracking](#error-tracking)
- [Request History](#request-history)
- [Cache Management](#cache-management)

---

## Provider Discovery

### Get All Providers

Lists all available OAuth2 providers.

**Endpoint**: `GET /api/providers`

**Response**:
```json
{
  "providers": [
    {
      "id": "gemini-cli",
      "name": "Gemini",
      "flow": "device_code",
      "scopes": ["openid", "email"]
    }
  ]
}
```

### Get Provider Configuration

Get detailed configuration for a specific provider.

**Endpoint**: `GET /api/providers/{provider}/config`

**Path Parameters**:
- `provider` - Provider ID (e.g., `gemini-cli`, `qwen`, `kiro`, `antigravity`, `iflow`)

**Response**:
```json
{
  "id": "gemini-cli",
  "name": "Gemini",
  "flow": "device_code",
  "scopes": ["openid", "email"],
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth",
  "token_url": "https://oauth2.googleapis.com/token",
  "description": "Google Gemini API"
}
```

---

## Token Management

### Get Selected Token

Get the current selected access token for a provider (used by token selection strategy).

**Endpoint**: `GET /api/token/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "access_token": "ya29.a0AfH6...",
  "email": "user@example.com",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### Refresh Provider Token

Refresh the access token for a provider (legacy endpoint).

**Endpoint**: `POST /api/token/{provider}/refresh`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "access_token": "ya29.a0AfH6...",
  "refresh_token": "1//0g...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### Delete Provider Token

Delete the stored token for a provider (legacy endpoint).

**Endpoint**: `DELETE /api/token/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "success": true,
  "message": "Token revoked successfully"
}
```

---

## Credentials Management

### List All Credentials

List all stored credentials across all providers.

**Endpoint**: `GET /api/credentials`

**Response**:
```json
{
  "credentials": [
    {
      "provider": "gemini-cli",
      "total_tokens": 3,
      "valid_tokens": 2,
      "settings": {
        "selection_strategy": "least_used",
        "refresh_buffer_sec": 300,
        "max_error_count": 5,
        "updated_at": 1710864000000
      },
      "tokens": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440000",
          "email": "user1@example.com",
          "expiry_date": 1710950400000,
          "expires_in": 86400,
          "token_type": "Bearer",
          "healthy": true,
          "health_score": 1.0,
          "last_used": 1710864000000,
          "created_at": 1710864000000,
          "error_count": 0
        }
      ]
    }
  ]
}
```

### Clear All Credentials

Delete all stored credentials across all providers.

**Endpoint**: `DELETE /api/credentials`

**Response**:
```json
{
  "success": true,
  "message": "Cleared 3 credential files",
  "cleared": 3
}
```

### Get Provider Credentials

Get all tokens for a specific provider.

**Endpoint**: `GET /api/credentials/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "tokens": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "user1@example.com",
      "expiry_date": 1710950400000,
      "expires_in": 86400,
      "token_type": "Bearer",
      "healthy": true,
      "health_score": 1.0,
      "last_used": 1710864000000,
      "created_at": 1710864000000,
      "error_count": 0
    }
  ],
  "settings": {
    "selection_strategy": "least_used",
    "refresh_buffer_sec": 300,
    "max_error_count": 5,
    "updated_at": 1710864000000
  }
}
```

### Add Token

Add a new token to a provider.

**Endpoint**: `POST /api/credentials/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Request Body**:
```json
{
  "access_token": "ya29.a0AfH6...",
  "refresh_token": "1//0g...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "resource_url": "https://generativelanguage.googleapis.com",
  "email": "user@example.com"
}
```

**Response**:
```json
{
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "success": true
}
```

### Delete Token

Delete a specific token.

**Endpoint**: `DELETE /api/credentials/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "success": true,
  "message": "Token deleted successfully"
}
```

### Refresh Token

Refresh a specific token.

**Endpoint**: `POST /api/credentials/{provider}/{tokenId}/refresh`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "success": true,
  "token": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "expiry_date": 1710950400000,
    "expires_in": 86400,
    "token_type": "Bearer",
    "healthy": true,
    "health_score": 1.0,
    "last_used": 1710864000000,
    "created_at": 1710864000000,
    "error_count": 0
  }
}
```

### Update Provider Settings

Update provider-level settings.

**Endpoint**: `PUT /api/credentials/{provider}/settings`

**Path Parameters**:
- `provider` - Provider ID

**Request Body**:
```json
{
  "selection_strategy": "least_used",
  "refresh_buffer_sec": 300,
  "max_error_count": 5
}
```

**Response**:
```json
{
  "success": true,
  "settings": {
    "selection_strategy": "least_used",
    "refresh_buffer_sec": 300,
    "max_error_count": 5,
    "updated_at": 1710864000000
  }
}
```

**Settings Fields**:
- `selection_strategy` - Token selection strategy: `"random"`, `"round_robin"`, or `"least_used"`
- `refresh_buffer_sec` - Seconds before expiry to refresh token
- `max_error_count` - Maximum errors before marking token as unhealthy

---

## Proxy Configuration

### Get Proxy Configuration

Get proxy configuration for a specific token.

**Endpoint**: `GET /api/credentials/{provider}/{tokenId}/proxy`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "proxy": {
    "type": "http",
    "host": "proxy.example.com",
    "port": 8080,
    "username": "user",
    "password": "***"
  },
  "health_status": {
    "last_check": 1710864000000,
    "is_healthy": true,
    "last_error": "",
    "consecutive_failures": 0,
    "average_latency_ms": 150,
    "health_score": 1.0
  }
}
```

### Update Proxy Configuration

Update proxy configuration for a specific token.

**Endpoint**: `PUT /api/credentials/{provider}/{tokenId}/proxy`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Request Body**:
```json
{
  "type": "http",
  "host": "proxy.example.com",
  "port": 8080,
  "username": "user",
  "password": "secret"
}
```

**Proxy Types**:
- `none` - No proxy (direct connection)
- `http` - HTTP proxy
- `https` - HTTPS proxy
- `socks5` - SOCKS5 proxy

**Response**:
```json
{
  "success": true,
  "message": "Proxy configuration updated"
}
```

### Delete Proxy Configuration

Remove proxy configuration from a specific token.

**Endpoint**: `DELETE /api/credentials/{provider}/{tokenId}/proxy`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "success": true,
  "message": "Proxy configuration removed"
}
```

### Test Proxy Connection

Test a proxy connection without saving configuration.

**Endpoint**: `POST /api/proxy/test`

**Request Body**:
```json
{
  "type": "http",
  "host": "proxy.example.com",
  "port": 8080,
  "username": "user",
  "password": "secret"
}
```

**Response** (Success):
```json
{
  "success": true,
  "latency_ms": 150,
  "message": "Proxy connection successful (150ms)"
}
```

**Response** (Failure):
```json
{
  "success": false,
  "latency_ms": 0,
  "error": "connection refused",
  "message": "Proxy connection failed"
}
```

---

## Rate Limit Configuration

### Get All Rate Limit Configs

Get rate limit configurations for all providers.

**Endpoint**: `GET /api/ratelimit/config`

**Response**:
```json
{
  "gemini-cli": {
    "provider_id": "gemini-cli",
    "requests_per_day": 15000,
    "requests_per_minute": 60,
    "tokens_per_minute": 32000,
    "enabled": true,
    "updated_at": "2024-03-19T12:00:00Z"
  },
  "qwen": {
    "provider_id": "qwen",
    "requests_per_day": 10000,
    "requests_per_minute": 50,
    "tokens_per_minute": 30000,
    "enabled": true,
    "updated_at": "2024-03-19T12:00:00Z"
  }
}
```

### Get Provider Rate Limit Config

Get rate limit configuration for a specific provider.

**Endpoint**: `GET /api/ratelimit/config/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "requests_per_day": 15000,
  "requests_per_minute": 60,
  "tokens_per_minute": 32000,
  "enabled": true,
  "updated_at": "2024-03-19T12:00:00Z"
}
```

### Update Rate Limit Config

Update rate limit configuration for a specific provider.

**Endpoint**: `PUT /api/ratelimit/config/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Request Body**:
```json
{
  "provider_id": "gemini-cli",
  "requests_per_day": 15000,
  "requests_per_minute": 60,
  "tokens_per_minute": 32000,
  "enabled": true
}
```

**Response**:
```json
{
  "message": "Configuration updated"
}
```

---

## Usage Tracking

### Get All Usage

Get usage metrics for all providers.

**Endpoint**: `GET /api/ratelimit/usage`

**Response**:
```json
{
  "gemini-cli": {
    "requests_today": 1250,
    "requests_in_minute": 5,
    "tokens_in_minute": 2500,
    "window_start": "2024-03-19T12:00:00Z",
    "day_start": "2024-03-19T00:00:00Z"
  },
  "qwen": {
    "requests_today": 800,
    "requests_in_minute": 3,
    "tokens_in_minute": 1800,
    "window_start": "2024-03-19T12:00:00Z",
    "day_start": "2024-03-19T00:00:00Z"
  }
}
```

### Get Provider Usage

Get usage metrics for a specific provider.

**Endpoint**: `GET /api/ratelimit/usage/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "token_id": "",
  "remaining_requests_per_day": 13750,
  "remaining_requests_per_minute": 55,
  "remaining_tokens_per_minute": 29500,
  "usage_percentage_per_day": 8.33,
  "usage_percentage_per_minute": 8.33,
  "token_usage_percentage_per_minute": 7.81,
  "seconds_until_day_reset": 43200,
  "seconds_until_minute_reset": 45,
  "is_limited": false,
  "limit_reasons": []
}
```

### Get Token Usage

Get usage metrics for a specific token.

**Endpoint**: `GET /api/ratelimit/usage/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "remaining_requests_per_day": 4500,
  "remaining_requests_per_minute": 18,
  "remaining_tokens_per_minute": 9800,
  "usage_percentage_per_day": 70.0,
  "usage_percentage_per_minute": 70.0,
  "token_usage_percentage_per_minute": 69.38,
  "seconds_until_day_reset": 43200,
  "seconds_until_minute_reset": 30,
  "is_limited": false,
  "limit_reasons": []
}
```

### Reset Provider Usage

Reset usage metrics for a provider.

**Endpoint**: `POST /api/ratelimit/reset/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "message": "Usage reset"
}
```

### Reset Token Usage

Reset usage metrics for a specific token.

**Endpoint**: `POST /api/ratelimit/reset/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "message": "Usage reset"
}
```

---

## Model Usage Tracking

### Get All Model Usage

Get model usage metrics for all providers.

**Endpoint**: `GET /api/ratelimit/model-usage`

**Response**:
```json
{
  "gemini-cli": {
    "gemini-pro": {
      "provider_id": "gemini-cli",
      "token_id": "",
      "model": "gemini-pro",
      "request_count": 500,
      "input_tokens": 25000,
      "output_tokens": 15000,
      "total_tokens": 40000,
      "window_start": "2024-03-19T00:00:00Z",
      "day_start": "2024-03-19T00:00:00Z"
    }
  },
  "qwen": {
    "qwen-turbo": {
      "provider_id": "qwen",
      "token_id": "",
      "model": "qwen-turbo",
      "request_count": 300,
      "input_tokens": 18000,
      "output_tokens": 9000,
      "total_tokens": 27000,
      "window_start": "2024-03-19T00:00:00Z",
      "day_start": "2024-03-19T00:00:00Z"
    }
  }
}
```

### Get Provider Model Usage

Get model usage metrics for a specific provider.

**Endpoint**: `GET /api/ratelimit/model-usage/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "gemini-pro": {
    "provider_id": "gemini-cli",
    "token_id": "",
    "model": "gemini-pro",
    "request_count": 500,
    "input_tokens": 25000,
    "output_tokens": 15000,
    "total_tokens": 40000,
    "window_start": "2024-03-19T00:00:00Z",
    "day_start": "2024-03-19T00:00:00Z"
  }
}
```

### Get Token Model Usage

Get model usage metrics for a specific token.

**Endpoint**: `GET /api/ratelimit/model-usage/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "gemini-pro": {
    "provider_id": "gemini-cli",
    "token_id": "550e8400-e29b-41d4-a716-446655440000",
    "model": "gemini-pro",
    "request_count": 200,
    "input_tokens": 10000,
    "output_tokens": 6000,
    "total_tokens": 16000,
    "window_start": "2024-03-19T00:00:00Z",
    "day_start": "2024-03-19T00:00:00Z"
  }
}
```

### Get Specific Model Usage

Get usage metrics for a specific model.

**Endpoint**: `GET /api/ratelimit/model-usage/{provider}/{tokenId}/{model}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID
- `model` - Model name

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": "gemini-pro",
  "request_count": 200,
  "input_tokens": 10000,
  "output_tokens": 6000,
  "total_tokens": 16000,
  "window_start": "2024-03-19T00:00:00Z",
  "day_start": "2024-03-19T00:00:00Z"
}
```

---

## Error Tracking

### Get Provider Errors

Get error information for all tokens of a provider.

**Endpoint**: `GET /api/ratelimit/errors?provider_id={provider}`

**Query Parameters**:
- `provider_id` - Provider ID (required)

**Response**:
```json
{
  "provider_id": "gemini-cli",
  "errors": {
    "550e8400-e29b-41d4-a716-446655440000": {
      "token_id": "550e8400-e29b-41d4-a716-446655440000",
      "error_count": 3,
      "last_error": "rate limit exceeded",
      "last_error_time": 1710864000000,
      "consecutive_errors": 2,
      "is_unhealthy": false
    }
  }
}
```

### Get Token Errors

Get error information for a specific token.

**Endpoint**: `GET /api/ratelimit/errors/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "error_count": 3,
  "last_error": "rate limit exceeded",
  "last_error_time": 1710864000000,
  "consecutive_errors": 2,
  "is_unhealthy": false
}
```

### Reset Token Errors

Reset error tracking for a specific token.

**Endpoint**: `POST /api/ratelimit/errors/reset/{provider}/{tokenId}`

**Path Parameters**:
- `provider` - Provider ID
- `tokenId` - Token ID

**Response**:
```json
{
  "message": "errors_reset",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "provider_id": "gemini-cli"
}
```

---

## Request History

### Get Request History

Get paginated request history with optional filters.

**Endpoint**: `GET /api/ratelimit/request-history`

**Query Parameters**:
- `token_id` - Filter by token ID (optional)
- `model` - Filter by model (optional)
- `start_time` - Start timestamp in milliseconds (optional)
- `end_time` - End timestamp in milliseconds (optional)
- `min_tokens` - Minimum total tokens (optional)
- `max_tokens` - Maximum total tokens (optional)
- `page` - Page number (default: 1)
- `page_size` - Page size (default: 100, max: 1000)

**Response**:
```json
{
  "records": [
    {
      "id": "req-123456",
      "token_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": "gemini-pro",
      "request_count": 1,
      "input_tokens": 100,
      "output_tokens": 50,
      "total_tokens": 150,
      "timestamp": 1710864000000,
      "created_at": 1710864000000
    }
  ],
  "total_count": 1500,
  "page": 1,
  "page_size": 100,
  "total_pages": 15,
  "has_next": true,
  "has_previous": false
}
```

### Get Token Request History

Get request history for a specific token.

**Endpoint**: `GET /api/ratelimit/request-history/{tokenId}`

**Path Parameters**:
- `tokenId` - Token ID

**Query Parameters**:
- `model` - Filter by model (optional)
- `start_time` - Start timestamp in milliseconds (optional)
- `end_time` - End timestamp in milliseconds (optional)
- `min_tokens` - Minimum total tokens (optional)
- `max_tokens` - Maximum total tokens (optional)
- `page` - Page number (default: 1)
- `page_size` - Page size (default: 100, max: 1000)

**Response**:
```json
{
  "records": [
    {
      "id": "req-123456",
      "token_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": "gemini-pro",
      "request_count": 1,
      "input_tokens": 100,
      "output_tokens": 50,
      "total_tokens": 150,
      "timestamp": 1710864000000,
      "created_at": 1710864000000
    }
  ],
  "total_count": 500,
  "page": 1,
  "page_size": 100,
  "total_pages": 5,
  "has_next": true,
  "has_previous": false
}
```

### Get Token Model Request History

Get request history for a specific token and model.

**Endpoint**: `GET /api/ratelimit/request-history/{tokenId}/{model}`

**Path Parameters**:
- `tokenId` - Token ID
- `model` - Model name

**Query Parameters**:
- `start_time` - Start timestamp in milliseconds (optional)
- `end_time` - End timestamp in milliseconds (optional)
- `min_tokens` - Minimum total tokens (optional)
- `max_tokens` - Maximum total tokens (optional)
- `page` - Page number (default: 1)
- `page_size` - Page size (default: 100, max: 1000)

**Response**:
```json
{
  "records": [
    {
      "id": "req-123456",
      "token_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": "gemini-pro",
      "request_count": 1,
      "input_tokens": 100,
      "output_tokens": 50,
      "total_tokens": 150,
      "timestamp": 1710864000000,
      "created_at": 1710864000000
    }
  ],
  "total_count": 200,
  "page": 1,
  "page_size": 100,
  "total_pages": 2,
  "has_next": true,
  "has_previous": false
}
```

### Get Request History Summary

Get aggregated statistics for request history.

**Endpoint**: `GET /api/ratelimit/request-history/summary`

**Query Parameters**:
- `token_id` - Filter by token ID (optional)
- `model` - Filter by model (optional)
- `start_time` - Start timestamp in milliseconds (optional)
- `end_time` - End timestamp in milliseconds (optional)

**Response**:
```json
{
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": "gemini-pro",
  "total_requests": 500,
  "total_input_tokens": 25000,
  "total_output_tokens": 15000,
  "total_tokens": 40000,
  "average_tokens": 80.0,
  "first_request": 1710864000000,
  "last_request": 1710950400000
}
```

### Delete Request History

Delete request history records based on filters.

**Endpoint**: `POST /api/ratelimit/request-history/delete` or `DELETE /api/ratelimit/request-history/delete`

**Query Parameters** (at least one required):
- `token_id` - Filter by token ID
- `model` - Filter by model
- `start_time` - Start timestamp in milliseconds
- `end_time` - End timestamp in milliseconds

**Response**:
```json
{
  "message": "request_history_deleted",
  "rows_affected": 1500
}
```

---

## Cache Management

### Invalidate All Cache

Invalidate all cached data.

**Endpoint**: `POST /api/cache/invalidate`

**Response**:
```json
{
  "message": "Cache invalidated successfully"
}
```

### Invalidate Provider Cache

Invalidate cached data for a specific provider.

**Endpoint**: `POST /api/cache/invalidate/provider/{provider}`

**Path Parameters**:
- `provider` - Provider ID

**Response**:
```json
{
  "message": "Provider cache invalidated successfully",
  "provider": "gemini-cli"
}
```

### Invalidate Token Cache

Invalidate cached data for a specific token.

**Endpoint**: `POST /api/cache/invalidate/token/{token}`

**Path Parameters**:
- `token` - Token ID

**Response**:
```json
{
  "message": "Token cache invalidated successfully",
  "token": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Get Cache Stats

Get cache statistics.

**Endpoint**: `GET /api/cache/stats`

**Response**:
```json
{
  "invalidations_total": 150,
  "invalidations_by_type": {
    "all": 10,
    "provider": 50,
    "token": 80,
    "config": 10
  },
  "last_invalidation": 1710950400000
}
```

---

## Error Response Format

All endpoints return errors in a consistent format:

```json
{
  "error": {
    "code": "error_code",
    "message": "Human readable error message",
    "details": {
      "additional": "information"
    }
  }
}
```

### Common Error Codes

- `method_not_allowed` - HTTP method not supported
- `invalid_request` - Invalid request parameters
- `not_found` - Resource not found
- `invalid_provider` - Invalid provider ID
- `invalid_json` - Invalid JSON in request body
- `update_failed` - Failed to update resource
- `delete_failed` - Failed to delete resource
- `query_failed` - Database query failed
- `validation_error` - Request validation failed
- `internal_error` - Internal server error

---

## Data Types Reference

### ProviderTokenInfo

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "expiry_date": 1710950400000,
  "expires_in": 86400,
  "token_type": "Bearer",
  "healthy": true,
  "health_score": 1.0,
  "last_used": 1710864000000,
  "created_at": 1710864000000,
  "error_count": 0
}
```

### StoreSettings

```json
{
  "selection_strategy": "least_used",
  "refresh_buffer_sec": 300,
  "max_error_count": 5,
  "updated_at": 1710864000000
}
```

### ProxyConfig

```json
{
  "type": "http",
  "host": "proxy.example.com",
  "port": 8080,
  "username": "user",
  "password": "secret"
}
```

### ProxyHealth

```json
{
  "last_check": 1710864000000,
  "is_healthy": true,
  "last_error": "",
  "consecutive_failures": 0,
  "average_latency_ms": 150,
  "health_score": 1.0
}
```

### ProviderRateLimitConfig

```json
{
  "provider_id": "gemini-cli",
  "requests_per_day": 15000,
  "requests_per_minute": 60,
  "tokens_per_minute": 32000,
  "enabled": true,
  "updated_at": "2024-03-19T12:00:00Z"
}
```

### UsageMetrics

```json
{
  "requests_today": 1250,
  "requests_in_minute": 5,
  "tokens_in_minute": 2500,
  "window_start": "2024-03-19T12:00:00Z",
  "day_start": "2024-03-19T00:00:00Z"
}
```

### QuotaStatus

```json
{
  "provider_id": "gemini-cli",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "remaining_requests_per_day": 13750,
  "remaining_requests_per_minute": 55,
  "remaining_tokens_per_minute": 29500,
  "usage_percentage_per_day": 8.33,
  "usage_percentage_per_minute": 8.33,
  "token_usage_percentage_per_minute": 7.81,
  "seconds_until_day_reset": 43200,
  "seconds_until_minute_reset": 45,
  "is_limited": false,
  "limit_reasons": []
}
```

### ModelUsageMetrics

```json
{
  "provider_id": "gemini-cli",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": "gemini-pro",
  "request_count": 500,
  "input_tokens": 25000,
  "output_tokens": 15000,
  "total_tokens": 40000,
  "window_start": "2024-03-19T00:00:00Z",
  "day_start": "2024-03-19T00:00:00Z"
}
```

### RequestHistoryRecord

```json
{
  "id": "req-123456",
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": "gemini-pro",
  "request_count": 1,
  "input_tokens": 100,
  "output_tokens": 50,
  "total_tokens": 150,
  "timestamp": 1710864000000,
  "created_at": 1710864000000
}
```

### TokenErrorInfo

```json
{
  "token_id": "550e8400-e29b-41d4-a716-446655440000",
  "error_count": 3,
  "last_error": "rate limit exceeded",
  "last_error_time": 1710864000000,
  "consecutive_errors": 2,
  "is_unhealthy": false
}
```
