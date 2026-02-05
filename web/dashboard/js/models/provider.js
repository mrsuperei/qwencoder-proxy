/**
 * Provider - Data model for OAuth providers
 * 
 * Represents an OAuth provider with its configuration including
 * the flow type, scopes, and endpoint URLs.
 */

export class Provider {
    /**
     * Create a new Provider instance
     * @param {string} id - The unique provider identifier (e.g., 'qwen', 'gemini')
     * @param {string} name - The display name of the provider
     * @param {string} flow - The OAuth flow type: 'device_code', 'authorization_code', or 'external'
     * @param {string[]} scopes - Array of OAuth scopes required by this provider
     * @param {string} authUrl - The authorization URL for the provider
     * @param {string} tokenUrl - The token endpoint URL for the provider
     * @param {string} deviceAuthUrl - The device authorization URL (for device code flow)
     */
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
