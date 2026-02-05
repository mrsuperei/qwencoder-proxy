/**
 * ProviderCredentials - Data model for provider credentials
 * 
 * Represents the credentials and tokens associated with a provider,
 * including the provider's settings.
 */

export class ProviderCredentials {
    /**
     * Create a new ProviderCredentials instance
     * @param {string} providerId - The unique provider identifier
     * @param {Token[]} tokens - Array of Token objects for this provider
     * @param {StoreSettings} settings - The provider's store settings
     */
    constructor(providerId, tokens, settings) {
        this.providerId = providerId;
        this.tokens = tokens; // Array of Token objects
        this.settings = settings; // StoreSettings object
    }
}

/**
 * StoreSettings - Data model for provider store settings
 * 
 * Represents the configuration settings for a provider's token store,
 * including selection strategy and error handling.
 */

export class StoreSettings {
    /**
     * Create a new StoreSettings instance
     * @param {string} selectionStrategy - The token selection strategy: 'random', 'round_robin', or 'least_used'
     * @param {number} refreshBufferSec - Buffer time in seconds before token expiry to refresh
     * @param {number} maxErrorCount - Maximum error count before marking token as unhealthy
     */
    constructor(selectionStrategy, refreshBufferSec, maxErrorCount) {
        this.selectionStrategy = selectionStrategy; // 'random', 'round_robin', 'least_used'
        this.refreshBufferSec = refreshBufferSec;
        this.maxErrorCount = maxErrorCount;
    }
}
