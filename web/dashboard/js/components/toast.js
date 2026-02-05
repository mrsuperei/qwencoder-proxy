/**
 * ToastContainer - Toast notification component
 * 
 * Displays temporary notification messages that appear
 * in the corner of the screen and auto-dismiss.
 */

import { Component } from './component.js';

export class ToastContainer extends Component {
    constructor() {
        super('toastContainer');
        if (!this.container) {
            this.createContainer();
        }
    }

    /**
     * Create the toast container element if it doesn't exist
     */
    createContainer() {
        this.container = document.createElement('div');
        this.container.id = 'toastContainer';
        this.container.className = 'toast-container';
        document.body.appendChild(this.container);
    }

    /**
     * Show a toast notification
     * @param {string} message - The message to display
     * @param {string} type - The type of toast: 'success', 'error', 'warning', or 'info'
     */
    show(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `<span>${this.getIcon(type)}</span><span>${this.escapeHtml(message)}</span>`;
        this.container.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => this.container.removeChild(toast), 300);
        }, 5000);
    }

    /**
     * Get the icon for a toast type
     * @param {string} type - The toast type
     * @returns {string} The icon character
     */
    getIcon(type) {
        const icons = { success: '✓', error: '✕', warning: '⚠', info: 'ℹ' };
        return icons[type] || 'ℹ';
    }

    /**
     * Escape HTML to prevent XSS attacks
     * @param {string} text - The text to escape
     * @returns {string} The escaped HTML
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
