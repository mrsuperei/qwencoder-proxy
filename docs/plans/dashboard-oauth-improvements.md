# Dashboard OAuth Flow Improvements

## Overview

This plan addresses the OAuth authentication issues in the dashboard, including:
1. Fixing the "Bad Request" error when starting OAuth flows
2. Adding support for device code flow (needed for Qwen provider)
3. Adding support for external flow (needed for Kiro provider)
4. Showing all providers with appropriate UI based on their flow type

## Current State Analysis

### Supported Providers

| Provider ID | Name | Flow Type | Currently Shown in Dashboard |
|-------------|------|-----------|------------------------------|
| `gemini-cli` | Gemini (Google) | authorization_code | ✅ Yes |
| `iflow` | iFlow | authorization_code | ✅ Yes |
| `qwen` | Qwen | device_code | ❌ No |
| `kiro` | Kiro (AWS SSO) | external | ❌ No |

### Root Cause of "Bad Request" Error

The dashboard JavaScript sends incorrect field names in the auth start request:

**Current (Incorrect):**
```javascript
body: JSON.stringify({
    provider_id: provider.id,    // ❌ Wrong field name
    callback_url: callbackUrl,   // ❌ Wrong field name
}),
```

**Expected by Server:**
```go
var req struct {
    Provider    string `json:"provider"`        // ✅ Correct
    RedirectURI string `json:"redirect_uri"`   // ✅ Correct
}
```

This JSON field name mismatch causes the `ParseJSON` function to fail, resulting in a 400 Bad Request error.

## Proposed Changes

### 1. Fix Field Name Mismatch (Critical - Fixes the "Bad Request" Error)

**File:** `internal/restapi/web/js/dashboard.js`

**Location:** Lines 177-186 (in the `startAuthFlow` function)

**Change:**
```javascript
// Before:
body: JSON.stringify({
    provider_id: provider.id,
    callback_url: callbackUrl,
}),

// After:
body: JSON.stringify({
    provider: provider.id,
    redirect_uri: callbackUrl,
}),
```

### 2. Add Device Flow Support for Dashboard

Device code flow is used by providers like Qwen that don't support redirect-based OAuth.

#### 2.1 Add HTML Elements for Device Flow

**File:** `internal/restapi/web/index.html`

**Add after the `auth-flow` section (around line 43):**
```html
<section id="device-flow" class="device-flow hidden">
    <div class="device-flow-content">
        <h2 id="device-provider-name">Device Authentication</h2>
        <p class="device-instructions">
            Follow these steps to authenticate:
        </p>
        <ol class="device-steps">
            <li>Click the button below to open the verification page</li>
            <li>Enter the code shown below on the verification page</li>
            <li>Wait for authentication to complete</li>
        </ol>
        <div class="device-code-container">
            <div class="device-code-label">Your Code:</div>
            <div id="device-code" class="device-code">Loading...</div>
            <button id="copy-code-btn" class="btn btn-secondary btn-sm">Copy Code</button>
        </div>
        <div class="verification-uri-container">
            <a id="verification-uri-link" href="#" target="_blank" class="btn btn-primary btn-block">
                Open Verification Page
            </a>
        </div>
        <div id="device-status" class="device-status">
            <div class="spinner"></div>
            <p>Waiting for authentication...</p>
        </div>
        <button id="device-cancel" class="btn btn-secondary">Cancel</button>
    </div>
</section>
```

#### 2.2 Add JavaScript Functions for Device Flow

**File:** `internal/restapi/web/js/dashboard.js`

**Add new DOM element references (around line 20):**
```javascript
const deviceFlowSection = document.getElementById('device-flow');
const deviceProviderName = document.getElementById('device-provider-name');
const deviceCode = document.getElementById('device-code');
const verificationUriLink = document.getElementById('verification-uri-link');
const deviceStatus = document.getElementById('device-status');
const deviceCancel = document.getElementById('device-cancel');
const copyCodeBtn = document.getElementById('copy-code-btn');
```

**Add to setupEventListeners function (around line 36):**
```javascript
deviceCancel.addEventListener('click', cancelDeviceFlow);
copyCodeBtn.addEventListener('click', copyDeviceCode);
```

**Add new state variables (around line 25):**
```javascript
let currentPollId = null;
let deviceCheckInterval = null;
```

**Add new functions:**
```javascript
// Start device code flow
async function startDeviceFlow(provider) {
    currentProvider = provider;
    hideProviders();
    showDeviceFlow(provider.name);

    try {
        const response = await fetch(`${API_BASE}/device/start`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                provider: provider.id,
            }),
        });

        if (!response.ok) {
            throw new Error(`Failed to start device flow: ${response.statusText}`);
        }

        const data = await response.json();

        // Update UI with device code and verification URI
        deviceCode.textContent = data.user_code;
        verificationUriLink.href = data.verification_uri_complete || data.verification_uri;

        // Start polling for status
        currentPollId = data.poll_id;
        startDeviceStatusCheck();

    } catch (error) {
        console.error('Error starting device flow:', error);
        showError(error.message || 'Failed to start device authentication flow.');
        showProviders();
    }
}

// Start checking for device authentication completion
function startDeviceStatusCheck() {
    deviceStatus.classList.remove('hidden');

    deviceCheckInterval = setInterval(async () => {
        try {
            const response = await fetch(`${API_BASE}/device/status/${currentPollId}`);

            if (!response.ok) {
                // Still pending or error
                return;
            }

            const data = await response.json();

            if (data.status === 'completed') {
                // Authentication successful
                stopDeviceStatusCheck();
                if (authWindow && !authWindow.closed) {
                    authWindow.close();
                }
                showSuccess(`Token added successfully for ${currentProvider.name}`);
                setTimeout(() => {
                    loadProviders();
                }, 2000);
            } else if (data.status === 'error') {
                // Authentication failed
                stopDeviceStatusCheck();
                showError(data.error || 'Device authentication failed.');
                showProviders();
            }
        } catch (error) {
            console.error('Error checking device status:', error);
        }
    }, 2000); // Check every 2 seconds
}

// Stop checking for device authentication completion
function stopDeviceStatusCheck() {
    if (deviceCheckInterval) {
        clearInterval(deviceCheckInterval);
        deviceCheckInterval = null;
    }
    deviceStatus.classList.add('hidden');
}

// Cancel device flow
function cancelDeviceFlow() {
    stopDeviceStatusCheck();
    currentPollId = null;
    currentProvider = null;
    showProviders();
}

// Copy device code to clipboard
function copyDeviceCode() {
    navigator.clipboard.writeText(deviceCode.textContent).then(() => {
        const originalText = copyCodeBtn.textContent;
        copyCodeBtn.textContent = 'Copied!';
        setTimeout(() => {
            copyCodeBtn.textContent = originalText;
        }, 2000);
    }).catch(err => {
        console.error('Failed to copy code:', err);
    });
}

// UI State Management for device flow
function showDeviceFlow(providerName) {
    deviceProviderName.textContent = `Authenticating with ${providerName}`;
    deviceFlowSection.classList.remove('hidden');
}

function hideDeviceFlow() {
    deviceFlowSection.classList.add('hidden');
}
```

#### 2.3 Add CSS Styles for Device Flow

**File:** `internal/restapi/web/css/dashboard.css`

**Add at the end of the file:**
```css
/* Device Flow Styles */
.device-flow {
    text-align: center;
    padding: 2rem;
    background: var(--card-background);
    border-radius: var(--radius);
    box-shadow: var(--shadow-md);
}

.device-flow-content h2 {
    margin-bottom: 1.5rem;
    font-size: 1.5rem;
}

.device-instructions {
    color: var(--text-secondary);
    margin-bottom: 1.5rem;
}

.device-steps {
    text-align: left;
    margin-bottom: 2rem;
    padding-left: 2rem;
    color: var(--text-secondary);
}

.device-steps li {
    margin-bottom: 0.5rem;
}

.device-code-container {
    margin-bottom: 2rem;
    padding: 1.5rem;
    background: var(--background-color);
    border-radius: var(--radius);
    border: 2px solid var(--border-color);
}

.device-code-label {
    font-size: 0.875rem;
    color: var(--text-secondary);
    margin-bottom: 0.5rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
}

.device-code {
    font-size: 2rem;
    font-weight: 700;
    font-family: 'Courier New', monospace;
    letter-spacing: 0.25em;
    color: var(--primary-color);
    margin-bottom: 1rem;
}

.verification-uri-container {
    margin-bottom: 1.5rem;
}

.device-status {
    margin: 1.5rem 0;
    padding: 1rem;
    background: var(--background-color);
    border-radius: var(--radius);
}

.device-status p {
    color: var(--text-secondary);
    margin-top: 0.5rem;
}
```

### 3. Update Provider Rendering Logic

**File:** `internal/restapi/web/js/dashboard.js`

**Modify the `renderProviders` function (lines 64-80):**

**Before:**
```javascript
function renderProviders(providers) {
    providerList.innerHTML = '';

    // Filter for authorization code flow providers only
    const authCodeProviders = providers.filter(p => p.flow === 'authorization_code');

    if (authCodeProviders.length === 0) {
        providerList.innerHTML = '<p class="no-tokens">No authorization code providers available.</p>';
        return;
    }

    authCodeProviders.forEach(provider => {
        const card = createProviderCard(provider);
        providerList.appendChild(card);
    });
}
```

**After:**
```javascript
function renderProviders(providers) {
    providerList.innerHTML = '';

    if (providers.length === 0) {
        providerList.innerHTML = '<p class="no-tokens">No providers available.</p>';
        return;
    }

    providers.forEach(provider => {
        const card = createProviderCard(provider);
        providerList.appendChild(card);
    });
}
```

**Modify the `createProviderCard` function (lines 83-117):**

**Before:**
```javascript
function createProviderCard(provider) {
    const card = document.createElement('div');
    card.className = 'provider-card';
    card.dataset.providerId = provider.id;

    const isAuthenticated = false; // Will be updated when we fetch credentials

    card.innerHTML = `
        <div class="provider-header">
            <h3 class="provider-name">${provider.name}</h3>
            <span class="provider-status ${isAuthenticated ? 'authenticated' : 'not-authenticated'}">
                <span class="status-dot"></span>
                ${isAuthenticated ? 'Authenticated' : 'Not Authenticated'}
            </span>
        </div>
        <p class="provider-description">OAuth 2.0 Authorization Code Flow</p>
        <div class="provider-actions">
            <button class="btn btn-primary btn-block" data-action="add-token" data-provider-id="${provider.id}">
                Add Token
            </button>
        </div>
        <div class="token-list" id="tokens-${provider.id}">
            <!-- Tokens will be loaded here -->
        </div>
    `;

    // Add event listener for add token button
    const addTokenButton = card.querySelector('[data-action="add-token"]');
    addTokenButton.addEventListener('click', () => startAuthFlow(provider));

    // Load credentials for this provider
    loadProviderCredentials(provider.id, card);

    return card;
}
```

**After:**
```javascript
function createProviderCard(provider) {
    const card = document.createElement('div');
    card.className = 'provider-card';
    card.dataset.providerId = provider.id;

    const isAuthenticated = false; // Will be updated when we fetch credentials

    // Determine flow type label and description
    let flowLabel, flowDescription, actionLabel, actionHandler;
    switch (provider.flow) {
        case 'authorization_code':
            flowLabel = 'Auth Code Flow';
            flowDescription = 'OAuth 2.0 Authorization Code Flow';
            actionLabel = 'Add Token';
            actionHandler = () => startAuthFlow(provider);
            break;
        case 'device_code':
            flowLabel = 'Device Flow';
            flowDescription = 'OAuth 2.0 Device Code Flow';
            actionLabel = 'Start Authentication';
            actionHandler = () => startDeviceFlow(provider);
            break;
        case 'external':
            flowLabel = 'External';
            flowDescription = provider.description || 'Uses external credentials';
            actionLabel = 'View Instructions';
            actionHandler = () => showExternalProviderInfo(provider);
            break;
        default:
            flowLabel = 'Unknown';
            flowDescription = 'Unknown authentication flow';
            actionLabel = 'Not Available';
            actionHandler = null;
    }

    card.innerHTML = `
        <div class="provider-header">
            <h3 class="provider-name">${provider.name}</h3>
            <span class="provider-status ${isAuthenticated ? 'authenticated' : 'not-authenticated'}">
                <span class="status-dot"></span>
                ${isAuthenticated ? 'Authenticated' : 'Not Authenticated'}
            </span>
        </div>
        <div class="provider-flow-badge">${flowLabel}</div>
        <p class="provider-description">${flowDescription}</p>
        <div class="provider-actions">
            ${actionHandler ? `
                <button class="btn btn-primary btn-block" data-action="add-token" data-provider-id="${provider.id}">
                    ${actionLabel}
                </button>
            ` : `
                <button class="btn btn-secondary btn-block" disabled>
                    ${actionLabel}
                </button>
            `}
        </div>
        <div class="token-list" id="tokens-${provider.id}">
            <!-- Tokens will be loaded here -->
        </div>
    `;

    // Add event listener for add token button
    if (actionHandler) {
        const addTokenButton = card.querySelector('[data-action="add-token"]');
        addTokenButton.addEventListener('click', actionHandler);
    }

    // Load credentials for this provider
    loadProviderCredentials(provider.id, card);

    return card;
}
```

**Add new function for external provider info:**
```javascript
function showExternalProviderInfo(provider) {
    alert(`External Provider: ${provider.name}\n\n${provider.description || 'This provider uses external credentials.'}\n\nPlease refer to the documentation for setup instructions.`);
}
```

### 4. Add CSS for Flow Badge

**File:** `internal/restapi/web/css/dashboard.css`

**Add after provider card styles:**
```css
/* Provider Flow Badge */
.provider-flow-badge {
    display: inline-block;
    padding: 0.25rem 0.75rem;
    background: var(--background-color);
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    margin-bottom: 0.75rem;
}
```

### 5. Update hideDeviceFlow in UI State Management

**File:** `internal/restapi/web/js/dashboard.js`

**Add to the hide functions section (around line 312):**
```javascript
function hideDeviceFlow() {
    deviceFlowSection.classList.add('hidden');
}
```

**Update `cancelAuthFlow` to also hide device flow:**
```javascript
function cancelAuthFlow() {
    stopAuthCheck();
    if (authWindow && !authWindow.closed) {
        authWindow.close();
    }
    currentProvider = null;
    showProviders();
    hideDeviceFlow(); // Add this line
}
```

## Implementation Order

1. **Fix Field Name Mismatch** (Critical - Fixes the immediate bug)
   - Modify `startAuthFlow` function in `dashboard.js`

2. **Add Device Flow HTML**
   - Add device flow section to `index.html`

3. **Add Device Flow CSS**
   - Add device flow styles to `dashboard.css`

4. **Add Device Flow JavaScript**
   - Add DOM element references
   - Add new functions for device flow
   - Update event listeners

5. **Update Provider Rendering**
   - Modify `renderProviders` to show all providers
   - Modify `createProviderCard` to handle different flow types
   - Add external provider info function

6. **Add Flow Badge CSS**
   - Add styles for the flow type badge

## Testing Checklist

- [ ] Authorization code flow works for Gemini
- [ ] Authorization code flow works for iFlow
- [ ] Device code flow works for Qwen
- [ ] External flow shows info for Kiro
- [ ] All providers are displayed
- [ ] Flow type badges are shown correctly
- [ ] Error handling works properly
- [ ] Copy device code button works
- [ ] Verification URI link opens in new tab
- [ ] Polling for device flow status works
- [ ] Cancel buttons work for both flows
- [ ] Token list updates after successful authentication

## Files to Modify

1. `internal/restapi/web/js/dashboard.js`
2. `internal/restapi/web/index.html`
3. `internal/restapi/web/css/dashboard.css`

## API Endpoints Used

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/providers` | GET | Get list of all providers |
| `/api/auth/start` | POST | Start authorization code flow |
| `/api/callback` | GET | OAuth callback handler |
| `/api/device/start` | POST | Start device code flow |
| `/api/device/status/{poll_id}` | GET | Check device flow status |
| `/api/credentials/{provider_id}` | GET | Get provider credentials/tokens |
