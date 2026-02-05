/**
 * LoadingOverlay - Loading indicator component
 * 
 * Displays a full-screen loading overlay with spinner
 * to indicate ongoing operations.
 */

import { Component } from './component.js';

export class LoadingOverlay extends Component {
    constructor() {
        super('loadingOverlay');
        if (!this.container) {
            this.createContainer();
        }
    }

    /**
     * Create the loading overlay element if it doesn't exist
     */
    createContainer() {
        this.container = document.createElement('div');
        this.container.id = 'loadingOverlay';
        this.container.className = 'loading-overlay';
        this.container.innerHTML = '<div class="loading-spinner"></div>';
        document.body.appendChild(this.container);
    }

    /**
     * Show the loading overlay
     */
    show() {
        this.container.classList.add('active');
    }

    /**
     * Hide the loading overlay
     */
    hide() {
        this.container.classList.remove('active');
    }
}
