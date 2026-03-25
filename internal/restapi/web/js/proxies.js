// QWEncoder Proxy Management JavaScript
// Uses shared utilities from shared.js

// DOM Elements
const loadingSection = document.getElementById('loading');
const errorSection = document.getElementById('error');
const errorMessage = document.getElementById('error-message');
const retryButton = document.getElementById('retry-button');
const emptySection = document.getElementById('empty');
const proxiesContainer = document.getElementById('proxies-container');
const proxyList = document.getElementById('proxy-list');
const addProxyBtn = document.getElementById('add-proxy-btn');
const addProxyBtnEmpty = document.getElementById('add-proxy-btn-empty');

// Proxy Modal Elements
const proxyModal = document.getElementById('proxy-modal');
const proxyModalTitle = document.getElementById('proxy-modal-title');
const proxyModalClose = document.getElementById('proxy-modal-close');
const proxyForm = document.getElementById('proxy-form');
const proxyType = document.getElementById('proxy-type');
const proxyHost = document.getElementById('proxy-host');
const proxyPort = document.getElementById('proxy-port');
const proxyUsername = document.getElementById('proxy-username');
const proxyPassword = document.getElementById('proxy-password');
const testProxyBtn = document.getElementById('test-proxy-btn');
const saveProxyBtn = document.getElementById('save-proxy-btn');
const cancelProxyBtn = document.getElementById('cancel-proxy-btn');
const testResult = document.getElementById('test-result');
const testResultIcon = document.getElementById('test-result-icon');
const testResultMessage = document.getElementById('test-result-message');

// Delete Modal Elements
const deleteModal = document.getElementById('delete-modal');
const deleteModalClose = document.getElementById('delete-modal-close');
const deleteProxyInfo = document.getElementById('delete-proxy-info');
const linkedTokensWarning = document.getElementById('linked-tokens-warning');
const linkedTokensCount = document.getElementById('linked-tokens-count');
const cancelDeleteBtn = document.getElementById('cancel-delete-btn');
const confirmDeleteBtn = document.getElementById('confirm-delete-btn');

// Linked Tokens Modal Elements
const linkedTokensModal = document.getElementById('linked-tokens-modal');
const linkedTokensModalClose = document.getElementById('linked-tokens-modal-close');
const linkedProxyInfo = document.getElementById('linked-proxy-info');
const linkedTokensList = document.getElementById('linked-tokens-list');
const noLinkedTokens = document.getElementById('no-linked-tokens');
const closeLinkedTokensBtn = document.getElementById('close-linked-tokens-btn');

// State
let proxiesData = [];
let currentProxy = null; // For editing
let proxyToDelete = null;

// Initialize proxies page
document.addEventListener('DOMContentLoaded', () => {
    loadProxies();
    setupEventListeners();
});

// Setup event listeners
function setupEventListeners() {
    retryButton.addEventListener('click', loadProxies);
    addProxyBtn.addEventListener('click', showAddProxyModal);
    addProxyBtnEmpty.addEventListener('click', showAddProxyModal);

    // Proxy modal events
    proxyModalClose.addEventListener('click', hideProxyModal);
    cancelProxyBtn.addEventListener('click', hideProxyModal);
    testProxyBtn.addEventListener('click', testProxyConnection);
    saveProxyBtn.addEventListener('click', saveProxy);

    // Delete modal events
    deleteModalClose.addEventListener('click', hideDeleteModal);
    cancelDeleteBtn.addEventListener('click', hideDeleteModal);
    confirmDeleteBtn.addEventListener('click', deleteProxy);

    // Linked tokens modal events
    linkedTokensModalClose.addEventListener('click', hideLinkedTokensModal);
    closeLinkedTokensBtn.addEventListener('click', hideLinkedTokensModal);

    // Close modals on overlay click
    proxyModal.addEventListener('click', (e) => {
        if (e.target === proxyModal) hideProxyModal();
    });
    deleteModal.addEventListener('click', (e) => {
        if (e.target === deleteModal) hideDeleteModal();
    });
    linkedTokensModal.addEventListener('click', (e) => {
        if (e.target === linkedTokensModal) hideLinkedTokensModal();
    });
}

// Load all proxies from API
async function loadProxies() {
    showLoading();
    hideError();
    hideEmpty();
    hideProxiesContainer();

    try {
        const response = await fetch('/api/proxies');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const data = await response.json();
        proxiesData = data.proxies || [];

        if (proxiesData.length === 0) {
            showEmpty();
        } else {
            renderProxies(proxiesData);
            showProxiesContainer();
        }
    } catch (error) {
        showError(error.message);
    }
}

// Render proxies list
function renderProxies(proxies) {
    proxyList.innerHTML = '';

    proxies.forEach(proxy => {
        const card = createProxyCard(proxy);
        proxyList.appendChild(card);
    });
}

// Create proxy card element
function createProxyCard(proxy) {
    const card = document.createElement('div');
    card.className = 'proxy-card';
    card.innerHTML = `
        <div class="proxy-card-header">
            <h3 class="proxy-card-title">
                <span class="proxy-type-badge ${proxy.type}">${proxy.type.toUpperCase()}</span>
            </h3>
            <div class="proxy-card-actions">
                <button class="btn btn-sm btn-secondary" onclick="editProxy('${proxy.id}')">
                    <span class="btn-icon">✏️</span> Edit
                </button>
                <button class="btn btn-sm btn-danger" onclick="showDeleteProxyModal('${proxy.id}')">
                    <span class="btn-icon">🗑️</span> Delete
                </button>
            </div>
        </div>
        <div class="proxy-details">
            <div class="proxy-detail-row">
                <span class="proxy-detail-label">Host:</span>
                <span class="proxy-detail-value">${escapeHtml(proxy.host)}</span>
            </div>
            <div class="proxy-detail-row">
                <span class="proxy-detail-label">Port:</span>
                <span class="proxy-detail-value">${proxy.port}</span>
            </div>
            ${proxy.username ? `
            <div class="proxy-detail-row">
                <span class="proxy-detail-label">Username:</span>
                <span class="proxy-detail-value">${escapeHtml(proxy.username)}</span>
            </div>
            ` : ''}
        </div>
        <div class="proxy-token-count">
            <div class="proxy-token-count-info">
                <span class="proxy-token-count-icon">🔑</span>
                <span class="proxy-token-count-value">${proxy.token_count} token${proxy.token_count !== 1 ? 's' : ''}</span>
            </div>
            ${proxy.token_count > 0 ? `
            <button class="view-tokens-btn" onclick="showLinkedTokens('${proxy.id}')">
                View Tokens
            </button>
            ` : ''}
        </div>
    `;
    return card;
}

// Show add proxy modal
function showAddProxyModal() {
    currentProxy = null;
    proxyModalTitle.textContent = 'Add Proxy';
    proxyForm.reset();
    proxyType.value = 'http';
    hideTestResult();
    showProxyModal();
}

// Edit proxy
async function editProxy(proxyId) {
    try {
        const response = await fetch(`/api/proxies/${proxyId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const proxy = await response.json();
        currentProxy = proxy;

        proxyModalTitle.textContent = 'Edit Proxy';
        proxyType.value = proxy.type;
        proxyHost.value = proxy.host;
        proxyPort.value = proxy.port;
        proxyUsername.value = proxy.username || '';
        proxyPassword.value = ''; // Don't populate password for security

        hideTestResult();
        showProxyModal();
    } catch (error) {
        showToast('error', 'Failed to load proxy details');
    }
}

// Test proxy connection
async function testProxyConnection() {
    const type = proxyType.value;
    const host = proxyHost.value.trim();
    const port = parseInt(proxyPort.value);
    const username = proxyUsername.value.trim();
    const password = proxyPassword.value;

    if (!host || !port) {
        showToast('error', 'Please enter host and port');
        return;
    }

    testProxyBtn.disabled = true;
    testProxyBtn.innerHTML = '<span class="btn-icon">⏳</span> Testing...';

    try {
        const response = await fetch('/api/proxy/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type, host, port, username, password })
        });

        const data = await response.json();

        if (data.success) {
            showTestResult(true, `Connection successful (${data.latency_ms}ms)`);
        } else {
            showTestResult(false, data.message || 'Connection failed');
        }
    } catch (error) {
        showTestResult(false, error.message);
    } finally {
        testProxyBtn.disabled = false;
        testProxyBtn.innerHTML = '<span class="btn-icon">🔍</span> Test Connection';
    }
}

// Show test result
function showTestResult(success, message) {
    testResult.classList.remove('hidden', 'success', 'error');
    testResult.classList.add(success ? 'success' : 'error');
    testResultIcon.textContent = success ? '✓' : '✗';
    testResultMessage.textContent = message;
}

// Hide test result
function hideTestResult() {
    testResult.classList.add('hidden');
}

// Save proxy
async function saveProxy() {
    const type = proxyType.value;
    const host = proxyHost.value.trim();
    const port = parseInt(proxyPort.value);
    const username = proxyUsername.value.trim();
    const password = proxyPassword.value;

    if (!host || !port) {
        showToast('error', 'Please enter host and port');
        return;
    }

    if (port < 1 || port > 65535) {
        showToast('error', 'Port must be between 1 and 65535');
        return;
    }

    saveProxyBtn.disabled = true;
    saveProxyBtn.innerHTML = '<span class="btn-icon">⏳</span> Saving...';

    try {
        const url = currentProxy ? `/api/proxies/${currentProxy.id}` : '/api/proxies';
        const method = currentProxy ? 'PUT' : 'POST';

        const response = await fetch(url, {
            method: method,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ type, host, port, username, password })
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error || `HTTP ${response.status}`);
        }

        showToast('success', currentProxy ? 'Proxy updated successfully' : 'Proxy added successfully');
        hideProxyModal();
        loadProxies();
    } catch (error) {
        showToast('error', error.message);
    } finally {
        saveProxyBtn.disabled = false;
        saveProxyBtn.innerHTML = '<span class="btn-icon">💾</span> Save Proxy';
    }
}

// Show delete proxy modal
async function showDeleteProxyModal(proxyId) {
    const proxy = proxiesData.find(p => p.id === proxyId);
    if (!proxy) {
        showToast('error', 'Proxy not found');
        return;
    }

    proxyToDelete = proxy;

    deleteProxyInfo.innerHTML = `
        <div class="delete-info-item">
            <span class="delete-info-label">Type:</span>
            <span class="delete-info-value">${proxy.type.toUpperCase()}</span>
        </div>
        <div class="delete-info-item">
            <span class="delete-info-label">Host:</span>
            <span class="delete-info-value">${escapeHtml(proxy.host)}</span>
        </div>
        <div class="delete-info-item">
            <span class="delete-info-label">Port:</span>
            <span class="delete-info-value">${proxy.port}</span>
        </div>
    `;

    if (proxy.token_count > 0) {
        linkedTokensCount.textContent = proxy.token_count;
        linkedTokensWarning.classList.remove('hidden');
    } else {
        linkedTokensWarning.classList.add('hidden');
    }

    showDeleteModal();
}

// Delete proxy
async function deleteProxy() {
    if (!proxyToDelete) return;

    confirmDeleteBtn.disabled = true;
    confirmDeleteBtn.innerHTML = '<span class="btn-icon">⏳</span> Deleting...';

    try {
        const response = await fetch(`/api/proxies/${proxyToDelete.id}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error || `HTTP ${response.status}`);
        }

        showToast('success', 'Proxy deleted successfully');
        hideDeleteModal();
        loadProxies();
    } catch (error) {
        showToast('error', error.message);
    } finally {
        confirmDeleteBtn.disabled = false;
        confirmDeleteBtn.innerHTML = '<span class="btn-icon">🗑️</span> Delete Proxy';
    }
}

// Show linked tokens
async function showLinkedTokens(proxyId) {
    const proxy = proxiesData.find(p => p.id === proxyId);
    if (!proxy) {
        showToast('error', 'Proxy not found');
        return;
    }

    linkedProxyInfo.innerHTML = `
        <div class="proxy-info-title">Proxy Details</div>
        <div class="proxy-info-row">
            <span class="proxy-info-label">Type:</span>
            <span class="proxy-info-value">${proxy.type.toUpperCase()}</span>
        </div>
        <div class="proxy-info-row">
            <span class="proxy-info-label">Host:</span>
            <span class="proxy-info-value">${escapeHtml(proxy.host)}:${proxy.port}</span>
        </div>
    `;

    linkedTokensList.innerHTML = '';
    noLinkedTokens.classList.add('hidden');

    try {
        const response = await fetch(`/api/proxies/${proxyId}/tokens`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const data = await response.json();

        if (data.tokens.length === 0) {
            noLinkedTokens.classList.remove('hidden');
        } else {
            data.tokens.forEach(token => {
                const tokenItem = createLinkedTokenItem(token);
                linkedTokensList.appendChild(tokenItem);
            });
        }

        showLinkedTokensModal();
    } catch (error) {
        showToast('error', 'Failed to load linked tokens');
    }
}

// Create linked token item
function createLinkedTokenItem(token) {
    const item = document.createElement('div');
    item.className = 'linked-token-item';
    item.innerHTML = `
        <div class="linked-token-info">
            <div class="linked-token-email">${escapeHtml(token.email)}</div>
            <div class="linked-token-provider">${escapeHtml(token.provider_id)}</div>
        </div>
        <div class="linked-token-status">
            <span class="status-badge ${token.healthy ? 'healthy' : 'unhealthy'}">
                ${token.healthy ? '✓ Healthy' : '✗ Unhealthy'}
            </span>
        </div>
    `;
    return item;
}

// UI State Management Functions
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

function showProxiesContainer() {
    proxiesContainer.classList.remove('hidden');
}

function hideProxiesContainer() {
    proxiesContainer.classList.add('hidden');
}

function showProxyModal() {
    proxyModal.classList.remove('hidden');
    proxyModal.classList.add('show');
}

function hideProxyModal() {
    proxyModal.classList.remove('show');
    proxyModal.classList.add('hidden');
}

function showDeleteModal() {
    deleteModal.classList.remove('hidden');
}

function hideDeleteModal() {
    deleteModal.classList.add('hidden');
}

function showLinkedTokensModal() {
    linkedTokensModal.classList.remove('hidden');
}

function hideLinkedTokensModal() {
    linkedTokensModal.classList.add('hidden');
}

// Utility Functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
