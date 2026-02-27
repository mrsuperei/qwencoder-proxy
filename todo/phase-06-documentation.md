# Phase 6: Documentation and Environment Variables

## Problem

### Missing Documentation for Environment Variables

After centralizing configuration in Phase 1, the project needs comprehensive documentation for:

1. **Environment variables** - List of all available environment variables
2. **Configuration file format** - Structure of `.env` file
3. **Security best practices** - How to handle secrets safely
4. **Development setup** - How to configure the project for local development
5. **Deployment guide** - How to configure for production

### Current State

The project has:
- An `example.env` file (but it's minimal)
- No comprehensive documentation on environment variables
- No security guidelines for handling credentials
- No guide for different deployment environments

---

## How to Fix

### Step 1: Create Comprehensive README for Configuration

Create `docs/configuration.md`:

```markdown
# Configuration Guide

This document describes how to configure qwencoder-proxy using environment variables and configuration files.

## Overview

qwencoder-proxy uses environment variables to configure provider credentials, API endpoints, and application settings. This approach allows:

- **Security**: Credentials are not hardcoded in source code
- **Flexibility**: Easy configuration for different environments
- **Separation**: Different configurations for dev/staging/production

## Quick Start

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your credentials:
   ```bash
   # Edit the file and fill in your values
   nano .env
   ```

3. Start the application:
   ```bash
   go run cmd/qwencoder-proxy/main.go
   ```

## Environment Variables

### Server Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `SERVER_PORT` | Port for the proxy server | `8143` | No |
| `DEBUG_MODE` | Enable debug logging | `false` | No |

### Storage Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `DB_PATH` | Path to SQLite database | `.credentials/tokens.db` | No |
| `CREDENTIALS_DIR` | Directory for credential files | `.credentials` | No |

### Gemini Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `GEMINI_BASE_URL` | Gemini API base URL | `https://cloudcode-pa.googleapis.com/v1internal` | No |
| `GEMINI_CLIENT_ID` | OAuth client ID | - | **Yes** |
| `GEMINI_CLIENT_SECRET` | OAuth client secret | - | **Yes** |
| `GEMINI_SCOPE` | OAuth scope | `https://www.googleapis.com/auth/cloud-platform ...` | No |
| `GEMINI_REDIRECT_PORT` | OAuth redirect port | `8085` | No |

### Qwen Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `QWEN_BASE_URL` | Qwen API base URL | `https://portal.qwen.ai/v1` | No |
| `QWEN_CLIENT_ID` | OAuth client ID | `f0304373b74a44d2b584a3fb70ca9e56` | No |
| `QWEN_SCOPE` | OAuth scope | `openid profile email model.completion` | No |
| `QWEN_TOKEN_URL` | OAuth token endpoint | `https://chat.qwen.ai/api/v1/oauth2/token` | No |
| `QWEN_DEVICE_AUTH_URL` | OAuth device auth endpoint | `https://chat.qwen.ai/api/v1/oauth2/device/code` | No |

### iFlow Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `IFLOW_AUTH_URL` | iFlow auth URL | `https://iflow.cn/oauth` | No |
| `IFLOW_TOKEN_URL` | iFlow token URL | `https://iflow.cn/oauth/token` | No |
| `IFLOW_USERINFO_URL` | iFlow user info URL | `https://iflow.cn/api/oauth/getUserInfo` | No |
| `IFLOW_APIKEY_URL` | iFlow API key URL | `https://platform.iflow.cn/api/openapi/apikey` | No |
| `IFLOW_API_BASE_URL` | iFlow API base URL | `https://apis.iflow.cn/v1` | No |
| `IFLOW_CLIENT_ID` | OAuth client ID | `10009311001` | No |
| `IFLOW_CLIENT_SECRET` | OAuth client secret | `4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW` | No |
| `IFLOW_DEFAULT_PORT` | iFlow default port | `11451` | No |

### Antigravity Configuration

| Variable | Description | Default | Required |
|----------|-------------|----------|-----------|
| `ANTIGRAVITY_DAILY_BASE_URL` | Antigravity daily URL | `https://daily-cloudcode-pa.sandbox.googleapis.com` | No |
| `ANTIGRAVITY_AUTOPUSH_BASE_URL` | Antigravity autopush URL | `https://autopush-cloudcode-pa.sandbox.googleapis.com` | No |

## Security Best Practices

### 1. Never Commit Credentials

Never commit `.env` files or any files containing credentials to version control. Add to `.gitignore`:

```gitignore
# Environment variables with secrets
.env
.env.local
.env.*.local
.env.production
.env.staging

# Credential directories
.credentials/
```

### 2. Use Different Environments

Use separate configuration files for different environments:

- `.env.local` - Local development
- `.env.staging` - Staging environment
- `.env.production` - Production environment

Load the appropriate file:

```bash
# Development
export $(cat .env.local | xargs)

# Production
export $(cat .env.production | xargs)
```

### 3. Rotate Credentials Regularly

Regularly rotate OAuth credentials:
- Set up credential rotation schedules
- Use credential management systems for production
- Monitor for credential leaks

### 4. Use Secrets Management (Production)

For production deployments, consider using a secrets management system:

- **AWS Secrets Manager**
- **HashiCorp Vault**
- **Azure Key Vault**
- **Google Secret Manager**

Example with AWS Secrets Manager:

```bash
# Load secrets from AWS
export GEMINI_CLIENT_ID=$(aws secretsmanager get-secret-value --secret-id qwencoder-gemini-client-id --query SecretString --output text)
export GEMINI_CLIENT_SECRET=$(aws secretsmanager get-secret-value --secret-id qwencoder-gemini-client-secret --query SecretString --output text)
```

## Development Setup

### Local Development

1. Clone the repository:
   ```bash
   git clone https://github.com/your-org/qwencoder-proxy.git
   cd qwencoder-proxy
   ```

2. Copy environment template:
   ```bash
   cp .env.example .env
   ```

3. Edit `.env` with your credentials:
   ```bash
   nano .env
   ```

4. Install dependencies:
   ```bash
   go mod download
   ```

5. Run the application:
   ```bash
   go run cmd/qwencoder-proxy/main.go
   ```

### Docker Development

1. Build Docker image:
   ```bash
   docker build -t qwencoder-proxy .
   ```

2. Run with environment file:
   ```bash
   docker run --env-file .env -p 8143:8143 qwencoder-proxy
   ```

## Deployment

### Production Deployment

1. **Prepare environment file**:
   ```bash
   # Create production environment file
   cp .env.example .env.production
   # Edit with production credentials
   nano .env.production
   ```

2. **Use secrets management**:
   ```bash
   # Load secrets from your secrets manager
   export GEMINI_CLIENT_ID=$(get-secret qwencoder-gemini-client-id)
   export GEMINI_CLIENT_SECRET=$(get-secret qwencoder-gemini-client-secret)
   ```

3. **Deploy**:
   ```bash
   # Using systemd
   sudo systemctl start qwencoder-proxy
   
   # Using Docker
   docker run -d --env-file .env.production -p 8143:8143 qwencoder-proxy:latest
   ```

## Troubleshooting

### Credentials Not Found

If you see "credentials not found" errors:

1. Check that `.env` file exists
2. Verify environment variables are set:
   ```bash
   env | grep GEMINI
   ```
3. Restart the application after changing `.env`

### Invalid Credentials

If you see "invalid credentials" errors:

1. Verify client ID and secret are correct
2. Check that credentials are not expired
3. Ensure OAuth redirect port is not blocked

### Proxy Issues

If you see proxy-related errors:

1. Check proxy configuration in database
2. Verify proxy server is accessible
3. Test proxy connection manually

## Additional Resources

- [API Documentation](./api-reference.md)
- [Authentication Guide](./authentication.md)
- [Troubleshooting](./troubleshooting.md)
```

### Step 2: Update Main README

Update `docs/README.md` to include configuration section:

```markdown
## Configuration

qwencoder-proxy uses environment variables for configuration. See [Configuration Guide](./configuration.md) for detailed information.

### Quick Start

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit with your credentials:
   ```bash
   nano .env
   ```

3. Run:
   ```bash
   go run cmd/qwencoder-proxy/main.go
   ```

### Environment Variables

Required environment variables:
- `GEMINI_CLIENT_ID` - Your Gemini OAuth client ID
- `GEMINI_CLIENT_SECRET` - Your Gemini OAuth client secret

See [Configuration Guide](./configuration.md) for all available options.
```

### Step 3: Create Security Guidelines

Create `docs/security.md`:

```markdown
# Security Guidelines

This document describes security best practices for qwencoder-proxy.

## Credential Management

### Never Commit Credentials

**CRITICAL**: Never commit credentials to version control.

Add these patterns to `.gitignore`:

```
# Environment files
.env
.env.*
!.env.example

# Credential directories
.credentials/
```

### Use Environment Variables

Always use environment variables for sensitive data:

- OAuth client IDs
- OAuth client secrets
- API keys
- Database passwords

### Rotate Credentials

Regularly rotate credentials:
- Set up automated rotation schedules
- Use credential management systems
- Monitor for unauthorized access

## Secrets Management

For production deployments, use a secrets management system:

### Recommended Solutions

1. **AWS Secrets Manager**
   - Secure storage
   - Automatic rotation
   - IAM integration

2. **HashiCorp Vault**
   - Open source
   - Flexible storage
   - Audit logging

3. **Google Secret Manager**
   - GCP integration
   - Automatic encryption
   - Access logging

### Example: AWS Secrets Manager

```bash
# Load secrets at runtime
export GEMINI_CLIENT_ID=$(aws secretsmanager get-secret-value \
  --secret-id qwencoder-gemini-client-id \
  --query SecretString \
  --output text)

export GEMINI_CLIENT_SECRET=$(aws secretsmanager get-secret-value \
  --secret-id qwencoder-gemini-client-secret \
  --query SecretString \
  --output text)
```

## Network Security

### Use HTTPS

All API endpoints should use HTTPS:
- Prevents man-in-the-middle attacks
- Encrypts data in transit
- Required by OAuth 2.0

### Validate Certificates

Enable certificate validation:
- Don't disable SSL verification
- Use proper CA certificates
- Monitor for certificate expiration

## Access Control

### Principle of Least Privilege

Grant minimum required permissions:
- Use specific OAuth scopes
- Limit API key permissions
- Rotate keys regularly

### Audit Logging

Enable audit logging:
- Log all credential access
- Monitor for unusual activity
- Set up alerts for suspicious behavior

## Incident Response

### Credential Compromise

If credentials are compromised:

1. **Immediately revoke**:
   - Revoke OAuth tokens
   - Rotate API keys
   - Update secrets in vault

2. **Investigate**:
   - Review access logs
   - Identify source of compromise
   - Patch vulnerabilities

3. **Prevent recurrence**:
   - Implement additional security controls
   - Update security policies
   - Train team on security

## Compliance

### Data Protection

- **GDPR**: Protect user data
- **SOC 2**: Maintain security controls
- **ISO 27001**: Follow security standards

### Regular Audits

- Conduct security audits annually
- Penetration testing quarterly
- Code reviews for security changes
```

### Step 4: Update .env.example

Update `.env.example` to be more comprehensive:

```bash
# ========================================
# qwencoder-proxy Configuration
# ========================================
# Copy this file to .env and fill in your credentials
# NEVER commit .env to version control!

# ----------------------------------------
# Server Configuration
# ----------------------------------------
SERVER_PORT=8143
DEBUG_MODE=false

# ----------------------------------------
# Storage Configuration
# ----------------------------------------
DB_PATH=.credentials/tokens.db
CREDENTIALS_DIR=.credentials

# ----------------------------------------
# Gemini Configuration
# ----------------------------------------
# Get your credentials from: https://console.cloud.google.com/
GEMINI_BASE_URL=https://cloudcode-pa.googleapis.com/v1internal
GEMINI_CLIENT_ID=your_gemini_client_id_here
GEMINI_CLIENT_SECRET=your_gemini_client_secret_here
GEMINI_REDIRECT_PORT=8085

# ----------------------------------------
# Qwen Configuration
# ----------------------------------------
# Get your credentials from: https://portal.qwen.ai/
QWEN_BASE_URL=https://portal.qwen.ai/v1
QWEN_CLIENT_ID=f0304373b74a44d2b584a3fb70ca9e56
QWEN_SCOPE=openid profile email model.completion
QWEN_TOKEN_URL=https://chat.qwen.ai/api/v1/oauth2/token
QWEN_DEVICE_AUTH_URL=https://chat.qwen.ai/api/v1/oauth2/device/code

# ----------------------------------------
# iFlow Configuration
# ----------------------------------------
# Get your credentials from: https://iflow.cn/
IFLOW_AUTH_URL=https://iflow.cn/oauth
IFLOW_TOKEN_URL=https://iflow.cn/oauth/token
IFLOW_USERINFO_URL=https://iflow.cn/api/oauth/getUserInfo
IFLOW_APIKEY_URL=https://platform.iflow.cn/api/openapi/apikey
IFLOW_API_BASE_URL=https://apis.iflow.cn/v1
IFLOW_CLIENT_ID=10009311001
IFLOW_CLIENT_SECRET=4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW
IFLOW_DEFAULT_PORT=11451

# ----------------------------------------
# Antigravity Configuration
# ----------------------------------------
ANTIGRAVITY_DAILY_BASE_URL=https://daily-cloudcode-pa.sandbox.googleapis.com
ANTIGRAVITY_AUTOPUSH_BASE_URL=https://autopush-cloudcode-pa.sandbox.googleapis.com
```

---

## Implementation Checklist

- [ ] Create `docs/configuration.md`
- [ ] Create `docs/security.md`
- [ ] Update `docs/README.md` with configuration section
- [ ] Update `.env.example` with comprehensive template
- [ ] Verify `.gitignore` includes `.env` and `.credentials/`
- [ ] Add configuration section to main README
- [ ] Test that environment variables are loaded correctly
- [ ] Document any additional environment-specific configurations

---

## Estimated Effort

| Task | Time |
|------|------|
| Create docs/configuration.md | 45 minutes |
| Create docs/security.md | 30 minutes |
| Update docs/README.md | 15 minutes |
| Update .env.example | 15 minutes |
| Verify documentation completeness | 15 minutes |
| **Total** | **2 hours** |

---

## Benefits

1. **Onboarding**: New developers can quickly configure the project
2. **Security**: Clear guidelines for handling credentials
3. **Flexibility**: Easy configuration for different environments
4. **Troubleshooting**: Documentation helps resolve configuration issues
5. **Compliance**: Security guidelines help meet compliance requirements
