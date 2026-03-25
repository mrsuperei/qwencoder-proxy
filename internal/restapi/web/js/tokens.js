// QWEncoder Proxy Token Management JavaScript
// Uses shared utilities from shared.js

// DOM Elements
const loadingSection = document.getElementById('loading');
const errorSection = document.getElementById('error');
const errorMessage = document.getElementById('error-message');
const retryButton = document.getElementById('retry-button');
const emptySection = document.getElementById('empty');
const tokensContainer = document.getElementById('tokens-container');
const providerGroups = document.getElementById('provider-groups');
const refreshAllBtn = document.getElementById('refresh-all-btn');

// Proxy Modal Elements
const proxyModal = document.getElementById('proxy-modal');
const proxyModalTitle = document.getElementById('proxy-modal-title');
const proxyModalClose = document.getElementById('proxy-modal-close');
const proxyTokenEmail = document.getElementById('proxy-token-email');
const proxyForm = document.getElementById('proxy-form');
const proxyType = document.getElementById('proxy-type');
const proxyFields = document.getElementById('proxy-fields');
const proxyHost = document.getElementById('proxy-host');
const proxyPort = document.getElementById('proxy-port');
const proxyUsername = document.getElementById('proxy-username');
const proxyPassword = document.getElementById('proxy-password');
const proxyHealthStatus = document.getElementById('proxy-health-status');
const healthStatusIndicator = document.getElementById('health-status-indicator');
const healthStatusMessage = document.getElementById('health-status-message');
const testProxyBtn = document.getElementById('test-proxy-btn');
const saveProxyBtn = document.getElementById('save-proxy-btn');
const cancelProxyBtn = document.getElementById('cancel-proxy-btn');

// Delete Modal Elements
const deleteModal = document.getElementById('delete-modal');
const deleteModalClose = document.getElementById('delete-modal-close');
const deleteTokenInfo = document.getElementById('delete-token-info');
const cancelDeleteBtn = document.getElementById('cancel-delete-btn');
const confirmDeleteBtn = document.getElementById('confirm-delete-btn');

// State
let credentialsData = null;
let currentToken = null;
let currentProviderId = null;
let providersList = [];

// Initialize tokens page
document.addEventListener('DOMContentLoaded', () => {
    loadTokens();
    setupEventListeners();
});

// Setup event listeners
function setupEventListeners() {
    retryButton.addEventListener('click', loadTokens);
    refreshAllBtn.addEventListener('click', loadTokens);

    // Proxy modal events
    proxyModalClose.addEventListener('click', hideProxyModal);
    cancelProxyBtn.addEventListener('click', hideProxyModal);
    proxyType.addEventListener('change', handleProxyTypeChange);
    testProxyBtn.addEventListener('click', testProxyConnection);
    saveProxyBtn.addEventListener('click', saveProxyConfig);

    // Delete modal events
    deleteModalClose.addEventListener('click', hideDeleteModal);
    cancelDeleteBtn.addEventListener('click', hideDeleteModal);
    confirmDeleteBtn.addEventListener('click', confirmDeleteToken);

    // Close modals on overlay click
    proxyModal.addEventListener('click', (e) => {
        if (e.target === proxyModal) hideProxyModal();
    });
    deleteModal.addEventListener('click', (e) => {
        if (e.target === deleteModal) hideDeleteModal();
    });
}

// Load all tokens from API
async function loadTokens() {
    showLoading();
    hideError();
    hideEmpty();
    hideTokensContainer();

    try {
        // Load providers list first
        const providersData = await api.get('/providers');
        providersList = providersData.providers;

        // Load all credentials
        credentialsData = await api.get('/credentials');

        if (!credentialsData.credentials || credentialsData.credentials.length === 0) {
            showEmpty();
            hideLoading();
            return;
        }

        renderTokensPage(credentialsData.credentials);
        showTokensContainer();
        hideLoading();
    } catch (error) {
        console.error('Error loading tokens:', error);
        showError(error.message || 'Unable to load tokens. Please try again.');
    }
}

// Render tokens page
function renderTokensPage(credentials) {
    providerGroups.innerHTML = '';

    credentials.forEach(providerCreds => {
        if (!providerCreds.tokens || providerCreds.tokens.length === 0) {
            return; // Skip providers with no tokens
        }

        const providerInfo = providersList.find(p => p.id === providerCreds.provider);
        const providerName = providerInfo ? providerInfo.name : providerCreds.provider;

        const group = createProviderGroup(providerName, providerCreds.provider, providerCreds.tokens);
        providerGroups.appendChild(group);
    });
}

// Create provider group element
function createProviderGroup(providerName, providerId, tokens) {
    const group = document.createElement('div');
    group.className = 'provider-group';
    group.dataset.providerId = providerId;

    const header = document.createElement('div');
    header.className = 'provider-group-header';
    header.innerHTML = `
        <div class="provider-group-title">
            <span class="provider-group-name">${escapeHTML(providerName)}</span>
            <span class="provider-group-count">${tokens.length}</span>
        </div>
        <div class="provider-group-toggle">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="6 9 12 15 18 9"></polyline>
            </svg>
        </div>
    `;

    const content = document.createElement('div');
    content.className = 'provider-group-content';

    const cardsContainer = document.createElement('div');
    cardsContainer.className = 'token-cards';

    tokens.forEach(token => {
        const card = createTokenCard(token, providerId);
        cardsContainer.appendChild(card);
    });

    content.appendChild(cardsContainer);
    group.appendChild(header);
    group.appendChild(content);

    // Toggle collapse on header click
    header.addEventListener('click', () => {
        group.classList.toggle('collapsed');
    });

    return group;
}

// Create token card element
function createTokenCard(token, providerId) {
    const card = document.createElement('div');
    card.className = 'token-card';
    card.dataset.tokenId = token.id;
    card.dataset.providerId = providerId;

    const expiry = formatExpiry(token.expiry_date);
    const expiryClass = getExpiryStatusClass(expiry.daysRemaining);
    const healthClass = token.healthy ? 'healthy' : 'unhealthy';
    const proxyClass = token.proxy && token.proxy.type !== 'none' ? 'has-proxy' : 'no-proxy';

    card.innerHTML = `
        <div class="token-header">
            <div>
                <div class="token-email">${escapeHTML(token.email || 'Unknown Email')}</div>
                <div class="token-id">${escapeHTML(token.id.substring(0, 8))}...</div>
            </div>
            <div class="token-status-badges">
                <span class="status-badge ${healthClass}">
                    ${token.healthy ? '✓ Healthy' : '✗ Unhealthy'}
                </span>
                <span class="status-badge ${proxyClass}">
                    ${token.proxy && token.proxy.type !== 'none' ? '🔗 Proxy' : 'No Proxy'}
                </span>
            </div>
        </div>
        <div class="token-info">
            <div class="token-info-row">
                <span class="token-info-label">Expires:</span>
                <span class="token-info-value ${expiryClass}">
                    ${expiry.formatted} (${expiry.relative})
                </span>
            </div>
            <div class="token-info-row">
                <span class="token-info-label">Created:</span>
                <span class="token-info-value">${formatDate(token.created_at)}</span>
            </div>
            <div class="token-info-row">
                <span class="token-info-label">Last Used:</span>
                <span class="token-info-value">${token.last_used ? formatDate(token.last_used) : 'Never'}</span>
            </div>
            <div class="token-info-row">
                <span class="token-info-label">Error Count:</span>
                <span class="token-info-value ${token.error_count > 0 ? 'status-expired' : 'status-ok'}">
                    ${token.error_count}
                </span>
            </div>
        </div>
        <div class="token-actions">
            <button class="btn btn-secondary btn-sm" data-action="refresh" data-provider-id="${providerId}" data-token-id="${token.id}">
                Refresh
            </button>
            <button class="btn btn-secondary btn-sm" data-action="configure-proxy" data-provider-id="${providerId}" data-token-id="${token.id}">
                Configure Proxy
            </button>
            <button class="btn btn-danger btn-sm" data-action="delete" data-provider-id="${providerId}" data-token-id="${token.id}">
                Delete
            </button>
        </div>
    `;

    // Add event listeners for buttons
    const refreshBtn = card.querySelector('[data-action="refresh"]');
    refreshBtn.addEventListener('click', () => refreshToken(providerId, token.id, card));

    const configureProxyBtn = card.querySelector('[data-action="configure-proxy"]');
    configureProxyBtn.addEventListener('click', () => showProxyModal(token, providerId));

    const deleteBtn = card.querySelector('[data-action="delete"]');
    deleteBtn.addEventListener('click', () => showDeleteModal(token, providerId));

    return card;
}

// Refresh a token
async function refreshToken(providerId, tokenId, card) {
    showElementLoading(card);

    try {
        const result = await api.post(`/credentials/${providerId}/${tokenId}/refresh`, {});
        if (result.success) {
            showToast('Token refreshed successfully', 'success');
            // Reload all tokens to get updated data
            await loadTokens();
        }
    } catch (error) {
        console.error('Error refreshing token:', error);
        showToast(error.message || 'Failed to refresh token', 'error');
    } finally {
        hideElementLoading(card);
    }
}

// Show proxy configuration modal
async function showProxyModal(token, providerId) {
    currentToken = token;
    currentProviderId = providerId;

    proxyTokenEmail.textContent = token.email || 'Unknown Email';
    proxyForm.reset();

    try {
        // Load existing proxy config
        const data = await api.get(`/credentials/${providerId}/${token.id}/proxy`);
        
        if (data.proxy && data.proxy.type !== 'none') {
            proxyType.value = data.proxy.type;
            proxyHost.value = data.proxy.host || '';
            proxyPort.value = data.proxy.port || '';
            proxyUsername.value = data.proxy.username || '';
            proxyPassword.value = ''; // Don't show password
            proxyFields.classList.remove('hidden');
        } else {
            proxyType.value = 'none';
            proxyFields.classList.add('hidden');
        }

        // Show health status if available
        if (data.health_status) {
            proxyHealthStatus.classList.remove('hidden');
            if (data.health_status.healthy) {
                healthStatusIndicator.className = 'health-status-indicator healthy';
                healthStatusMessage.textContent = `Healthy (${data.health_status.latency_ms}ms)`;
            } else {
                healthStatusIndicator.className = 'health-status-indicator unhealthy';
                healthStatusMessage.textContent = data.health_status.error || 'Unhealthy';
            }
        } else {
            proxyHealthStatus.classList.add('hidden');
        }
    } catch (error) {
        console.error('Error loading proxy config:', error);
        // Show modal with default values
        proxyType.value = 'none';
        proxyFields.classList.add('hidden');
        proxyHealthStatus.classList.add('hidden');
    }

    proxyModal.classList.remove('hidden');
}

// Hide proxy modal
function hideProxyModal() {
    proxyModal.classList.add('hidden');
    currentToken = null;
    currentProviderId = null;
}

// Handle proxy type change
function handleProxyTypeChange() {
    if (proxyType.value === 'none') {
        proxyFields.classList.add('hidden');
    } else {
        proxyFields.classList.remove('hidden');
    }
}

// Test proxy connection
async function testProxyConnection() {
    if (proxyType.value === 'none') {
        showToast('Please select a proxy type', 'warning');
        return;
    }

    const proxyConfig = {
        type: proxyType.value,
        host: proxyHost.value.trim(),
        port: parseInt(proxyPort.value, 10),
        username: proxyUsername.value.trim() || undefined,
        password: proxyPassword.value || undefined,
    };

    // Validate required fields
    if (!proxyConfig.host) {
        showToast('Host is required', 'error');
        return;
    }

    if (!proxyConfig.port || proxyConfig.port < 1 || proxyConfig.port > 65535) {
        showToast('Port must be between 1 and 65535', 'error');
        return;
    }

    testProxyBtn.disabled = true;
    testProxyBtn.textContent = 'Testing...';

    try {
        const result = await api.post('/proxy/test', proxyConfig);
        if (result.success) {
            showToast(`Connection successful (${result.latency_ms}ms)`, 'success');
            proxyHealthStatus.classList.remove('hidden');
            healthStatusIndicator.className = 'health-status-indicator healthy';
            healthStatusMessage.textContent = `Healthy (${result.latency_ms}ms)`;
        } else {
            showToast(`Connection failed: ${result.error}`, 'error');
            proxyHealthStatus.classList.remove('hidden');
            healthStatusIndicator.className = 'health-status-indicator unhealthy';
            healthStatusMessage.textContent = result.error || 'Unhealthy';
        }
    } catch (error) {
        console.error('Error testing proxy:', error);
        showToast(error.message || 'Failed to test proxy connection', 'error');
    } finally {
        testProxyBtn.disabled = false;
        testProxyBtn.textContent = 'Test Connection';
    }
}

// Save proxy configuration
async function saveProxyConfig() {
    if (!currentToken || !currentProviderId) {
        return;
    }

    const proxyConfig = {
        type: proxyType.value,
    };

    if (proxyType.value !== 'none') {
        proxyConfig.host = proxyHost.value.trim();
        proxyConfig.port = parseInt(proxyPort.value, 10);
        
        if (proxyUsername.value.trim()) {
            proxyConfig.username = proxyUsername.value.trim();
        }
        if (proxyPassword.value) {
            proxyConfig.password = proxyPassword.value;
        }
    }

    // Validate required fields
    if (proxyConfig.type !== 'none' && !proxyConfig.host) {
        showToast('Host is required', 'error');
        return;
    }

    if (proxyConfig.type !== 'none' && (!proxyConfig.port || proxyConfig.port < 1 || proxyConfig.port > 65535)) {
        showToast('Port must be between 1 and 65535', 'error');
        return;
    }

    saveProxyBtn.disabled = true;
    saveProxyBtn.textContent = 'Saving...';

    try {
        await api.put(`/credentials/${currentProviderId}/${currentToken.id}/proxy`, proxyConfig);
        showToast('Proxy configuration saved', 'success');
        hideProxyModal();
        // Reload tokens to show updated proxy status
        await loadTokens();
    } catch (error) {
        console.error('Error saving proxy config:', error);
        showToast(error.message || 'Failed to save proxy configuration', 'error');
    } finally {
        saveProxyBtn.disabled = false;
        saveProxyBtn.textContent = 'Save';
    }
}

// Show delete confirmation modal
function showDeleteModal(token, providerId) {
    currentToken = token;
    currentProviderId = providerId;

    deleteTokenInfo.textContent = `${token.email || 'Unknown Email'} (${token.id.substring(0, 8)}...)`;
    deleteModal.classList.remove('hidden');
}

// Hide delete modal
function hideDeleteModal() {
    deleteModal.classList.add('hidden');
    currentToken = null;
    currentProviderId = null;
}

// Confirm delete token
async function confirmDeleteToken() {
    if (!currentToken || !currentProviderId) {
        return;
    }

    confirmDeleteBtn.disabled = true;
    confirmDeleteBtn.textContent = 'Deleting...';

    try {
        await api.delete(`/credentials/${currentProviderId}/${currentToken.id}`);
        showToast('Token deleted successfully', 'success');
        hideDeleteModal();
        // Reload tokens to update the display
        await loadTokens();
    } catch (error) {
        console.error('Error deleting token:', error);
        showToast(error.message || 'Failed to delete token', 'error');
    } finally {
        confirmDeleteBtn.disabled = false;
        confirmDeleteBtn.textContent = 'Delete';
    }
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

function showEmpty() {
    emptySection.classList.remove('hidden');
}

function hideEmpty() {
    emptySection.classList.add('hidden');
}

function showTokensContainer() {
    tokensContainer.classList.remove('hidden');
}

function hideTokensContainer() {
    tokensContainer.classList.add('hidden');
}
