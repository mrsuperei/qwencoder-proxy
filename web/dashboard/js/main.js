/**
 * Main entry point for the Dashboard application
 * 
 * Initializes the Dashboard class when the DOM is ready.
 * This is the primary entry point for the application.
 */

import { Dashboard } from './dashboard.js';

document.addEventListener('DOMContentLoaded', () => {
    window.dashboard = new Dashboard();
});
