/**
 * StateManager - Application state management
 * 
 * Manages application state with localStorage persistence.
 * Provides getters and setters for all state properties.
 */

export class StateManager {
    constructor() {
        this.STORAGE_KEY = 'qwencoder_dashboard_state';
        this.state = this.loadState();
    }

    /**
     * Load state from localStorage or return initial state
     * @returns {Object} The application state
     */
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

    /**
     * Get the initial/default state
     * @returns {Object} The default application state
     */
    getInitialState() {
        return {
            apiBaseUrl: window.location.origin,
            activeTab: 'overview',
            autoRefreshInterval: 30,
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
            logLevel: 'all',
            logs: []
        };
    }

    /**
     * Save current state to localStorage
     */
    saveState() {
        localStorage.setItem(this.STORAGE_KEY, JSON.stringify(this.state));
    }

    // ============================================
    // Getters
    // ============================================

    getApiBaseUrl() { return this.state.apiBaseUrl; }
    getActiveTab() { return this.state.activeTab; }
    getAutoRefreshInterval() { return this.state.autoRefreshInterval; }
    getProviders() { return this.state.providers; }
    getCredentials() { return this.state.credentials; }
    getFilters() { return this.state.filters; }
    getSort() { return this.state.sort; }
    getLogLevel() { return this.state.logLevel; }
    getLogs() { return this.state.logs; }

    // ============================================
    // Setters
    // ============================================

    setApiBaseUrl(url) {
        this.state.apiBaseUrl = url;
        this.saveState();
    }

    setActiveTab(tab) {
        this.state.activeTab = tab;
        this.saveState();
    }

    setAutoRefreshInterval(interval) {
        this.state.autoRefreshInterval = interval;
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

    setLogLevel(level) {
        this.state.logLevel = level;
        this.saveState();
    }

    /**
     * Add a log entry to the logs array
     * @param {Object} log - The log entry to add
     */
    addLog(log) {
        this.state.logs.unshift(log);
        if (this.state.logs.length > 500) {
            this.state.logs = this.state.logs.slice(0, 500);
        }
        this.saveState();
    }

    /**
     * Clear all log entries
     */
    clearLogs() {
        this.state.logs = [];
        this.saveState();
    }

    /**
     * Reset state to initial values and clear localStorage
     */
    reset() {
        localStorage.removeItem(this.STORAGE_KEY);
        this.state = this.getInitialState();
    }
}
