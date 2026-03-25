# Phase 6: Rate Limit Management

**Priority:** HIGH  
**Estimated Time:** 1 day  
**Complexity:** Medium  
**Files to Create:** 1  
**Files to Modify:** 1

---

## Executive Summary

This phase implements rate limit configuration management for providers. Users can configure rate limits per provider (requests per day, requests per minute, tokens per minute) and monitor usage against those limits.

---

## Problem Description

Rate limiting is critical for managing API quotas and preventing abuse. Users need a way to configure and monitor rate limits per provider.

### Requirements

- View rate limit configuration for all providers
- Configure rate limits per provider (RPD, RPM, TPM)
- Enable/disable rate limiting per provider
- View current usage against limits
- Reset usage statistics

---

## Solution Architecture

### Design Principles

1. **Per-Provider Configuration:** Each provider has independent rate limits
2. **Visual Feedback:** Show usage percentage visually
3. **Easy Configuration:** Simple form for setting limits
4. **Real-time Updates:** Usage updates in real-time
5. **Reset Capability:** Ability to reset usage statistics

### Component Overview

```
js/views/
└── RateLimits.js      # Rate limit management view
```

---

## Implementation Plan

### Step 1: Implement Rate Limits View

Create comprehensive rate limit management interface:

```javascript
/**
 * Rate Limits View
 * 
 * Displays rate limit configurations for providers and allows
 * configuring and monitoring rate limits.
 */

import { View } from '../components/View.js';
import { Modal } from '../components/Modal.js';
import { Form } from '../components/Form.js';
import { toast } from '../components/Toast.js';
import { selectors, actions } from '../state/store.js';

export default class RateLimitsView extends View {
    async render(container) {
        await super.render(container);

        this.subscribe('rateLimitConfigs', () => this.render());
        this.subscribe('usage', () => this.render());

        this.renderContent();
    }

    renderContent() {
        const providers = this.store.get('providers');
        const rateLimitConfigs = this.store.get('rateLimitConfigs');
        const usage = this.store.get('usage');

        this.container.innerHTML = `
            <div class="view-header">
                <h1>Rate Limits</h1>
            </div>

            <div class="rate-limits-grid">
                ${providers.map(provider => {
                    const config = rateLimitConfigs[provider.id] || {};
                    const providerUsage = usage[provider.id] || {};
                    return this.renderProviderCard(provider, config, providerUsage);
                }).join('')}
            </div>
        `;
    }

    renderProviderCard(provider, config, providerUsage) {
        const enabled = config.enabled !== false;
        const requestsToday = providerUsage.requests_today || 0;
        const requestsPerMinute = providerUsage.requests_in_minute || 0;
        const tokensPerMinute = providerUsage.tokens_in_minute || 0;

        const rpdLimit = config.requests_per_day || 0;
        const rpmLimit = config.requests_per_minute || 0;
        const tpmLimit = config.tokens_per_minute || 0;

        const rpdPercentage = rpdLimit > 0 ? (requestsToday / rpdLimit) * 100 : 0;
        const rpmPercentage = rpmLimit > 0 ? (requestsPerMinute / rpmLimit) * 100 : 0;
        const tpmPercentage = tpmLimit > 0 ? (tokensPerMinute / tpmLimit) * 100 : 0;

        const getPercentageClass = (percentage) => {
            if (percentage >= 90) return 'danger';
            if (percentage >= 70) return 'warning';
            return 'success';
        };

        return `
            <div class="card rate-limit-card ${enabled ? 'enabled' : 'disabled'}">
                <div class="card-header">
                    <h3 class="card-title">${provider.name}</h3>
                    <div class="card-actions">
                        <label class="toggle-switch">
                            <input type="checkbox" ${enabled ? 'checked' : ''} data-action="toggle" data-provider="${provider.id}">
                            <span class="toggle-slider"></span>
                        </label>
                        <button class="btn-icon" data-action="configure" data-provider="${provider.id}" title="Configure">
                            ⚙️
                        </button>
                    </div>
                </div>

                <div class="card-body">
                    ${enabled ? `
                        <div class="limit-metrics">
                            <div class="metric">
                                <div class="metric-header">
                                    <span class="metric-label">Requests/Day</span>
                                    <span class="metric-limit">${requestsToday} / ${rpdLimit}</span>
                                </div>
                                <div class="metric-bar">
                                    <div class="metric-fill ${getPercentageClass(rpdPercentage)}" style="width: ${Math.min(rpdPercentage, 100)}%"></div>
                                </div>
                                <div class="metric-percentage">${rpdPercentage.toFixed(1)}%</div>
                            </div>

                            <div class="metric">
                                <div class="metric-header">
                                    <span class="metric-label">Requests/Min</span>
                                    <span class="metric-limit">${requestsPerMinute} / ${rpmLimit}</span>
                                </div>
                                <div class="metric-bar">
                                    <div class="metric-fill ${getPercentageClass(rpmPercentage)}" style="width: ${Math.min(rpmPercentage, 100)}%"></div>
                                </div>
                                <div class="metric-percentage">${rpmPercentage.toFixed(1)}%</div>
                            </div>

                            <div class="metric">
                                <div class="metric-header">
                                    <span class="metric-label">Tokens/Min</span>
                                    <span class="metric-limit">${tokensPerMinute} / ${tpmLimit}</span>
                                </div>
                                <div class="metric-bar">
                                    <div class="metric-fill ${getPercentageClass(tpmPercentage)}" style="width: ${Math.min(tpmPercentage, 100)}%"></div>
                                </div>
                                <div class="metric-percentage">${tpmPercentage.toFixed(1)}%</div>
                            </div>
                        </div>
                    ` : `
                        <div class="disabled-message">
                            <p>Rate limiting is disabled for this provider</p>
                            <p class="help-text">Enable rate limiting to configure limits</p>
                        </div>
                    `}
                </div>

                <div class="card-footer">
                    <button class="btn btn-secondary btn-sm" data-action="reset-usage" data-provider="${provider.id}">
                        Reset Usage
                    </button>
                </div>
            </div>
        `;
    }

    onMount() {
        super.onMount();

        this.container.addEventListener('click', (e) => {
            const action = e.target.closest('[data-action]');
            if (!action) return;

            const actionType = action.dataset.action;
            const providerId = action.dataset.provider;

            switch (actionType) {
                case 'toggle':
                    this.toggleRateLimit(providerId);
                    break;
                case 'configure':
                    this.showConfigureModal(providerId);
                    break;
                case 'reset-usage':
                    this.resetUsage(providerId);
                    break;
            }
        });
    }

    async toggleRateLimit(providerId) {
        const configs = this.store.get('rateLimitConfigs');
        const currentConfig = configs[providerId] || {};
        const newEnabled = currentConfig.enabled !== false;

        const config = {
            provider_id: providerId,
            requests_per_day: currentConfig.requests_per_day || 15000,
            requests_per_minute: currentConfig.requests_per_minute || 60,
            tokens_per_minute: currentConfig.tokens_per_minute || 32000,
            enabled: !newEnabled
        };

        try {
            await this.api.updateRateLimitConfig(providerId, config);
            toast.success(`Rate limiting ${newEnabled ? 'disabled' : 'enabled'} for provider`);
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to toggle rate limiting: ${error.message}`);
        }
    }

    showConfigureModal(providerId) {
        const providers = this.store.get('providers');
        const rateLimitConfigs = this.store.get('rateLimitConfigs');
        const provider = providers.find(p => p.id === providerId);
        const currentConfig = rateLimitConfigs[providerId] || {};

        const modal = new Modal({
            title: `Configure Rate Limits - ${provider.name}`,
            size: 'md',
            content: '',
            footer: `
                <button class="btn btn-secondary modal-cancel">Cancel</button>
                <button class="btn btn-primary modal-save">Save Configuration</button>
            `
        });

        modal.open();

        // Create form
        const formContainer = modal.element.querySelector('.modal-body');
        const form = new Form({
            fields: [
                {
                    name: 'enabled',
                    label: 'Enable Rate Limiting',
                    type: 'checkbox',
                    checked: currentConfig.enabled !== false
                },
                {
                    name: 'requests_per_day',
                    label: 'Requests Per Day',
                    type: 'number',
                    min: 0,
                    max: 1000000,
                    value: currentConfig.requests_per_day || 15000,
                    helpText: 'Maximum requests allowed per 24-hour period'
                },
                {
                    name: 'requests_per_minute',
                    label: 'Requests Per Minute',
                    type: 'number',
                    min: 0,
                    max: 10000,
                    value: currentConfig.requests_per_minute || 60,
                    helpText: 'Maximum requests allowed per minute'
                },
                {
                    name: 'tokens_per_minute',
                    label: 'Tokens Per Minute',
                    type: 'number',
                    min: 0,
                    max: 1000000,
                    value: currentConfig.tokens_per_minute || 32000,
                    helpText: 'Maximum tokens allowed per minute'
                }
            ],
            onSubmit: async (values) => {
                const config = {
                    provider_id: providerId,
                    requests_per_day: parseInt(values.requests_per_day) || 0,
                    requests_per_minute: parseInt(values.requests_per_minute) || 0,
                    tokens_per_minute: parseInt(values.tokens_per_minute) || 0,
                    enabled: values.enabled
                };

                try {
                    await this.api.updateRateLimitConfig(providerId, config);
                    modal.close();
                    toast.success('Rate limit configuration saved');
                    await this.app.loadInitialData();
                } catch (error) {
                    toast.error(`Failed to save configuration: ${error.message}`);
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

    async resetUsage(providerId) {
        const confirmed = await confirmModal({
            title: 'Reset Usage',
            message: 'Are you sure you want to reset usage statistics for this provider?',
            size: 'sm'
        });

        if (!confirmed) return;

        try {
            await this.api.resetProviderUsage(providerId);
            toast.success('Usage statistics reset');
            await this.app.loadInitialData();
        } catch (error) {
            toast.error(`Failed to reset usage: ${error.message}`);
        }
    }
}
```

**Verification:**
- [ ] Rate limits view renders correctly
- [ ] Provider cards display correctly
- [ ] Usage percentages calculate correctly
- [ ] Toggle works
- [ ] Configuration modal works
- [ ] Usage reset works
- [ ] Visual indicators work

---

## Testing Strategy

### Unit Tests
- [ ] Rate limits view renders correctly
- [ ] Form validation works
- [ ] Percentage calculations correct

### Integration Tests
- [ ] Configuration saves correctly
- [ ] Usage reset works
- [ ] Toggle updates state

### Manual Testing
- [ ] Can enable/disable rate limiting
- [ ] Can configure all limits
- [ ] Can reset usage
- [ ] Visual feedback accurate

---

## Verification Checklist

- [ ] Rate limits view implemented
- [ ] Provider cards display correctly
- [ ] Usage metrics display correctly
- [ ] Visual indicators work
- [ ] Configuration modal works
- [ ] Usage reset works
- [ ] No console errors

---

## Next Steps

After completing Phase 6, proceed to **Phase 7: Usage Tracking & Analytics** to implement usage tracking and analytics features.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
