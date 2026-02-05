/**
 * API Endpoints - Centralized API endpoint definitions
 * 
 * Provides a single source of truth for all API endpoint URLs
 * and helper functions for building dynamic endpoints.
 */

export const ENDPOINTS = {
    // Provider discovery
    GET_PROVIDERS: '/api/providers',
    GET_PROVIDER_CONFIG: (providerId) => `/api/providers/${providerId}/config`,

    // Device code flow
    START_DEVICE_FLOW: '/api/device/start',
    GET_DEVICE_STATUS: (pollId) => `/api/device/status/${pollId}`,

    // Authorization code flow
    START_AUTH: '/api/auth/start',

    // Token management
    GET_TOKEN: (providerId) => `/api/token/${providerId}`,
    REFRESH_TOKEN: (providerId) => `/api/token/${providerId}/refresh`,
    DELETE_TOKEN: (providerId) => `/api/token/${providerId}`,

    // Credentials management
    GET_CREDENTIALS: '/api/credentials',
    CLEAR_ALL_CREDENTIALS: '/api/credentials',
    GET_PROVIDER_CREDENTIALS: (providerId) => `/api/credentials/${providerId}`,
    ADD_TOKEN: (providerId) => `/api/credentials/${providerId}`,
    DELETE_TOKEN_BY_ID: (providerId, tokenId) => `/api/credentials/${providerId}/${tokenId}`,
    REFRESH_TOKEN_BY_ID: (providerId, tokenId) => `/api/credentials/${providerId}/${tokenId}/refresh`,
    UPDATE_PROVIDER_SETTINGS: (providerId) => `/api/credentials/${providerId}/settings`,

    // Proxy configuration
    GET_PROXY_CONFIG: (providerId, tokenId) => `/api/credentials/${providerId}/${tokenId}/proxy`,
    UPDATE_PROXY_CONFIG: (providerId, tokenId) => `/api/credentials/${providerId}/${tokenId}/proxy`,
    DELETE_PROXY_CONFIG: (providerId, tokenId) => `/api/credentials/${providerId}/${tokenId}/proxy`
};
