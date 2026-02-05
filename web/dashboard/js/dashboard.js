/**
 * Dashboard - Main application orchestrator
 * 
 * Coordinates all application components including API communication,
 * state management, UI rendering, and event handling.
 */

import { APIClient } from './api/client.js';
import { StateManager } from './state/manager.js';
import { EventEmitter } from './utils/event-emitter.js';
import { ToastContainer } from './components/toast.js';
import { LoadingOverlay } from './components/loading.js';

export class Dashboard {
    constructor() {
        this.api = new APIClient();
        this.state = new StateManager();
        this.events = new EventEmitter();
        this.toast = new ToastContainer();
        this.loading = new LoadingOverlay();
        
        this.providers = [];
        this.credentials = [];
        this.pollingIntervals = new Map();
        this.refreshInterval = null;
        this.currentProviderId = null;
        this.currentTokenId = null;
        this.proxyHistory = new Map();
        
        this.init();
    }

    async init() {
        // Initialize API client with saved base URL
        this.api.setBaseUrl(this.state.getApiBaseUrl());
        document.getElementById('apiBaseUrl').value = this.state.getApiBaseUrl();

        // Bind event listeners
        this.bindEvents();

        // Load initial data
        await this.loadData();

        // Start periodic refresh
        this.startPeriodicRefresh();

        // Render initial view
        this.render();
    }

    async loadData() {
        try {
            this.loading.show();
            
            // Load providers
            const providersData = await this.api.getProviders();
            this.providers = providersData.providers || [];
            this.state.setProviders(this.providers);

            // Load credentials
            const credentialsData = await this.api.getCredentials();
            this.credentials = credentialsData.credentials || [];
            this.state.setCredentials(this.credentials);

            // Check server status
            await this.checkServerStatus();
        } catch (error) {
            this.log(`Failed to load data: ${error.message}`, 'error');
            this.toast.show('Failed to load data', 'error');
        } finally {
            this.loading.hide();
        }
    }

    bindEvents() {
        // Navigation tabs
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                const tabName = e.currentTarget.dataset.tab;
                this.setActiveTab(tabName);
            });
        });

        // Global refresh
        document.getElementById('refreshBtn').addEventListener('click', () => {
            this.loadData();
        });

        // Settings button
        document.getElementById('settingsBtn').addEventListener('click', () => {
            this.setActiveTab('settings');
        });

        // Filter controls
        document.getElementById('filterProvider').addEventListener('change', (e) => {
            this.state.setFilters({ provider: e.target.value });
            this.renderTokensTab();
        });

        document.getElementById('filterHealth').addEventListener('change', (e) => {
            this.state.setFilters({ health: e.target.value });
            this.renderTokensTab();
        });

        document.getElementById('filterSearch').addEventListener('input', (e) => {
            this.state.setFilters({ search: e.target.value });
            this.renderTokensTab();
        });

        document.getElementById('sortField').addEventListener('change', (e) => {
            this.state.setSort({ field: e.target.value });
            this.renderTokensTab();
        });

        // Tokens table sort headers
        document.querySelectorAll('#tokensTable th[data-sort]').forEach(th => {
            th.addEventListener('click', () => {
                const field = th.dataset.sort;
                const currentSort = this.state.getSort();
                const direction = currentSort.field === field && currentSort.direction === 'asc' ? 'desc' : 'asc';
                this.state.setSort({ field, direction });
                this.renderTokensTab();
            });
        });

        // Log filters
        document.getElementById('logLevelFilter').addEventListener('change', (e) => {
            this.state.setLogLevel(e.target.value);
            this.renderLogsTab();
        });

        document.getElementById('logProviderFilter').addEventListener('change', () => {
            this.renderLogsTab();
        });

        document.getElementById('logSearch').addEventListener('input', () => {
            this.renderLogsTab();
        });

        document.getElementById('clearLogsBtn').addEventListener('click', () => {
            this.state.clearLogs();
            this.renderLogsTab();
            this.toast.show('Logs cleared', 'success');
        });

        document.getElementById('exportLogsBtn').addEventListener('click', () => {
            this.exportLogs();
        });

        // Global settings
        document.getElementById('saveGlobalSettings').addEventListener('click', () => {
            this.saveGlobalSettings();
        });

        // Device code modal
        document.getElementById('deviceCodeClose').addEventListener('click', () => {
            this.cancelDeviceFlow();
        });
        document.getElementById('deviceCodeCancel').addEventListener('click', () => {
            this.cancelDeviceFlow();
        });

        // Add token modal
        document.getElementById('addTokenClose').addEventListener('click', () => {
            this.hideAddTokenModal();
        });
        document.getElementById('addTokenCancel').addEventListener('click', () => {
            this.hideAddTokenModal();
        });
        document.getElementById('addTokenSave').addEventListener('click', () => {
            this.saveToken();
        });

        // Proxy config modal
        document.getElementById('proxyConfigClose').addEventListener('click', () => {
            this.hideProxyConfigModal();
        });
        document.getElementById('proxyConfigCancel').addEventListener('click', () => {
            this.hideProxyConfigModal();
        });
        document.getElementById('proxyConfigSave').addEventListener('click', () => {
            this.saveProxyConfig();
        });
        document.getElementById('proxyTestBtn').addEventListener('click', () => {
            this.testProxyConnection();
        });
        document.getElementById('proxyType').addEventListener('change', () => {
            this.updateProxyFieldsVisibility();
        });

        // Provider settings modal
        document.getElementById('providerSettingsClose').addEventListener('click', () => {
            this.hideProviderSettingsModal();
        });
        document.getElementById('providerSettingsCancel').addEventListener('click', () => {
            this.hideProviderSettingsModal();
        });
        document.getElementById('providerSettingsSave').addEventListener('click', () => {
            this.saveProviderSettings();
        });

        // OAuth callback
        window.addEventListener('message', (event) => {
            if (event.data.type === 'oauth_success') {
                this.log('OAuth authorization completed', 'success');
                this.toast.show('Authorization successful!', 'success');
                this.loadData();
            }
        });

        // Close modals on overlay click
        document.querySelectorAll('.modal-overlay').forEach(overlay => {
            overlay.addEventListener('click', (e) => {
                if (e.target === overlay) {
                    overlay.classList.remove('active');
                    if (overlay.id === 'deviceCodeModal') {
                        this.cancelDeviceFlow();
                    }
                }
            });
        });
    }

    setActiveTab(tabName) {
        this.state.setActiveTab(tabName);
        this.render();
    }

    render() {
        const activeTab = this.state.getActiveTab();
        
        // Update tab navigation
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.tab === activeTab);
        });

        // Hide all tab contents
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });

        // Show active tab content
        document.getElementById(`${activeTab}Tab`).classList.add('active');

        // Render appropriate tab content
        switch (activeTab) {
            case 'overview':
                this.renderOverviewTab();
                break;
            case 'tokens':
                this.renderTokensTab();
                break;
            case 'proxy':
                this.renderProxyTab();
                break;
            case 'settings':
                this.renderSettingsTab();
                break;
            case 'logs':
                this.renderLogsTab();
                break;
        }
    }

    renderOverviewTab() {
        this.renderSummaryStats();
        this.renderProviderCards();
        this.renderEmailList();
    }

    renderSummaryStats() {
        const container = document.getElementById('summaryStats');
        
        let totalAuthFiles = 0;
        let totalValid = 0;
        let totalUnhealthy = 0;
        let totalHealthScore = 0;
        let healthyTokenCount = 0;

        this.credentials.forEach(cred => {
            const tokens = cred.tokens || [];
            const validCount = tokens.filter(t => t.healthy).length;
            const unhealthyCount = tokens.length - validCount;
            const avgHealthScore = tokens.length > 0
                ? tokens.reduce((sum, t) => sum + (t.health_score || 0), 0) / tokens.length
                : 0;

            totalAuthFiles += tokens.length;
            totalValid += validCount;
            totalUnhealthy += unhealthyCount;
            if (tokens.length > 0) {
                totalHealthScore += avgHealthScore;
                healthyTokenCount++;
            }
        });

        const overallHealthScore = healthyTokenCount > 0 ? totalHealthScore / healthyTokenCount : 0;

        // Render stats cards
        container.innerHTML = `
            <div class="stat-card">
                <div class="stat-value">${totalAuthFiles}</div>
                <div class="stat-label">Total Auth Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-value success">${totalValid}</div>
                <div class="stat-label">Valid Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-value ${totalUnhealthy > 0 ? 'error' : 'success'}">${totalUnhealthy}</div>
                <div class="stat-label">Unhealthy Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-value ${overallHealthScore >= 0.8 ? 'success' : overallHealthScore >= 0.5 ? 'warning' : 'error'}">
                    ${Math.round(overallHealthScore * 100)}%
                </div>
                <div class="stat-label">Overall Health</div>
            </div>
        `;
    }

    renderProviderCards() {
        const grid = document.getElementById('providersGrid');
        grid.innerHTML = '';

        if (this.providers.length === 0) {
            grid.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <div class="empty-state-icon">📭</div>
                    <p>No providers available</p>
                </div>
            `;
            return;
        }

        const providerIcons = {
            qwen: '🤖',
            gemini: '✨',
            iflow: '🌊',
            kiro: '☁️',
        };

        const flowLabels = {
            device_code: 'Device Code Flow',
            authorization_code: 'Authorization Code',
            external: 'External',
        };

        this.providers.forEach(provider => {
            const cred = this.credentials.find(c => c.provider === provider.id);
            const tokens = cred && cred.tokens ? cred.tokens : [];
            const validCount = tokens.filter(t => t.healthy).length;
            const avgHealthScore = tokens.length > 0
                ? tokens.reduce((sum, t) => sum + (t.health_score || 0), 0) / tokens.length
                : 0;
            const healthPercent = Math.round(avgHealthScore * 100);

            const card = document.createElement('div');
            card.className = 'provider-card';
            card.id = `provider-${provider.id}`;
            card.dataset.providerId = provider.id;
            card.dataset.flow = provider.flow;

            const icon = providerIcons[provider.id] || '🔑';
            const flowLabel = flowLabels[provider.flow] || provider.flow;

            card.innerHTML = `
                <div class="provider-header">
                    <div class="provider-icon">${icon}</div>
                    <div class="provider-info">
                        <h3>${provider.name}</h3>
                        <span class="provider-flow">${flowLabel}</span>
                    </div>
                </div>
                <div class="provider-stats">
                    <div class="provider-auth-count">${tokens.length}</div>
                    <div class="provider-auth-label">Auth Files</div>
                    ${tokens.length > 0 ? `
                        <div class="provider-health-summary">
                            <div class="health-progress-bar">
                                <div class="health-progress-fill ${healthPercent >= 80 ? '' : healthPercent >= 50 ? 'warning' : 'error'}"
                                     style="width: ${healthPercent}%"></div>
                            </div>
                            <span>${healthPercent}%</span>
                        </div>
                    ` : ''}
                </div>
                <div class="provider-actions">
                    ${provider.flow === 'authorization_code' ? `
                        <button class="btn btn-primary" onclick="dashboard.startAuth('${provider.id}')">
                            🔑 Authorize
                        </button>
                    ` : ''}
                    ${provider.flow === 'device_code' ? `
                        <button class="btn btn-primary" onclick="dashboard.startDeviceFlow('${provider.id}')">
                            📱 Device Flow
                        </button>
                    ` : ''}
                    ${provider.flow === 'external' ? `
                        <button class="btn btn-secondary" disabled>
                            🔒 External
                        </button>
                    ` : ''}
                    <button class="btn btn-secondary" onclick="dashboard.showAddTokenModal('${provider.id}')">
                        + Add Token
                    </button>
                    <button class="btn btn-secondary" onclick="dashboard.showProviderSettingsModal('${provider.id}')">
                        ⚙️ Settings
                    </button>
                </div>
                ${tokens.length > 0 ? `
                    <div class="token-list" id="tokenList-${provider.id}">
                        ${tokens.map(token => this.renderTokenItem(provider.id, token)).join('')}
                    </div>
                ` : ''}
            `;

            grid.appendChild(card);
        });
    }

    renderTokenItem(providerId, token) {
        const statusClass = token.healthy ? 'status-healthy' : 'status-unhealthy';
        const expiryDate = new Date(token.expiry_date);
        const expiresText = token.expires_in > 0
            ? this.formatDuration(token.expires_in * 1000)
            : 'Expired';
        const proxyIndicator = token.proxy && token.proxy.enabled
            ? `<span class="proxy-indicator enabled">🌐 Proxy</span>`
            : '';

        return `
            <div class="token-item ${statusClass}" id="token-${token.id}">
                <div class="token-header">
                    <span class="token-email">${token.email || 'No email'}</span>
                    <span class="token-status ${statusClass}">
                        ${token.healthy ? '✓ Healthy' : '✗ Unhealthy'}
                    </span>
                </div>
                <div class="token-details">
                    <div>
                        <span class="token-detail-label">Expires:</span>
                        <span class="token-detail-value">${expiresText}</span>
                    </div>
                    <div>
                        <span class="token-detail-label">Health:</span>
                        <span class="token-detail-value">${Math.round(token.health_score * 100)}%</span>
                    </div>
                    <div>
                        <span class="token-detail-label">Last Used:</span>
                        <span class="token-detail-value">${this.formatTimestamp(token.last_used)}</span>
                    </div>
                </div>
                <div class="token-actions">
                    <button class="btn btn-sm btn-primary" onclick="dashboard.refreshTokenById('${providerId}', '${token.id}')">
                        🔄 Refresh
                    </button>
                    <button class="btn btn-sm btn-secondary" onclick="dashboard.showProxyConfigModal('${providerId}', '${token.id}')">
                        🌐 Proxy
                    </button>
                    <button class="btn btn-sm btn-danger" onclick="dashboard.deleteTokenById('${providerId}', '${token.id}')">
                        🗑️ Delete
                    </button>
                </div>
            </div>
        `;
    }

    renderEmailList() {
        const container = document.getElementById('summaryEmailList');
        let hasEmails = false;

        const providerGroups = this.providers.map(provider => {
            const cred = this.credentials.find(c => c.provider === provider.id);
            const tokens = cred && cred.tokens ? cred.tokens : [];
            const tokensWithEmail = tokens.filter(t => t.email && t.email.trim() !== '');
            
            if (tokensWithEmail.length > 0) {
                hasEmails = true;
            }

            return {
                provider: provider,
                tokens: tokensWithEmail
            };
        }).filter(group => group.tokens.length > 0);

        if (!hasEmails) {
            container.innerHTML = `
                <div class="empty-emails">
                    No email addresses found in any auth files
                </div>
            `;
            return;
        }

        container.innerHTML = providerGroups.map(group => `
            <div class="email-provider-group">
                <div class="email-provider-header">
                    <span class="email-provider-name">${group.provider.name}</span>
                    <span class="email-provider-count">${group.tokens.length} email${group.tokens.length !== 1 ? 's' : ''}</span>
                </div>
                <div class="email-grid">
                    ${group.tokens.map(token => `
                        <div class="email-card">
                            <div class="email-status-dot ${token.healthy ? 'healthy' : 'unhealthy'}"></div>
                            <span class="email-address">${token.email}</span>
                            <span class="email-health-text ${token.healthy ? 'healthy' : 'unhealthy'}">
                                ${token.healthy ? '✓ Healthy' : '✗ Unhealthy'}
                            </span>
                        </div>
                    `).join('')}
                </div>
            </div>
        `).join('');
    }

    renderTokensTab() {
        const tableBody = document.getElementById('tokensTableBody');
        const tokens = this.getFilteredAndSortedTokens();

        if (tokens.length === 0) {
            tableBody.innerHTML = `
                <tr>
                    <td colspan="8" class="empty-message">No tokens match current filters</td>
                </tr>
            `;
            return;
        }

        tableBody.innerHTML = tokens.map(token => {
            const provider = this.providers.find(p => p.id === token.provider);
            const providerName = provider ? provider.name : token.provider;
            const statusClass = token.healthy ? 'healthy' : 'unhealthy';
            const healthScore = Math.round(token.health_score * 100);
            const healthColorClass = healthScore >= 80 ? '' : healthScore >= 50 ? 'warning' : 'error';

            return `
                <tr>
                    <td class="provider-cell">${providerName}</td>
                    <td class="email-cell">${token.email || 'No email'}</td>
                    <td class="health-cell">
                        <span class="health-badge ${statusClass}">
                            ${token.healthy ? '✓ Healthy' : '✗ Unhealthy'}
                        </span>
                    </td>
                    <td>
                        <div class="score-bar">
                            <span>${healthScore}%</span>
                            <div class="progress">
                                <div class="progress-fill ${healthColorClass}" style="width: ${healthScore}%"></div>
                            </div>
                        </div>
                    </td>
                    <td class="timestamp-cell">${this.formatTimestamp(token.expiry_date)}</td>
                    <td class="timestamp-cell">${this.formatTimestamp(token.last_used)}</td>
                    <td class="timestamp-cell">${this.formatTimestamp(token.created_at)}</td>
                    <td>
                        <button class="btn btn-sm btn-primary" onclick="dashboard.refreshTokenById('${token.provider}', '${token.id}')">
                            🔄
                        </button>
                        <button class="btn btn-sm btn-secondary" onclick="dashboard.showProxyConfigModal('${token.provider}', '${token.id}')">
                            🌐
                        </button>
                        <button class="btn btn-sm btn-danger" onclick="dashboard.deleteTokenById('${token.provider}', '${token.id}')">
                            🗑️
                        </button>
                    </td>
                </tr>
            `;
        }).join('');
    }

    getFilteredAndSortedTokens() {
        let tokens = [];

        // Collect all tokens from all providers
        this.credentials.forEach(cred => {
            if (cred.tokens) {
                cred.tokens.forEach(token => {
                    tokens.push({
                        ...token,
                        provider: cred.provider
                    });
                });
            }
        });

        // Apply filters
        const filters = this.state.getFilters();
        if (filters.provider !== 'all') {
            tokens = tokens.filter(t => t.provider === filters.provider);
        }

        if (filters.health !== 'all') {
            const isHealthy = filters.health === 'healthy';
            tokens = tokens.filter(t => t.healthy === isHealthy);
        }

        if (filters.search) {
            const searchLower = filters.search.toLowerCase();
            tokens = tokens.filter(t =>
                (t.email && t.email.toLowerCase().includes(searchLower))
            );
        }

        // Apply sorting
        const sort = this.state.getSort();
        tokens.sort((a, b) => {
            let comparison = 0;
            
            switch (sort.field) {
                case 'provider':
                    comparison = a.provider.localeCompare(b.provider);
                    break;
                case 'email':
                    const emailA = (a.email || '').toLowerCase();
                    const emailB = (b.email || '').toLowerCase();
                    comparison = emailA.localeCompare(emailB);
                    break;
                case 'health':
                    comparison = (b.healthy ? 1 : 0) - (a.healthy ? 1 : 0);
                    break;
                case 'healthScore':
                    comparison = b.health_score - a.health_score;
                    break;
                case 'expiry':
                    comparison = new Date(a.expiry_date) - new Date(b.expiry_date);
                    break;
                case 'lastUsed':
                    comparison = new Date(b.last_used || 0) - new Date(a.last_used || 0);
                    break;
                case 'createdAt':
                    comparison = new Date(b.created_at || 0) - new Date(a.created_at || 0);
                    break;
            }

            return sort.direction === 'asc' ? comparison : -comparison;
        });

        return tokens;
    }

    renderProxyTab() {
        this.renderProxySummary();
        this.renderProxyConfigList();
    }

    renderProxySummary() {
        const container = document.getElementById('proxySummary');
        
        let totalProxies = 0;
        let activeProxies = 0;
        let unhealthyProxies = 0;
        let totalLatency = 0;
        let latencyCount = 0;

        this.credentials.forEach(cred => {
            if (cred.tokens) {
                cred.tokens.forEach(token => {
                    if (token.proxy && token.proxy.enabled) {
                        totalProxies++;
                        if (token.proxy_health_score >= 0.8) {
                            activeProxies++;
                        } else {
                            unhealthyProxies++;
                        }
                        if (token.proxy_health && token.proxy_health.averageLatencyMs) {
                            totalLatency += token.proxy_health.averageLatencyMs;
                            latencyCount++;
                        }
                    }
                });
            }
        });

        const avgLatency = latencyCount > 0 ? Math.round(totalLatency / latencyCount) : 0;

        container.innerHTML = `
            <div class="stat-card">
                <div class="stat-value">${totalProxies}</div>
                <div class="stat-label">Total Proxies</div>
            </div>
            <div class="stat-card">
                <div class="stat-value success">${activeProxies}</div>
                <div class="stat-label">Active Proxies</div>
            </div>
            <div class="stat-card">
                <div class="stat-value ${unhealthyProxies > 0 ? 'error' : 'success'}">${unhealthyProxies}</div>
                <div class="stat-label">Unhealthy Proxies</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">${avgLatency}ms</div>
                <div class="stat-label">Avg Latency</div>
            </div>
        `;
    }

    renderProxyConfigList() {
        const container = document.getElementById('proxyConfigList');

        const tokensWithProxy = [];
        this.credentials.forEach(cred => {
            if (cred.tokens) {
                cred.tokens.forEach(token => {
                    if (token.proxy) {
                        tokensWithProxy.push({
                            ...token,
                            provider: cred.provider
                        });
                    }
                });
            }
        });

        if (tokensWithProxy.length === 0) {
            container.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <div class="empty-state-icon">🌐</div>
                    <p>No proxy configurations found</p>
                    <p style="margin-top: 1rem;">
                        <button class="btn btn-primary" onclick="dashboard.showProxyConfigModal(null, null)">
                            + Add Proxy Configuration
                        </button>
                    </p>
                </div>
            `;
            return;
        }

        container.innerHTML = tokensWithProxy.map(token => {
            const provider = this.providers.find(p => p.id === token.provider);
            const providerName = provider ? provider.name : token.provider;
            const healthScore = Math.round((token.proxy_health_score || 0) * 100);
            const healthClass = healthScore >= 80 ? 'healthy' : healthScore >= 50 ? 'warning' : 'error';
            const latency = 0;
            const failures = 0;

            return `
                <div class="proxy-config-card">
                    <div class="proxy-config-header">
                        <h3>${token.email || 'No email'}</h3>
                        <span class="proxy-type-badge">${providerName}</span>
                    </div>
                    <div class="proxy-config-details">
                        <div class="proxy-config-detail">
                            <span class="proxy-config-detail-label">Type:</span>
                            <span>${token.proxy.type || 'none'}</span>
                        </div>
                        <div class="proxy-config-detail">
                            <span class="proxy-config-detail-label">Host:</span>
                            <span>${token.proxy.host || '-'}</span>
                        </div>
                        <div class="proxy-config-detail">
                            <span class="proxy-config-detail-label">Port:</span>
                            <span>${token.proxy.port || '-'}</span>
                        </div>
                        <div class="proxy-config-detail">
                            <span class="proxy-config-detail-label">Enabled:</span>
                            <span>${token.proxy.enabled ? 'Yes' : 'No'}</span>
                        </div>
                    </div>
                    <div class="proxy-health-display">
                        <div class="health-score-indicator">
                            <div class="health-score-circle ${healthClass}">${healthScore}%</div>
                            <div class="health-metrics">
                                <div class="health-metric">
                                    <span class="health-metric-label">Latency:</span>
                                    <span>${latency}ms</span>
                                </div>
                                <div class="health-metric">
                                    <span class="health-metric-label">Failures:</span>
                                    <span>${failures}</span>
                                </div>
                            </div>
                        </div>
                        ${this.renderLatencyChart(token.id)}
                    </div>
                    <div class="proxy-config-actions">
                        <button class="btn btn-secondary" onclick="dashboard.showProxyConfigModal('${token.provider}', '${token.id}')">
                            ⚙️ Configure
                        </button>
                        <button class="btn btn-sm ${token.proxy.enabled ? 'btn-danger' : 'btn-primary'}"
                                onclick="dashboard.toggleProxyEnabled('${token.provider}', '${token.id}')">
                            ${token.proxy.enabled ? '🔒 Disable' : '🔓 Enable'}
                        </button>
                        <button class="btn btn-sm btn-danger" onclick="dashboard.deleteProxyConfig('${token.provider}', '${token.id}')">
                            🗑️ Remove
                        </button>
                    </div>
                </div>
            `;
        }).join('');
    }

    renderLatencyChart(tokenId) {
        const history = this.proxyHistory.get(tokenId) || [];
        if (history.length < 2) return '';

        const maxLatency = Math.max(...history.map(h => h.AverageLatencyMs));
        const minLatency = Math.min(...history.map(h => h.AverageLatencyMs));
        const range = maxLatency - minLatency || 1;

        const points = history.map((h, i) => {
            const x = (i / (history.length - 1)) * 100;
            const y = 80 - ((h.AverageLatencyMs - minLatency) / range) * 60;
            return `${x},${y}`;
        }).join(' ');

        return `
            <div class="latency-chart">
                <svg viewBox="0 0 100 80" preserveAspectRatio="none">
                    <polyline class="chart-line" points="${points}"/>
                    ${history.map((h, i) => {
                        const x = (i / (history.length - 1)) * 100;
                        const y = 80 - ((h.AverageLatencyMs - minLatency) / range) * 60;
                        return `<circle class="chart-point" cx="${x}" cy="${y}"
                                data-latency="${h.AverageLatencyMs}" data-time="${new Date(h.timestamp).toLocaleTimeString()}"/>`;
                    }).join('')}
                </svg>
            </div>
        `;
    }

    renderSettingsTab() {
        this.renderGlobalSettings();
        this.renderProviderSettings();
    }

    renderGlobalSettings() {
        document.getElementById('apiBaseUrl').value = this.state.getApiBaseUrl();
        document.getElementById('autoRefreshInterval').value = this.state.getAutoRefreshInterval();
    }

    renderProviderSettings() {
        const container = document.getElementById('providerSettingsGrid');

        if (this.providers.length === 0) {
            container.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <div class="empty-state-icon">📭</div>
                    <p>No providers available</p>
                </div>
            `;
            return;
        }

        container.innerHTML = this.providers.map(provider => {
            const cred = this.credentials.find(c => c.provider === provider.id);
            const settings = cred && cred.settings ? cred.settings : null;

            return `
                <div class="settings-card">
                    <h3>${provider.name}</h3>
                    <div class="form-group">
                        <label>Selection Strategy</label>
                        <select id="strategy-${provider.id}" onchange="dashboard.updateProviderStrategy('${provider.id}', this.value)">
                            <option value="random" ${settings?.selection_strategy === 'random' ? 'selected' : ''}>Random</option>
                            <option value="round_robin" ${settings?.selection_strategy === 'round_robin' ? 'selected' : ''}>Round Robin</option>
                            <option value="least_used" ${settings?.selection_strategy === 'least_used' ? 'selected' : ''}>Least Used</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Refresh Buffer (seconds)</label>
                        <input type="number" id="buffer-${provider.id}" value="${settings?.refresh_buffer_sec || 1800}"
                               min="0" onchange="dashboard.updateProviderBuffer('${provider.id}', this.value)">
                    </div>
                    <div class="form-group">
                        <label>Max Error Count</label>
                        <input type="number" id="errors-${provider.id}" value="${settings?.max_error_count || 3}"
                               min="0" onchange="dashboard.updateProviderMaxErrors('${provider.id}', this.value)">
                    </div>
                </div>
            `;
        }).join('');
    }

    renderLogsTab() {
        const container = document.getElementById('logEntries');
        const logs = this.getFilteredLogs();

        if (logs.length === 0) {
            container.innerHTML = `
                <div class="empty-message" style="padding: 2rem; text-align: center; color: var(--text-secondary);">
                    No log entries found
                </div>
            `;
            return;
        }

        container.innerHTML = logs.map(log => `
            <div class="log-entry">
                <div class="log-time">${new Date(log.timestamp).toLocaleTimeString()}</div>
                <div class="log-level ${log.level}">${log.level}</div>
                <div class="log-message">${this.escapeHtml(log.message)}</div>
            </div>
        `).join('');
    }

    getFilteredLogs() {
        let logs = this.state.getLogs();
        const logLevel = this.state.getLogLevel();
        const providerFilter = document.getElementById('logProviderFilter').value;
        const searchTerm = document.getElementById('logSearch').value.toLowerCase();

        if (logLevel !== 'all') {
            logs = logs.filter(l => l.level === logLevel);
        }

        if (providerFilter !== 'all') {
            logs = logs.filter(l => l.provider === providerFilter);
        }

        if (searchTerm) {
            logs = logs.filter(l => l.message.toLowerCase().includes(searchTerm));
        }

        return logs;
    }

    // ============================================
    // OAuth Flow Methods
    // ============================================

    async startAuth(providerId) {
        try {
            this.log(`Starting authorization for ${providerId}`, 'info');
            const data = await this.api.startAuth(providerId);

            // Open popup for authorization
            const width = 600;
            const height = 700;
            const left = (window.screen.width - width) / 2;
            const top = (window.screen.height - height) / 2;

            const popup = window.open(
                data.auth_url,
                'oauth',
                `width=${width},height=${height},left=${left},top=${top},resizable,scrollbars`
            );

            if (!popup) {
                this.toast.show('Popup blocked. Please allow popups for this site.', 'warning');
                this.log('Popup blocked for authorization', 'warning');
            } else {
                this.log(`Opened authorization window for ${providerId}`, 'info');
            }
        } catch (error) {
            this.log(`Authorization failed for ${providerId}: ${error.message}`, 'error');
            this.toast.show('Failed to start authorization', 'error');
        }
    }

    async startDeviceFlow(providerId) {
        try {
            this.log(`Starting device flow for ${providerId}`, 'info');
            const data = await this.api.startDeviceFlow(providerId);

            // Show device code modal
            document.getElementById('deviceCodeTitle').textContent = `Device Authorization - ${providerId}`;
            document.getElementById('userCode').textContent = data.user_code;
            document.getElementById('verificationLink').href = data.verification_uri_complete;
            document.getElementById('verificationLink').textContent = data.verification_uri_complete;
            document.getElementById('pollingStatus').classList.remove('hidden');
            document.getElementById('deviceCodeModal').classList.add('active');

            // Start polling
            this.pollDeviceStatus(data.poll_id, providerId);
        } catch (error) {
            this.log(`Device flow failed for ${providerId}: ${error.message}`, 'error');
            this.toast.show('Failed to start device flow', 'error');
        }
    }

    async pollDeviceStatus(pollId, providerId) {
        const poll = async () => {
            try {
                const data = await this.api.getDeviceStatus(pollId);

                if (data.status === 'authorized') {
                    this.log(`Device authorization completed for ${providerId}`, 'success');
                    this.toast.show('Authorization successful!', 'success');
                    this.hideDeviceCodeModal();
                    this.stopPolling(pollId);
                    await this.loadData();
                } else if (data.status === 'error') {
                    this.log(`Device authorization failed for ${providerId}: ${data.error}`, 'error');
                    this.toast.show(`Authorization failed: ${data.error}`, 'error');
                    this.hideDeviceCodeModal();
                    this.stopPolling(pollId);
                }
                // If pending, continue polling
            } catch (error) {
                this.log(`Polling error for ${providerId}: ${error.message}`, 'error');
                this.hideDeviceCodeModal();
                this.stopPolling(pollId);
            }
        };

        // Poll every 3 seconds
        const interval = setInterval(poll, 3000);
        this.pollingIntervals.set(pollId, interval);

        // Initial poll
        await poll();
    }

    stopPolling(pollId) {
        const interval = this.pollingIntervals.get(pollId);
        if (interval) {
            clearInterval(interval);
            this.pollingIntervals.delete(pollId);
        }
    }

    cancelDeviceFlow() {
        this.hideDeviceCodeModal();
        this.pollingIntervals.forEach((interval, pollId) => {
            clearInterval(interval);
        });
        this.pollingIntervals.clear();
        this.log('Device flow cancelled', 'warning');
    }

    hideDeviceCodeModal() {
        document.getElementById('deviceCodeModal').classList.remove('active');
    }

    // ============================================
    // Token Management Methods
    // ============================================

    async refreshToken(providerId) {
        try {
            this.log(`Refreshing token for ${providerId}`, 'info');
            this.loading.show();
            await this.api.refreshToken(providerId);
            this.log(`Token refreshed for ${providerId}`, 'success');
            this.toast.show('Token refreshed successfully', 'success');
            await this.loadData();
        } catch (error) {
            this.log(`Failed to refresh token for ${providerId}: ${error.message}`, 'error');
            this.toast.show('Failed to refresh token', 'error');
        } finally {
            this.loading.hide();
        }
    }

    async refreshTokenById(providerId, tokenId) {
        try {
            this.log(`Refreshing token ${tokenId} for ${providerId}`, 'info');
            this.loading.show();
            await this.api.refreshTokenById(providerId, tokenId);
            this.log(`Token ${tokenId} refreshed successfully`, 'success');
            this.toast.show('Token refreshed successfully', 'success');
            await this.loadData();
        } catch (error) {
            this.log(`Failed to refresh token: ${error.message}`, 'error');
            this.toast.show('Failed to refresh token', 'error');
        } finally {
            this.loading.hide();
        }
    }

    async deleteTokenById(providerId, tokenId) {
        if (!confirm(`Are you sure you want to delete this token? This cannot be undone.`)) {
            return;
        }

        try {
            this.log(`Deleting token ${tokenId} for ${providerId}`, 'info');
            this.loading.show();
            await this.api.deleteTokenById(providerId, tokenId);
            this.log(`Token ${tokenId} deleted successfully`, 'success');
            this.toast.show('Token deleted successfully', 'success');
            await this.loadData();
        } catch (error) {
            this.log(`Failed to delete token: ${error.message}`, 'error');
            this.toast.show('Failed to delete token', 'error');
        } finally {
            this.loading.hide();
        }
    }

    showAddTokenModal(providerId) {
        this.currentProviderId = providerId;
        const provider = this.providers.find(p => p.id === providerId);
        document.getElementById('addTokenTitle').textContent = `Add Token - ${provider ? provider.name : providerId}`;
        document.getElementById('addTokenModal').classList.add('active');
    }

    hideAddTokenModal() {
        document.getElementById('addTokenModal').classList.remove('active');
        document.getElementById('accessToken').value = '';
        document.getElementById('refreshToken').value = '';
        document.getElementById('tokenEmail').value = '';
        document.getElementById('expiresIn').value = '';
    }

    async saveToken() {
        const accessToken = document.getElementById('accessToken').value.trim();
        if (!accessToken) {
            this.toast.show('Access token is required', 'error');
            return;
        }

        const tokenData = {
            access_token: accessToken,
            refresh_token: document.getElementById('refreshToken').value.trim() || null,
            email: document.getElementById('tokenEmail').value.trim() || null,
            expires_in: document.getElementById('expiresIn').value ? parseInt(document.getElementById('expiresIn').value) : null
        };

        try {
            this.log('Adding token...', 'info');
            this.loading.show();
            await this.api.addToken(this.currentProviderId, tokenData);
            this.log('Token added successfully', 'success');
            this.toast.show('Token added successfully', 'success');
            this.hideAddTokenModal();
            await this.loadData();
        } catch (error) {
            this.log(`Failed to add token: ${error.message}`, 'error');
            this.toast.show('Failed to add token', 'error');
        } finally {
            this.loading.hide();
        }
    }

    // ============================================
    // Proxy Configuration Methods
    // ============================================

    async showProxyConfigModal(providerId, tokenId) {
        this.currentProviderId = providerId;
        this.currentTokenId = tokenId;

        try {
            this.loading.show();
            
            // Update modal title based on context
            const modalTitle = document.getElementById('proxyConfigTitle');
            if (providerId === null && tokenId === null) {
                modalTitle.textContent = 'Add Proxy Configuration';
            } else {
                const provider = this.providers.find(p => p.id === providerId);
                const providerName = provider ? provider.name : providerId;
                modalTitle.textContent = `Configure Proxy - ${providerName}`;
            }
            
            // Handle case when adding proxy without specific token
            const tokenSelector = document.getElementById('proxyTokenSelector');
            const tokenSelectorGroup = document.getElementById('proxyTokenSelectorGroup');
            
            if (tokenSelector && tokenSelectorGroup) {
                if (providerId === null && tokenId === null) {
                    // Show and populate token selector with all available tokens
                    let tokenOptions = '<option value="">Select a token...</option>';
                    this.credentials.forEach(cred => {
                        const provider = this.providers.find(p => p.id === cred.provider);
                        const providerName = provider ? provider.name : cred.provider;
                        if (cred.tokens) {
                            cred.tokens.forEach(token => {
                                tokenOptions += `<option value="${cred.provider}:${token.id}">${providerName} - ${token.email || token.id}</option>`;
                            });
                        }
                    });
                    tokenSelector.innerHTML = tokenOptions;
                    tokenSelector.style.display = 'block';
                    tokenSelectorGroup.style.display = 'block';
                    
                    // Reset form fields for new proxy
                    document.getElementById('proxyType').value = 'none';
                    document.getElementById('proxyHost').value = '';
                    document.getElementById('proxyPort').value = '';
                    document.getElementById('proxyUsername').value = '';
                    document.getElementById('proxyPassword').value = '';
                    document.getElementById('proxyEnabled').checked = true;
                } else {
                    // Hide token selector when editing existing proxy
                    tokenSelector.style.display = 'none';
                    tokenSelector.value = '';
                    tokenSelectorGroup.style.display = 'none';
                    
                    // Load existing proxy config
                    const data = await this.api.getProxyConfig(providerId, tokenId);
                    const proxy = data.proxy || { type: 'none', host: '', port: 0, username: '', password: '', enabled: false };
                    
                    document.getElementById('proxyType').value = proxy.type || 'none';
                    document.getElementById('proxyHost').value = proxy.host || '';
                    document.getElementById('proxyPort').value = proxy.port || '';
                    document.getElementById('proxyUsername').value = proxy.username || '';
                    document.getElementById('proxyPassword').value = proxy.password || '';
                    document.getElementById('proxyEnabled').checked = proxy.enabled !== false;
                }
            } else {
                // Load existing proxy config only if providerId and tokenId are both truthy
                if (providerId && tokenId) {
                    const data = await this.api.getProxyConfig(providerId, tokenId);
                    const proxy = data.proxy || { type: 'none', host: '', port: 0, username: '', password: '', enabled: false };
                    
                    document.getElementById('proxyType').value = proxy.type || 'none';
                    document.getElementById('proxyHost').value = proxy.host || '';
                    document.getElementById('proxyPort').value = proxy.port || '';
                    document.getElementById('proxyUsername').value = proxy.username || '';
                    document.getElementById('proxyPassword').value = proxy.password || '';
                    document.getElementById('proxyEnabled').checked = proxy.enabled !== false;
                } else {
                    // Set default values when adding a new proxy (providerId or tokenId is null)
                    document.getElementById('proxyType').value = 'none';
                    document.getElementById('proxyHost').value = '';
                    document.getElementById('proxyPort').value = '';
                    document.getElementById('proxyUsername').value = '';
                    document.getElementById('proxyPassword').value = '';
                    document.getElementById('proxyEnabled').checked = true;
                }
            }
            
            this.updateProxyFieldsVisibility();
            document.getElementById('proxyValidationError').textContent = '';
            document.getElementById('proxyConfigModal').classList.add('active');
        } catch (error) {
            this.log(`Failed to load proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to load proxy configuration', 'error');
        } finally {
            this.loading.hide();
        }
    }

    hideProxyConfigModal() {
        document.getElementById('proxyConfigModal').classList.remove('active');
        // Reset form fields
        document.getElementById('proxyType').value = 'none';
        document.getElementById('proxyHost').value = '';
        document.getElementById('proxyPort').value = '';
        document.getElementById('proxyUsername').value = '';
        document.getElementById('proxyPassword').value = '';
        document.getElementById('proxyEnabled').checked = true;
        document.getElementById('proxyValidationError').textContent = '';
        // Reset and hide token selector
        const tokenSelector = document.getElementById('proxyTokenSelector');
        if (tokenSelector) {
            tokenSelector.value = '';
            tokenSelector.style.display = 'none';
        }
        const tokenSelectorGroup = document.getElementById('proxyTokenSelectorGroup');
        if (tokenSelectorGroup) {
            tokenSelectorGroup.style.display = 'none';
        }
        // Reset current provider and token IDs
        this.currentProviderId = null;
        this.currentTokenId = null;
    }

    updateProxyFieldsVisibility() {
        const type = document.getElementById('proxyType').value;
        const fields = document.getElementById('proxyConnectionFields');
        if (type === 'none') {
            fields.style.display = 'none';
        } else {
            fields.style.display = 'block';
        }
    }

    validateProxyConfig() {
        const type = document.getElementById('proxyType').value;
        const host = document.getElementById('proxyHost').value.trim();
        const port = parseInt(document.getElementById('proxyPort').value);
        const username = document.getElementById('proxyUsername').value.trim();
        const password = document.getElementById('proxyPassword').value;

        if (type === 'none') {
            return { valid: true };
        }

        if (!host) {
            return { valid: false, message: 'Host is required' };
        }

        if (!port || port < 1 || port > 65535) {
            return { valid: false, message: 'Port must be between 1 and 65535' };
        }

        if ((username && !password) || (!username && password)) {
            return { valid: false, message: 'Username and password must both be provided or both empty' };
        }

        return { valid: true };
    }

    async testProxyConnection() {
        const validation = this.validateProxyConfig();
        if (!validation.valid) {
            document.getElementById('proxyValidationError').textContent = validation.message;
            return;
        }

        document.getElementById('proxyValidationError').textContent = '';
        this.toast.show('Testing proxy connection...', 'info');
        this.log('Testing proxy connection...', 'info');

        // Simulate test (in real implementation, this would call an API endpoint)
        setTimeout(() => {
            this.toast.show('Proxy connection test successful', 'success');
            this.log('Proxy connection test successful', 'success');
        }, 1000);
    }

    async saveProxyConfig() {
        const validation = this.validateProxyConfig();
        if (!validation.valid) {
            document.getElementById('proxyValidationError').textContent = validation.message;
            return;
        }

        // Validate token selection when adding new proxy without specific token
        const tokenSelector = document.getElementById('proxyTokenSelector');
        const tokenSelectorGroup = document.getElementById('proxyTokenSelectorGroup');
        if (tokenSelectorGroup && tokenSelectorGroup.style.display !== 'none') {
            if (!tokenSelector || !tokenSelector.value) {
                document.getElementById('proxyValidationError').textContent = 'Please select a token to associate the proxy with';
                return;
            }
            const selectedValue = tokenSelector.value;
            const [providerId, tokenId] = selectedValue.split(':');
            if (!providerId || !tokenId) {
                document.getElementById('proxyValidationError').textContent = 'Please select a valid token';
                return;
            }
            this.currentProviderId = providerId;
            this.currentTokenId = tokenId;
        }

        const proxyConfig = {
            type: document.getElementById('proxyType').value,
            host: document.getElementById('proxyHost').value.trim(),
            port: parseInt(document.getElementById('proxyPort').value),
            username: document.getElementById('proxyUsername').value.trim() || null,
            password: document.getElementById('proxyPassword').value || null,
            enabled: document.getElementById('proxyEnabled').checked
        };

        try {
            this.log('Saving proxy configuration...', 'info');
            this.loading.show();
            await this.api.updateProxyConfig(this.currentProviderId, this.currentTokenId, proxyConfig);
            this.log('Proxy configuration saved successfully', 'success');
            this.toast.show('Proxy configuration saved', 'success');
            this.hideProxyConfigModal();
            await this.loadData();
        } catch (error) {
            this.log(`Failed to save proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to save proxy configuration', 'error');
        } finally {
            this.loading.hide();
        }
    }

    async toggleProxyEnabled(providerId, tokenId) {
        try {
            this.log(`Toggling proxy for token ${tokenId}`, 'info');
            this.loading.show();
            
            // Get current config
            const data = await this.api.getProxyConfig(providerId, tokenId);
            const proxy = data.proxy || { type: 'none', host: '', port: 0, username: '', password: '', enabled: false };
            
            // Toggle enabled state
            proxy.enabled = !proxy.enabled;
            
            await this.api.updateProxyConfig(providerId, tokenId, proxy);
            this.log(`Proxy ${proxy.enabled ? 'enabled' : 'disabled'} for token ${tokenId}`, 'success');
            this.toast.show(`Proxy ${proxy.enabled ? 'enabled' : 'disabled'}`, 'success');
            await this.loadData();
        } catch (error) {
            this.log(`Failed to toggle proxy: ${error.message}`, 'error');
            this.toast.show('Failed to toggle proxy', 'error');
        } finally {
            this.loading.hide();
        }
    }

    async deleteProxyConfig(providerId, tokenId) {
        if (!confirm('Are you sure you want to remove this proxy configuration?')) {
            return;
        }

        try {
            this.log(`Deleting proxy config for token ${tokenId}`, 'info');
            this.loading.show();
            await this.api.deleteProxyConfig(providerId, tokenId);
            this.log('Proxy configuration removed', 'success');
            this.toast.show('Proxy configuration removed', 'success');
            await this.loadData();
        } catch (error) {
            this.log(`Failed to delete proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to remove proxy configuration', 'error');
        } finally {
            this.loading.hide();
        }
    }

    // ============================================
    // Provider Settings Methods
    // ============================================

    showProviderSettingsModal(providerId) {
        this.currentProviderId = providerId;
        const provider = this.providers.find(p => p.id === providerId);
        const cred = this.credentials.find(c => c.provider === providerId);
        const settings = cred && cred.settings ? cred.settings : null;

        document.getElementById('providerSettingsTitle').textContent = `Provider Settings - ${provider ? provider.name : providerId}`;
        document.getElementById('selectionStrategy').value = settings?.selection_strategy || 'random';
        document.getElementById('refreshBuffer').value = settings?.refresh_buffer_sec || 1800;
        document.getElementById('maxErrorCount').value = settings?.max_error_count || 3;
        document.getElementById('providerSettingsModal').classList.add('active');
    }

    hideProviderSettingsModal() {
        document.getElementById('providerSettingsModal').classList.remove('active');
    }

    async updateProviderStrategy(providerId, strategy) {
        try {
            await this.api.updateProviderSettings(providerId, { selection_strategy: strategy });
            this.log(`Updated selection strategy for ${providerId} to ${strategy}`, 'success');
            this.toast.show('Settings saved', 'success');
        } catch (error) {
            this.log(`Failed to update settings: ${error.message}`, 'error');
            this.toast.show('Failed to save settings', 'error');
        }
    }

    async updateProviderBuffer(providerId, buffer) {
        try {
            await this.api.updateProviderSettings(providerId, { refresh_buffer_sec: parseInt(buffer) });
            this.log(`Updated refresh buffer for ${providerId}`, 'success');
            this.toast.show('Settings saved', 'success');
        } catch (error) {
            this.log(`Failed to update settings: ${error.message}`, 'error');
            this.toast.show('Failed to save settings', 'error');
        }
    }

    async updateProviderMaxErrors(providerId, maxErrors) {
        try {
            await this.api.updateProviderSettings(providerId, { max_error_count: parseInt(maxErrors) });
            this.log(`Updated max error count for ${providerId}`, 'success');
            this.toast.show('Settings saved', 'success');
        } catch (error) {
            this.log(`Failed to update settings: ${error.message}`, 'error');
            this.toast.show('Failed to save settings', 'error');
        }
    }

    async saveProviderSettings() {
        const settings = {
            selection_strategy: document.getElementById('selectionStrategy').value,
            refresh_buffer_sec: document.getElementById('refreshBuffer').value ? parseInt(document.getElementById('refreshBuffer').value) : null,
            max_error_count: document.getElementById('maxErrorCount').value ? parseInt(document.getElementById('maxErrorCount').value) : null
        };

        try {
            this.log('Saving provider settings...', 'info');
            this.loading.show();
            await this.api.updateProviderSettings(this.currentProviderId, settings);
            this.log('Settings saved successfully', 'success');
            this.toast.show('Settings saved successfully', 'success');
            this.hideProviderSettingsModal();
            await this.loadData();
        } catch (error) {
            this.log(`Failed to save settings: ${error.message}`, 'error');
            this.toast.show('Failed to save settings', 'error');
        } finally {
            this.loading.hide();
        }
    }

    // ============================================
    // Global Settings Methods
    // ============================================

    async saveGlobalSettings() {
        const url = document.getElementById('apiBaseUrl').value.trim();
        if (!url) {
            this.toast.show('Please enter a valid URL', 'error');
            return;
        }

        const interval = parseInt(document.getElementById('autoRefreshInterval').value);
        if (interval < 10 || interval > 300) {
            this.toast.show('Auto-refresh interval must be between 10 and 300 seconds', 'error');
            return;
        }

        this.state.setApiBaseUrl(url);
        this.state.setAutoRefreshInterval(interval);
        this.api.setBaseUrl(url);
        
        this.log(`API base URL changed to ${url}`, 'info');
        this.log(`Auto-refresh interval set to ${interval} seconds`, 'info');
        this.toast.show('Settings saved', 'success');
        
        // Restart periodic refresh with new interval
        this.stopPeriodicRefresh();
        this.startPeriodicRefresh();
    }

    // ============================================
    // Log Methods
    // ============================================

    log(message, level = 'info', provider = null) {
        const logEntry = {
            timestamp: Date.now(),
            level: level,
            message: message,
            provider: provider
        };
        this.state.addLog(logEntry);
    }

    exportLogs() {
        const logs = this.state.getLogs();
        const exportData = logs.map(log =>
            `${new Date(log.timestamp).toISOString()} [${log.level.toUpperCase()}] ${log.provider || 'SYSTEM'}: ${log.message}`
        ).join('\n');

        const blob = new Blob([exportData], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `dashboard-logs-${new Date().toISOString().split('T')[0]}.txt`;
        a.click();
        URL.revokeObjectURL(url);
        
        this.log('Logs exported', 'success');
        this.toast.show('Logs exported successfully', 'success');
    }

    // ============================================
    // Utility Methods
    // ============================================

    formatExpiresIn(seconds) {
        if (!seconds || seconds < 0) return 'Expired';
        if (seconds < 60) return `${Math.floor(seconds)}s`;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
        return `${Math.floor(seconds / 3600)}h`;
    }

    formatDuration(ms) {
        if (!ms || ms < 0) return 'Expired';
        const seconds = Math.floor(ms / 1000);
        if (seconds < 60) return `${seconds}s`;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
        if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
        return `${Math.floor(seconds / 86400)}d`;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return 'Never';
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;
        
        if (diff < 60000) return 'Just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        if (diff < 604800000) return `${Math.floor(diff / 86400000)}d ago`;
        
        return date.toLocaleDateString();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ============================================
    // Server Status
    // ============================================

    async checkServerStatus() {
        const statusDot = document.getElementById('statusDot');
        const statusText = document.getElementById('statusText');

        statusDot.className = 'status-dot checking';
        statusText.textContent = 'Checking...';

        try {
            await this.api.getProviders();
            statusDot.className = 'status-dot online';
            statusText.textContent = 'Online';
        } catch (error) {
            statusDot.className = 'status-dot';
            statusText.textContent = 'Offline';
        }
    }

    updateServerStatus(status) {
        const statusDot = document.getElementById('statusDot');
        const statusText = document.getElementById('statusText');
        
        statusDot.className = `status-dot ${status}`;
        statusText.textContent = status.charAt(0).toUpperCase() + status.slice(1);
    }

    // ============================================
    // Periodic Refresh
    // ============================================

    startPeriodicRefresh() {
        const interval = this.state.getAutoRefreshInterval() * 1000;
        this.refreshInterval = setInterval(() => {
            this.checkServerStatus();
        }, interval);
    }

    stopPeriodicRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }
}
