# Phase 5: Proxy Configuration

**Priority:** HIGH  
**Estimated Time:** 1 day  
**Complexity:** Medium  
**Files to Create:** 1  
**Files to Modify:** 1

---

## Executive Summary

This phase implements proxy configuration management for tokens. Users can configure HTTP, HTTPS, SOCKS5 proxies for individual tokens, test proxy connections, and monitor proxy health.

---

## Problem Description

Tokens may need to use proxy servers for various reasons (geo-restrictions, rate limiting, privacy). Users need a way to configure and manage proxy settings per token.

### Requirements

- View proxy configuration for each token
- Add/update proxy configuration (HTTP, HTTPS, SOCKS5)
- Remove proxy configuration
- Test proxy connections before saving
- View proxy health status (latency, consecutive failures)

---

## Solution Architecture

### Design Principles

1. **Per-Token Configuration:** Each token can have its own proxy
2. **Test Before Save:** Users can test proxy before committing
3. **Health Monitoring:** Display proxy health metrics
4. **Security:** Passwords are masked in display
5. **Multiple Proxy Types:** Support HTTP, HTTPS, SOCKS5

### Component Overview

```
js/views/
└── Proxies.js         # Proxy management view
```

---

## Implementation Plan

### Step 1: Implement Proxies View

Create comprehensive proxy management interface:

```javascript
/**
 * Proxies View
 * 
 * Displays proxy configurations for tokens and allows
 * adding, updating, and testing proxy settings.
 */

import { View } from '../components/View.js';
import { Table } from '../components/Table.js';
import { Modal } from '../components/Modal.js';
import { Form } from '../components/Form.js';
import { toast } from '../components/Toast.js';
import { selectors, actions } from '../state/store.js';

export default class ProxiesView extends View {
    async render(container) {
        await super.render(container);

        this.subscribe('credentials', () => this.render());

        this.renderContent();
    }

    renderContent() {
        const credentials = this.store.get('credentials');

        this.container.innerHTML = `
            <div class="view-header">
                <h1>Proxy Configuration</h1>
            </div>

            <div class="proxy-summary">
                <div class="summary-card">
                    <span class="summary-label">Tokens with Proxy</span>
                    <span class="summary-value">${credentials.filter(c => c.proxy).length}</span>
                </div>
                <div class="summary-card">
                    <span class="summary-label">Healthy Proxies</span>
                    <span class="summary-value">${credentials.filter(c => c.proxy?.health_status?.is_healthy).length}</span>
                </div>
            </div>

            <div id="proxiesTable"></div>
        `;

        this.renderTable();
        this.setupEventListeners();
    }

    renderTable() {
        const credentials = this.store.get('credentials');

        // Fetch proxy configs for all tokens
        const proxyConfigs = await Promise.all(
            credentials.map(async (token) => {
                try {
                    const config = await this.api.getProxyConfig(token.provider, token.id);
                    return { token, config };
                } catch (error) {
                    return { token, config: null };
                }
            })
        );

        const table = new Table({
            columns: [
                {
                    key: 'email',
                    label: 'Token',
                    render: (value, row) => `
                        <div class="token-cell">
                            <span class="token-email">${value}</span>
                            <span class="token-id">${row.id.substring(0, 8)}...</span>
                        </div>
                    `
                },
                {
                    key: 'provider',
                    label: 'Provider'
                },
                {
                    key: 'proxy',
                    label: 'Proxy Configuration',
                    render: (value) => {
                        if (!value) {
                            return '<span class="no-proxy">No proxy configured</span>';
                        }
                        return `
                            <div class="proxy-config">
                                <span class="proxy-type">${value.type.toUpperCase()}</span>
                                <span class="proxy-host">${value.host}:${value.port}</span>
                            </div>
                        `;
                    }
                },
                {
                    key: 'health',
                    label: 'Health',
                    render: (value) => {
                        if (!value) {
                            return '-';
                        }
                        const health = value.health_status;
                        const latency = health.average_latency_ms || 0;
                        const isHealthy = health.is_healthy;
                        
                        return `
                            <div class="health-indicator ${isHealthy ? 'healthy' : 'unhealthy'}">
                                <span class="status-dot ${isHealthy ? 'healthy' : 'unhealthy'}"></span>
                                <span class="health-text">${isHealthy ? 'Healthy' : 'Unhealthy'}</span>
                                ${latency > 0 ? `<span class="health-latency">${latency}ms</span>` : ''}
                            </div>
                        `;
                    }
                },
                {
                    key: 'actions',
                    label: 'Actions',
                    render: (_, row) => `
                        <div class="table-actions">
                            <button class="btn-icon btn-sm" data-action="test" data-provider="${row.provider}" data-token="${row.id}" title="Test Proxy">
                                🧪
                            </button>
                            <button class="btn-icon btn-sm" data-action="edit" data-provider="${row.provider}" data-token="${row.id}" title="Edit Proxy">
                                ✏️
                            </button>
                            <button class="btn-icon btn-sm" data-action="delete" data-provider="${row.provider}" data-token="${row.id}" title="Remove Proxy">
                                🗑
                            </button>
                        </div>
                    `
                }
            ],
            data: proxyConfigs,
            emptyMessage: 'No tokens configured'
        });

        table.render(document.getElementById('proxiesTable'));
    }

    setupEventListeners() {
        document.getElementById('proxiesTable').addEventListener('click', (e) => {
            const action = e.target.closest('[data-action]');
            if (!action) return;

            const actionType = action.dataset.action;
            const providerId = action.dataset.provider;
            const tokenId = action.dataset.token;

            switch (actionType) {
                case 'test':
                    this.testProxy(providerId, tokenId);
                    break;
                case 'edit':
                    this.showProxyModal(providerId, tokenId);
                    break;
                case 'delete':
                    this.deleteProxy(providerId, tokenId);
                    break;
            }
        });
    }

    async showProxyModal(providerId, tokenId, existingConfig = null) {
        const credentials = this.store.get('credentials');
        const token = credentials.find(c => c.id === tokenId);

        if (!token) return;

        const modal = new Modal({
            title: existingConfig ? 'Edit Proxy Configuration' : 'Add Proxy Configuration',
            size: 'md',
            content: '',
            footer: `
                <button class="btn btn-secondary modal-cancel">Cancel</button>
                <button class="btn btn-primary modal-save">${existingConfig ? 'Update' : 'Add'} Proxy</button>
            `
        });

        modal.open();

        // Create form
        const formContainer = modal.element.querySelector('.modal-body');
        const form = new Form({
            fields: [
                {
                    name: 'type',
                    label: 'Proxy Type',
                    type: 'select',
                    options: [
                        { value: 'none', label: 'No Proxy (Direct Connection)' },
                        { value: 'http', label: 'HTTP' },
                        { value: 'https', label: 'HTTPS' },
                        { value: 'socks5', label: 'SOCKS5' }
                    ],
                    value: existingConfig?.proxy?.type || 'none'
                },
                {
                    name: 'host',
                    label: 'Host',
                    type: 'text',
                    placeholder: 'proxy.example.com',
                    value: existingConfig?.proxy?.host || '',
                    helpText: 'Proxy server hostname or IP address'
                },
                {
                    name: 'port',
                    label: 'Port',
                    type: 'number',
                    min: 1,
                    max: 65535,
                    value: existingConfig?.proxy?.port || '',
                    helpText: 'Proxy server port number'
                },
                {
                    name: 'username',
                    label: 'Username (optional)',
                    type: 'text',
                    value: existingConfig?.proxy?.username || '',
                    helpText: 'Username for proxy authentication'
                },
                {
                    name: 'password',
                    label: 'Password (optional)',
                    type: 'password',
                    value: '',
                    helpText: 'Password for proxy authentication'
                }
            ],
            onSubmit: async (values) => {
                // Handle "none" type
                if (values.type === 'none') {
                    await this.deleteProxy(providerId, tokenId);
                    modal.close();
                    return;
                }

                // Build proxy config
                const proxyConfig = {
                    type: values.type,
                    host: values.host,
                    port: parseInt(values.port),
                    username: values.username || undefined,
                    password: values.password || undefined
                };

                try {
                    await this.api.updateProxyConfig(providerId, tokenId, proxyConfig);
                    modal.close();
                    toast.success('Proxy configuration updated successfully');
                    await this.renderTable();
                } catch (error) {
                    toast.error(`Failed to update proxy: ${error.message}`);
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

    async testProxy(providerId, tokenId) {
        const credentials = this.store.get('credentials');
        const token = credentials.find(c => c.id === tokenId);

        if (!token) return;

        // Get existing config
        let proxyConfig;
        try {
            const response = await this.api.getProxyConfig(providerId, tokenId);
            proxyConfig = response.proxy;
        } catch (error) {
            // No proxy configured
            toast.info('No proxy configured for this token');
            return;
        }

        // Show test modal
        const modal = new Modal({
            title: 'Test Proxy Connection',
            size: 'sm',
            content: `
                <div class="proxy-test">
                    <div class="test-info">
                        <p><strong>Type:</strong> ${proxyConfig.type.toUpperCase()}</p>
                        <p><strong>Host:</strong> ${proxyConfig.host}:${proxyConfig.port}</p>
                        ${proxyConfig.username ? `<p><strong>Username:</strong> ${proxyConfig.username}</p>` : ''}
                    </div>
                    <div class="test-status" id="testStatus">
                        <div class="spinner"></div>
                        <p>Testing connection...</p>
                    </div>
                </div>
            `,
            footer: `
                <button class="btn btn-secondary modal-cancel">Close</button>
            `
        });

        modal.open();

        modal.element.querySelector('.modal-cancel').addEventListener('click', () => modal.close());

        try {
            const result = await this.api.testProxy(proxyConfig);
            const statusEl = modal.element.querySelector('#testStatus');

            if (result.success) {
                statusEl.innerHTML = `
                    <div class="test-result test-success">
                        <span class="result-icon">✓</span>
                        <div class="result-info">
                            <p class="result-message">Connection successful!</p>
                            <p class="result-latency">Latency: ${result.latency_ms}ms</p>
                        </div>
                    </div>
                `;
            } else {
                statusEl.innerHTML = `
                    <div class="test-result test-failure">
                        <span class="result-icon">✕</span>
                        <div class="result-info">
                            <p class="result-message">Connection failed</p>
                            <p class="result-error">${result.error}</p>
                        </div>
                    </div>
                `;
            }
        } catch (error) {
            const statusEl = modal.element.querySelector('#testStatus');
            statusEl.innerHTML = `
                <div class="test-result test-failure">
                    <span class="result-icon">✕</span>
                    <div class="result-info">
                        <p class="result-message">Test failed</p>
                        <p class="result-error">${error.message}</p>
                    </div>
                </div>
            `;
        }
    }

    async deleteProxy(providerId, tokenId) {
        const confirmed = await confirmModal({
            title: 'Remove Proxy Configuration',
            message: 'Are you sure you want to remove the proxy configuration for this token?',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.deleteProxyConfig(providerId, tokenId);
            toast.success('Proxy configuration removed');
            await this.renderTable();
        } catch (error) {
            toast.error(`Failed to remove proxy: ${error.message}`);
        }
    }
}
```

**Verification:**
- [ ] Proxies view renders correctly
- [ ] Proxy configuration modal works
- [ ] All proxy types supported
- [ ] Proxy test works
- [ ] Proxy deletion works
- [ ] Health status displays correctly

---

## Testing Strategy

### Unit Tests
- [ ] Proxy view renders correctly
- [ ] Form validation works
- [ ] Proxy test handles errors

### Integration Tests
- [ ] Proxy configuration saves correctly
- [ ] Proxy deletion works
- [ ] Test results display correctly

### Manual Testing
- [ ] Can add HTTP proxy
- [ ] Can add HTTPS proxy
- [ ] Can add SOCKS5 proxy
- [ ] Can remove proxy
- [ ] Proxy test works
- [ ] Health status updates

---

## Verification Checklist

- [ ] Proxies view implemented
- [ ] Proxy configuration modal works
- [ ] All proxy types supported
- [ ] Proxy test works
- [ ] Proxy deletion works
- [ ] Health status displays
- [ ] No console errors

---

## Next Steps

After completing Phase 5, proceed to **Phase 6: Rate Limit Management** to implement rate limit configuration features.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
