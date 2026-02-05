/**
 * Token - Data model for OAuth tokens
 * 
 * Represents an OAuth token with its metadata including
 * expiry, health status, and proxy configuration.
 */

export class Token {
    /**
     * Create a new Token instance
     * @param {string} id - The unique token identifier
     * @param {string} accessToken - The OAuth access token
     * @param {string} refreshToken - The OAuth refresh token (optional)
     * @param {string} tokenType - The type of token (e.g., 'Bearer')
     * @param {Date|string} expiryDate - When the token expires
     * @param {string} email - The email associated with the token
     * @param {string} resourceUrl - The resource URL for the token
     * @param {boolean} healthy - Whether the token is healthy
     * @param {number} healthScore - Health score between 0.0 and 1.0
     * @param {Date|string} lastUsed - Timestamp of last token usage
     * @param {Date|string} createdAt - Timestamp of token creation
     * @param {number} errorCount - Number of errors encountered
     * @param {string} lastError - The last error message (if any)
     * @param {ProxyConfig} proxy - The proxy configuration for this token
     * @param {number} proxyHealthScore - Proxy health score between 0.0 and 1.0
     */
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
