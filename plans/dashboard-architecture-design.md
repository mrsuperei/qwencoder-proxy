# QWEncoder Proxy Dashboard - Comprehensive Architecture Design

## Table of Contents
1. [Dashboard Layout & Navigation](#dashboard-layout--navigation)
2. [UI Components Specification](#ui-components-specification)
3. [JavaScript Architecture](#javascript-architecture)
4. [New Features to Implement](#new-features-to-implement)
5. [Data Structures](#data-structures)
6. [Implementation Plan](#implementation-plan)

---

## Dashboard Layout & Navigation

### Overall Page Structure

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Header                                                          │
│ ┌─────────────────────────────┐ ┌─────────────────────────────┐   │
│ Logo & Title              │ │ Server Status | Settings │   │
│ └─────────────────────────────┘ └─────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│ Navigation Tabs                                                 │
│ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐  │
│ Overview│ │Tokens  │ │Proxy   │ │Settings│ │Logs    │  │
│ └────────┘ └────────┘ └────────┘ └────────┘ └────────┘  │
├─────────────────────────────────────────────────────────────────────────┤
│ Main Content Area (Dynamic based on active tab)                    │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────────┐   │
│ │                                                         │   │
│ │  Content varies by selected tab                             │   │
│ │                                                         │   │
│ └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│ Activity Log (Collapsible)                                      │
│ ┌─────────────────────────────────────────────────────────────────┐   │
│ │ Timestamp │ Message                                    │   │
│ │ ─────────┼─────────────────────────────────────────────│   │
│ │ 12:34:56 │ Token added for user@example.com       │   │
│ │ 12:35:02 │ Proxy configured for token abc-123     │   │
│ └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Navigation System

The dashboard uses a tab-based navigation system with the following tabs:

| Tab | Description | Content |
|------|-------------|----------|
| **Overview** | High-level summary of all providers and tokens | Provider cards, summary stats, health overview |
| **Tokens** | Detailed token management per provider | Token list, add/edit/delete tokens, token details |
| **Proxy** | Proxy configuration management | Proxy settings per token, health monitoring |
| **Settings** | Global and provider-specific settings | API base URL, provider settings, refresh intervals |
| **Logs** | Activity log and debugging information | Filterable log entries, export functionality |

### Responsive Design Approach

- **Desktop (> 1200px)**: 3-column layout for provider cards, full-width tables
- **Tablet (768px - 1200px)**: 2-column layout for cards, scrollable tables
- **Mobile (< 768px)**: Single column, stacked navigation, collapsible sections

### Page Sections

1. **Header Section**
   - Logo/Title on the left
   - Server status indicator (online/offline/checking)
   - Settings button (opens settings modal)

2. **Navigation Section**
   - Tab-based navigation
   - Active tab highlighting
   - Badge indicators for counts (e.g., Tokens: 5, Unhealthy: 2)

3. **Main Content Section**
   - Dynamic content based on active tab
   - Scrollable area for large datasets

4. **Activity Log Section**
   - Collapsible panel
   - Auto-scroll to newest entries
   - Filterable by log level (info, warning, error, success)

---

## UI Components Specification

### Component Hierarchy

```
DashboardApplication
├── Header
│   ├── Logo
│   ├── Title
│   ├── ServerStatusIndicator
│   └── SettingsButton
├── NavigationTabs
│   ├── Tab (Overview)
│   ├── Tab (Tokens)
│   ├── Tab (Proxy)
│   ├── Tab (Settings)
│   └── Tab (Logs)
├── ContentArea
│   ├── OverviewTab
│   │   ├── SummaryStats
│   │   ├── ProviderCardsGrid
│   │   │   └── ProviderCard
│   │   │       ├── ProviderHeader
│   │   │       ├── AuthStatus
│   │   │       ├── TokenCount
│   │   │       ├── HealthProgress
│   │   │       ├── EmailList
│   │   │       └── ProviderActions
│   │   └── EmailAddressesSection
│   ├── TokensTab
│   │   ├── FilterBar
│   │   ├── TokensTable
│   │   │   └── TokenRow
│   │   │       ├── TokenInfo
│   │   │       ├── TokenActions
│   │   │       └── ProxyIndicator
│   │   └── TokenDetailPanel (expandable)
│   ├── ProxyTab
│   │   ├── ProxySummary
│   │   ├── ProxyConfigList
│   │   │   └── ProxyConfigCard
│   │   │       ├── ProxyInfo
│   │   │       ├── ProxyForm
│   │   │       ├── HealthIndicator
│   │   │       └── ProxyActions
│   │   └── HealthVisualization
│   │       ├── LatencyChart
│   │       └── HealthTrend
│   ├── SettingsTab
│   │   ├── GlobalSettings
│   │   ├── ProviderSettingsList
│   │   │   └── ProviderSettingsCard
│   │   └── AboutSection
│   └── LogsTab
│       ├── LogFilters
│       ├── LogEntriesList
│       └── LogExportButton
├── ActivityLog
│   ├── LogHeader
│   ├── LogEntries
│   └── LogFooter
└── ToastContainer
    └── ToastNotification
```

### Component Specifications

#### 1. Header Components

**ServerStatusIndicator**
- Visual indicator: Green dot (online), Red dot (offline), Yellow dot (checking)
- Status text: "Online", "Offline", "Checking..."
- Auto-refresh status every 30 seconds

**SettingsButton**
- Opens global settings modal
- Icon: ⚙️

#### 2. Navigation Components

**NavigationTabs**
- Tab-based navigation
- Active tab highlighting with bottom border
- Badge indicators for counts
- Responsive: Hamburger menu on mobile

#### 3. Overview Tab Components

**SummaryStats**
- Cards showing:
  - Total Auth Files
  - Valid Files
  - Unhealthy Files
  - Overall Health Score
- Color-coded values (green/yellow/red)

**ProviderCard**
- Provider icon and name
- OAuth flow type badge (Device Code / Authorization Code / External)
- Auth count display
- Health progress bar
- Expandable email list
- Action buttons:
  - Authorize (for auth code flow)
  - Device Flow (for device code flow)
  - Add Token
  - Settings

**EmailAddressesSection**
- Grouped by provider
- Email cards with health status
- Click to view token details

#### 4. Tokens Tab Components

**FilterBar**
- Provider dropdown filter
- Health status filter (All/Healthy/Unhealthy)
- Search input
- Sort dropdown

**TokensTable**
- Columns: Provider, Email, Health, Health Score, Expiry, Last Used, Created, Actions
- Sortable headers
- Row hover effects
- Click to expand details

**TokenRow**
- Token information display
- Health badge
- Action buttons: Refresh, Delete, Configure Proxy

**TokenDetailPanel**
- Expandable panel showing full token details
- Proxy configuration summary
- Health metrics
- Error history

#### 5. Proxy Tab Components (NEW)

**ProxySummary**
- Stats cards:
  - Total configured proxies
  - Active proxies
  - Unhealthy proxies
  - Average latency

**ProxyConfigCard**
- Token identifier (email)
- Proxy type selector (none/http/https/socks5)
- Host input
- Port input
- Username/password inputs (optional)
- Enable/disable toggle
- Test connection button
- Health indicator with:
  - Health score (0-100%)
  - Last check timestamp
  - Consecutive failures
  - Average latency

**HealthVisualization**
- Latency chart (last 10 measurements)
- Health trend indicator
- Error history

#### 6. Settings Tab Components

**GlobalSettings**
- API Base URL input
- Auto-refresh interval
- Theme selection (optional)

**ProviderSettingsCard**
- Provider name
- Selection strategy (random/round_robin/least_used)
- Refresh buffer (seconds)
- Max error count
- Save/Cancel buttons

#### 7. Logs Tab Components

**LogFilters**
- Log level filter (All/Info/Warning/Error/Success)
- Provider filter
- Search input
- Clear logs button

**LogEntriesList**
- Timestamp column
- Message column with color coding
- Auto-scroll to newest

#### 8. Modal Components

**ProxyConfigModal** (NEW)
- Title: "Configure Proxy for {email}"
- Form fields:
  - Proxy Type (select: none/http/https/socks5)
  - Host (text input, required when type != none)
  - Port (number input, required when type != none, range 1-65535)
  - Username (text input, optional)
  - Password (password input, optional)
  - Enabled (checkbox)
- Validation messages
- Test Connection button
- Save/Cancel buttons

**DeviceCodeModal** (existing)
- User code display (large, copyable)
- Verification URL (clickable link)
- Polling status indicator
- Cancel button

**AddTokenModal** (existing)
- Access token (textarea, required)
- Refresh token (text input, optional)
- Email (email input, optional)
- Expires in (number input, optional)
- Add/Cancel buttons

**SettingsModal** (existing)
- API Base URL (text input)
- Save/Cancel buttons

#### 9. Utility Components

**ToastNotification**
- Auto-dismiss after 5 seconds
- Type: success, error, warning, info
- Icon and message
- Slide-in animation

**LoadingOverlay**
- Full-screen overlay
- Spinner animation
- Message text

**ConfirmDialog**
- Custom confirmation dialog
- Yes/No buttons
- Danger styling for destructive actions

### Data Flow Between Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    LocalStorage (State)                      │
│  - apiBaseUrl                                            │
│  - providers (cached)                                     │
│  - credentials (cached)                                    │
│  - activeTab                                             │
│  - filters                                               │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │
                              │ read/write
                              │
┌─────────────────────────────────────────────────────────────────┐
│                 DashboardApplication                         │
│  ┌──────────────────────────────────────────────────────┐   │
│  │          APIClient                          │   │
│  │  - getProviders()                                 │   │
│  │  - getCredentials()                               │   │
│  │  - getProxyConfig(provider, tokenID)               │   │
│  │  - updateProxyConfig(provider, tokenID, config)     │   │
│  │  - deleteProxyConfig(provider, tokenID)            │   │
│  └──────────────────────────────────────────────────────┘   │
│                           │                                 │
│                           │ API calls                      │
│                           ▼                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              REST API Server                      │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## JavaScript Architecture

### Class Structure

```javascript
// ============================================
// Data Models
// ============================================

class Provider {
    constructor(id, name, flow, scopes, authUrl, tokenUrl, deviceAuthUrl) {
        this.id = id;
        this.name = name;
        this.flow = flow; // 'device_code', 'authorization_code', 'external'
        this.scopes = scopes;
        this.authUrl = authUrl;
        this.tokenUrl = tokenUrl;
        this.deviceAuthUrl = deviceAuthUrl;
    }
}

class ProxyConfig {
    constructor(type, host, port, username, password, enabled) {
        this.type = type; // 'none', 'http', 'https', 'socks5'
        this.host = host;
        this.port = port;
        this.username = username;
        this.password = password;
        this.enabled = enabled;
    }

    validate() {
        // Validation logic
        if (this.type === 'none') return true;
        if (!this.host) return false;
        if (this.port < 1 || this.port > 65535) return false;
        if ((this.username && !this.password) || (!this.username && this.password)) return false;
        return true;
    }
}

class ProxyHealth {
    constructor(lastCheck, isHealthy, lastError, consecutiveFailures, averageLatencyMs, healthScore) {
        this.lastCheck = lastCheck;
        this.isHealthy = isHealthy;
        this.lastError = lastError;
        this.consecutiveFailures = consecutiveFailures;
        this.averageLatencyMs = averageLatencyMs;
        this.healthScore = healthScore; // 0.0 - 1.0
    }
}

class Token {
    constructor(id, accessToken, refreshToken, tokenType, expiryDate, email, resourceUrl, 
                healthy, healthScore, lastUsed, createdAt, errorCount, lastError, 
                proxy, proxyHealthScore) {
        this.id = id;
        this.accessToken = accessToken;
        this.refreshToken = refreshToken;
        this.tokenType = tokenType;
        this.expiryDate = expiryDate;
        this.email = email;
        this.resourceUrl = resourceUrl;
        this.healthy = healthy;
        this.healthScore = healthScore; // 0.0 - 1.0
        this.lastUsed = lastUsed;
        this.createdAt = createdAt;
        this.errorCount = errorCount;
        this.lastError = lastError;
        this.proxy = proxy; // ProxyConfig object or null
        this.proxyHealthScore = proxyHealthScore; // 0.0 - 1.0
    }
}

class ProviderCredentials {
    constructor(providerId, tokens, settings) {
        this.providerId = providerId;
        this.tokens = tokens; // Array of Token objects
        this.settings = settings; // StoreSettings object
    }
}

class StoreSettings {
    constructor(selectionStrategy, refreshBufferSec, maxErrorCount) {
        this.selectionStrategy = selectionStrategy; // 'random', 'round_robin', 'least_used'
        this.refreshBufferSec = refreshBufferSec;
        this.maxErrorCount = maxErrorCount;
    }
}

// ============================================
// API Client
// ============================================

class APIClient {
    constructor(baseUrl = window.location.origin) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
    }

    setBaseUrl(baseUrl) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
    }

    async request(method, path, body = null) {
        const url = `${this.baseUrl}${path}`;
        const options = {
            method,
            headers: { 'Content-Type': 'application/json' },
        };
        if (body) options.body = JSON.stringify(body);

        const response = await fetch(url, options);
        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error?.message || data.message || `HTTP ${response.status}`);
        }
        return data;
    }

    // Provider discovery
    async getProviders() {
        return this.request('GET', '/api/providers');
    }

    async getProviderConfig(providerId) {
        return this.request('GET', `/api/providers/${providerId}/config`);
    }

    // Device code flow
    async startDeviceFlow(providerId) {
        return this.request('POST', '/api/device/start', { provider: providerId });
    }

    async getDeviceStatus(pollId) {
        return this.request('GET', `/api/device/status/${pollId}`);
    }

    // Authorization code flow
    async startAuth(providerId, redirectUri = null) {
        const body = { provider: providerId };
        if (redirectUri) body.redirect_uri = redirectUri;
        return this.request('POST', '/api/auth/start', body);
    }

    // Token management
    async getToken(providerId) {
        return this.request('GET', `/api/token/${providerId}`);
    }

    async refreshToken(providerId) {
        return this.request('POST', `/api/token/${providerId}/refresh`);
    }

    async deleteToken(providerId) {
        return this.request('DELETE', `/api/token/${providerId}`);
    }

    // Credentials management
    async getCredentials() {
        return this.request('GET', '/api/credentials');
    }

    async clearAllCredentials() {
        return this.request('DELETE', '/api/credentials');
    }

    async getProviderCredentials(providerId) {
        return this.request('GET', `/api/credentials/${providerId}`);
    }

    async addToken(providerId, tokenData) {
        return this.request('POST', `/api/credentials/${providerId}`, tokenData);
    }

    async deleteTokenById(providerId, tokenId) {
        return this.request('DELETE', `/api/credentials/${providerId}/${tokenId}`);
    }

    async refreshTokenById(providerId, tokenId) {
        return this.request('POST', `/api/credentials/${providerId}/${tokenId}/refresh`);
    }

    async updateProviderSettings(providerId, settings) {
        return this.request('PUT', `/api/credentials/${providerId}/settings`, settings);
    }

    // Proxy configuration (NEW)
    async getProxyConfig(providerId, tokenId) {
        return this.request('GET', `/api/credentials/${providerId}/${tokenId}/proxy`);
    }

    async updateProxyConfig(providerId, tokenId, proxyConfig) {
        return this.request('PUT', `/api/credentials/${providerId}/${tokenId}/proxy`, proxyConfig);
    }

    async deleteProxyConfig(providerId, tokenId) {
        return this.request('DELETE', `/api/credentials/${providerId}/${tokenId}/proxy`);
    }
}

// ============================================
// State Manager
// ============================================

class StateManager {
    constructor() {
        this.STORAGE_KEY = 'qwencoder_dashboard_state';
        this.state = this.loadState();
    }

    loadState() {
        const stored = localStorage.getItem(this.STORAGE_KEY);
        if (!stored) {
            return this.getInitialState();
        }
        try {
            return { ...this.getInitialState(), ...JSON.parse(stored) };
        } catch (e) {
            return this.getInitialState();
        }
    }

    getInitialState() {
        return {
            apiBaseUrl: window.location.origin,
            activeTab: 'overview',
            providers: [],
            credentials: [],
            filters: {
                provider: 'all',
                health: 'all',
                search: ''
            },
            sort: {
                field: 'createdAt',
                direction: 'desc'
            },
            expandedTokens: new Set(),
            logLevel: 'all'
        };
    }

    saveState() {
        localStorage.setItem(this.STORAGE_KEY, JSON.stringify(this.state));
    }

    // Getters
    getApiBaseUrl() { return this.state.apiBaseUrl; }
    getActiveTab() { return this.state.activeTab; }
    getProviders() { return this.state.providers; }
    getCredentials() { return this.state.credentials; }
    getFilters() { return this.state.filters; }
    getSort() { return this.state.sort; }
    isTokenExpanded(tokenId) { return this.state.expandedTokens.has(tokenId); }

    // Setters
    setApiBaseUrl(url) {
        this.state.apiBaseUrl = url;
        this.saveState();
    }

    setActiveTab(tab) {
        this.state.activeTab = tab;
        this.saveState();
    }

    setProviders(providers) {
        this.state.providers = providers;
        this.saveState();
    }

    setCredentials(credentials) {
        this.state.credentials = credentials;
        this.saveState();
    }

    setFilters(filters) {
        this.state.filters = { ...this.state.filters, ...filters };
        this.saveState();
    }

    setSort(sort) {
        this.state.sort = { ...this.state.sort, ...sort };
        this.saveState();
    }

    toggleTokenExpanded(tokenId) {
        if (this.state.expandedTokens.has(tokenId)) {
            this.state.expandedTokens.delete(tokenId);
        } else {
            this.state.expandedTokens.add(tokenId);
        }
        this.saveState();
    }

    reset() {
        localStorage.removeItem(this.STORAGE_KEY);
        this.state = this.getInitialState();
    }
}

// ============================================
// Event Emitter (for component communication)
// ============================================

class EventEmitter {
    constructor() {
        this.events = {};
    }

    on(event, callback) {
        if (!this.events[event]) {
            this.events[event] = [];
        }
        this.events[event].push(callback);
    }

    off(event, callback) {
        if (!this.events[event]) return;
        this.events[event] = this.events[event].filter(cb => cb !== callback);
    }

    emit(event, data) {
        if (!this.events[event]) return;
        this.events[event].forEach(callback => callback(data));
    }
}

// ============================================
// UI Components
// ============================================

class Component {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.events = new EventEmitter();
    }

    render() {
        // To be implemented by subclasses
    }

    destroy() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }
}

class ToastContainer extends Component {
    constructor() {
        super('toastContainer');
        if (!this.container) {
            this.createContainer();
        }
    }

    createContainer() {
        this.container = document.createElement('div');
        this.container.id = 'toastContainer';
        this.container.className = 'toast-container';
        document.body.appendChild(this.container);
    }

    show(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `<span>${this.getIcon(type)}</span><span>${this.escapeHtml(message)}</span>`;
        this.container.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => this.container.removeChild(toast), 300);
        }, 5000);
    }

    getIcon(type) {
        const icons = { success: '✓', error: '✕', warning: '⚠', info: 'ℹ' };
        return icons[type] || 'ℹ';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

class LoadingOverlay extends Component {
    constructor() {
        super('loadingOverlay');
        if (!this.container) {
            this.createContainer();
        }
    }

    createContainer() {
        this.container = document.createElement('div');
        this.container.id = 'loadingOverlay';
        this.container.className = 'loading-overlay';
        this.container.innerHTML = '<div class="loading-spinner"></div>';
        document.body.appendChild(this.container);
    }

    show() {
        this.container.classList.add('active');
    }

    hide() {
        this.container.classList.remove('active');
    }
}

class Modal extends Component {
    constructor(modalId) {
        super(modalId);
        this.isOpen = false;
    }

    open() {
        this.isOpen = true;
        this.container.classList.add('active');
    }

    close() {
        this.isOpen = false;
        this.container.classList.remove('active');
    }

    toggle() {
        if (this.isOpen) {
            this.close();
        } else {
            this.open();
        }
    }
}

// ============================================
// Main Dashboard Application
// ============================================

class Dashboard {
    constructor() {
        this.api = new APIClient();
        this.state = new StateManager();
        this.events = new EventEmitter();
        this.toast = new ToastContainer();
        this.loading = new LoadingOverlay();
        
        this.providers = [];
        this.credentials = [];
        this.pollingIntervals = new Map();
        
        this.init();
    }

    async init() {
        // Initialize API client with saved base URL
        this.api.setBaseUrl(this.state.getApiBaseUrl());

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
            this.providers = providersData.providers.map(p => new Provider(
                p.id, p.name, p.flow, p.scopes, 
                p.auth_url, p.token_url, p.device_auth_url
            ));
            this.state.setProviders(this.providers);

            // Load credentials
            const credentialsData = await this.api.getCredentials();
            this.credentials = credentialsData.credentials.map(c => new ProviderCredentials(
                c.provider,
                c.tokens.map(t => new Token(
                    t.id, t.access_token, t.refresh_token, t.token_type,
                    t.expiry_date, t.email, t.resource_url, t.healthy,
                    t.health_score, t.last_used, t.created_at,
                    t.error_count, t.last_error, t.proxy, t.proxy_health_score
                )),
                new StoreSettings(
                    c.settings.selection_strategy,
                    c.settings.refresh_buffer_sec,
                    c.settings.max_error_count
                )
            ));
            this.state.setCredentials(this.credentials);

            // Check server status
            await this.checkServerStatus();
        } catch (error) {
            this.toast.show(`Failed to load data: ${error.message}`, 'error');
        } finally {
            this.loading.hide();
        }
    }

    bindEvents() {
        // Navigation tabs
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                const tabName = e.target.dataset.tab;
                this.setActiveTab(tabName);
            });
        });

        // Global refresh
        document.getElementById('refreshBtn')?.addEventListener('click', () => {
            this.loadData();
        });

        // Settings button
        document.getElementById('settingsBtn')?.addEventListener('click', () => {
            this.openSettingsModal();
        });

        // OAuth callback
        window.addEventListener('message', (event) => {
            if (event.data.type === 'oauth_success') {
                this.toast.show('Authorization successful!', 'success');
                this.loadData();
            }
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

        // Render appropriate tab content
        const contentArea = document.getElementById('contentArea');
        switch (activeTab) {
            case 'overview':
                this.renderOverviewTab(contentArea);
                break;
            case 'tokens':
                this.renderTokensTab(contentArea);
                break;
            case 'proxy':
                this.renderProxyTab(contentArea);
                break;
            case 'settings':
                this.renderSettingsTab(contentArea);
                break;
            case 'logs':
                this.renderLogsTab(contentArea);
                break;
        }
    }

    renderOverviewTab(container) {
        // Implementation for overview tab
        // Summary stats, provider cards, email list
    }

    renderTokensTab(container) {
        // Implementation for tokens tab
        // Filter bar, tokens table, token details
    }

    renderProxyTab(container) {
        // Implementation for proxy tab
        // Proxy summary, proxy config list, health visualization
    }

    renderSettingsTab(container) {
        // Implementation for settings tab
        // Global settings, provider settings
    }

    renderLogsTab(container) {
        // Implementation for logs tab
        // Log filters, log entries list
    }

    async startAuth(providerId) {
        // Implementation for authorization code flow
    }

    async startDeviceFlow(providerId) {
        // Implementation for device code flow
    }

    async openProxyConfigModal(providerId, tokenId) {
        // Implementation for proxy configuration modal
    }

    async saveProxyConfig(providerId, tokenId, proxyConfig) {
        // Implementation for saving proxy configuration
    }

    async testProxyConnection(providerId, tokenId) {
        // Implementation for testing proxy connection
    }

    startPeriodicRefresh() {
        // Refresh data every 30 seconds
        setInterval(() => {
            this.checkServerStatus();
        }, 30000);
    }

    async checkServerStatus() {
        try {
            await this.api.getProviders();
            this.updateServerStatus('online');
        } catch (error) {
            this.updateServerStatus('offline');
        }
    }

    updateServerStatus(status) {
        const statusDot = document.getElementById('statusDot');
        const statusText = document.getElementById('statusText');
        
        statusDot.className = `status-dot ${status}`;
        statusText.textContent = status.charAt(0).toUpperCase() + status.slice(1);
    }
}

// Initialize dashboard on DOM ready
document.addEventListener('DOMContentLoaded', () => {
    window.dashboard = new Dashboard();
});
```

### State Management Approach

The dashboard uses a centralized state management pattern:

1. **StateManager Class**: Single source of truth for application state
2. **LocalStorage Persistence**: State automatically persisted to LocalStorage
3. **Event Emitter**: Components can subscribe to state changes
4. **Immutable Updates**: State is never mutated directly, always replaced

### Event Handling Strategy

1. **Event Delegation**: Use event delegation for dynamic elements
2. **Custom Events**: Use EventEmitter for component communication
3. **Browser Events**: Standard DOM events for user interactions
4. **Async Handlers**: All async operations wrapped in try-catch with error handling

---

## New Features to Implement

### 1. Proxy Configuration UI

#### ProxyConfigModal

**Form Fields:**
- Proxy Type (select): none, http, https, socks5
- Host (text input): Required when type != none
- Port (number input): Required when type != none, range 1-65535
- Username (text input): Optional
- Password (password input): Optional
- Enabled (checkbox): Default true

**Validation:**
- Host required when type != none
- Port in valid range
- Username and password both provided or both empty
- Real-time validation feedback

**Actions:**
- Test Connection: Validates proxy settings and tests connectivity
- Save: Commits configuration to server
- Cancel: Discards changes

#### Proxy Config Card (in Proxy Tab)

**Display:**
- Token identifier (email)
- Current proxy type
- Proxy status indicator
- Health score visualization
- Last check timestamp

**Actions:**
- Configure: Opens proxy config modal
- Enable/Disable: Toggles proxy without changing settings
- Remove: Deletes proxy configuration

### 2. Proxy Health Visualization

#### Health Score Indicator

**Visual Elements:**
- Circular progress indicator (0-100%)
- Color coding:
  - Green (80-100%): Healthy
  - Yellow (50-79%): Warning
  - Red (0-49%): Unhealthy
- Animated transitions

**Metrics Display:**
- Health score percentage
- Last check time (relative: "2 minutes ago")
- Consecutive failures count
- Average latency (ms)

#### Latency Chart

**Implementation:**
- SVG-based line chart
- Shows last 10 latency measurements
- X-axis: Time (relative)
- Y-axis: Latency (ms)
- Hover tooltips for data points

**Data Source:**
- Poll health status every 30 seconds
- Store historical data in LocalStorage (max 100 points)

#### Health Trend Indicator

**Visual:**
- Sparkline showing health score over time
- Color-coded based on current health
- Trend arrow (up/down/sideways)

### 3. Enhanced OAuth Flow Handling

#### Authorization Code Flow (Gemini, iFlow)

**Process:**
1. User clicks "Authorize" button
2. Dashboard calls `/api/auth/start` with provider ID
3. Dashboard opens popup with auth URL
4. User completes authorization in popup
5. Provider redirects to callback
6. Callback page sends postMessage to parent
7. Dashboard receives message and refreshes data

**UI Elements:**
- Authorize button
- Loading state during auth
- Success/error feedback
- Popup blocker detection

#### Device Code Flow (Qwen)

**Process:**
1. User clicks "Device Flow" button
2. Dashboard calls `/api/device/start`
3. Dashboard shows device code modal with:
   - User code (large, copyable)
   - Verification URL (clickable)
   - Polling status indicator
4. Dashboard polls `/api/device/status/{poll_id}` every 3 seconds
5. On success, close modal and refresh data

**UI Elements:**
- Device code modal
- Copy to clipboard button
- Polling animation
- Countdown timer (if expires_in provided)

#### External Flow (Kiro)

**Process:**
1. User clicks "Load Credentials" button
2. Dashboard prompts for credential file path or content
3. Dashboard parses and validates credentials
4. Dashboard calls `/api/credentials/{provider}/add` with token data

**UI Elements:**
- File upload input
- Paste text area
- Validation feedback
- Success/error messages

### 4. Per-Token Settings Management

#### Token Settings Panel

**Settings:**
- Selection strategy: random, round_robin, least_used
- Refresh buffer: seconds before expiry
- Max error count: threshold for marking unhealthy

**UI:**
- Expandable panel in token detail view
- Form inputs with validation
- Save/Cancel buttons
- Real-time preview of changes

---

## Data Structures

### Client-Side Data Models

#### Provider Model
```javascript
{
    id: string,           // 'qwen', 'gemini', 'iflow', 'kiro'
    name: string,         // 'Qwen', 'Gemini', 'iFlow', 'Kiro'
    flow: string,         // 'device_code', 'authorization_code', 'external'
    scopes: string[],      // OAuth scopes
    authUrl: string,      // Authorization URL (for auth code flow)
    tokenUrl: string,     // Token endpoint URL
    deviceAuthUrl: string // Device authorization URL (for device code flow)
}
```

#### Token Model
```javascript
{
    id: string,               // UUID
    accessToken: string,
    refreshToken: string,
    tokenType: string,        // 'Bearer'
    expiryDate: number,      // Unix timestamp in milliseconds
    email: string,
    resourceUrl: string,      // For Qwen
    scope: string,           // For Gemini
    apiKey: string,          // For IFlow
    healthy: boolean,
    healthScore: number,      // 0.0 - 1.0
    lastUsed: number,         // Unix timestamp in milliseconds
    createdAt: number,        // Unix timestamp in milliseconds
    errorCount: number,
    lastError: string,
    proxy: ProxyConfig | null,
    proxyHealthScore: number  // 0.0 - 1.0
}
```

#### ProxyConfig Model
```javascript
{
    type: string,      // 'none', 'http', 'https', 'socks5'
    host: string,
    port: number,
    username: string,
    password: string,
    enabled: boolean
}
```

#### ProxyHealth Model
```javascript
{
    lastCheck: number,         // Unix timestamp in milliseconds
    isHealthy: boolean,
    lastError: string,
    consecutiveFailures: number,
    averageLatencyMs: number,
    healthScore: number       // 0.0 - 1.0
}
```

#### ProviderCredentials Model
```javascript
{
    providerId: string,
    tokens: Token[],
    settings: StoreSettings
}
```

#### StoreSettings Model
```javascript
{
    selectionStrategy: string,  // 'random', 'round_robin', 'least_used'
    refreshBufferSec: number,
    maxErrorCount: number
}
```

### API Request/Response Mappings

#### GET /api/providers
**Response:**
```javascript
{
    providers: Provider[]
}
```

#### GET /api/credentials
**Response:**
```javascript
{
    credentials: ProviderCredentials[]
}
```

#### GET /api/credentials/{provider}/{token_id}/proxy
**Response:**
```javascript
{
    token_id: string,
    proxy: ProxyConfig | null,
    health_status: ProxyHealth | null
}
```

#### PUT /api/credentials/{provider}/{token_id}/proxy
**Request:**
```javascript
{
    type: string,
    host: string,
    port: number,
    username: string,
    password: string,
    enabled: boolean
}
```

**Response:**
```javascript
{
    success: boolean,
    message: string
}
```

#### DELETE /api/credentials/{provider}/{token_id}/proxy
**Response:**
```javascript
{
    success: boolean,
    message: string
}
```

### LocalStorage Schema

```javascript
{
    apiBaseUrl: string,
    activeTab: string,
    providers: Provider[],
    credentials: ProviderCredentials[],
    filters: {
        provider: string,
        health: string,
        search: string
    },
    sort: {
        field: string,
        direction: 'asc' | 'desc'
    },
    expandedTokens: string[],
    logLevel: string,
    proxyHistory: {
        [tokenId]: {
            measurements: {
                timestamp: number,
                latencyMs: number
            }[]
        }
    }
}
```

---

## Implementation Plan

### Phase 1: Core Architecture (Priority: Critical)

1. **Create base HTML structure**
   - Header with server status
   - Navigation tabs
   - Content area containers
   - Activity log section
   - Toast container
   - Loading overlay

2. **Implement core JavaScript classes**
   - `APIClient` with all REST API methods
   - `StateManager` with LocalStorage persistence
   - `EventEmitter` for component communication
   - Base `Component` class

3. **Implement utility components**
   - `ToastContainer`
   - `LoadingOverlay`
   - `Modal` base class

### Phase 2: Overview Tab (Priority: High)

4. **Implement Overview Tab**
   - Summary stats cards
   - Provider cards grid
   - Email addresses section
   - Expandable/collapsible sections

5. **Add provider actions**
   - Start authorization flow
   - Start device flow
   - Add token button
   - Provider settings button

### Phase 3: Tokens Tab (Priority: High)

6. **Implement Tokens Tab**
   - Filter bar (provider, health, search)
   - Tokens table with sorting
   - Token detail panel (expandable)
   - Token actions (refresh, delete)

7. **Add token management**
   - Add token modal
   - Refresh token by ID
   - Delete token by ID
   - Token validation

### Phase 4: Proxy Configuration (Priority: Critical)

8. **Implement Proxy Tab**
   - Proxy summary stats
   - Proxy config list
   - Proxy config cards
   - Health visualization

9. **Create proxy configuration modal**
   - Proxy type selector
   - Host/port inputs
   - Username/password inputs
   - Enable/disable toggle
   - Form validation
   - Test connection button

10. **Implement proxy API integration**
    - GET proxy config
    - PUT proxy config
    - DELETE proxy config
    - Error handling

### Phase 5: Proxy Health Visualization (Priority: Medium)

11. **Implement health score indicator**
    - Circular progress indicator
    - Color coding (green/yellow/red)
    - Animated transitions

12. **Implement latency chart**
    - SVG-based line chart
    - Historical data tracking
    - Hover tooltips

13. **Implement health trend**
    - Sparkline visualization
    - Trend indicator
    - Historical data display

### Phase 6: Settings Tab (Priority: Medium)

14. **Implement Settings Tab**
    - Global settings (API base URL)
    - Provider settings list
    - Provider settings cards
    - About section

15. **Add settings management**
    - Update provider settings
    - Save settings to API
    - Settings validation

### Phase 7: Logs Tab (Priority: Low)

16. **Implement Logs Tab**
    - Log filters (level, provider, search)
    - Log entries list
    - Color-coded messages
    - Export functionality

17. **Add log management**
    - Clear logs button
    - Auto-scroll to newest
    - Log level filtering

### Phase 8: OAuth Flow Enhancements (Priority: High)

18. **Enhance authorization code flow**
    - Popup management
    - PostMessage handling
    - Success/error feedback
    - Popup blocker detection

19. **Enhance device code flow**
    - Device code modal
    - Copy to clipboard
    - Polling mechanism
    - Countdown timer

20. **Implement external flow**
    - File upload
    - Text paste
    - Credential validation
    - Success/error messages

### Phase 9: Polish & Testing (Priority: Medium)

21. **Add responsive design**
    - Mobile layouts
    - Tablet layouts
    - Touch interactions

22. **Add accessibility features**
    - ARIA labels
    - Keyboard navigation
    - Screen reader support

23. **Add error handling**
    - Network error recovery
    - Retry mechanisms
    - User-friendly error messages

24. **Add animations**
    - Smooth transitions
    - Loading states
    - Success feedback

### Phase 10: Documentation (Priority: Low)

25. **Create user documentation**
    - Getting started guide
    - OAuth flow instructions
    - Proxy configuration guide
    - Troubleshooting section

---

## Appendix: CSS Architecture

### CSS Variables (Theming)

```css
:root {
    /* Colors */
    --primary-color: #2563eb;
    --primary-hover: #1d4ed8;
    --success-color: #10b981;
    --warning-color: #f59e0b;
    --error-color: #ef4444;
    
    /* Backgrounds */
    --bg-color: #f8fafc;
    --card-bg: #ffffff;
    
    /* Text */
    --text-primary: #1e293b;
    --text-secondary: #64748b;
    
    /* Borders */
    --border-color: #e2e8f0;
    
    /* Shadows */
    --shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1);
    --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
}
```

### Component-Based CSS

Each component should have its own CSS block with:
- Component-specific variables
- Component-specific styles
- Responsive breakpoints
- Dark mode support (optional)

---

## Summary

This architecture design provides:

1. **Complete REST API Coverage**: All endpoints supported
2. **Proxy Configuration**: Full UI for per-token proxy settings
3. **Health Visualization**: Comprehensive proxy health monitoring
4. **OAuth Flow Support**: All provider flows implemented
5. **Responsive Design**: Works on all screen sizes
6. **Pure HTML/JS**: No external dependencies
7. **LocalStorage Persistence**: State survives page reloads
8. **SPA Architecture**: Fast, seamless navigation

The implementation is organized into 10 phases, with critical features (proxy configuration) prioritized first. Each phase builds upon the previous one, ensuring a systematic and testable development process.
