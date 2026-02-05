/**
 * EventEmitter - Base class for pub/sub pattern
 * 
 * Provides event subscription, unsubscription, and emission capabilities.
 * Used as a foundation for other components that need to communicate via events.
 */

export class EventEmitter {
    constructor() {
        this.events = {};
    }

    /**
     * Subscribe to an event
     * @param {string} event - The event name to listen for
     * @param {Function} callback - The function to call when the event is emitted
     */
    on(event, callback) {
        if (!this.events[event]) {
            this.events[event] = [];
        }
        this.events[event].push(callback);
    }

    /**
     * Unsubscribe from an event
     * @param {string} event - The event name to stop listening for
     * @param {Function} callback - The function to remove from the event listeners
     */
    off(event, callback) {
        if (!this.events[event]) return;
        this.events[event] = this.events[event].filter(cb => cb !== callback);
    }

    /**
     * Emit an event to all subscribers
     * @param {string} event - The event name to emit
     * @param {*} data - The data to pass to all event listeners
     */
    emit(event, data) {
        if (!this.events[event]) return;
        this.events[event].forEach(callback => callback(data));
    }
}
