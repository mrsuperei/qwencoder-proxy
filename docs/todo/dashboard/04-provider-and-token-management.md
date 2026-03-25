# Phase 4: Provider & Token Management

**Priority:** CRITICAL  
**Estimated Time:** 2 days  
**Complexity:** Medium  
**Files to Create:** 2  
**Files to Modify:** 1

---

## Executive Summary

This phase implements comprehensive provider and token management features. Users can view providers, add new OAuth tokens, manage existing tokens, and configure provider settings.

---

## Problem Description

Users need to manage OAuth tokens for multiple providers. This includes adding new tokens via OAuth flows, viewing token details, refreshing tokens, and deleting tokens.

### Requirements

- View all providers and their configurations
- Add new tokens via device code flow
- Add new tokens via authorization code flow
- View all tokens with health status
- Refresh individual tokens
- Delete tokens
- Configure provider settings (selection strategy, refresh buffer, error threshold)

---

## Solution Architecture

### Design Principles

1. **OAuth Flow Support:** Support both device code and authorization code flows
2. **Real-time Updates:** Token status updates in real-time
3. **Health Monitoring:** Visual indication of token health
4. **Bulk Operations:** Support bulk actions on tokens
5. **Filtering & Sorting:** Easy navigation of large token lists

### Component Overview

```
js/views/
├── Providers.js       # Provider management view
└── Tokens.js          # Token management view
```

---

## Implementation Plan

### Step 1: Implement Providers View

Create comprehensive provider management interface:

```javascript
/**
 * Providers View
 * 
 * Displays all providers with their configurations
 * and allows managing provider settings.
 */

import { View } from '../components/View.js';
import { Table } from '../components/Table.js';
import { Modal } from '../components/Modal.js';
import { Form } from '../components/Form.js';
import { toast } from '../components/Toast.js';
import { selectors, actions } from '../state/store.js';

export default class ProvidersView extends View {
    async render(container) {
        await super.render(container);

        this.subscribe('providers', () => this.render());
        this.subscribe('credentials', () => this.render());

        this.renderContent();
    }

    renderContent() {
        const providers = this.store.get('providers');
        const credentials = this.store.get('credentials');

        this.container.innerHTML = `
            <div class="view-header">
                <h1>Providers</h1>
            </div>
            <div class="providers-grid">
                ${providers.map(provider => this.renderProviderCard(provider, credentials)).join('')}
            </div>
        `;
    }

    renderProviderCard(provider, credentials) {
        const providerCredentials = credentials.filter(c => c.provider === provider.id);
        const healthyCount = providerCredentials.filter(c => c.healthy).length;
        const totalCount = providerCredentials.length;

        return `
            <div class="card provider-card">
                <div class="provider-header">
                    <div class="provider-info">
                        <h3>${provider.name}</h3>
                        <span class="badge badge-info">${provider.flow}</span>
                    </div>
                    <div class="provider-actions">
                        <button class="btn btn-primary btn-sm" data-action="add-token" data-provider="${provider.id}">
                            + Add Token
                        </button>
                        <button class="btn btn-secondary btn-sm" data-action="settings" data-provider="${provider.id}">
                            ⚙️
                        </button>
                    </div>
                </div>
                <div class="provider-stats">
                    <div class="stat-row">
                        <span class="stat-label">Tokens</span>
                        <span class="stat-value">${totalCount}</span>
                    </div>
                    <div class="stat-row">
                        <span class="stat-label">Healthy</span>
                        <span class="stat-value ${healthyCount === totalCount ? 'text-success' : 'text-warning'}">
                            ${healthyCount}/${totalCount}
                        </span>
                    </div>
                </div>
                <div class="provider-tokens">
                    <h4>Tokens</h4>
                    ${providerCredentials.length > 0 ? `
                        <div class="token-list">
                            ${providerCredentials.map(token => this.renderTokenItem(token)).join('')}
                        </div>
                    ` : '<p class="empty-text">No tokens configured</p>'}
                </div>
            </div>
        `;
    }

    renderTokenItem(token) {
        const expiryDate = new Date(token.expiry_date);
        const isExpiringSoon = expiryDate < new Date(Date.now() + 24 * 60 * 60 * 1000);
        const isExpired = expiryDate < new Date();

        return `
            <div class="token-item ${isExpired ? 'expired' : ''} ${isExpiringSoon && !isExpired ? 'expiring' : ''}">
                <div class="token-info">
                    <span class="token-email">${token.email}</span>
                    <span class="token-id">${token.id.substring(0, 8)}...</span>
                </div>
                <div class="token-status">
                    <span class="status-dot ${token.healthy ? 'healthy' : 'unhealthy'}"></span>
                    <span class="token-expiry">
                        ${isExpired ? 'Expired' : isExpiringSoon ? 'Expiring soon' : 'Valid'}
                    </span>
                </div>
                <div class="token-actions">
                    <button class="btn-icon btn-sm" data-action="refresh-token" data-provider="${token.provider}" data-token="${token.id}" title="Refresh token">
                        🔄
                    </button>
                    <button class="btn-icon btn-sm" data-action="delete-token" data-provider="${token.provider}" data-token="${token.id}" title="Delete token">
                        🗑
                    </button>
                </div>
            </div>
        `;
    }

    onMount() {
        super.onMount();

        // Setup event listeners
        this.container.addEventListener('click', (e) => {
            const action = e.target.closest('[data-action]');
            if (!action) return;

            const actionType = action.dataset.action;
            const providerId = action.dataset.provider;
            const tokenId = action.dataset.token;

            switch (actionType) {
                case 'add-token':
                    this.showAddTokenModal(providerId);
                    break;
                case 'settings':
                    this.showSettingsModal(providerId);
                    break;
                case 'refresh-token':
                    this.refreshToken(providerId, tokenId);
                    break;
                case 'delete-token':
                    this.deleteToken(providerId, tokenId);
                    break;
            }
        });
    }

    async showAddTokenModal(providerId) {
        const providers = this.store.get('providers');
        const provider = providers.find(p => p.id === providerId);

        if (!provider) return;

        // Create modal based on flow type
        if (provider.flow === 'device_code') {
            this.showDeviceCodeModal(provider);
        } else if (provider.flow === 'authorization_code') {
            this.showAuthCodeModal(provider);
        } else {
            this.showManualTokenModal(provider);
        }
    }

    async showDeviceCodeModal(provider) {
        try {
            const response = await this.api.startDeviceFlow(provider.id);
            const pollId = response.poll_id;

            const modal = new Modal({
                title: `Add Token - ${provider.name}`,
                size: 'md',
                content: `
                    <div class="device-code-flow">
                        <div class="code-display">
                            <label>Device Code:</label>
                            <div class="code-value">${response.user_code}</div>
                        </div>
                        <div class="instructions">
                            <p>1. Visit this URL:</p>
                            <a href="${response.verification_uri}" target="_blank" class="link">${response.verification_uri}</a>
                            <p>2. Enter the code: <strong>${response.user_code}</strong></p>
                        </div>
                        <div class="status" id="deviceCodeStatus">
                            <div class="spinner"></div>
                            <p>Waiting for authorization...</p>
                        </div>
                    </div>
                `,
                footer: `
                    <button class="btn btn-secondary modal-cancel">Cancel</button>
                `
            });

            modal.open();

            // Poll for status
            const pollInterval = setInterval(async () => {
                try {
                    const status = await this.api.getDeviceStatus(pollId);

                    if (status.status === 'authorized') {
                        clearInterval(pollInterval);
                        modal.close();
                        toast.success('Token added successfully');
                        await this.app.loadInitialData();
                    } else if (status.status === 'error') {
                        clearInterval(pollInterval);
                        modal.close();
                        toast.error(`Authorization failed: ${status.error_description}`);
                    }

                } catch (error) {
                    console.error('Poll error:', error);
                }
            }, response.interval * 1000);

            // Cancel button
            modal.element.querySelector('.modal-cancel').addEventListener('click', () => {
                clearInterval(pollInterval);
                modal.close();
            });

        } catch (error) {
            toast.error(`Failed to start device flow: ${error.message}`);
        }
    }

    async showAuthCodeModal(provider) {
        try {
            const response = await this.api.startAuth(provider.id);

            const modal = new Modal({
                title: `Add Token - ${provider.name}`,
                size: 'md',
                content: `
                    <div class="auth-code-flow">
                        <p>Click the button below to open the authorization page:</p>
                        <a href="${response.auth_url}" target="_blank" class="btn btn-primary btn-lg">
                            Open Authorization Page
                        </a>
                        <p class="help-text">After authorizing, return here to complete the process.</p>
                        <div class="status" id="authCodeStatus">
                            <div class="spinner"></div>
                            <p>Waiting for authorization...</p>
                        </div>
                    </div>
                `,
                footer: `
                    <button class="btn btn-secondary modal-cancel">Cancel</button>
                `
            });

            modal.open();

            // Listen for OAuth success message
            const messageHandler = (event) => {
                if (event.data.type === 'oauth_success') {
                    window.removeEventListener('message', messageHandler);
                    modal.close();
                    toast.success('Token added successfully');
                    this.app.loadInitialData();
                }
            };

            window.addEventListener('message', messageHandler);

            // Cancel button
            modal.element.querySelector('.modal-cancel').addEventListener('click', () => {
                window.removeEventListener('message', messageHandler);
                modal.close();
            });

        } catch (error) {
            toast.error(`Failed to start auth flow: ${error.message}`);
        }
    }

    showManualTokenModal(provider) {
        const modal = new Modal({
            title: `Add Token - ${provider.name}`,
            size: 'md',
            content: '',
            footer: `
                <button class="btn btn-secondary modal-cancel">Cancel</button>
                <button class="btn btn-primary modal-save">Add Token</button>
            `
        });

        modal.open();

        // Create form
        const formContainer = modal.element.querySelector('.modal-body');
        const form = new Form({
            fields: [
                {
                    name: 'access_token',
                    label: 'Access Token',
                    type: 'text',
                    required: true,
                    helpText: 'Paste your access token here'
                },
                {
                    name: 'refresh_token',
                    label: 'Refresh Token (optional)',
                    type: 'text',
                    helpText: 'If available, paste your refresh token'
                },
                {
                    name: 'email',
                    label: 'Email',
                    type: 'email',
                    required: true,
                    helpText: 'Email associated with this token'
                }
            ],
            onSubmit: async (values) => {
                try {
                    await this.api.addToken(provider.id, values);
                    modal.close();
                    toast.success('Token added successfully');
                    await this.app.loadInitialData();
                } catch (error) {
                    toast.error(`Failed to add token: ${error.message}`);
                }
            }
        });

        formContainer.appendChild(form.createElement());

        // Modal buttons
        modal.element.querySelector('.modal-cancel').addEventListener('click', () => modal.close());
        modal.element.querySelector('.modal-save').addEventListener('click', () => {
            form.element.dispatchEvent(new Event('submit'));
        });
    }

    async showSettingsModal(providerId) {
        const providers = this.store.get('providers');
        const credentials = this.store.get('credentials');
        const provider = providers.find(p => p.id === providerId);
        const providerCreds = credentials.filter(c => c.provider === providerId);

        if (!provider) return;

        const modal = new Modal({
            title: `Settings - ${provider.name}`,
            size: 'md',
            content: '',
            footer: `
                <button class="btn btn-secondary modal-cancel">Cancel</button>
                <button class="btn btn-primary modal-save">Save Settings</button>
            `
        });

        modal.open();

        // Create form
        const formContainer = modal.element.querySelector('.modal-body');
        const form = new Form({
            fields: [
                {
                    name: 'selection_strategy',
                    label: 'Token Selection Strategy',
                    type: 'select',
                    options: [
                        { value: 'random', label: 'Random' },
                        { value: 'round_robin', label: 'Round Robin' },
                        { value: 'least_used', label: 'Least Used' }
                    ],
                    helpText: 'How tokens are selected for requests'
                },
                {
                    name: 'refresh_buffer_sec',
                    label: 'Refresh Buffer (seconds)',
                    type: 'number',
                    min: 0,
                    max: 3600,
                    value: 300,
                    helpText: 'Seconds before expiry to refresh token'
                },
                {
                    name: 'max_error_count',
                    label: 'Max Error Count',
                    type: 'number',
                    min: 1,
                    max: 100,
                    value: 5,
                    helpText: 'Maximum errors before marking token as unhealthy'
                }
            ],
            onSubmit: async (values) => {
                try {
                    await this.api.updateProviderSettings(providerId, values);
                    modal.close();
                    toast.success('Settings saved successfully');
                    await this.app.loadInitialData();
                } catch (error) {
                    toast.error(`Failed to save settings: ${error.message}`);
                }
            }
        });

        formContainer.appendChild(form.createElement());

        // Modal buttons
        modal.element.querySelector('.modal-cancel').addEventListener('click', () => modal.close());
        modal.element.querySelector('.modal-save').addEventListener('click', () => {
            form.element.dispatchEvent(new Event('submit'));
        });
    }

    async refreshToken(providerId, tokenId) {
        try {
            await this.api.refreshToken(providerId, tokenId);
            toast.success('Token refreshed successfully');
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to refresh token: ${error.message}`);
        }
    }

    async deleteToken(providerId, tokenId) {
        const confirmed = await confirmModal({
            title: 'Delete Token',
            message: 'Are you sure you want to delete this token? This action cannot be undone.',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.deleteToken(providerId, tokenId);
            toast.success('Token deleted successfully');
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to delete token: ${error.message}`);
        }
    }
}
```

**Verification:**
- [ ] Providers display correctly
- [ ] Device code flow works
- [ ] Auth code flow works
- [ ] Manual token addition works
- [ ] Token refresh works
- [ ] Token deletion works
- [ ] Settings modal works

---

### Step 2: Implement Tokens View

Create comprehensive token management interface:

```javascript
/**
 * Tokens View
 * 
 * Displays all tokens with filtering, sorting,
 * and bulk operations.
 */

import { View } from '../components/View.js';
import { Table } from '../components/Table.js';
import { Modal } from '../components/Modal.js';
import { Form } from '../components/Form.js';
import { toast } from '../components/Toast.js';
import { selectors, actions } from '../state/store.js';

export default class TokensView extends View {
    constructor(app) {
        super(app);
        this.filters = {
            provider: 'all',
            health: 'all',
            search: ''
        };
        this.sort = {
            column: 'createdAt',
            direction: 'desc'
        };
    }

    async render(container) {
        await super.render(container);

        this.subscribe('credentials', () => this.render());
        this.subscribe('providers', () => this.render());

        this.renderContent();
    }

    renderContent() {
        const providers = this.store.get('providers');
        const credentials = this.store.get('credentials');

        this.container.innerHTML = `
            <div class="view-header">
                <h1>Tokens</h1>
                <div class="header-actions">
                    <button class="btn btn-primary" data-action="add-token">
                        + Add Token
                    </button>
                </div>
            </div>

            <div class="filters-bar">
                <select class="filter-input" id="filterProvider">
                    <option value="all">All Providers</option>
                    ${providers.map(p => `
                        <option value="${p.id}">${p.name}</option>
                    `).join('')}
                </select>
                <select class="filter-input" id="filterHealth">
                    <option value="all">All Health Status</option>
                    <option value="healthy">Healthy Only</option>
                    <option value="unhealthy">Unhealthy Only</option>
                </select>
                <input type="text" class="filter-input" id="filterSearch" placeholder="Search by email...">
            </div>

            <div id="tokensTable"></div>
        `;

        this.renderTable();
        this.setupFilters();
    }

    getFilteredTokens() {
        const credentials = this.store.get('credentials');
        const providers = this.store.get('providers');

        let filtered = [...credentials];

        // Provider filter
        if (this.filters.provider !== 'all') {
            filtered = filtered.filter(c => c.provider === this.filters.provider);
        }

        // Health filter
        if (this.filters.health !== 'all') {
            const isHealthy = this.filters.health === 'healthy';
            filtered = filtered.filter(c => c.healthy === isHealthy);
        }

        // Search filter
        if (this.filters.search) {
            const search = this.filters.search.toLowerCase();
            filtered = filtered.filter(c => 
                c.email?.toLowerCase().includes(search) ||
                c.id.toLowerCase().includes(search)
            );
        }

        // Sort
        filtered.sort((a, b) => {
            const aVal = a[this.sort.column];
            const bVal = b[this.sort.column];

            if (aVal < bVal) return this.sort.direction === 'asc' ? -1 : 1;
            if (aVal > bVal) return this.sort.direction === 'asc' ? 1 : -1;
            return 0;
        });

        return filtered;
    }

    renderTable() {
        const tokens = this.getFilteredTokens();
        const providers = this.store.get('providers');

        const table = new Table({
            columns: [
                {
                    key: 'email',
                    label: 'Email',
                    sortable: true
                },
                {
                    key: 'provider',
                    label: 'Provider',
                    render: (value) => {
                        const provider = providers.find(p => p.id === value);
                        return provider ? provider.name : value;
                    },
                    sortable: true
                },
                {
                    key: 'healthy',
                    label: 'Health',
                    render: (value) => `
                        <span class="status-dot ${value ? 'healthy' : 'unhealthy'}"></span>
                        ${value ? 'Healthy' : 'Unhealthy'}
                    `,
                    sortable: true
                },
                {
                    key: 'expiry_date',
                    label: 'Expires',
                    render: (value) => {
                        const date = new Date(value);
                        const isExpiringSoon = date < new Date(Date.now() + 24 * 60 * 60 * 1000);
                        const isExpired = date < new Date();
                        
                        let className = '';
                        if (isExpired) className = 'text-danger';
                        else if (isExpiringSoon) className = 'text-warning';
                        
                        return `<span class="${className}">${date.toLocaleString()}</span>`;
                    },
                    sortable: true
                },
                {
                    key: 'last_used',
                    label: 'Last Used',
                    render: (value) => {
                        if (!value) return 'Never';
                        const date = new Date(value);
                        return date.toLocaleString();
                    },
                    sortable: true
                },
                {
                    key: 'actions',
                    label: 'Actions',
                    render: (_, row) => `
                        <div class="table-actions">
                            <button class="btn-icon btn-sm" data-action="refresh" data-provider="${row.provider}" data-token="${row.id}" title="Refresh">
                                🔄
                            </button>
                            <button class="btn-icon btn-sm" data-action="delete" data-provider="${row.provider}" data-token="${row.id}" title="Delete">
                                🗑
                            </button>
                        </div>
                    `
                }
            ],
            data: tokens,
            emptyMessage: 'No tokens found matching your filters'
        });

        table.render(document.getElementById('tokensTable'));
    }

    setupFilters() {
        // Provider filter
        document.getElementById('filterProvider').addEventListener('change', (e) => {
            this.filters.provider = e.target.value;
            this.renderTable();
        });

        // Health filter
        document.getElementById('filterHealth').addEventListener('change', (e) => {
            this.filters.health = e.target.value;
            this.renderTable();
        });

        // Search filter
        document.getElementById('filterSearch').addEventListener('input', (e) => {
            this.filters.search = e.target.value;
            this.renderTable();
        });

        // Add token button
        this.container.querySelector('[data-action="add-token"]').addEventListener('click', () => {
            this.showProviderSelectModal();
        });

        // Table actions
        document.getElementById('tokensTable').addEventListener('click', (e) => {
            const action = e.target.closest('[data-action]');
            if (!action) return;

            const actionType = action.dataset.action;
            const providerId = action.dataset.provider;
            const tokenId = action.dataset.token;

            switch (actionType) {
                case 'refresh':
                    this.refreshToken(providerId, tokenId);
                    break;
                case 'delete':
                    this.deleteToken(providerId, tokenId);
                    break;
            }
        });
    }

    showProviderSelectModal() {
        const providers = this.store.get('providers');

        const modal = new Modal({
            title: 'Select Provider',
            size: 'sm',
            content: `
                <div class="provider-select">
                    ${providers.map(provider => `
                        <button class="provider-option" data-provider="${provider.id}">
                            <span class="provider-name">${provider.name}</span>
                            <span class="provider-flow">${provider.flow}</span>
                        </button>
                    `).join('')}
                </div>
            `
        });

        modal.open();

        // Provider selection
        modal.element.querySelectorAll('.provider-option').forEach(btn => {
            btn.addEventListener('click', () => {
                const providerId = btn.dataset.provider;
                modal.close();
                this.showAddTokenModal(providerId);
            });
        });
    }

    async showAddTokenModal(providerId) {
        const providers = this.store.get('providers');
        const provider = providers.find(p => p.id === providerId);

        if (!provider) return;

        // Delegate to ProvidersView logic
        // This could be refactored into a shared service
        if (provider.flow === 'device_code') {
            this.showDeviceCodeModal(provider);
        } else if (provider.flow === 'authorization_code') {
            this.showAuthCodeModal(provider);
        } else {
            this.showManualTokenModal(provider);
        }
    }

    async refreshToken(providerId, tokenId) {
        try {
            await this.api.refreshToken(providerId, tokenId);
            toast.success('Token refreshed successfully');
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to refresh token: ${error.message}`);
        }
    }

    async deleteToken(providerId, tokenId) {
        const confirmed = await confirmModal({
            title: 'Delete Token',
            message: 'Are you sure you want to delete this token? This action cannot be undone.',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.deleteToken(providerId, tokenId);
            toast.success('Token deleted successfully');
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to delete token: ${error.message}`);
        }
    }
}
```

**Verification:**
- [ ] Tokens display correctly
- [ ] Filtering works
- [ ] Sorting works
- [ ] Search works
- [ ] Token refresh works
- [ ] Token deletion works

---

## Testing Strategy

### Unit Tests
- [ ] Provider view renders correctly
- [ ] Token view renders correctly
- [ ] Filters work correctly
- [ ] Sort works correctly
- [ ] OAuth flows work

### Integration Tests
- [ ] Token addition updates state
- [ ] Token refresh updates state
- [ ] Token deletion updates state
- [ ] Settings save correctly

### Manual Testing
- [ ] Device code flow completes
- [ ] Auth code flow completes
- [ ] Manual token addition works
- [ ] All CRUD operations work

---

## Verification Checklist

- [ ] Providers view implemented
- [ ] Tokens view implemented
- [ ] Device code flow works
- [ ] Auth code flow works
- [ ] Manual token addition works
- [ ] Token refresh works
- [ ] Token deletion works
- [ ] Provider settings work
- [ ] Filtering works
- [ ] Sorting works
- [ ] Search works
- [ ] No console errors

---

## Next Steps

After completing Phase 4, proceed to **Phase 5: Proxy Configuration** to implement proxy management features.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
