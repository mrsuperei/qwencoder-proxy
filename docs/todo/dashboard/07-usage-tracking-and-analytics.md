# Phase 7: Usage Tracking & Analytics

**Priority:** HIGH  
**Estimated Time:** 2 days  
**Complexity:** Medium  
**Files to Create:** 1  
**Files to Modify:** 1

---

## Executive Summary

This phase implements usage tracking and analytics features. Users can view request history, model usage statistics, error tracking, and cache statistics.

---

## Problem Description

Users need visibility into how tokens are being used. This includes request history, model usage, error tracking, and cache statistics.

### Requirements

- View request history with filtering and pagination
- View model usage statistics (by provider, token, model)
- View error tracking information
- View cache statistics
- Delete request history records
- Reset error tracking

---

## Solution Architecture

### Design Principles

1. **Comprehensive Tracking:** Track all usage metrics
2. **Flexible Filtering:** Filter by token, model, time range
3. **Visual Analytics:** Charts and graphs for insights
4. **Export Capability:** Export data for analysis
5. **Real-time Updates:** Data updates in real-time

### Component Overview

```
js/views/
└── Usage.js          # Usage tracking and analytics view
```

---

## Implementation Plan

### Step 1: Implement Usage View

Create comprehensive usage tracking interface:

```javascript
/**
 * Usage View
 * 
 * Displays usage tracking, request history, model usage,
 * error tracking, and cache statistics.
 */

import { View } from '../components/View.js';
import { Table } from '../components/Table.js';
import { Modal } from '../components/Modal.js';
import { Form } from '../components/Form.js';
import { toast } from '../components/Toast.js';
import { selectors, actions } from '../state/store.js';

export default class UsageView extends View {
    constructor(app) {
        super(app);
        this.activeTab = 'history';
        this.filters = {
            token: '',
            model: '',
            startTime: '',
            endTime: '',
            minTokens: '',
            maxTokens: ''
        };
        this.pagination = {
            page: 1,
            pageSize: 50
        };
    }

    async render(container) {
        await super.render(container);

        this.subscribe('requestHistory', () => this.render());
        this.subscribe('modelUsage', () => this.render());
        this.subscribe('errors', () => this.render());
        this.subscribe('cacheStats', () => this.render());

        this.renderContent();
    }

    renderContent() {
        this.container.innerHTML = `
            <div class="view-header">
                <h1>Usage & Analytics</h1>
                <div class="header-actions">
                    <button class="btn btn-primary" data-action="refresh">
                        🔄 Refresh
                    </button>
                </div>
            </div>

            <div class="usage-tabs">
                <button class="usage-tab ${this.activeTab === 'history' ? 'active' : ''}" data-tab="history">
                    📋 Request History
                </button>
                <button class="usage-tab ${this.activeTab === 'models' ? 'active' : ''}" data-tab="models">
                    🤖 Model Usage
                </button>
                <button class="usage-tab ${this.activeTab === 'errors' ? 'active' : ''}" data-tab="errors">
                    ⚠️ Errors
                </button>
                <button class="usage-tab ${this.activeTab === 'cache' ? 'active' : ''}" data-tab="cache">
                    💾 Cache
                </button>
            </div>

            <div class="usage-content">
                <div id="usageTabContent"></div>
            </div>
        `;

        this.setupEventListeners();
        this.renderActiveTab();
    }

    setupEventListeners() {
        // Tab switching
        this.container.querySelectorAll('.usage-tab').forEach(tab => {
            tab.addEventListener('click', () => {
                this.activeTab = tab.dataset.tab;
                this.renderContent();
            });
        });

        // Refresh button
        this.container.querySelector('[data-action="refresh"]').addEventListener('click', () => {
            this.refreshData();
        });
    }

    async renderActiveTab() {
        const container = document.getElementById('usageTabContent');

        switch (this.activeTab) {
            case 'history':
                await this.renderRequestHistory(container);
                break;
            case 'models':
                await this.renderModelUsage(container);
                break;
            case 'errors':
                await this.renderErrors(container);
                break;
            case 'cache':
                await this.renderCache(container);
                break;
        }
    }

    async renderRequestHistory(container) {
        const credentials = this.store.get('credentials');

        container.innerHTML = `
            <div class="history-filters">
                <select class="filter-input" id="filterToken">
                    <option value="">All Tokens</option>
                    ${credentials.map(c => `
                        <option value="${c.id}">${c.email}</option>
                    `).join('')}
                </select>
                <input type="text" class="filter-input" id="filterModel" placeholder="Filter by model...">
                <input type="date" class="filter-input" id="filterStartTime" placeholder="Start date">
                <input type="date" class="filter-input" id="filterEndTime" placeholder="End date">
                <button class="btn btn-secondary" data-action="clear-filters">Clear</button>
                <button class="btn btn-primary" data-action="apply-filters">Apply</button>
                <button class="btn btn-danger" data-action="delete-history">Delete Records</button>
            </div>
            <div id="historyTable"></div>
            <div id="historyPagination"></div>
        `;

        await this.loadHistoryData();

        // Setup filter event listeners
        container.querySelector('[data-action="clear-filters"]').addEventListener('click', () => {
            this.clearFilters();
        });
        container.querySelector('[data-action="apply-filters"]').addEventListener('click', () => {
            this.pagination.page = 1;
            this.loadHistoryData();
        });
        container.querySelector('[data-action="delete-history"]').addEventListener('click', () => {
            this.deleteHistoryRecords();
        });
    }

    async loadHistoryData() {
        const params = {
            page: this.pagination.page,
            page_size: this.pagination.pageSize
        };

        if (this.filters.token) params.token_id = this.filters.token;
        if (this.filters.model) params.model = this.filters.model;
        if (this.filters.startTime) params.start_time = new Date(this.filters.startTime).getTime();
        if (this.filters.endTime) params.end_time = new Date(this.filters.endTime).getTime();

        try {
            const response = await this.api.getRequestHistory(params);
            this.renderHistoryTable(response);
            this.renderPagination(response);
        } catch (error) {
            toast.error(`Failed to load history: ${error.message}`);
        }
    }

    renderHistoryTable(response) {
        const credentials = this.store.get('credentials');

        const table = new Table({
            columns: [
                {
                    key: 'timestamp',
                    label: 'Time',
                    render: (value) => new Date(value).toLocaleString(),
                    sortable: true
                },
                {
                    key: 'token_id',
                    label: 'Token',
                    render: (value) => {
                        const token = credentials.find(c => c.id === value);
                        return token ? token.email : value.substring(0, 8) + '...';
                    }
                },
                {
                    key: 'model',
                    label: 'Model',
                    sortable: true
                },
                {
                    key: 'request_count',
                    label: 'Requests',
                    sortable: true
                },
                {
                    key: 'input_tokens',
                    label: 'Input Tokens',
                    sortable: true
                },
                {
                    key: 'output_tokens',
                    label: 'Output Tokens',
                    sortable: true
                },
                {
                    key: 'total_tokens',
                    label: 'Total Tokens',
                    sortable: true
                }
            ],
            data: response.records,
            emptyMessage: 'No request history found'
        });

        table.render(document.getElementById('historyTable'));
    }

    renderPagination(response) {
        const container = document.getElementById('historyPagination');
        const { page, total_count, page_size, total_pages } = response;

        container.innerHTML = `
            <div class="pagination">
                <span class="pagination-info">
                    Showing ${Math.min((page - 1) * page_size + 1, total_count)} - ${Math.min(page * page_size, total_count)} of ${total_count}
                </span>
                <div class="pagination-controls">
                    <button class="btn btn-sm ${page === 1 ? 'disabled' : ''}" data-action="prev-page">
                        ← Previous
                    </button>
                    <span class="pagination-page">${page} / ${total_pages}</span>
                    <button class="btn btn-sm ${page === total_pages ? 'disabled' : ''}" data-action="next-page">
                        Next →
                    </button>
                </div>
            </div>
        `;

        container.querySelector('[data-action="prev-page"]')?.addEventListener('click', () => {
            if (this.pagination.page > 1) {
                this.pagination.page--;
                this.loadHistoryData();
            }
        });

        container.querySelector('[data-action="next-page"]')?.addEventListener('click', () => {
            if (this.pagination.page < total_pages) {
                this.pagination.page++;
                this.loadHistoryData();
            }
        });
    }

    clearFilters() {
        this.filters = {
            token: '',
            model: '',
            startTime: '',
            endTime: '',
            minTokens: '',
            maxTokens: ''
        };
        document.getElementById('filterToken').value = '';
        document.getElementById('filterModel').value = '';
        document.getElementById('filterStartTime').value = '';
        document.getElementById('filterEndTime').value = '';
        this.pagination.page = 1;
        this.loadHistoryData();
    }

    async deleteHistoryRecords() {
        const confirmed = await confirmModal({
            title: 'Delete Request History',
            message: 'Are you sure you want to delete request history records? This action cannot be undone.',
            size: 'sm'
        });

        if (!confirmed) return;

        const params = {};
        if (this.filters.token) params.token_id = this.filters.token;
        if (this.filters.model) params.model = this.filters.model;
        if (this.filters.startTime) params.start_time = new Date(this.filters.startTime).getTime();
        if (this.filters.endTime) params.end_time = new Date(this.filters.endTime).getTime();

        try {
            await this.api.deleteRequestHistory(params);
            toast.success('Request history deleted');
            await this.loadHistoryData();
        } catch (error) {
            toast.error(`Failed to delete history: ${error.message}`);
        }
    }

    async renderModelUsage(container) {
        const modelUsage = this.store.get('modelUsage');
        const credentials = this.store.get('credentials');

        container.innerHTML = `
            <div class="model-usage-summary">
                ${Object.entries(modelUsage).map(([providerId, models]) => {
                    const provider = this.store.get('providers').find(p => p.id === providerId);
                    if (!provider) return '';

                    return `
                        <div class="card model-usage-card">
                            <div class="card-header">
                                <h3 class="card-title">${provider.name}</h3>
                            </div>
                            <div class="card-body">
                                ${Object.entries(models).map(([modelName, usage]) => `
                                    <div class="model-usage-item">
                                        <div class="model-name">${modelName}</div>
                                        <div class="model-stats">
                                            <div class="stat">
                                                <span class="stat-label">Requests</span>
                                                <span class="stat-value">${usage.request_count.toLocaleString()}</span>
                                            </div>
                                            <div class="stat">
                                                <span class="stat-label">Input Tokens</span>
                                                <span class="stat-value">${usage.input_tokens.toLocaleString()}</span>
                                            </div>
                                            <div class="stat">
                                                <span class="stat-label">Output Tokens</span>
                                                <span class="stat-value">${usage.output_tokens.toLocaleString()}</span>
                                            </div>
                                            <div class="stat">
                                                <span class="stat-label">Total Tokens</span>
                                                <span class="stat-value">${usage.total_tokens.toLocaleString()}</span>
                                            </div>
                                        </div>
                                    </div>
                                `).join('')}
                            </div>
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    }

    async renderErrors(container) {
        const credentials = this.store.get('credentials');

        container.innerHTML = `
            <div class="errors-filters">
                <select class="filter-input" id="errorFilterToken">
                    <option value="">All Tokens</option>
                    ${credentials.map(c => `
                        <option value="${c.id}">${c.email}</option>
                    `).join('')}
                </select>
                <button class="btn btn-primary" data-action="load-errors">Load Errors</button>
            </div>
            <div id="errorsContainer"></div>
        `;

        container.querySelector('[data-action="load-errors"]').addEventListener('click', () => {
            this.loadErrors();
        });
    }

    async loadErrors() {
        const tokenId = document.getElementById('errorFilterToken').value;

        if (!tokenId) {
            toast.info('Please select a token to view errors');
            return;
        }

        try {
            const credentials = this.store.get('credentials');
            const token = credentials.find(c => c.id === tokenId);
            
            const response = await this.api.getTokenErrors(token.provider, tokenId);
            this.renderErrorsList(response, token);
        } catch (error) {
            toast.error(`Failed to load errors: ${error.message}`);
        }
    }

    renderErrorsList(response, token) {
        const container = document.getElementById('errorsContainer');

        container.innerHTML = `
            <div class="card error-details-card">
                <div class="card-header">
                    <h3 class="card-title">${token.email}</h3>
                    <button class="btn btn-sm btn-secondary" data-action="reset-errors">
                        Reset Errors
                    </button>
                </div>
                <div class="card-body">
                    <div class="error-summary">
                        <div class="error-stat">
                            <span class="stat-label">Error Count</span>
                            <span class="stat-value ${response.error_count > 0 ? 'text-danger' : 'text-success'}">
                                ${response.error_count}
                            </span>
                        </div>
                        <div class="error-stat">
                            <span class="stat-label">Consecutive Errors</span>
                            <span class="stat-value ${response.consecutive_errors > 0 ? 'text-danger' : 'text-success'}">
                                ${response.consecutive_errors}
                            </span>
                        </div>
                        <div class="error-stat">
                            <span class="stat-label">Health Status</span>
                            <span class="stat-value ${response.is_unhealthy ? 'text-danger' : 'text-success'}">
                                ${response.is_unhealthy ? 'Unhealthy' : 'Healthy'}
                            </span>
                        </div>
                    </div>
                    ${response.last_error ? `
                        <div class="last-error">
                            <h4>Last Error</h4>
                            <p class="error-message">${response.last_error}</p>
                            <p class="error-time">${new Date(response.last_error_time).toLocaleString()}</p>
                        </div>
                    ` : '<p class="no-errors">No errors recorded</p>'}
                </div>
            </div>
        `;

        container.querySelector('[data-action="reset-errors"]').addEventListener('click', () => {
            this.resetErrors(token);
        });
    }

    async resetErrors(token) {
        const confirmed = await confirmModal({
            title: 'Reset Errors',
            message: 'Are you sure you want to reset error tracking for this token?',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.resetTokenErrors(token.provider, token.id);
            toast.success('Error tracking reset');
            await this.loadErrors();
        } catch (error) {
            toast.error(`Failed to reset errors: ${error.message}`);
        }
    }

    async renderCache(container) {
        const cacheStats = this.store.get('cacheStats');

        container.innerHTML = `
            <div class="cache-stats">
                <div class="card cache-stats-card">
                    <div class="card-header">
                        <h3 class="card-title">Cache Statistics</h3>
                        <button class="btn btn-primary" data-action="refresh-cache">
                            🔄 Refresh
                        </button>
                    </div>
                    <div class="card-body">
                        ${cacheStats ? `
                            <div class="stats-grid">
                                <div class="stat-item">
                                    <span class="stat-label">Total Invalidations</span>
                                    <span class="stat-value">${cacheStats.invalidations_total.toLocaleString()}</span>
                                </div>
                                <div class="stat-item">
                                    <span class="stat-label">Last Invalidation</span>
                                    <span class="stat-value">${new Date(cacheStats.last_invalidation).toLocaleString()}</span>
                                </div>
                            </div>
                            <h4>Invalidations by Type</h4>
                            <div class="invalidation-types">
                                ${Object.entries(cacheStats.invalidations_by_type).map(([type, count]) => `
                                    <div class="invalidation-type">
                                        <span class="type-label">${type}</span>
                                        <span class="type-value">${count.toLocaleString()}</span>
                                    </div>
                                `).join('')}
                            </div>
                            <div class="cache-actions">
                                <button class="btn btn-danger" data-action="invalidate-all">
                                    Invalidate All Cache
                                </button>
                            </div>
                        ` : '<p class="no-data">No cache statistics available</p>'}
                    </div>
                </div>
            </div>
        `;

        container.querySelector('[data-action="refresh-cache"]')?.addEventListener('click', () => {
            this.refreshCacheStats();
        });

        container.querySelector('[data-action="invalidate-all"]')?.addEventListener('click', () => {
            this.invalidateAllCache();
        });
    }

    async refreshCacheStats() {
        try {
            const stats = await this.api.getCacheStats();
            this.store.dispatch(actions.setCacheStats(stats));
            toast.success('Cache statistics refreshed');
        } catch (error) {
            toast.error(`Failed to refresh cache stats: ${error.message}`);
        }
    }

    async invalidateAllCache() {
        const confirmed = await confirmModal({
            title: 'Invalidate All Cache',
            message: 'Are you sure you want to invalidate all cached data?',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.invalidateAllCache();
            toast.success('All cache invalidated');
            await this.refreshCacheStats();
        } catch (error) {
            toast.error(`Failed to invalidate cache: ${error.message}`);
        }
    }

    async refreshData() {
        try {
            // Load request history
            const history = await this.api.getRequestHistory();
            this.store.dispatch(actions.setRequestHistory(history.records));

            // Load model usage
            const modelUsage = await this.api.getAllModelUsage();
            this.store.dispatch(actions.setModelUsage(modelUsage));

            // Load cache stats
            const cacheStats = await this.api.getCacheStats();
            this.store.dispatch(actions.setCacheStats(cacheStats));

            toast.success('Usage data refreshed');
            this.renderActiveTab();
        } catch (error) {
            toast.error(`Failed to refresh data: ${error.message}`);
        }
    }
}
```

**Verification:**
- [ ] Usage view renders correctly
- [ ] Request history displays correctly
- [ ] Model usage displays correctly
- [ ] Errors display correctly
- [ ] Cache stats display correctly
- [ ] Filtering works
- [ ] Pagination works

---

## Testing Strategy

### Unit Tests
- [ ] Usage view renders correctly
- [ ] Filters work correctly
- [ ] Pagination works correctly

### Integration Tests
- [ ] Data loads correctly
- [ ] Tab switching works
- [ ] Refresh works

### Manual Testing
- [ ] Can view request history
- [ ] Can filter history
- [ ] Can view model usage
- [ ] Can view errors
- [ ] Can view cache stats
- [ ] Can invalidate cache

---

## Verification Checklist

- [ ] Usage view implemented
- [ ] Request history works
- [ ] Model usage works
- [ ] Error tracking works
- [ ] Cache stats work
- [ ] Filtering works
- [ ] Pagination works
- [ ] No console errors

---

## Next Steps

After completing Phase 7, proceed to **Phase 8: Server Integration** to integrate the dashboard with the server.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
