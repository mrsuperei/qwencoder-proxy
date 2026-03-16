# Phase 7: Dashboard UI - Admin Dashboard Interface

**Phase Goal:** Implement admin dashboard UI for rate limit management and usage monitoring.

**Duration:** Week 6-7  
**Status:** Ready to Implement  
**Dependencies:** Phase 6 (Admin API)

---

## Task Overview

This phase implements the admin dashboard UI with:

1. Creating Rate Limits tab UI
2. Creating Usage tab UI
3. Adding quota info to Tokens tab
4. Implementing charts (using Chart.js)
5. Adding export functionality
6. Writing UI tests

---

## Task 7.1: Create Rate Limits Tab UI

**File:** `qwencoder-proxy/web/dashboard/templates/rate-limits-tab.html`

Create the Rate Limits tab template.

**Implementation Requirements:**

```html
<!-- Rate Limits Tab -->
<div class="tab-content" id="rateLimitsTab">
    <!-- Summary Cards -->
    <div class="summary-stats" id="rateLimitSummary">
        <div class="stat-card">
            <h3>Total Limits</h3>
            <span class="stat-value" id="totalLimits">0</span>
        </div>
        <div class="stat-card">
            <h3>Active Limits</h3>
            <span class="stat-value" id="activeLimits">0</span>
        </div>
        <div class="stat-card">
            <h3>Providers Configured</h3>
            <span class="stat-value" id="configuredProviders">0</span>
        </div>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar">
        <select class="filter-input" id="filterProvider">
            <option value="all">All Providers</option>
            <option value="qwen">Qwen</option>
            <option value="gemini-cli">Gemini</option>
            <option value="antigravity">Antigravity</option>
            <option value="kiro">Kiro</option>
            <option value="iflow">iFlow</option>
        </select>
        <select class="filter-input" id="filterLimitType">
            <option value="all">All Limit Types</option>
            <option value="daily">Daily</option>
            <option value="burst">Burst</option>
            <option value="token">Token</option>
        </select>
        <select class="filter-input" id="filterEnabled">
            <option value="all">All Status</option>
            <option value="true">Enabled</option>
            <option value="false">Disabled</option>
        </select>
        <button class="btn btn-secondary" id="refreshRateLimitsBtn">
            🔄 Refresh
        </button>
        <button class="btn btn-primary" id="addRateLimitBtn">
            + Add Rate Limit
        </button>
    </div>

    <!-- Rate Limits Table -->
    <div class="table-container">
        <table id="rateLimitsTable">
            <thead>
                <tr>
                    <th data-sort="provider">Provider <span class="sort-icon">⇅</span></th>
                    <th data-sort="token">Token <span class="sort-icon">⇅</span></th>
                    <th data-sort="model">Model <span class="sort-icon">⇅</span></th>
                    <th data-sort="limitType">Limit Type <span class="sort-icon">⇅</span></th>
                    <th data-sort="limitValue">Limit Value <span class="sort-icon">⇅</span></th>
                    <th data-sort="timeWindow">Time Window <span class="sort-icon">⇅</span></th>
                    <th data-sort="enabled">Enabled <span class="sort-icon">⇅</span></th>
                    <th data-sort="priority">Priority <span class="sort-icon">⇅</span></th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody id="rateLimitsTableBody">
                <!-- Dynamic content -->
            </tbody>
        </table>
    </div>
</div>
```

**File:** `qwencoder-proxy/web/dashboard/js/rate-limits.js`

Create the JavaScript for Rate Limits tab.

```javascript
/**
 * Rate Limits Tab
 * Manages rate limit configuration and display
 */

import { APIClient } from './api/client.js';
import { Modal } from './components/modal.js';
import { Toast } from './components/toast.js';

export class RateLimitsTab {
    constructor(api) {
        this.api = api;
        this.rateLimits = [];
        this.filteredLimits = [];
    }

    async init() {
        this.bindEvents();
        await this.loadRateLimits();
        this.render();
    }

    bindEvents() {
        document.getElementById('refreshRateLimitsBtn').addEventListener('click', () => {
            this.loadRateLimits();
        });

        document.getElementById('addRateLimitBtn').addEventListener('click', () => {
            this.showAddModal();
        });

        document.getElementById('filterProvider').addEventListener('change', () => {
            this.applyFilters();
        });

        document.getElementById('filterLimitType').addEventListener('change', () => {
            this.applyFilters();
        });

        document.getElementById('filterEnabled').addEventListener('change', () => {
            this.applyFilters();
        });

        // Table sort headers
        document.querySelectorAll('#rateLimitsTable th[data-sort]').forEach(th => {
            th.addEventListener('click', () => {
                const field = th.dataset.sort;
                this.sortLimits(field);
            });
        });
    }

    async loadRateLimits() {
        try {
            const data = await this.api.getRateLimits();
            this.rateLimits = data.rate_limits || [];
            this.applyFilters();
            this.render();
            this.updateSummary();
        } catch (error) {
            console.error('Failed to load rate limits:', error);
            Toast.show('Failed to load rate limits', 'error');
        }
    }

    applyFilters() {
        const provider = document.getElementById('filterProvider').value;
        const limitType = document.getElementById('filterLimitType').value;
        const enabled = document.getElementById('filterEnabled').value;

        this.filteredLimits = this.rateLimits.filter(limit => {
            if (provider !== 'all' && limit.provider_id !== provider) {
                return false;
            }
            if (limitType !== 'all' && limit.limit_type !== limitType) {
                return false;
            }
            if (enabled !== 'all' && String(limit.enabled) !== enabled) {
                return false;
            }
            return true;
        });

        this.renderTable();
    }

    sortLimits(field) {
        this.filteredLimits.sort((a, b) => {
            const aVal = a[field];
            const bVal = b[field];
            if (aVal < bVal) return -1;
            if (aVal > bVal) return 1;
            return 0;
        });
        this.renderTable();
    }

    render() {
        this.renderTable();
        this.updateSummary();
    }

    renderTable() {
        const tbody = document.getElementById('rateLimitsTableBody');
        tbody.innerHTML = '';

        this.filteredLimits.forEach(limit => {
            const row = document.createElement('tr');
            row.dataset.id = limit.id;
            row.innerHTML = `
                <td>${this.formatProvider(limit.provider_id)}</td>
                <td>${limit.token_id || '-'}</td>
                <td>${limit.model || '-'}</td>
                <td>${this.formatLimitType(limit.limit_type)}</td>
                <td>${limit.limit_value.toLocaleString()}</td>
                <td>${this.formatTimeWindow(limit.time_window)}</td>
                <td>${limit.enabled ? '✅' : '❌'}</td>
                <td>${limit.priority}</td>
                <td>
                    <button class="btn btn-sm btn-secondary" data-action="edit" data-id="${limit.id}">Edit</button>
                    <button class="btn btn-sm ${limit.enabled ? 'btn-warning' : 'btn-success'}" 
                            data-action="${limit.enabled ? 'disable' : 'enable'}" 
                            data-id="${limit.id}">
                        ${limit.enabled ? 'Disable' : 'Enable'}
                    </button>
                    <button class="btn btn-sm btn-danger" data-action="delete" data-id="${limit.id}">Delete</button>
                </td>
            `;
            tbody.appendChild(row);
        });

        // Bind action buttons
        tbody.querySelectorAll('button[data-action]').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const action = e.target.dataset.action;
                const id = e.target.dataset.id;
                this.handleAction(action, id);
            });
        });
    }

    updateSummary() {
        document.getElementById('totalLimits').textContent = this.rateLimits.length;
        document.getElementById('activeLimits').textContent = 
            this.rateLimits.filter(l => l.enabled).length;
        
        const providers = new Set(this.rateLimits.map(l => l.provider_id));
        document.getElementById('configuredProviders').textContent = providers.size;
    }

    async handleAction(action, id) {
        switch (action) {
            case 'edit':
                this.showEditModal(id);
                break;
            case 'enable':
                await this.toggleLimit(id, true);
                break;
            case 'disable':
                await this.toggleLimit(id, false);
                break;
            case 'delete':
                await this.deleteLimit(id);
                break;
        }
    }

    async toggleLimit(id, enabled) {
        try {
            const endpoint = enabled ? 'enable' : 'disable';
            await this.api.post(`/api/rate-limits/${id}/${endpoint}`);
            Toast.show(`Rate limit ${enabled ? 'enabled' : 'disabled'}`, 'success');
            await this.loadRateLimits();
        } catch (error) {
            console.error('Failed to toggle rate limit:', error);
            Toast.show('Failed to toggle rate limit', 'error');
        }
    }

    async deleteLimit(id) {
        if (!confirm('Are you sure you want to delete this rate limit?')) {
            return;
        }

        try {
            await this.api.delete(`/api/rate-limits/${id}`);
            Toast.show('Rate limit deleted', 'success');
            await this.loadRateLimits();
        } catch (error) {
            console.error('Failed to delete rate limit:', error);
            Toast.show('Failed to delete rate limit', 'error');
        }
    }

    showAddModal() {
        const modal = new Modal({
            title: 'Add Rate Limit',
            content: this.getFormContent(),
            onConfirm: async (formData) => {
                await this.createRateLimit(formData);
            }
        });
        modal.show();
    }

    showEditModal(id) {
        const limit = this.rateLimits.find(l => l.id === id);
        if (!limit) return;

        const modal = new Modal({
            title: 'Edit Rate Limit',
            content: this.getFormContent(limit),
            onConfirm: async (formData) => {
                await this.updateRateLimit(id, formData);
            }
        });
        modal.show();
    }

    async createRateLimit(formData) {
        try {
            await this.api.post('/api/rate-limits', formData);
            Toast.show('Rate limit created', 'success');
            await this.loadRateLimits();
        } catch (error) {
            console.error('Failed to create rate limit:', error);
            Toast.show('Failed to create rate limit', 'error');
        }
    }

    async updateRateLimit(id, formData) {
        try {
            await this.api.put(`/api/rate-limits/${id}`, formData);
            Toast.show('Rate limit updated', 'success');
            await this.loadRateLimits();
        } catch (error) {
            console.error('Failed to update rate limit:', error);
            Toast.show('Failed to update rate limit', 'error');
        }
    }

    getFormContent(limit = null) {
        const providers = ['qwen', 'gemini-cli', 'antigravity', 'kiro', 'iflow'];
        const limitTypes = ['daily', 'burst', 'token'];

        return `
            <form id="rateLimitForm">
                <div class="form-group">
                    <label for="provider_id">Provider</label>
                    <select id="provider_id" name="provider_id" required>
                        ${providers.map(p => 
                            `<option value="${p}" ${limit && limit.provider_id === p ? 'selected' : ''}>${p}</option>`
                        ).join('')}
                    </select>
                </div>
                <div class="form-group">
                    <label for="token_id">Token ID (optional)</label>
                    <input type="text" id="token_id" name="token_id" 
                           value="${limit ? limit.token_id || '' : ''}" 
                           placeholder="Leave empty for provider-wide limit">
                </div>
                <div class="form-group">
                    <label for="model">Model (optional)</label>
                    <input type="text" id="model" name="model" 
                           value="${limit ? limit.model || '' : ''}" 
                           placeholder="Leave empty for all models">
                </div>
                <div class="form-group">
                    <label for="limit_type">Limit Type</label>
                    <select id="limit_type" name="limit_type" required>
                        ${limitTypes.map(t => 
                            `<option value="${t}" ${limit && limit.limit_type === t ? 'selected' : ''}>${t}</option>`
                        ).join('')}
                    </select>
                </div>
                <div class="form-group">
                    <label for="limit_value">Limit Value</label>
                    <input type="number" id="limit_value" name="limit_value" 
                           value="${limit ? limit.limit_value : ''}" 
                           min="1" required>
                </div>
                <div class="form-group">
                    <label for="time_window">Time Window (seconds)</label>
                    <input type="number" id="time_window" name="time_window" 
                           value="${limit ? limit.time_window / 1000000000 : 86400}" 
                           min="1" required>
                    <small>86400 = 1 day, 60 = 1 minute</small>
                </div>
                <div class="form-group">
                    <label for="priority">Priority</label>
                    <input type="number" id="priority" name="priority" 
                           value="${limit ? limit.priority : 10}" 
                           min="0" required>
                    <small>Higher priority = checked first</small>
                </div>
            </form>
        `;
    }

    formatProvider(providerId) {
        const names = {
            'qwen': 'Qwen',
            'gemini-cli': 'Gemini',
            'antigravity': 'Antigravity',
            'kiro': 'Kiro',
            'iflow': 'iFlow'
        };
        return names[providerId] || providerId;
    }

    formatLimitType(type) {
        return type.charAt(0).toUpperCase() + type.slice(1);
    }

    formatTimeWindow(seconds) {
        if (seconds >= 86400) {
            return `${seconds / 86400} day(s)`;
        } else if (seconds >= 3600) {
            return `${seconds / 3600} hour(s)`;
        } else if (seconds >= 60) {
            return `${seconds / 60} minute(s)`;
        }
        return `${seconds} second(s)`;
    }
}
```

---

## Task 7.2: Create Usage Tab UI

**File:** `qwencoder-proxy/web/dashboard/templates/usage-tab.html`

Create the Usage tab template.

```html
<!-- Usage Tab -->
<div class="tab-content" id="usageTab">
    <!-- Time Range Selector -->
    <div class="filter-bar">
        <select id="timeRange">
            <option value="hour">Last Hour</option>
            <option value="day" selected>Last 24 Hours</option>
            <option value="week">Last 7 Days</option>
            <option value="month">Last 30 Days</option>
        </select>
        <select id="usageProvider">
            <option value="all">All Providers</option>
            <option value="qwen">Qwen</option>
            <option value="gemini-cli">Gemini</option>
            <option value="antigravity">Antigravity</option>
            <option value="kiro">Kiro</option>
            <option value="iflow">iFlow</option>
        </select>
        <select id="usageToken">
            <option value="all">All Tokens</option>
        </select>
        <button class="btn btn-secondary" id="exportUsageBtn">
            📥 Export
        </button>
        <button class="btn btn-secondary" id="refreshUsageBtn">
            🔄 Refresh
        </button>
    </div>

    <!-- Usage Charts -->
    <div class="charts-container">
        <div class="chart-card">
            <h3>Requests Over Time</h3>
            <canvas id="requestsChart"></canvas>
        </div>
        <div class="chart-card">
            <h3>Tokens Over Time</h3>
            <canvas id="tokensChart"></canvas>
        </div>
    </div>

    <!-- Usage Table -->
    <div class="table-container">
        <table id="usageTable">
            <thead>
                <tr>
                    <th>Provider</th>
                    <th>Token</th>
                    <th>Model</th>
                    <th>Requests</th>
                    <th>Tokens</th>
                    <th>Success Rate</th>
                    <th>Avg Response Time</th>
                </tr>
            </thead>
            <tbody id="usageTableBody">
                <!-- Dynamic content -->
            </tbody>
        </table>
    </div>
</div>
```

**File:** `qwencoder-proxy/web/dashboard/js/usage.js`

Create the JavaScript for Usage tab.

```javascript
/**
 * Usage Tab
 * Manages usage monitoring and visualization
 */

import { APIClient } from './api/client.js';
import { Toast } from './components/toast.js';

export class UsageTab {
    constructor(api) {
        this.api = api;
        this.usageData = [];
        this.charts = {};
    }

    async init() {
        await this.loadUsage();
        this.bindEvents();
        this.render();
    }

    bindEvents() {
        document.getElementById('refreshUsageBtn').addEventListener('click', () => {
            this.loadUsage();
        });

        document.getElementById('exportUsageBtn').addEventListener('click', () => {
            this.exportUsage();
        });

        document.getElementById('timeRange').addEventListener('change', () => {
            this.loadUsage();
        });

        document.getElementById('usageProvider').addEventListener('change', () => {
            this.loadUsage();
        });

        document.getElementById('usageToken').addEventListener('change', () => {
            this.loadUsage();
        });
    }

    async loadUsage() {
        try {
            const timeRange = document.getElementById('timeRange').value;
            const provider = document.getElementById('usageProvider').value;
            const token = document.getElementById('usageToken').value;

            const params = {
                time_range: timeRange,
                provider_id: provider === 'all' ? undefined : provider,
                token_id: token === 'all' ? undefined : token
            };

            const data = await this.api.getUsage(params);
            this.usageData = data.usage || [];
            this.render();
            this.renderCharts();
        } catch (error) {
            console.error('Failed to load usage:', error);
            Toast.show('Failed to load usage', 'error');
        }
    }

    render() {
        this.renderTable();
    }

    renderTable() {
        const tbody = document.getElementById('usageTableBody');
        tbody.innerHTML = '';

        // Aggregate usage by provider, token, model
        const aggregated = this.aggregateUsage();

        aggregated.forEach(item => {
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${this.formatProvider(item.provider_id)}</td>
                <td>${item.token_id || '-'}</td>
                <td>${item.model || '-'}</td>
                <td>${item.request_count.toLocaleString()}</td>
                <td>${item.token_count.toLocaleString()}</td>
                <td>${this.formatSuccessRate(item.success_rate)}</td>
                <td>${this.formatResponseTime(item.avg_response_time_ms)}</td>
            `;
            tbody.appendChild(row);
        });
    }

    aggregateUsage() {
        const aggregated = new Map();

        this.usageData.forEach(record => {
            const key = `${record.provider_id}:${record.token_id}:${record.model}`;
            
            if (!aggregated.has(key)) {
                aggregated.set(key, {
                    provider_id: record.provider_id,
                    token_id: record.token_id,
                    model: record.model,
                    request_count: 0,
                    token_count: 0,
                    success_count: 0,
                    total_response_time: 0
                });
            }

            const item = aggregated.get(key);
            item.request_count += record.request_count;
            item.token_count += record.token_count;
            item.total_response_time += record.response_time_ms;
            if (record.success) {
                item.success_count++;
            }
        });

        // Calculate averages and success rates
        return Array.from(aggregated.values()).map(item => ({
            ...item,
            success_rate: item.request_count > 0 ? item.success_count / item.request_count : 0,
            avg_response_time_ms: item.request_count > 0 ? item.total_response_time / item.request_count : 0
        }));
    }

    renderCharts() {
        this.renderRequestsChart();
        this.renderTokensChart();
    }

    renderRequestsChart() {
        const ctx = document.getElementById('requestsChart').getContext('2d');
        
        if (this.charts.requests) {
            this.charts.requests.destroy();
        }

        const data = this.prepareChartData('request_count');
        
        this.charts.requests = new Chart(ctx, {
            type: 'line',
            data: data,
            options: {
                responsive: true,
                plugins: {
                    title: {
                        display: true,
                        text: 'Requests Over Time'
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    }

    renderTokensChart() {
        const ctx = document.getElementById('tokensChart').getContext('2d');
        
        if (this.charts.tokens) {
            this.charts.tokens.destroy();
        }

        const data = this.prepareChartData('token_count');
        
        this.charts.tokens = new Chart(ctx, {
            type: 'line',
            data: data,
            options: {
                responsive: true,
                plugins: {
                    title: {
                        display: true,
                        text: 'Tokens Over Time'
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    }

    prepareChartData(metric) {
        // Group data by time interval
        const timeRange = document.getElementById('timeRange').value;
        const interval = this.getInterval(timeRange);
        
        const grouped = new Map();
        
        this.usageData.forEach(record => {
            const timeKey = this.getTimeKey(record.timestamp, interval);
            
            if (!grouped.has(timeKey)) {
                grouped.set(timeKey, {
                    time: timeKey,
                    [metric]: 0
                });
            }
            
            grouped.get(timeKey)[metric] += record[metric];
        });

        return {
            labels: Array.from(grouped.keys()),
            datasets: [{
                label: metric === 'request_count' ? 'Requests' : 'Tokens',
                data: Array.from(grouped.values()).map(d => d[metric]),
                borderColor: 'rgb(75, 192, 192)',
                backgroundColor: 'rgba(75, 192, 192, 0.2)',
                tension: 0.1
            }]
        };
    }

    getTimeKey(timestamp, interval) {
        const date = new Date(timestamp);
        
        switch (interval) {
            case 'minute':
                return date.toISOString().slice(0, 16); // YYYY-MM-DDTHH:mm
            case 'hour':
                return date.toISOString().slice(0, 13); // YYYY-MM-DDTHH
            case 'day':
                return date.toISOString().slice(0, 10); // YYYY-MM-DD
            default:
                return date.toISOString().slice(0, 10);
        }
    }

    getInterval(timeRange) {
        switch (timeRange) {
            case 'hour':
                return 'minute';
            case 'day':
                return 'hour';
            case 'week':
                return 'day';
            case 'month':
                return 'day';
            default:
                return 'hour';
        }
    }

    async exportUsage() {
        try {
            const format = 'csv';
            const data = await this.api.get(`/api/usage/export?format=${format}`);
            
            // Create download link
            const blob = new Blob([data], { type: 'text/csv' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `usage_${new Date().toISOString().slice(0, 10)}.csv`;
            a.click();
            URL.revokeObjectURL(url);
            
            Toast.show('Usage exported successfully', 'success');
        } catch (error) {
            console.error('Failed to export usage:', error);
            Toast.show('Failed to export usage', 'error');
        }
    }

    formatProvider(providerId) {
        const names = {
            'qwen': 'Qwen',
            'gemini-cli': 'Gemini',
            'antigravity': 'Antigravity',
            'kiro': 'Kiro',
            'iflow': 'iFlow'
        };
        return names[providerId] || providerId;
    }

    formatSuccessRate(rate) {
        return `${(rate * 100).toFixed(1)}%`;
    }

    formatResponseTime(ms) {
        if (ms < 1000) {
            return `${ms.toFixed(0)}ms`;
        }
        return `${(ms / 1000).toFixed(2)}s`;
    }
}
```

---

## Task 7.3: Add Quota Info to Tokens Tab

Update the existing tokens tab to show quota information.

**File:** `qwencoder-proxy/web/dashboard/js/tokens.js`

Add quota information rendering:

```javascript
// Add to renderTokenRow method
renderTokenRow(token) {
    const quota = this.getQuotaForToken(token.id);
    
    return `
        <tr data-token-id="${token.id}">
            <td>${this.formatProvider(token.provider)}</td>
            <td>${token.email}</td>
            <td>${token.healthy ? '✅' : '❌'}</td>
            <td>${token.health_score.toFixed(2)}</td>
            <td>${this.formatDate(token.expiry_date)}</td>
            <td>${this.formatDate(token.last_used)}</td>
            <td>${this.formatDate(token.created_at)}</td>
            <td>
                ${this.renderQuotaInfo(quota)}
            </td>
            <td>
                <!-- Actions -->
            </td>
        </tr>
    `;
}

renderQuotaInfo(quota) {
    if (!quota) {
        return '<span class="text-muted">No quota info</span>';
    }

    const dailyPercent = quota.daily_limit > 0 
        ? (quota.daily_used / quota.daily_limit) * 100 
        : 0;
    
    const burstPercent = quota.burst_limit > 0 
        ? (quota.burst_used / quota.burst_limit) * 100 
        : 0;

    return `
        <div class="quota-info">
            <div class="quota-item">
                <div class="quota-label">Daily</div>
                <div class="quota-bar">
                    <div class="quota-fill ${dailyPercent > 80 ? 'warning' : ''}" 
                         style="width: ${dailyPercent}%"></div>
                </div>
                <div class="quota-text">${quota.daily_used}/${quota.daily_limit}</div>
            </div>
            <div class="quota-item">
                <div class="quota-label">Burst</div>
                <div class="quota-bar">
                    <div class="quota-fill ${burstPercent > 80 ? 'warning' : ''}" 
                         style="width: ${burstPercent}%"></div>
                </div>
                <div class="quota-text">${quota.burst_used}/${quota.burst_limit}</div>
            </div>
        </div>
    `;
}

async getQuotaForToken(tokenId) {
    try {
        const data = await this.api.get(`/api/quota/token/${tokenId}`);
        return data;
    } catch (error) {
        console.error('Failed to get quota:', error);
        return null;
    }
}
```

---

## Task 7.4: Implement Charts (Using Chart.js)

Add Chart.js to the dashboard and implement charts.

**File:** `qwencoder-proxy/web/dashboard/index.html`

Add Chart.js CDN:

```html
<head>
    <!-- ... existing CSS ... -->
    
    <!-- Chart.js -->
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
</head>
```

---

## Task 7.5: Add Export Functionality

Export functionality is already implemented in `UsageTab.exportUsage()`. Ensure:

1. CSV export works
2. JSON export works
3. Filename includes date
4. Download is triggered

---

## Task 7.6: Write UI Tests

**File:** `qwencoder-proxy/web/dashboard/js/rate-limits.test.js`

Write UI tests for the dashboard.

**Test Cases:**

1. **Rate Limits Tab:**
   - Test loading rate limits
   - Test filtering rate limits
   - Test creating rate limit
   - Test editing rate limit
   - Test deleting rate limit

2. **Usage Tab:**
   - Test loading usage data
   - Test filtering usage
   - Test exporting usage

**Test Template:**

```javascript
import { RateLimitsTab } from './rate-limits.js';
import { APIClient } from './api/client.js';

describe('RateLimitsTab', () => {
    let api;
    let tab;

    beforeEach(() => {
        api = new APIClient();
        tab = new RateLimitsTab(api);
    });

    test('should load rate limits', async () => {
        const mockData = { rate_limits: [] };
        jest.spyOn(api, 'getRateLimits').mockResolvedValue(mockData);

        await tab.loadRateLimits();

        expect(api.getRateLimits).toHaveBeenCalled();
        expect(tab.rateLimits).toEqual(mockData.rate_limits);
    });

    test('should apply filters', () => {
        tab.rateLimits = [
            { provider_id: 'qwen', limit_type: 'daily', enabled: true },
            { provider_id: 'gemini-cli', limit_type: 'burst', enabled: false }
        ];

        // Mock DOM elements
        document.getElementById('filterProvider').value = 'qwen';
        document.getElementById('filterLimitType').value = 'daily';
        document.getElementById('filterEnabled').value = 'true';

        tab.applyFilters();

        expect(tab.filteredLimits).toHaveLength(1);
        expect(tab.filteredLimits[0].provider_id).toBe('qwen');
    });

    // Implement more tests...
});
```

---

## Deliverables

After completing this phase, you should have:

1. ✅ Rate Limits tab UI
2. ✅ Usage tab UI
3. ✅ Enhanced Tokens tab with quota info
4. ✅ Charts implemented with Chart.js
5. ✅ Export functionality
6. ✅ UI tests

---

## Success Criteria

- [ ] Rate Limits tab displays all rate limits
- [ ] Rate limits can be created, edited, deleted
- [ ] Usage tab displays charts and tables
- [ ] Usage can be filtered by time range and provider
- [ ] Usage can be exported as CSV
- [ ] Quota information is displayed on Tokens tab
- [ ] All UI tests pass
- [ ] Dashboard is responsive

---

## Next Phase

After completing Phase 7, proceed to **Phase 8: Testing & Optimization** which implements comprehensive testing and performance optimization.

**File:** `../todo/phase-8-testing-optimization.md`
