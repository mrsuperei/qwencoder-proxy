// QWEncoder Proxy Shared JavaScript Utilities

// API Base URL
const API_BASE = '/api';

// API Client class for making HTTP requests
class APIClient {
    constructor(baseURL = API_BASE) {
        this.baseURL = baseURL;
        this.timeout = 30000; // 30 seconds default timeout
    }

    // Generic request method with timeout
    async request(url, options = {}) {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), options.timeout || this.timeout);

        try {
            const response = await fetch(`${this.baseURL}${url}`, {
                ...options,
                signal: controller.signal,
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers,
                },
            });
            clearTimeout(timeoutId);

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                throw new APIError(
                    errorData.error || errorData.message || `HTTP ${response.status}`,
                    response.status,
                    errorData
                );
            }

            return await response.json();
        } catch (error) {
            clearTimeout(timeoutId);
            if (error.name === 'AbortError') {
                throw new APIError('Request timeout', 408);
            }
            throw error;
        }
    }

    // GET request
    async get(url, options = {}) {
        return this.request(url, { ...options, method: 'GET' });
    }

    // POST request
    async post(url, data, options = {}) {
        return this.request(url, {
            ...options,
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    // PUT request
    async put(url, data, options = {}) {
        return this.request(url, {
            ...options,
            method: 'PUT',
            body: JSON.stringify(data),
        });
    }

    // DELETE request
    async delete(url, options = {}) {
        return this.request(url, { ...options, method: 'DELETE' });
    }
}

// Custom API Error class
class APIError extends Error {
    constructor(message, status, details = {}) {
        super(message);
        this.name = 'APIError';
        this.status = status;
        this.details = details;
    }
}

// Create global API client instance
const api = new APIClient();

// Utility Functions

/**
 * Format a timestamp to a human-readable date string
 * @param {number} timestamp - Unix timestamp in milliseconds
 * @param {boolean} includeTime - Whether to include time
 * @returns {string} Formatted date string
 */
function formatDate(timestamp, includeTime = true) {
    if (!timestamp) return 'N/A';
    const date = new Date(timestamp);
    const options = {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
    };
    if (includeTime) {
        options.hour = '2-digit';
        options.minute = '2-digit';
    }
    return date.toLocaleDateString(undefined, options);
}

/**
 * Format expiry date with relative time
 * @param {number} expiryTimestamp - Unix timestamp in milliseconds
 * @returns {object} Object with formatted date and relative time
 */
function formatExpiry(expiryTimestamp) {
    if (!expiryTimestamp) {
        return {
            formatted: 'Never',
            relative: 'Never expires',
            isExpiring: false,
            isExpired: false,
            daysRemaining: Infinity,
        };
    }

    const now = Date.now();
    const expiryDate = new Date(expiryTimestamp);
    const msRemaining = expiryTimestamp - now;
    const daysRemaining = Math.floor(msRemaining / (1000 * 60 * 60 * 24));
    const hoursRemaining = Math.floor(msRemaining / (1000 * 60 * 60));
    const minutesRemaining = Math.floor(msRemaining / (1000 * 60));

    let relative;
    if (msRemaining < 0) {
        relative = 'Expired';
    } else if (daysRemaining > 7) {
        relative = `${daysRemaining} days`;
    } else if (daysRemaining > 0) {
        relative = `${daysRemaining} day${daysRemaining > 1 ? 's' : ''}`;
    } else if (hoursRemaining > 0) {
        relative = `${hoursRemaining} hour${hoursRemaining > 1 ? 's' : ''}`;
    } else {
        relative = `${minutesRemaining} minute${minutesRemaining > 1 ? 's' : ''}`;
    }

    return {
        formatted: expiryDate.toLocaleString(),
        relative,
        isExpiring: daysRemaining <= 7 && daysRemaining > 0,
        isExpired: msRemaining < 0,
        daysRemaining,
    };
}

/**
 * Show a toast notification
 * @param {string} message - Message to display
 * @param {string} type - Type: 'success', 'error', 'warning', 'info'
 * @param {number} duration - Duration in milliseconds (default: 3000)
 */
function showToast(message, type = 'info', duration = 3000) {
    // Remove existing toast if any
    const existingToast = document.querySelector('.toast');
    if (existingToast) {
        existingToast.remove();
    }

    // Create toast element
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;

    // Add to document
    document.body.appendChild(toast);

    // Trigger animation
    requestAnimationFrame(() => {
        toast.classList.add('show');
    });

    // Auto dismiss
    setTimeout(() => {
        toast.classList.remove('show');
        setTimeout(() => toast.remove(), 300);
    }, duration);
}

/**
 * Show a confirmation dialog
 * @param {string} message - Message to display
 * @param {string} title - Dialog title
 * @returns {Promise<boolean>} Promise that resolves to true if confirmed
 */
function showConfirm(message, title = 'Confirm') {
    return new Promise((resolve) => {
        // Create modal overlay
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';

        // Create modal
        const modal = document.createElement('div');
        modal.className = 'modal modal-confirm';

        modal.innerHTML = `
            <div class="modal-header">
                <h3>${title}</h3>
            </div>
            <div class="modal-body">
                <p>${message}</p>
            </div>
            <div class="modal-footer">
                <button class="btn btn-secondary" id="confirm-cancel">Cancel</button>
                <button class="btn btn-primary" id="confirm-ok">Confirm</button>
            </div>
        `;

        overlay.appendChild(modal);
        document.body.appendChild(overlay);

        // Handle buttons
        const cancelBtn = modal.querySelector('#confirm-cancel');
        const okBtn = modal.querySelector('#confirm-ok');

        const cleanup = () => {
            overlay.remove();
            cancelBtn.removeEventListener('click', onCancel);
            okBtn.removeEventListener('click', onOk);
        };

        const onCancel = () => {
            cleanup();
            resolve(false);
        };

        const onOk = () => {
            cleanup();
            resolve(true);
        };

        cancelBtn.addEventListener('click', onCancel);
        okBtn.addEventListener('click', onOk);

        // Focus on OK button
        okBtn.focus();
    });
}

/**
 * Show loading state
 * @param {string} message - Loading message (optional)
 */
function showLoading(message = 'Loading...') {
    let overlay = document.querySelector('.loading-overlay');
    if (!overlay) {
        overlay = document.createElement('div');
        overlay.className = 'loading-overlay';
        document.body.appendChild(overlay);
    }
    overlay.innerHTML = `
        <div class="loading-spinner"></div>
        <p>${message}</p>
    `;
    overlay.classList.add('show');
}

/**
 * Hide loading state
 */
function hideLoading() {
    const overlay = document.querySelector('.loading-overlay');
    if (overlay) {
        overlay.classList.remove('show');
        setTimeout(() => overlay.remove(), 300);
    }
}

/**
 * Show element loading state
 * @param {HTMLElement} element - Element to show loading on
 */
function showElementLoading(element) {
    element.classList.add('loading');
    const existingSpinner = element.querySelector('.element-spinner');
    if (!existingSpinner) {
        const spinner = document.createElement('div');
        spinner.className = 'element-spinner';
        element.appendChild(spinner);
    }
}

/**
 * Hide element loading state
 * @param {HTMLElement} element - Element to hide loading on
 */
function hideElementLoading(element) {
    element.classList.remove('loading');
    const spinner = element.querySelector('.element-spinner');
    if (spinner) {
        spinner.remove();
    }
}

/**
 * Copy text to clipboard
 * @param {string} text - Text to copy
 * @returns {Promise<boolean>} Promise that resolves to true if successful
 */
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('Copied to clipboard', 'success');
        return true;
    } catch (err) {
        console.error('Failed to copy:', err);
        showToast('Failed to copy to clipboard', 'error');
        return false;
    }
}

/**
 * Truncate text with ellipsis
 * @param {string} text - Text to truncate
 * @param {number} maxLength - Maximum length
 * @returns {string} Truncated text
 */
function truncateText(text, maxLength = 50) {
    if (!text) return '';
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
}

/**
 * Debounce function execution
 * @param {Function} func - Function to debounce
 * @param {number} wait - Wait time in milliseconds
 * @returns {Function} Debounced function
 */
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Escape HTML to prevent XSS
 * @param {string} text - Text to escape
 * @returns {string} Escaped text
 */
function escapeHTML(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Get status class based on expiry
 * @param {number} daysRemaining - Days remaining until expiry
 * @returns {string} CSS class name
 */
function getExpiryStatusClass(daysRemaining) {
    if (daysRemaining < 0) return 'status-expired';
    if (daysRemaining < 1) return 'status-expiring-soon';
    if (daysRemaining <= 7) return 'status-expiring';
    return 'status-ok';
}

/**
 * Get health status class
 * @param {boolean} healthy - Whether token is healthy
 * @returns {string} CSS class name
 */
function getHealthStatusClass(healthy) {
    return healthy ? 'status-healthy' : 'status-unhealthy';
}

/**
 * Get proxy status class
 * @param {object} proxy - Proxy configuration object
 * @returns {string} CSS class name
 */
function getProxyStatusClass(proxy) {
    if (!proxy || proxy.type === 'none') return 'status-no-proxy';
    return 'status-has-proxy';
}

/**
 * Format proxy URL for display
 * @param {object} proxy - Proxy configuration object
 * @returns {string} Formatted proxy URL
 */
function formatProxyURL(proxy) {
    if (!proxy || proxy.type === 'none') return 'None';
    return `${proxy.type}://${proxy.host}:${proxy.port}`;
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        APIClient,
        APIError,
        api,
        formatDate,
        formatExpiry,
        showToast,
        showConfirm,
        showLoading,
        hideLoading,
        showElementLoading,
        hideElementLoading,
        copyToClipboard,
        truncateText,
        debounce,
        escapeHTML,
        getExpiryStatusClass,
        getHealthStatusClass,
        getProxyStatusClass,
        formatProxyURL,
    };
}
