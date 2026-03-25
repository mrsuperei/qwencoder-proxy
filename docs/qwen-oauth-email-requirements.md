# Qwen OAuth Email Requirements

## Overview

Qwen's OAuth 2.0 implementation does NOT provide email information in the OAuth token response or via a user info endpoint. This is a limitation of Qwen's OAuth implementation.

To properly track and manage multiple Qwen tokens from different users, the email or alias must be provided by the user during the authentication flow.

## Implementation Details

### Device Flow (CLI)

When using the device flow via CLI, the user will be prompted to enter their email or alias:

```
=== Qwen Email/Alias Required ===
Qwen's OAuth does not provide email information.
Please provide your email or alias to track this token.

Enter your email or alias: user@example.com
Using email/alias: user@example.com
```

The email/alias is **required** and cannot be empty.

### REST API (Dashboard)

When using the dashboard for Qwen OAuth, the user will be prompted to enter their email or alias via a browser dialog:

1. Click "Add Token" for Qwen provider
2. A dialog appears: "Qwen requires your email or alias for token tracking. Please enter your email or alias:"
3. Enter email or alias and click OK
4. Complete OAuth flow in the provider's page

The email/alias is **required** and cannot be empty.

### API Endpoints

#### POST /api/auth/start

Starts OAuth authorization code flow. For Qwen provider, `email` field is required:

```json
{
  "provider": "qwen",
  "redirect_uri": "https://example.com/api/callback",
  "email": "user@example.com"  // Required for Qwen
}
```

**Response**: Returns `400 Bad Request` with error code `email_required` if email is not provided for Qwen.

#### POST /api/device/start

Starts device code flow. For Qwen provider, `email` field is required:

```json
{
  "provider": "qwen",
  "email": "user@example.com"  // Required for Qwen
}
```

**Response**: Returns `400 Bad Request` with error code `email_required` if email is not provided for Qwen.

#### GET /api/callback

OAuth callback endpoint. Accepts optional `email` query parameter:

```
https://example.com/api/callback?code=...&state=...&email=user@example.com
```

The email from the query parameter takes precedence over the email stored in the OAuth state.

## Email Validation

Emails are validated with the following rules:

- Must not be empty after trimming whitespace
- Must contain `@` symbol
- Both local part and domain part must be non-empty
- Email is converted to lowercase
- Leading/trailing whitespace is trimmed

**Note**: The validation is intentionally permissive to allow aliases (e.g., `user@local`) for tracking purposes.

## Error Messages

### Email Required

When email is not provided for Qwen:

**HTTP Response**:
```json
{
  "error": "email_required",
  "message": "Email or alias is required for Qwen provider"
}
```

**CLI Prompt**:
```
Email/alias is required. Please try again.
```

**Dashboard Dialog**:
```
Email/alias is required for Qwen
```

### Invalid Email Format

If the email format is invalid:

**CLI Prompt**:
```
Email/alias is too short. Please try again.
```

## Token Storage

Tokens are stored with the provided email/alias in the SQLite database:

```sql
INSERT INTO tokens (
  id, provider_id, access_token, refresh_token, token_type,
  expiry_date, email, resource_url, ...
) VALUES (?, ?, ?, ?, ?, ?, ?, ...)
```

The `email` field is used to distinguish between multiple tokens from different users.

## Security Considerations

1. **Email Privacy**: The email/alias is stored locally in the SQLite database and is not transmitted to Qwen.
2. **No Email Verification**: The system does not verify that the provided email/alias belongs to the user authenticating with Qwen.
3. **Alias Support**: Users can use any alias format (e.g., `user@local`, `work-account`, etc.) for tracking purposes.

## Migration Notes

### Existing Tokens

Existing Qwen tokens with `unknown@example.com` as email will continue to work but cannot be distinguished from each other. To improve tracking:

1. Delete existing Qwen tokens
2. Re-authenticate with email/alias

### Database Schema

The database schema already supports the `email` field, so no migration is required.

## Troubleshooting

### Issue: Token saved with "unknown@example.com"

**Cause**: Email was not provided during authentication.

**Solution**: 
- For CLI: Re-run device flow and provide email when prompted
- For Dashboard: Re-authenticate and provide email when prompted

### Issue: Multiple tokens show same email

**Cause**: Same email/alias was used for multiple authentications.

**Solution**: Use different emails/aliases for each Qwen account (e.g., `user1@local`, `user2@local`).

### Issue: Email prompt appears for other providers

**Cause**: This is expected behavior only for Qwen provider.

**Solution**: Other providers (Gemini, Antigravity, etc.) extract email from their OAuth responses and do not require user input.
