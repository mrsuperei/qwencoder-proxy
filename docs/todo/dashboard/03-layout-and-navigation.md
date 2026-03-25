# Phase 3: Layout & Navigation

**Priority:** HIGH  
**Estimated Time:** 1 day  
**Complexity:** Low  
**Files to Create:** 8  
**Files to Modify:** 0

---

## Executive Summary

This phase implements the core layout components and navigation system. It creates reusable view components, modal dialogs, and notification system that will be used throughout the application.

---

## Problem Description

The application needs a consistent layout system with reusable components. Without proper abstraction, UI code becomes duplicated and difficult to maintain.

### Requirements

- Reusable view base class
- Modal dialog system
- Toast notification system
- Card components
- Table components
- Form components

---

## Solution Architecture

### Design Principles

1. **Component-Based:** All UI elements are reusable components
2. **Base Class Pattern:** Views extend a common base class
3. **Event Delegation:** Events are handled efficiently
4. **Accessibility:** All components are keyboard accessible
5. **Responsive:** Components work on all screen sizes

### Component Overview

```
js/
├── components/
│   ├── View.js            # Base view class
│   ├── Modal.js           # Modal dialog component
│   ├── Toast.js           # Toast notification component
│   ├── Card.js            # Card component
│   ├── Table.js           # Table component
│   └── Form.js           # Form component
└── views/
    ├── Overview.js        # Overview view
    ├── Providers.js       # Providers view
    ├── Tokens.js          # Tokens view
    ├── Proxies.js         # Proxies view
    ├── RateLimits.js      # Rate limits view
    ├── Usage.js          # Usage view
    └── Settings.js       # Settings view
```

---

## Implementation Plan

### Step 1: Create Base View Class (js/components/View.js)

Implement base class for all views:

```javascript
/**
 * Base View Class
 * 
 * Provides common functionality for all view components.
 * All views should extend this class.
 */

export class View {
    constructor(app) {
        this.app = app;
        this.api = app.api;
        this.store = app.store;
        this.container = null;
        this.subscriptions = [];
    }

    /**
     * Render view
     * @param {HTMLElement} container - Container element
     */
    async render(container) {
        this.container = container;
        this.onMount();
    }

    /**
     * Called when view is mounted
     */
    onMount() {
        // Override in subclasses
    }

    /**
     * Called when view is unmounted
     */
    onUnmount() {
        // Unsubscribe from all subscriptions
        this.subscriptions.forEach(unsubscribe => unsubscribe());
        this.subscriptions = [];
    }

    /**
     * Subscribe to state
     * @param {string} key - State key
     * @param {Function} listener - Listener function
     */
    subscribe(key, listener) {
        const unsubscribe = this.store.subscribe(key, listener);
        this.subscriptions.push(unsubscribe);
    }

    /**
     * Show loading state
     */
    showLoading() {
        if (this.container) {
            this.container.innerHTML = `
                <div class="loading-spinner">
                    <div class="spinner"></div>
                    <p>Loading...</p>
                </div>
            `;
        }
    }

    /**
     * Show error state
     * @param {Error} error - Error object
     */
    showError(error) {
        if (this.container) {
            this.container.innerHTML = `
                <div class="error-state">
                    <div class="error-icon">⚠️</div>
                    <h2>Error</h2>
                    <p>${error.message}</p>
                    <button class="btn btn-primary" onclick="location.reload()">
                        Retry
                    </button>
                </div>
            `;
        }
    }

    /**
     * Show empty state
     * @param {string} message - Empty state message
     * @param {string} icon - Empty state icon
     */
    showEmpty(message, icon = '📭') {
        if (this.container) {
            this.container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-icon">${icon}</div>
                    <p>${message}</p>
                </div>
            `;
        }
    }
}
```

**Verification:**
- [ ] Base class works
- [ ] Subscriptions work
- [ ] Loading/error/empty states work

---

### Step 2: Create Modal Component (js/components/Modal.js)

Implement modal dialog system:

```javascript
/**
 * Modal Component
 * 
 * Provides a reusable modal dialog system with
 * support for different sizes and content types.
 */

import { store, actions } from '../state/store.js';

export class Modal {
    constructor(options = {}) {
        this.id = options.id || `modal-${Date.now()}`;
        this.title = options.title || '';
        this.content = options.content || '';
        this.footer = options.footer || '';
        this.size = options.size || 'md'; // sm, md, lg, xl
        this.closeOnOverlay = options.closeOnOverlay !== false;
        this.closeOnEscape = options.closeOnEscape !== false;
        this.onClose = options.onClose || null;
        this.element = null;
    }

    /**
     * Create modal element
     * @returns {HTMLElement} Modal element
     */
    createElement() {
        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.id = this.id;
        modal.setAttribute('role', 'dialog');
        modal.setAttribute('aria-modal', 'true');
        modal.setAttribute('aria-labelledby', `${this.id}-title`);

        modal.innerHTML = `
            <div class="modal modal-${this.size}">
                <div class="modal-header">
                    <h2 class="modal-title" id="${this.id}-title">${this.title}</h2>
                    <button class="modal-close" aria-label="Close modal">
                        ✕
                    </button>
                </div>
                <div class="modal-body">
                    ${this.content}
                </div>
                ${this.footer ? `<div class="modal-footer">${this.footer}</div>` : ''}
            </div>
        `;

        // Setup event listeners
        this.setupEventListeners(modal);

        return modal;
    }

    /**
     * Setup event listeners
     * @param {HTMLElement} modal - Modal element
     */
    setupEventListeners(modal) {
        // Close button
        const closeBtn = modal.querySelector('.modal-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', () => this.close());
        }

        // Overlay click
        if (this.closeOnOverlay) {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    this.close();
                }
            });
        }

        // Escape key
        if (this.closeOnEscape) {
            const escapeHandler = (e) => {
                if (e.key === 'Escape') {
                    this.close();
                }
            };
            document.addEventListener('keydown', escapeHandler);
            this.escapeHandler = escapeHandler;
        }

        // Focus trap
        this.setupFocusTrap(modal);
    }

    /**
     * Setup focus trap for accessibility
     * @param {HTMLElement} modal - Modal element
     */
    setupFocusTrap(modal) {
        const focusableElements = modal.querySelectorAll(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );
        const firstFocusable = focusableElements[0];
        const lastFocusable = focusableElements[focusableElements.length - 1];

        const trapFocus = (e) => {
            if (e.key === 'Tab') {
                if (e.shiftKey) {
                    if (document.activeElement === firstFocusable) {
                        lastFocusable.focus();
                        e.preventDefault();
                    }
                } else {
                    if (document.activeElement === lastFocusable) {
                        firstFocusable.focus();
                        e.preventDefault();
                    }
                }
            }
        };

        modal.addEventListener('keydown', trapFocus);
        this.trapFocusHandler = trapFocus;

        // Focus first element
        setTimeout(() => firstFocusable?.focus(), 0);
    }

    /**
     * Open modal
     */
    open() {
        // Create element if not exists
        if (!this.element) {
            this.element = this.createElement();
        }

        // Add to DOM
        document.getElementById('modals').appendChild(this.element);

        // Prevent body scroll
        document.body.style.overflow = 'hidden';

        // Update state
        store.dispatch(actions.openModal(this.id, {
            title: this.title,
            size: this.size
        }));

        // Focus trap
        this.element.focus();
    }

    /**
     * Close modal
     */
    close() {
        if (!this.element) return;

        // Remove from DOM
        this.element.remove();

        // Restore body scroll
        document.body.style.overflow = '';

        // Update state
        store.dispatch(actions.closeModal());

        // Call close callback
        if (this.onClose) {
            this.onClose();
        }

        // Cleanup event listeners
        if (this.escapeHandler) {
            document.removeEventListener('keydown', this.escapeHandler);
        }
        if (this.trapFocusHandler) {
            this.element.removeEventListener('keydown', this.trapFocusHandler);
        }

        this.element = null;
    }

    /**
     * Update modal content
     * @param {string} content - New content
     */
    updateContent(content) {
        this.content = content;
        if (this.element) {
            const body = this.element.querySelector('.modal-body');
            if (body) {
                body.innerHTML = content;
            }
        }
    }

    /**
     * Update modal footer
     * @param {string} footer - New footer
     */
    updateFooter(footer) {
        this.footer = footer;
        if (this.element) {
            const footerEl = this.element.querySelector('.modal-footer');
            if (footerEl) {
                footerEl.innerHTML = footer;
            } else if (footer) {
                const modal = this.element.querySelector('.modal');
                const footerDiv = document.createElement('div');
                footerDiv.className = 'modal-footer';
                footerDiv.innerHTML = footer;
                modal.appendChild(footerDiv);
            }
        }
    }
}

/**
 * Create confirm modal
 * @param {Object} options - Modal options
 * @returns {Promise<boolean>} User choice
 */
export function confirmModal(options) {
    return new Promise((resolve) => {
        const modal = new Modal({
            title: options.title || 'Confirm',
            content: options.message || 'Are you sure?',
            footer: `
                <button class="btn btn-secondary modal-cancel">Cancel</button>
                <button class="btn btn-danger modal-confirm">Confirm</button>
            `,
            size: options.size || 'sm',
            onClose: () => resolve(false)
        });

        modal.open();

        // Setup button handlers
        const confirmBtn = modal.element.querySelector('.modal-confirm');
        const cancelBtn = modal.element.querySelector('.modal-cancel');

        confirmBtn.addEventListener('click', () => {
            modal.close();
            resolve(true);
        });

        cancelBtn.addEventListener('click', () => {
            modal.close();
            resolve(false);
        });
    });
}
```

**Verification:**
- [ ] Modal opens/closes
- [ ] Focus trap works
- [ ] Escape key closes
- [ ] Overlay click closes
- [ ] Confirm modal works

---

### Step 3: Create Toast Component (js/components/Toast.js)

Implement toast notification system:

```javascript
/**
 * Toast Component
 * 
 * Provides a toast notification system for displaying
 * success, error, warning, and info messages.
 */

export class Toast {
    constructor(options = {}) {
        this.id = `toast-${Date.now()}`;
        this.type = options.type || 'info'; // success, error, warning, info
        this.title = options.title || '';
        this.message = options.message || '';
        this.duration = options.duration || 5000;
        this.closeable = options.closeable !== false;
        this.element = null;
        this.timeoutId = null;
    }

    /**
     * Get icon for toast type
     * @returns {string} Icon emoji
     */
    getIcon() {
        const icons = {
            success: '✓',
            error: '✕',
            warning: '⚠',
            info: 'ℹ'
        };
        return icons[this.type] || 'ℹ';
    }

    /**
     * Create toast element
     * @returns {HTMLElement} Toast element
     */
    createElement() {
        const toast = document.createElement('div');
        toast.className = `toast toast-${this.type}`;
        toast.id = this.id;
        toast.setAttribute('role', 'alert');
        toast.setAttribute('aria-live', 'polite');

        toast.innerHTML = `
            <div class="toast-icon">${this.getIcon()}</div>
            <div class="toast-content">
                ${this.title ? `<div class="toast-title">${this.title}</div>` : ''}
                <div class="toast-message">${this.message}</div>
            </div>
            ${this.closeable ? `
                <button class="toast-close" aria-label="Close notification">
                    ✕
                </button>
            ` : ''}
        `;

        // Setup event listeners
        this.setupEventListeners(toast);

        return toast;
    }

    /**
     * Setup event listeners
     * @param {HTMLElement} toast - Toast element
     */
    setupEventListeners(toast) {
        // Close button
        const closeBtn = toast.querySelector('.toast-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', () => this.close());
        }

        // Click to close
        toast.addEventListener('click', (e) => {
            if (e.target === toast || e.target.closest('.toast-content')) {
                this.close();
            }
        });
    }

    /**
     * Show toast
     */
    show() {
        // Create element
        this.element = this.createElement();

        // Add to container
        const container = document.getElementById('notifications');
        container.appendChild(this.element);

        // Auto dismiss
        if (this.duration > 0) {
            this.timeoutId = setTimeout(() => {
                this.close();
            }, this.duration);
        }
    }

    /**
     * Close toast
     */
    close() {
        if (!this.element) return;

        // Clear timeout
        if (this.timeoutId) {
            clearTimeout(this.timeoutId);
        }

        // Add hiding class
        this.element.classList.add('hiding');

        // Remove after animation
        setTimeout(() => {
            this.element?.remove();
            this.element = null;
        }, 300);
    }
}

/**
 * Toast manager
 */
export const toast = {
    /**
     * Show success toast
     * @param {string} message - Message
     * @param {Object} options - Additional options
     */
    success(message, options = {}) {
        this.show({ ...options, type: 'success', message });
    },

    /**
     * Show error toast
     * @param {string} message - Message
     * @param {Object} options - Additional options
     */
    error(message, options = {}) {
        this.show({ ...options, type: 'error', message });
    },

    /**
     * Show warning toast
     * @param {string} message - Message
     * @param {Object} options - Additional options
     */
    warning(message, options = {}) {
        this.show({ ...options, type: 'warning', message });
    },

    /**
     * Show info toast
     * @param {string} message - Message
     * @param {Object} options - Additional options
     */
    info(message, options = {}) {
        this.show({ ...options, type: 'info', message });
    },

    /**
     * Show toast
     * @param {Object} options - Toast options
     */
    show(options) {
        const toastInstance = new Toast(options);
        toastInstance.show();
        return toastInstance;
    }
};
```

**Verification:**
- [ ] Toasts display correctly
- [ ] All types work
- [ ] Auto dismiss works
- [ ] Manual close works
- [ ] Multiple toasts stack

---

### Step 4: Create Card Component (js/components/Card.js)

Implement reusable card component:

```javascript
/**
 * Card Component
 * 
 * Provides a reusable card component for displaying
 * grouped content with optional header and footer.
 */

export class Card {
    constructor(options = {}) {
        this.title = options.title || '';
        this.content = options.content || '';
        this.footer = options.footer || '';
        this.className = options.className || '';
        this.element = null;
    }

    /**
     * Create card element
     * @returns {HTMLElement} Card element
     */
    createElement() {
        const card = document.createElement('div');
        card.className = `card ${this.className}`.trim();

        let html = '';
        
        if (this.title) {
            html += `
                <div class="card-header">
                    <h3 class="card-title">${this.title}</h3>
                </div>
            `;
        }

        html += `
            <div class="card-body">
                ${this.content}
            </div>
        `;

        if (this.footer) {
            html += `
                <div class="card-footer">
                    ${this.footer}
                </div>
            `;
        }

        card.innerHTML = html;
        this.element = card;

        return card;
    }

    /**
     * Render card
     * @param {HTMLElement} container - Container element
     */
    render(container) {
        const card = this.createElement();
        container.appendChild(card);
        return card;
    }

    /**
     * Update content
     * @param {string} content - New content
     */
    updateContent(content) {
        this.content = content;
        if (this.element) {
            const body = this.element.querySelector('.card-body');
            if (body) {
                body.innerHTML = content;
            }
        }
    }

    /**
     * Update footer
     * @param {string} footer - New footer
     */
    updateFooter(footer) {
        this.footer = footer;
        if (this.element) {
            const footerEl = this.element.querySelector('.card-footer');
            if (footerEl) {
                footerEl.innerHTML = footer;
            } else if (footer) {
                const footerDiv = document.createElement('div');
                footerDiv.className = 'card-footer';
                footerDiv.innerHTML = footer;
                this.element.appendChild(footerDiv);
            }
        }
    }
}

/**
 * Create stat card
 * @param {Object} options - Card options
 * @returns {HTMLElement} Stat card element
 */
export function createStatCard(options) {
    const { label, value, icon, trend, color = 'primary' } = options;

    const card = document.createElement('div');
    card.className = 'card stat-card';

    card.innerHTML = `
        <div class="stat-header">
            <span class="stat-icon">${icon}</span>
            <span class="stat-label">${label}</span>
        </div>
        <div class="stat-value">${value}</div>
        ${trend ? `
            <div class="stat-trend ${trend > 0 ? 'positive' : 'negative'}">
                ${trend > 0 ? '↑' : '↓'} ${Math.abs(trend)}%
            </div>
        ` : ''}
    `;

    return card;
}
```

**Verification:**
- [ ] Card renders correctly
- [ ] Header/footer work
- [ ] Content updates work
- [ ] Stat card works

---

### Step 5: Create Table Component (js/components/Table.js)

Implement reusable table component:

```javascript
/**
 * Table Component
 * 
 * Provides a reusable table component with support for
 * sorting, filtering, and pagination.
 */

export class Table {
    constructor(options = {}) {
        this.columns = options.columns || [];
        this.data = options.data || [];
        this.className = options.className || '';
        this.emptyMessage = options.emptyMessage || 'No data available';
        this.sortable = options.sortable !== false;
        this.sortColumn = null;
        this.sortDirection = 'asc';
        this.element = null;
    }

    /**
     * Create table element
     * @returns {HTMLElement} Table element
     */
    createElement() {
        const container = document.createElement('div');
        container.className = 'table-container';

        if (this.data.length === 0) {
            container.innerHTML = `
                <div class="table-empty">
                    <div class="table-empty-icon">📭</div>
                    <p>${this.emptyMessage}</p>
                </div>
            `;
            this.element = container;
            return container;
        }

        const table = document.createElement('table');
        table.className = `table ${this.className}`.trim();

        // Create header
        const thead = document.createElement('thead');
        const headerRow = document.createElement('tr');

        this.columns.forEach(column => {
            const th = document.createElement('th');
            th.textContent = column.label;
            
            if (this.sortable && column.sortable !== false) {
                th.className = 'table-sortable';
                th.dataset.column = column.key;
                th.addEventListener('click', () => this.sort(column.key));
                
                if (this.sortColumn === column.key) {
                    th.classList.add(this.sortDirection);
                }
            }
            
            headerRow.appendChild(th);
        });

        thead.appendChild(headerRow);
        table.appendChild(thead);

        // Create body
        const tbody = document.createElement('tbody');
        
        this.data.forEach(row => {
            const tr = document.createElement('tr');
            
            this.columns.forEach(column => {
                const td = document.createElement('td');
                const value = this.getCellValue(row, column.key);
                
                if (column.render) {
                    td.innerHTML = column.render(value, row);
                } else {
                    td.textContent = value;
                }
                
                tr.appendChild(td);
            });
            
            tbody.appendChild(tr);
        });

        table.appendChild(tbody);
        container.appendChild(table);

        this.element = container;
        return container;
    }

    /**
     * Get cell value from row data
     * @param {Object} row - Row data
     * @param {string} key - Column key
     * @returns {*} Cell value
     */
    getCellValue(row, key) {
        return key.split('.').reduce((obj, k) => obj?.[k], row);
    }

    /**
     * Sort table
     * @param {string} column - Column key
     */
    sort(column) {
        if (this.sortColumn === column) {
            this.sortDirection = this.sortDirection === 'asc' ? 'desc' : 'asc';
        } else {
            this.sortColumn = column;
            this.sortDirection = 'asc';
        }

        this.data.sort((a, b) => {
            const aVal = this.getCellValue(a, column);
            const bVal = this.getCellValue(b, column);

            if (aVal < bVal) return this.sortDirection === 'asc' ? -1 : 1;
            if (aVal > bVal) return this.sortDirection === 'asc' ? 1 : -1;
            return 0;
        });

        this.render(this.element.parentElement);
    }

    /**
     * Render table
     * @param {HTMLElement} container - Container element
     */
    render(container) {
        const newElement = this.createElement();
        if (this.element) {
            this.element.replaceWith(newElement);
        } else {
            container.appendChild(newElement);
        }
    }

    /**
     * Update data
     * @param {Array} data - New data
     */
    setData(data) {
        this.data = data;
        if (this.element) {
            this.render(this.element.parentElement);
        }
    }
}
```

**Verification:**
- [ ] Table renders correctly
- [ ] Sorting works
- [ ] Empty state works
- [ ] Custom renderers work

---

### Step 6: Create Form Component (js/components/Form.js)

Implement reusable form component:

```javascript
/**
 * Form Component
 * 
 * Provides a reusable form component with validation
 * and submission handling.
 */

export class Form {
    constructor(options = {}) {
        this.fields = options.fields || [];
        this.onSubmit = options.onSubmit || null;
        this.className = options.className || '';
        this.element = null;
        this.values = {};
    }

    /**
     * Create form element
     * @returns {HTMLElement} Form element
     */
    createElement() {
        const form = document.createElement('form');
        form.className = `form ${this.className}`.trim();
        form.noValidate = true;

        // Create field groups
        this.fields.forEach(field => {
            const fieldGroup = this.createField(field);
            form.appendChild(fieldGroup);
        });

        // Setup submit handler
        form.addEventListener('submit', (e) => this.handleSubmit(e));

        this.element = form;
        return form;
    }

    /**
     * Create field element
     * @param {Object} field - Field configuration
     * @returns {HTMLElement} Field element
     */
    createField(field) {
        const group = document.createElement('div');
        group.className = 'form-group';

        // Label
        if (field.label) {
            const label = document.createElement('label');
            label.textContent = field.label;
            label.htmlFor = field.name;
            group.appendChild(label);
        }

        // Input based on type
        let input;
        switch (field.type) {
            case 'select':
                input = this.createSelect(field);
                break;
            case 'textarea':
                input = this.createTextarea(field);
                break;
            case 'checkbox':
                input = this.createCheckbox(field);
                break;
            default:
                input = this.createInput(field);
        }

        input.name = field.name;
        input.id = field.name;
        input.required = field.required || false;

        if (field.placeholder) {
            input.placeholder = field.placeholder;
        }

        if (field.value !== undefined) {
            input.value = field.value;
        }

        group.appendChild(input);

        // Help text
        if (field.helpText) {
            const helpText = document.createElement('div');
            helpText.className = 'help-text';
            helpText.textContent = field.helpText;
            group.appendChild(helpText);
        }

        // Validation message
        const validationMessage = document.createElement('div');
        validationMessage.className = 'validation-message hidden';
        validationMessage.id = `${field.name}-validation`;
        group.appendChild(validationMessage);

        return group;
    }

    /**
     * Create input element
     * @param {Object} field - Field configuration
     * @returns {HTMLElement} Input element
     */
    createInput(field) {
        const input = document.createElement('input');
        input.type = field.type || 'text';
        input.className = 'input';
        
        if (field.min !== undefined) input.min = field.min;
        if (field.max !== undefined) input.max = field.max;
        if (field.step !== undefined) input.step = field.step;
        
        return input;
    }

    /**
     * Create select element
     * @param {Object} field - Field configuration
     * @returns {HTMLElement} Select element
     */
    createSelect(field) {
        const select = document.createElement('select');
        select.className = 'select';

        if (field.options) {
            field.options.forEach(option => {
                const optionEl = document.createElement('option');
                optionEl.value = option.value;
                optionEl.textContent = option.label;
                select.appendChild(optionEl);
            });
        }

        return select;
    }

    /**
     * Create textarea element
     * @param {Object} field - Field configuration
     * @returns {HTMLElement} Textarea element
     */
    createTextarea(field) {
        const textarea = document.createElement('textarea');
        textarea.className = 'textarea';
        
        if (field.rows) {
            textarea.rows = field.rows;
        }
        
        return textarea;
    }

    /**
     * Create checkbox element
     * @param {Object} field - Field configuration
     * @returns {HTMLElement} Checkbox element
     */
    createCheckbox(field) {
        const wrapper = document.createElement('div');
        wrapper.className = 'checkbox-group';

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.className = 'checkbox';

        if (field.checked) {
            checkbox.checked = true;
        }

        wrapper.appendChild(checkbox);

        if (field.label) {
            const label = document.createElement('label');
            label.textContent = field.label;
            label.htmlFor = field.name;
            wrapper.appendChild(label);
        }

        return wrapper;
    }

    /**
     * Handle form submission
     * @param {Event} e - Submit event
     */
    async handleSubmit(e) {
        e.preventDefault();

        // Get form values
        const formData = new FormData(this.element);
        this.values = Object.fromEntries(formData.entries());

        // Validate
        if (!this.validate()) {
            return;
        }

        // Submit
        if (this.onSubmit) {
            try {
                await this.onSubmit(this.values);
            } catch (error) {
                this.showError(error.message);
            }
        }
    }

    /**
     * Validate form
     * @returns {boolean} Valid
     */
    validate() {
        let valid = true;

        this.fields.forEach(field => {
            const input = this.element.elements[field.name];
            const validationEl = document.getElementById(`${field.name}-validation`);

            // Required validation
            if (field.required && !input.value) {
                this.showFieldError(field.name, 'This field is required');
                valid = false;
                return;
            }

            // Custom validation
            if (field.validate && !field.validate(input.value)) {
                this.showFieldError(field.name, field.validationMessage || 'Invalid value');
                valid = false;
                return;
            }

            // Clear validation
            if (validationEl) {
                validationEl.classList.add('hidden');
            }
            input.classList.remove('error');
        });

        return valid;
    }

    /**
     * Show field error
     * @param {string} fieldName - Field name
     * @param {string} message - Error message
     */
    showFieldError(fieldName, message) {
        const input = this.element.elements[fieldName];
        const validationEl = document.getElementById(`${fieldName}-validation`);

        if (input) {
            input.classList.add('error');
        }

        if (validationEl) {
            validationEl.textContent = message;
            validationEl.classList.remove('hidden');
        }
    }

    /**
     * Show form error
     * @param {string} message - Error message
     */
    showError(message) {
        const errorEl = document.createElement('div');
        errorEl.className = 'form-error';
        errorEl.textContent = message;
        this.element.insertBefore(errorEl, this.element.firstChild);

        setTimeout(() => errorEl.remove(), 5000);
    }

    /**
     * Reset form
     */
    reset() {
        this.element.reset();
        this.values = {};
    }

    /**
     * Get values
     * @returns {Object} Form values
     */
    getValues() {
        return { ...this.values };
    }
}
```

**Verification:**
- [ ] Form renders correctly
- [ ] Validation works
- [ ] Submit handler works
- [ ] All field types work

---

### Step 7: Create Overview View (js/views/Overview.js)

Implement overview dashboard view:

```javascript
/**
 * Overview View
 * 
 * Displays summary statistics and provider overview.
 */

import { View } from '../components/View.js';
import { createStatCard } from '../components/Card.js';
import { selectors, actions } from '../state/store.js';

export default class OverviewView extends View {
    async render(container) {
        await super.render(container);

        this.subscribe('providers', () => this.render());
        this.subscribe('credentials', () => this.render());
        this.subscribe('usage', () => this.render());

        this.renderContent();
    }

    renderContent() {
        const providers = this.store.get('providers');
        const credentials = this.store.get('credentials');
        const usage = this.store.get('usage');

        // Calculate stats
        const totalProviders = providers.length;
        const totalTokens = credentials.length;
        const healthyTokens = credentials.filter(c => c.healthy).length;
        const totalRequests = Object.values(usage).reduce((sum, u) => sum + (u.requests_today || 0), 0);

        this.container.innerHTML = `
            <div class="stats-grid">
                ${createStatCard({
                    label: 'Providers',
                    value: totalProviders,
                    icon: '🔑',
                    color: 'primary'
                }).outerHTML}
                ${createStatCard({
                    label: 'Tokens',
                    value: totalTokens,
                    icon: '🎫',
                    color: 'success'
                }).outerHTML}
                ${createStatCard({
                    label: 'Healthy Tokens',
                    value: healthyTokens,
                    icon: '✓',
                    color: 'success'
                }).outerHTML}
                ${createStatCard({
                    label: 'Requests Today',
                    value: totalRequests.toLocaleString(),
                    icon: '📊',
                    color: 'info'
                }).outerHTML}
            </div>

            <div class="providers-section">
                <h2>Providers</h2>
                <div class="providers-grid">
                    ${providers.map(provider => this.renderProviderCard(provider)).join('')}
                </div>
            </div>
        `;
    }

    renderProviderCard(provider) {
        const credentials = this.store.get('credentials').filter(c => c.provider === provider.id);
        const usage = this.store.get('usage')[provider.id] || {};

        return `
            <div class="card provider-card">
                <div class="provider-header">
                    <h3>${provider.name}</h3>
                    <span class="badge badge-info">${provider.flow}</span>
                </div>
                <div class="provider-stats">
                    <div class="provider-stat">
                        <span class="stat-label">Tokens</span>
                        <span class="stat-value">${credentials.length}</span>
                    </div>
                    <div class="provider-stat">
                        <span class="stat-label">Requests Today</span>
                        <span class="stat-value">${usage.requests_today || 0}</span>
                    </div>
                    <div class="provider-stat">
                        <span class="stat-label">Tokens/Min</span>
                        <span class="stat-value">${usage.tokens_in_minute || 0}</span>
                    </div>
                </div>
            </div>
        `;
    }
}
```

**Verification:**
- [ ] Overview renders correctly
- [ ] Stats display correctly
- [ ] Provider cards work
- [ ] Updates on state change

---

### Step 8: Create Placeholder Views

Create placeholder views for other sections:

```javascript
// js/views/Providers.js
import { View } from '../components/View.js';

export default class ProvidersView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Provider management coming soon', '🔑');
    }
}

// js/views/Tokens.js
import { View } from '../components/View.js';

export default class TokensView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Token management coming soon', '🎫');
    }
}

// js/views/Proxies.js
import { View } from '../components/View.js';

export default class ProxiesView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Proxy management coming soon', '🌐');
    }
}

// js/views/RateLimits.js
import { View } from '../components/View.js';

export default class RateLimitsView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Rate limit management coming soon', '⚡');
    }
}

// js/views/Usage.js
import { View } from '../components/View.js';

export default class UsageView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Usage tracking coming soon', '📈');
    }
}

// js/views/Settings.js
import { View } from '../components/View.js';

export default class SettingsView extends View {
    async render(container) {
        await super.render(container);
        this.showEmpty('Settings coming soon', '⚙️');
    }
}
```

**Verification:**
- [ ] All views load
- [ ] Empty states display
- [ ] Navigation works between views

---

## Testing Strategy

### Component Tests
- [ ] View base class works
- [ ] Modal opens/closes
- [ ] Toast notifications work
- [ ] Card renders correctly
- [ ] Table renders correctly
- [ ] Form validates correctly

### Integration Tests
- [ ] Components integrate with state
- [ ] Views render correctly
- [ ] Navigation works end-to-end

### Manual Testing
- [ ] All components render
- [ ] Interactions work
- [ ] Accessibility features work

---

## Verification Checklist

- [ ] All component files created
- [ ] View base class works
- [ ] Modal system works
- [ ] Toast system works
- [ ] Card component works
- [ ] Table component works
- [ ] Form component works
- [ ] Overview view works
- [ ] All placeholder views work
- [ ] Navigation works
- [ ] No console errors

---

## Next Steps

After completing Phase 3, proceed to **Phase 4: Provider & Token Management** to implement the provider and token management features.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
