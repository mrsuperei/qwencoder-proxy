/**
 * Modal - Modal dialog component
 * 
 * Provides functionality for showing and hiding modal dialogs
 * with overlay and transition effects.
 */

import { Component } from './component.js';

export class Modal extends Component {
    /**
     * Create a new Modal instance
     * @param {string} modalId - The ID of the modal element
     */
    constructor(modalId) {
        super(modalId);
        this.isOpen = false;
    }

    /**
     * Open the modal
     */
    open() {
        this.isOpen = true;
        this.container.classList.add('active');
    }

    /**
     * Close the modal
     */
    close() {
        this.isOpen = false;
        this.container.classList.remove('active');
    }

    /**
     * Toggle the modal open/closed state
     */
    toggle() {
        if (this.isOpen) {
            this.close();
        } else {
            this.open();
        }
    }
}
