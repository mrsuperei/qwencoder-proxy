/**
 * TokenService - Token management operations
 * 
 * Provides methods for managing OAuth tokens
 * including adding, refreshing, and deleting tokens.
 */

export class TokenService {
    /**
     * Create a new TokenService instance
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
     * Add a new token to a provider
     * @param {string} providerId - The provider identifier
     * @param {Object} tokenData - The token data to add
     * @returns {Promise<Object>} Confirmation of addition
     * @throws {Error} If request fails
     */
    async addToken(providerId, tokenData) {
        try {
            this.log('Adding token...', 'info');
            await this.api.addToken(providerId, tokenData);
            this.log('Token added successfully', 'success');
            this.toast.show('Token added successfully', 'success');
        } catch (error) {
            this.log(`Failed to add token: ${error.message}`, 'error');
            this.toast.show('Failed to add token', 'error');
            throw error;
        }
    }

    /**
     * Refresh a specific token by ID
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} The refreshed token data
     * @throws {Error} If request fails
     */
    async refreshTokenById(providerId, tokenId) {
        try {
            this.log(`Refreshing token ${tokenId} for ${providerId}`, 'info');
            await this.api.refreshTokenById(providerId, tokenId);
            this.log(`Token ${tokenId} refreshed successfully`, 'success');
            this.toast.show('Token refreshed successfully', 'success');
        } catch (error) {
            this.log(`Failed to refresh token: ${error.message}`, 'error');
            this.toast.show('Failed to refresh token', 'error');
            throw error;
        }
    }

    /**
     * Delete a specific token by ID
     * @param {string} providerId - The provider identifier
     * @param {string} tokenId - The token identifier
     * @returns {Promise<Object>} Confirmation of deletion
     * @throws {Error} If request fails
     */
    async deleteTokenById(providerId, tokenId) {
        try {
            this.log(`Deleting token ${tokenId} for ${providerId}`, 'info');
            await this.api.deleteTokenById(providerId, tokenId);
            this.log(`Token ${tokenId} deleted successfully`, 'success');
            this.toast.show('Token deleted successfully', 'success');
        } catch (error) {
            this.log(`Failed to delete token: ${error.message}`, 'error');
            this.toast.show('Failed to delete token', 'error');
            throw error;
        }
    }
}
