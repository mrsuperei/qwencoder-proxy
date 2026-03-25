// QWEncoder Proxy Dashboard JavaScript
// Uses shared utilities from shared.js

// API client is provided by shared.js
// const api = new APIClient();

// DOM Elements
const loadingSection = document.getElementById('loading');
const errorSection = document.getElementById('error');
const errorMessage = document.getElementById('error-message');
const retryButton = document.getElementById('retry-button');
const providersSection = document.getElementById('providers');
const providerList = document.getElementById('provider-list');
const authFlowSection = document.getElementById('auth-flow');
const authProviderName = document.getElementById('auth-provider-name');
const authStatus = document.getElementById('auth-status');
const authLoading = document.getElementById('auth-loading');
const authCancel = document.getElementById('auth-cancel');
const successSection = document.getElementById('success');
const successMessage = document.getElementById('success-message');
const backButton = document.getElementById('back-button');
const deviceFlowSection = document.getElementById('device-flow');
const deviceProviderName = document.getElementById('device-provider-name');
const deviceCode = document.getElementById('device-code');
const verificationUriLink = document.getElementById('verification-uri-link');
const deviceStatus = document.getElementById('device-status');
const deviceCancel = document.getElementById('device-cancel');
const copyCodeBtn = document.getElementById('copy-code-btn');

// State
let currentProvider = null;
let authWindow = null;
let authCheckInterval = null;
let currentPollId = null;
let deviceCheckInterval = null;
let authStartTime = null;
let authTimeout = null;

// Initialize dashboard
document.addEventListener('DOMContentLoaded', () => {
    loadProviders();
    setupEventListeners();
    setupMessageListener();
});

// Setup event listeners
function setupEventListeners() {
    retryButton.addEventListener('click', loadProviders);
    authCancel.addEventListener('click', cancelAuthFlow);
    backButton.addEventListener('click', showProviders);
    deviceCancel.addEventListener('click', cancelDeviceFlow);
    copyCodeBtn.addEventListener('click', copyDeviceCode);
}

// Setup message listener for OAuth callback
function setupMessageListener() {
    window.addEventListener('message', (event) => {
        // Verify origin for security (in production, be more strict)
        if (event.data) {
            if (event.data.type === 'oauth_success') {
                console.log('Received oauth_success message from popup');
                // The popup will close itself after sending this message
                // We'll detect success via credentials check in polling
            } else if (event.data.type === 'oauth_error') {
                console.log('Received oauth_error message from popup:', event.data.error);
                // The popup encountered an error
                // We'll detect this via the polling not finding credentials
            }
        }
    });
}

// Load providers from API
async function loadProviders() {
    showLoading();
    hideError();
    hideProviders();
    hideSuccess();
    hideAuthFlow();

    try {
        const data = await api.get('/providers');
        renderProviders(data.providers);
        showProviders();
        hideLoading();
    } catch (error) {
        console.error('Error loading providers:', error);
        showError(error.message || 'Unable to load providers. Please try again.');
    }
}

// Render provider cards
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

// Create provider card element
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

// Load provider credentials
async function loadProviderCredentials(providerId, card) {
    try {
        const data = await api.get(`/credentials/${providerId}`);
        updateProviderCard(card, data);
    } catch (error) {
        // Provider might not have any credentials yet
        if (error.status !== 404) {
            console.error(`Error loading credentials for ${providerId}:`, error);
        }
    }
}

// Update provider card with credentials
function updateProviderCard(card, credentials) {
    const statusElement = card.querySelector('.provider-status');
    const tokenListElement = card.querySelector(`[id^="tokens-"]`);

    if (credentials.tokens && credentials.tokens.length > 0) {
        // Update status to authenticated
        statusElement.classList.remove('not-authenticated');
        statusElement.classList.add('authenticated');
        statusElement.innerHTML = '<span class="status-dot"></span>Authenticated';

        // Render tokens
        tokenListElement.innerHTML = credentials.tokens.map(token => {
            const expiryDate = new Date(token.expiry_date * 1000);
            const expiryFormatted = expiryDate.toLocaleString();
            const healthyClass = token.healthy ? 'token-healthy' : 'token-unhealthy';
            const healthyText = token.healthy ? 'Healthy' : 'Unhealthy';

            return `
                <div class="token-item">
                    <div class="token-email">${token.email || 'Unknown Email'}</div>
                    <div class="token-meta">
                        <span>Expires: ${expiryFormatted}</span>
                        <span class="${healthyClass}">${healthyText}</span>
                    </div>
                </div>
            `;
        }).join('');
    } else {
        // No tokens
        tokenListElement.innerHTML = '<p class="no-tokens">No tokens added yet.</p>';
    }
}

// Prompt for email/alias for providers that require it
async function promptForEmail(providerName) {
    return new Promise((resolve, reject) => {
        const email = prompt(
            `${providerName} requires your email or alias for token tracking.\n\nPlease enter your email or alias:`,
            ''
        );
        if (email === null) {
            // User cancelled
            reject(new Error('Email input cancelled'));
        } else if (email.trim() === '') {
            reject(new Error('Email/alias is required'));
        } else {
            resolve(email.trim());
        }
    });
}

// Start OAuth authorization flow
async function startAuthFlow(provider) {
    currentProvider = provider;

    try {
        // Get callback URL
        const callbackUrl = `${window.location.origin}/api/callback`;

        // For Qwen, prompt for email
        let email = '';
        if (provider.id === 'qwen') {
            try {
                email = await promptForEmail(provider.name);
            } catch (error) {
                console.error('Email prompt cancelled:', error);
                showError(error.message || 'Email/alias is required for Qwen');
                return;
            }
        }

        // Start auth flow
        const requestBody = {
            provider: provider.id,
            redirect_uri: callbackUrl,
        };
        if (email) {
            requestBody.email = email;
        }

        const data = await api.post('/auth/start', requestBody);

        // Show auth flow UI
        showAuthFlow(provider.name);

        // Use redirect-based OAuth flow instead of popup for better Brave compatibility
        console.log('[Auth] Redirecting to OAuth provider:', data.auth_url.substring(0, 100) + '...');
        
        // Store current provider in sessionStorage for callback handling
        sessionStorage.setItem('oauth_provider_id', provider.id);
        sessionStorage.setItem('oauth_provider_name', provider.name);
        
        // Redirect to OAuth provider
        window.location.href = data.auth_url;

    } catch (error) {
        console.error('Error starting auth flow:', error);
        showError(error.message || 'Failed to start authentication flow.');
        showProviders();
    }
}

// Start checking for authentication completion
function startAuthCheck(providerId) {
    authStartTime = Date.now();
    authStatus.textContent = 'Please complete the authentication in the popup window...';
    authLoading.classList.remove('hidden');

    // Set timeout to prevent dashboard from hanging indefinitely (5 minutes)
    authTimeout = setTimeout(() => {
        stopAuthCheck();
        showError('Authentication timed out. Please try again.');
        showProviders();
    }, 5 * 60 * 1000);

    let windowClosedManually = false;
    let canAccessWindow = true;

    authCheckInterval = setInterval(async () => {
        try {
            console.log('[AuthCheck] Polling...', Date.now() - authStartTime, 'ms elapsed');
            
            // Check if window is closed - but only if it was previously open
            // This prevents false positives when the window is still loading
            if (authWindow) {
                try {
                    // Try to access window.closed - this will throw if window is closed
                    // or if it's on a different domain due to Cross-Origin-Opener-Policy (COOP)
                    const isClosed = authWindow.closed;
                    console.log('[AuthCheck] Window state: closed=', isClosed);
                    
                    // Only treat as closed if we're sure it's closed
                    // and we haven't detected credentials yet
                    if (isClosed && !windowClosedManually) {
                        console.log('[AuthCheck] Window detected as closed, checking credentials...');
                        // Check if we have credentials before giving up
                        const response = await fetch(`${API_BASE}/credentials/${providerId}`);
                        if (response.ok) {
                            const data = await response.json();
                            if (data.tokens && data.tokens.length > 0) {
                                // Authentication was successful!
                                stopAuthCheck();
                                showSuccess(`Token added successfully for ${currentProvider.name}`);
                                setTimeout(() => {
                                    loadProviders();
                                }, 2000);
                                return;
                            }
                        }
                        
                        // No credentials found and window is closed
                        stopAuthCheck();
                        showError('Authentication was cancelled or the popup was closed.');
                        showProviders();
                        return;
                    }
                } catch (e) {
                    // Accessing window properties failed - window might be closed
                    // or on a different domain (COOP blocking)
                    if (e.message && e.message.includes('Cross-Origin-Opener-Policy')) {
                        canAccessWindow = false;
                        console.log('[AuthCheck] COOP blocking window access, relying on credentials check');
                    } else {
                        console.log('[AuthCheck] Window access check failed:', e.message);
                    }
                    // Don't treat this as an error, just continue polling
                    // We'll rely on the credentials check below
                }
            }

            // Check credentials (this works regardless of COOP)
            console.log('[AuthCheck] Checking credentials...');
            try {
                const data = await api.get(`/credentials/${providerId}`);
                console.log('[AuthCheck] Credentials response:', data.tokens ? `${data.tokens.length} tokens` : 'no tokens');
                if (data.tokens && data.tokens.length > 0) {
                    // Authentication successful
                    console.log('[AuthCheck] Authentication successful!');
                    windowClosedManually = true; // Prevent false closure detection
                    stopAuthCheck();
                    if (authWindow && !authWindow.closed) {
                        try {
                            authWindow.close();
                        } catch (e) {
                            // Window might already be closed or inaccessible
                            console.log('[AuthCheck] Could not close window:', e.message);
                        }
                    }
                    showSuccess(`Token added successfully for ${currentProvider.name}`);
                    setTimeout(() => {
                        loadProviders();
                    }, 2000);
                }
            } catch (error) {
                // No credentials yet or error fetching
                console.log('[AuthCheck] No credentials yet or error:', error);
            }
        } catch (error) {
            console.error('Error checking auth status:', error);
        }
    }, 1000);
}

// Stop checking for authentication completion
function stopAuthCheck() {
    if (authCheckInterval) {
        clearInterval(authCheckInterval);
        authCheckInterval = null;
    }
    if (authTimeout) {
        clearTimeout(authTimeout);
        authTimeout = null;
    }
    authLoading.classList.add('hidden');
}

// Cancel auth flow
function cancelAuthFlow() {
    stopAuthCheck();
    if (authWindow && !authWindow.closed) {
        authWindow.close();
    }
    currentProvider = null;
    showProviders();
    hideDeviceFlow();
}

// Start device code flow
async function startDeviceFlow(provider) {
    currentProvider = provider;

    // For Qwen, prompt for email before showing device flow UI
    let email = '';
    if (provider.id === 'qwen') {
        try {
            email = await promptForEmail(provider.name);
        } catch (error) {
            console.error('Email prompt cancelled:', error);
            showError(error.message || 'Email/alias is required for Qwen');
            return;
        }
    }

    hideProviders();
    showDeviceFlow(provider.name);

    try {
        const requestBody = {
            provider: provider.id,
        };
        if (email) {
            requestBody.email = email;
        }

        const data = await api.post('/device/start', requestBody);

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
            const data = await api.get(`/device/status/${currentPollId}`);

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

// Show external provider info
function showExternalProviderInfo(provider) {
    alert(`External Provider: ${provider.name}\n\n${provider.description || 'This provider uses external credentials.'}\n\nPlease refer to the documentation for setup instructions.`);
}

// UI State Management
function showLoading() {
    loadingSection.classList.remove('hidden');
}

function hideLoading() {
    loadingSection.classList.add('hidden');
}

function showError(message) {
    errorMessage.textContent = message;
    errorSection.classList.remove('hidden');
}

function hideError() {
    errorSection.classList.add('hidden');
}

function showProviders() {
    providersSection.classList.remove('hidden');
}

function hideProviders() {
    providersSection.classList.add('hidden');
}

function showAuthFlow(providerName) {
    authProviderName.textContent = `Authenticating with ${providerName}`;
    authFlowSection.classList.remove('hidden');
}

function hideAuthFlow() {
    authFlowSection.classList.add('hidden');
}

function showDeviceFlow(providerName) {
    deviceProviderName.textContent = `Authenticating with ${providerName}`;
    deviceFlowSection.classList.remove('hidden');
}

function hideDeviceFlow() {
    deviceFlowSection.classList.add('hidden');
}

function showSuccess(message) {
    successMessage.textContent = message;
    successSection.classList.remove('hidden');
}

function hideSuccess() {
    successSection.classList.add('hidden');
}
