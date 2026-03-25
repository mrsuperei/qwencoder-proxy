# QWEncoder Proxy Dashboard Guide

## Overview

The QWEncoder Proxy Dashboard is a simple HTML/JS/CSS web interface that allows you to manage OAuth tokens for your LLM providers. It provides an easy way to add new tokens using the authorization code flow.

## Features

- **Provider Management**: View all supported OAuth providers
- **OAuth Flow**: Add tokens for authorization code flow providers (Gemini, iFlow, Antigravity)
- **Token Display**: View token information including email, expiry, and health status
- **Responsive Design**: Works on desktop and mobile devices
- **No Dependencies**: Pure HTML/JS/CSS - no npm or build tools required

## Accessing the Dashboard

Once the QWEncoder Proxy server is running, access the dashboard at:

```
http://localhost:8143/
```

## Supported Providers

The dashboard currently supports the following OAuth providers using the authorization code flow:

| Provider | ID | Description |
|----------|-----|-------------|
| Gemini (Google) | `gemini-cli` | Google's Gemini AI models |
| iFlow | `iflow` | iFlow AI platform |
| Antigravity | `antigravity` | Antigravity AI platform |

**Note**: Qwen provider uses device code flow and will be added in a future update.

## Adding a Token

1. Navigate to the dashboard at `http://localhost:8143/`
2. Find the provider you want to add a token for
3. Click the "Add Token" button
4. A popup window will open with the provider's OAuth consent page
5. Complete the authentication flow in the popup
6. Once authenticated, the popup will close and your token will appear in the dashboard

## Viewing Token Information

For each provider, you can view:

- **Email**: The email associated with the token
- **Expiry Date**: When the token expires
- **Health Status**: Whether the token is healthy or unhealthy

## API Endpoints

The dashboard uses the following REST API endpoints:

### Providers

- `GET /api/providers` - List all providers
- `GET /api/providers/{id}` - Get provider configuration

### Authentication

- `POST /api/auth/start` - Start authorization code flow
  ```json
  {
    "provider_id": "gemini-cli",
    "callback_url": "http://localhost:8143/api/callback"
  }
  ```
- `GET /api/callback` - OAuth callback endpoint

### Credentials

- `GET /api/credentials` - Get all provider credentials
- `GET /api/credentials/{id}` - Get credentials for specific provider

## Troubleshooting

### Dashboard Won't Load

1. Check that the server is running:
   ```bash
   go run cmd/main.go
   ```
2. Verify the port is correct (default: 8143)
3. Check the server logs for errors

### Popup Window Blocked

If the popup window is blocked by your browser:
1. Click the "Add Token" button again
2. Allow popups for localhost in your browser settings
3. Try again

### OAuth Flow Fails

1. Check the server logs for error messages
2. Verify the callback URL is correct: `http://localhost:8143/api/callback`
3. Ensure you have valid OAuth credentials configured for the provider

### Token Not Showing After Authentication

1. Wait a few seconds for the dashboard to refresh
2. Click the "Retry" button if available
3. Check the browser console for JavaScript errors
4. Verify the token was saved by checking `/api/credentials/{provider_id}`

## Development

### File Structure

```
qwencoder-proxy/
├── internal/
│   └── restapi/
│       ├── dashboard_handler.go    # Dashboard HTTP handler
│       └── rest_api.go           # REST API server
├── internal/restapi/web/          # Embedded static files
│   ├── index.html                 # Dashboard HTML
│   ├── css/
│   │   └── dashboard.css          # Dashboard styles
│   └── js/
│       └── dashboard.js           # Dashboard JavaScript
└── cmd/
    └── main.go                   # Main server with dashboard integration
```

### Building

```bash
go build -o qwencoder-proxy.exe ./cmd/main.go
```

### Running

```bash
./qwencoder-proxy.exe
```

## Future Enhancements

- Device code flow support for Qwen provider
- Token deletion functionality
- Proxy configuration per token
- Rate limiting information display
- Usage analytics and statistics
- Settings page for configuration options

## Support

For issues or questions:
1. Check the server logs for error messages
2. Review the API documentation in `docs/rest_api.md`
3. Check the troubleshooting section above
