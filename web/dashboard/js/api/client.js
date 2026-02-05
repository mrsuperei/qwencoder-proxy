/**
 * APIClient - Main HTTP client for API communication
 * 
 * Provides methods for all API interactions with the backend server.
 * Handles request/response formatting and error handling.
 */

import { ENDPOINTS } from './endpoints.js';

export class APIClient {
    /**
     * Create a new APIClient instance
     * @param {string} baseUrl - The base URL for the API (defaults to current origin)
     */
    constructor(baseUrl = window.location.origin) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
    }

    /**
     * Set the base URL for the API
     * @param {string} baseUrl - The new base URL
     */
    setBaseUrl(baseUrl) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
    }

    /**
     * Make an HTTP request to the API
     * @param {string} method - The HTTP method (GET, POST, PUT, DELETE)
     * @param {string} path - The API endpoint path
     * @param {Object} body - Optional request body
     * @returns {Promise<Object>} The parsed JSON response
     * @throws {Error} If the request fails
     */
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

    // ============================================
    // Provider Discovery
    // ============================================

    /**
     * Get all available providers
     * @returns {Promise<Object>} Object containing providers array
     */
    async getProviders() {
        return this.request('GET', ENDPOINTS.GET_PROVIDERS);
    }

    /**
     * Get configuration for a specific provider
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} The provider configuration
     */
    async getProviderConfig(providerId) {
        return this.request('GET', ENDPOINTS.GET_PROVIDER_CONFIG(providerId));
    }

    // ============================================
    // Device Code Flow
    // ============================================

    /**
     * Start the device code authorization flow
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} Object containing user_code, verification_uri, and poll_id
     */
    async startDeviceFlow(providerId) {
        return this.request('POST', ENDPOINTS.START_DEVICE_FLOW, { provider: providerId });
    }

    /**
     * Get the status of a device code authorization
     * @param {string} pollId - The polling ID from the device flow start
     * @returns {Promise<Object>} Object containing status and error (if any)
     */
    async getDeviceStatus(pollId) {
        return this.request('GET', ENDPOINTS.GET_DEVICE_STATUS(pollId));
    }

    // ============================================
    // Authorization Code Flow
    // ============================================

    /**
     * Start the authorization code flow
     * @param {string} providerId - The provider identifier
     * @param {string} redirectUri - Optional redirect URI for the OAuth flow
     * @returns {Promise<Object>} Object containing auth_url
     */
    async startAuth(providerId, redirectUri = null) {
        const body = { provider: providerId };
        if (redirectUri) body.redirect_uri = redirectUri;
        return this.request('POST', ENDPOINTS.START_AUTH, body);
    }

    // ============================================
    // Token Management
    // ============================================

    /**
     * Get the token for a provider
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} The token data
     */
    async getToken(providerId) {
        return this.request('GET', ENDPOINTS.GET_TOKEN(providerId));
    }

    /**
     * Refresh the token for a provider
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} The refreshed token data
     */
    async refreshToken(providerId) {
        return this.request('POST', ENDPOINTS.REFRESH_TOKEN(providerId));
    }

    /**
     * Delete the token for a provider
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} Confirmation of deletion
     */
    async deleteToken(providerId) {
        return this.request('DELETE', ENDPOINTS.DELETE_TOKEN(providerId));
    }

    // ============================================
    // Credentials Management
    // ============================================

    /**
     * Get all credentials
     * @returns {Promise<Object>} Object containing credentials array
     */
    async getCredentials() {
        return this.request('GET', ENDPOINTS.GET_CREDENTIALS);
    }

    /**
     * Clear all credentials
     * @returns {Promise<Object>} Confirmation of deletion
     */
    async clearAllCredentials() {
        return this.request('DELETE', ENDPOINTS.CLEAR_ALL_CREDENTIALS);
    }

    /**
     * Get credentials for a specific provider
     * @param {string} providerId - The provider identifier
     * @returns {Promise<Object>} The provider's credentials
     */
    async getProviderCredentials(providerId) {
        return this.request('GET', ENDPOINTS.GET_PROVIDER_CREDENTIALS(providerId));
    }

    /**
     * Add a token to a provider
     * @param {string} providerId - The provider identifier
     * @param {Object} tokenData - The token data to add
     * @returns {Promise<Object>} Confirmation of addition
     */
    async addToken(providerId, tokenData) {
        return this.request('POST', ENDPOINTS.ADD_TOKEN(providerId), tokenData);
    }

    /**
     * Delete a specific token by ID
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} Confirmation of deletion
     */
    async deleteTokenById(providerId, tokenId) {
        return this.request('DELETE', ENDPOINTS.DELETE_TOKEN_BY_ID(providerId, tokenId));
    }

    /**
     * Refresh a specific token by ID
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} The refreshed token data
     */
    async refreshTokenById(providerId, tokenId) {
        return this.request('POST', ENDPOINTS.REFRESH_TOKEN_BY_ID(providerId, tokenId));
    }

    /**
     * Update provider settings
     * @param {string} providerId - The provider identifier
     * @param {Object} settings - The settings to update
     * @returns {Promise<Object>} Confirmation of update
     */
    async updateProviderSettings(providerId, settings) {
        return this.request('PUT', ENDPOINTS.UPDATE_PROVIDER_SETTINGS(providerId), settings);
    }

    // ============================================
    // Proxy Configuration
    // ============================================

    /**
     * Get the proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} The proxy configuration
     */
    async getProxyConfig(providerId, tokenId) {
        return this.request('GET', ENDPOINTS.GET_PROXY_CONFIG(providerId, tokenId));
    }

    /**
     * Update the proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @param {Object} proxyConfig - The proxy configuration to set
     * @returns {Promise<Object>} Confirmation of update
     */
    async updateProxyConfig(providerId, tokenId, proxyConfig) {
        return this.request('PUT', ENDPOINTS.UPDATE_PROXY_CONFIG(providerId, tokenId), proxyConfig);
    }

    /**
     * Delete the proxy configuration for a token
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} Confirmation of deletion
     */
    async deleteProxyConfig(providerId, tokenId) {
        return this.request('DELETE', ENDPOINTS.DELETE_PROXY_CONFIG(providerId, tokenId));
    }
}
