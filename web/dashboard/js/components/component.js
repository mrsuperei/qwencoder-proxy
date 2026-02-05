/**
 * Component - Base class for UI components
 * 
 * Provides common functionality for UI components including
 * container reference, event handling, and lifecycle methods.
 */

import { EventEmitter } from '../utils/event-emitter.js';

export class Component {
    /**
     * Create a new Component instance
     * @param {string} containerId - The ID of the DOM element to render into
     */
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.events = new EventEmitter();
    }

    /**
     * Render the component (to be implemented by subclasses)
     */
    render() {
        // To be implemented by subclasses
    }

    /**
     * Destroy the component and clean up DOM
     */
    destroy() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }
}
