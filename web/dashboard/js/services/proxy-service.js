/**
 * ProxyService - Proxy management operations
 * 
 * Provides methods for managing proxy configurations
 * including validation, CRUD operations, and health tracking.
 */

export class ProxyService {
    /**
     * Create a new ProxyService instance
     * @param {APIClient} api - The API client instance
     * @param {Function} log - The logging function
     * @param {ToastContainer} toast - The toast notification component
     */
    constructor(api, log, toast) {
        this.api = api;
        this.log = log;
        this.toast = toast;
    }

    /**
     * Get proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} The proxy configuration
     * @throws {Error} If request fails
     */
    async getProxyConfig(providerId, tokenId) {
        try {
            return await this.api.getProxyConfig(providerId, tokenId);
        } catch (error) {
            this.log(`Failed to load proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to load proxy configuration', 'error');
            throw error;
        }
    }

    /**
     * Update proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @param {Object} proxyConfig - The proxy configuration to set
     * @returns {Promise<Object>} Confirmation of update
     * @throws {Error} If validation fails or request fails
     */
    async updateProxyConfig(providerId, tokenId, proxyConfig) {
        // Validate proxy configuration
        const validation = this.validateProxyConfig(proxyConfig);
        if (!validation.valid) {
            throw new Error(validation.message);
        }

        try {
            this.log('Saving proxy configuration...', 'info');
            await this.api.updateProxyConfig(providerId, tokenId, proxyConfig);
            this.log('Proxy configuration saved successfully', 'success');
            this.toast.show('Proxy configuration saved', 'success');
        } catch (error) {
            this.log(`Failed to save proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to save proxy configuration', 'error');
            throw error;
        }
    }

    /**
     * Delete proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} Confirmation of deletion
     * @throws {Error} If request fails
     */
    async deleteProxyConfig(providerId, tokenId) {
        try {
            this.log(`Deleting proxy config for token ${tokenId}`, 'info');
            await this.api.deleteProxyConfig(providerId, tokenId);
            this.log('Proxy configuration removed', 'success');
            this.toast.show('Proxy configuration removed', 'success');
        } catch (error) {
            this.log(`Failed to delete proxy config: ${error.message}`, 'error');
            this.toast.show('Failed to remove proxy configuration', 'error');
            throw error;
        }
    }

    /**
     * Toggle proxy enabled state for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} Confirmation of update
     * @throws {Error} If request fails
     */
    async toggleProxyEnabled(providerId, tokenId) {
        try {
            this.log(`Toggling proxy for token ${tokenId}`, 'info');
            const data = await this.api.getProxyConfig(providerId, tokenId);
            const proxy = data.proxy || { type: 'none', host: '', port: 0, username: '', password: '', enabled: false };
            proxy.enabled = !proxy.enabled;
            await this.api.updateProxyConfig(providerId, tokenId, proxy);
            this.log(`Proxy ${proxy.enabled ? 'enabled' : 'disabled'} for token ${tokenId}`, 'success');
            this.toast.show(`Proxy ${proxy.enabled ? 'enabled' : 'disabled'}`, 'success');
        } catch (error) {
            this.log(`Failed to toggle proxy: ${error.message}`, 'error');
            this.toast.show('Failed to toggle proxy', 'error');
            throw error;
        }
    }

    /**
     * Validate proxy configuration
     * @param {Object} config - The proxy configuration to validate
     * @returns {{valid: boolean, message?: string}} Validation result
     */
    validateProxyConfig(config) {
        if (config.type === 'none') return { valid: true };
        if (!config.host || !config.host.trim()) {
            return { valid: false, message: 'Host is required' };
        }
        if (!config.port || config.port < 1 || config.port > 65535) {
            return { valid: false, message: 'Port must be between 1 and 65535' };
        }
        if ((config.username && !config.password) || (!config.username && config.password)) {
            return { valid: false, message: 'Username and password must both be provided or both empty' };
        }
        return { valid: true };
    }
}
