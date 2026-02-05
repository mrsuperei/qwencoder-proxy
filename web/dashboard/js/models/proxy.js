/**
 * ProxyConfig - Data model for proxy configuration
 * 
 * Represents a proxy configuration with connection details
 * and validation capabilities.
 */

export class ProxyConfig {
    /**
     * Create a new ProxyConfig instance
     * @param {string} type - The proxy type: 'none', 'http', 'https', or 'socks5'
     * @param {string} host - The proxy host address
     * @param {number} port - The proxy port number
     * @param {string} username - Optional username for authentication
     * @param {string} password - Optional password for authentication
     * @param {boolean} enabled - Whether the proxy is enabled
     */
    constructor(type, host, port, username, password, enabled) {
        this.type = type; // 'none', 'http', 'https', 'socks5'
        this.host = host;
        this.port = port;
        this.username = username;
        this.password = password;
        this.enabled = enabled;
    }

    /**
     * Validate the proxy configuration
     * @returns {{valid: boolean, message?: string}} Validation result
     */
    validate() {
        if (this.type === 'none') return { valid: true };
        if (!this.host || !this.host.trim()) {
            return { valid: false, message: 'Host is required' };
        }
        if (!this.port || this.port < 1 || this.port > 65535) {
            return { valid: false, message: 'Port must be between 1 and 65535' };
        }
        if ((this.username && !this.password) || (!this.username && this.password)) {
            return { valid: false, message: 'Username and password must both be provided or both empty' };
        }
        return { valid: true };
    }
}

/**
 * ProxyHealth - Data model for proxy health tracking
 * 
 * Represents the health status and metrics of a proxy connection.
 */

export class ProxyHealth {
    /**
     * Create a new ProxyHealth instance
     * @param {Date} lastCheck - Timestamp of the last health check
     * @param {boolean} isHealthy - Whether the proxy is currently healthy
     * @param {string} lastError - The last error message (if any)
     * @param {number} consecutiveFailures - Number of consecutive failed checks
     * @param {number} averageLatencyMs - Average latency in milliseconds
     * @param {number} healthScore - Health score between 0.0 and 1.0
     */
    constructor(lastCheck, isHealthy, lastError, consecutiveFailures, averageLatencyMs, healthScore) {
        this.lastCheck = lastCheck;
        this.isHealthy = isHealthy;
        this.lastError = lastError;
        this.consecutiveFailures = consecutiveFailures;
        this.averageLatencyMs = averageLatencyMs;
        this.healthScore = healthScore; // 0.0 - 1.0
    }
}
