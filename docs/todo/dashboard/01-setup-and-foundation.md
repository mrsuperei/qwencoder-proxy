# Phase 1: Setup & Foundation

**Priority:** CRITICAL  
**Estimated Time:** 1 day  
**Complexity:** Low  
**Files to Create:** 10  
**Files to Modify:** 0

---

## Executive Summary

This phase establishes the foundational structure for the new web2 dashboard. It creates the directory layout, base HTML structure, CSS framework, and JavaScript module system. The foundation is designed to be modular, maintainable, and fully static (no build step required).

---

## Problem Description

The current `/web/dashboard` has issues and needs to be replaced with a new implementation in `/web2`. A clean foundation is needed to avoid carrying forward any existing problems while establishing best practices for the new dashboard.

### Current State

**Existing Dashboard Issues:**
- Located in `/web/dashboard` (to be deprecated)
- Complex JavaScript structure with potential maintenance issues
- Unclear separation of concerns

**Requirements for New Dashboard:**
- Static HTML/JS only (no build step)
- Embedded in server (served when program launches)
- Located in `/web2` directory
- Clean, modular architecture
- Modern, responsive design

---

## Solution Architecture

### Design Principles

1. **No Build Step:** All JavaScript should be ES6 modules that work natively in modern browsers
2. **Modular Structure:** Clear separation between UI, data, and business logic
3. **Progressive Enhancement:** Core functionality works without JavaScript, enhanced with JS
4. **Responsive First:** Mobile-first design approach
5. **Accessibility:** WCAG 2.1 AA compliance

### Directory Structure

```
web2/
├── index.html                 # Main entry point
├── css/
│   ├── reset.css             # CSS reset and base styles
│   ├── variables.css         # CSS custom properties (colors, spacing)
│   ├── layout.css           # Grid and layout systems
│   ├── components.css        # Reusable UI components
│   ├── forms.css            # Form styling
│   ├── tables.css           # Table styling
│   ├── cards.css            # Card component styling
│   ├── modals.css          # Modal dialogs
│   ├── notifications.css     # Toasts and alerts
│   └── responsive.css       # Media queries
├── js/
│   ├── main.js             # Application entry point
│   ├── app.js              # Main application class
│   ├── api/
│   │   ├── client.js       # HTTP client wrapper
│   │   └── endpoints.js   # API endpoint definitions
│   ├── state/
│   │   └── store.js       # State management
│   ├── ui/
│   │   ├── router.js       # Client-side routing
│   │   └── renderer.js    # DOM manipulation helpers
│   └── utils/
│       ├── date.js         # Date formatting utilities
│       ├── format.js       # Number/string formatting
│       └── validation.js   # Form validation
└── assets/
    └── images/            # Static images and icons
```

### Technology Stack

- **HTML5:** Semantic markup
- **CSS3:** Custom properties, Grid, Flexbox
- **JavaScript ES6+:** Modules, async/await, classes
- **No Frameworks:** Vanilla JS for simplicity and performance
- **Icons:** SVG icons (inline for performance)

---

## Implementation Plan

### Step 1: Create Directory Structure

Create the complete directory structure for `/web2`:

```bash
mkdir -p web2/css
mkdir -p web2/js/api
mkdir -p web2/js/state
mkdir -p web2/js/ui
mkdir -p web2/js/utils
mkdir -p web2/assets/images
```

**Verification:**
- [ ] All directories created
- [ ] Directory structure matches plan

---

### Step 2: Create Base HTML (index.html)

Create the main HTML entry point with semantic structure:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="description" content="QWEncoder Proxy Dashboard - Manage OAuth tokens, proxies, and rate limits">
    <title>QWEncoder Proxy Dashboard</title>
    
    <!-- CSS Files -->
    <link rel="stylesheet" href="css/reset.css">
    <link rel="stylesheet" href="css/variables.css">
    <link rel="stylesheet" href="css/layout.css">
    <link rel="stylesheet" href="css/components.css">
    <link rel="stylesheet" href="css/forms.css">
    <link rel="stylesheet" href="css/tables.css">
    <link rel="stylesheet" href="css/cards.css">
    <link rel="stylesheet" href="css/modals.css">
    <link rel="stylesheet" href="css/notifications.css">
    <link rel="stylesheet" href="css/responsive.css">
</head>
<body>
    <!-- App Container -->
    <div id="app">
        <!-- Loading State -->
        <div id="loading" class="loading-screen">
            <div class="spinner"></div>
            <p>Loading dashboard...</p>
        </div>
        
        <!-- Error State -->
        <div id="error" class="error-screen hidden">
            <h2>Dashboard Error</h2>
            <p id="errorMessage">Unable to load dashboard</p>
            <button onclick="location.reload()">Retry</button>
        </div>
        
        <!-- Main App (hidden until loaded) -->
        <div id="main" class="hidden">
            <!-- Header -->
            <header class="app-header">
                <div class="header-brand">
                    <span class="logo-icon">🔐</span>
                    <h1>QWEncoder Proxy</h1>
                </div>
                <div class="header-actions">
                    <button class="btn-icon" id="refreshBtn" title="Refresh">
                        <svg>...</svg>
                    </button>
                    <button class="btn-icon" id="themeBtn" title="Toggle theme">
                        <svg>...</svg>
                    </button>
                </div>
            </header>
            
            <!-- Navigation -->
            <nav class="app-nav">
                <button class="nav-item active" data-view="overview">
                    <span class="nav-icon">📊</span>
                    <span class="nav-label">Overview</span>
                </button>
                <button class="nav-item" data-view="providers">
                    <span class="nav-icon">🔑</span>
                    <span class="nav-label">Providers</span>
                </button>
                <button class="nav-item" data-view="tokens">
                    <span class="nav-icon">🎫</span>
                    <span class="nav-label">Tokens</span>
                </button>
                <button class="nav-item" data-view="proxies">
                    <span class="nav-icon">🌐</span>
                    <span class="nav-label">Proxies</span>
                </button>
                <button class="nav-item" data-view="ratelimits">
                    <span class="nav-icon">⚡</span>
                    <span class="nav-label">Rate Limits</span>
                </button>
                <button class="nav-item" data-view="usage">
                    <span class="nav-icon">📈</span>
                    <span class="nav-label">Usage</span>
                </button>
                <button class="nav-item" data-view="settings">
                    <span class="nav-icon">⚙️</span>
                    <span class="nav-label">Settings</span>
                </button>
            </nav>
            
            <!-- Main Content -->
            <main class="app-content">
                <div id="viewContainer">
                    <!-- Views will be dynamically loaded here -->
                </div>
            </main>
            
            <!-- Footer -->
            <footer class="app-footer">
                <p>QWEncoder Proxy Dashboard v1.0</p>
            </footer>
        </div>
    </div>
    
    <!-- Notification Container -->
    <div id="notifications" class="notification-container"></div>
    
    <!-- Modal Container -->
    <div id="modals" class="modal-container"></div>
    
    <!-- JavaScript Entry Point -->
    <script type="module" src="js/main.js"></script>
</body>
</html>
```

**Key Features:**
- Semantic HTML5 structure
- Progressive loading states
- Accessibility attributes
- No external dependencies
- Module-based JavaScript

**Verification:**
- [ ] HTML validates
- [ ] All CSS files linked
- [ ] JavaScript module properly referenced
- [ ] Accessibility attributes present

---

### Step 3: Create CSS Reset (css/reset.css)

Implement a modern CSS reset:

```css
/* CSS Reset - Normalize browser defaults */

/* Box Sizing */
*, *::before, *::after {
    box-sizing: border-box;
}

/* Remove Margins */
body, h1, h2, h3, h4, h5, h6, p, ul, ol, li, figure, figcaption,
blockquote, dl, dd {
    margin: 0;
}

/* Typography */
html {
    -ms-text-size-adjust: 100%;
    -webkit-text-size-adjust: 100%;
}

body {
    line-height: 1.5;
    font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}

/* Links */
a {
    color: inherit;
    text-decoration: none;
}

/* Images */
img, picture, video, canvas, svg {
    display: block;
    max-width: 100%;
}

/* Forms */
input, button, textarea, select {
    font: inherit;
}

button {
    cursor: pointer;
    border: none;
    background: none;
}

/* Lists */
ul, ol {
    list-style: none;
}

/* Tables */
table {
    border-collapse: collapse;
    border-spacing: 0;
}

/* Utilities */
.hidden {
    display: none !important;
}

.sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
}
```

**Verification:**
- [ ] Reset applied consistently
- [ ] Browser defaults normalized
- [ ] Utility classes work

---

### Step 4: Create CSS Variables (css/variables.css)

Define design tokens using CSS custom properties:

```css
:root {
    /* Colors - Light Theme */
    --color-primary: #2563eb;
    --color-primary-hover: #1d4ed8;
    --color-primary-light: #dbeafe;
    
    --color-success: #10b981;
    --color-success-light: #d1fae5;
    
    --color-warning: #f59e0b;
    --color-warning-light: #fef3c7;
    
    --color-danger: #ef4444;
    --color-danger-light: #fee2e2;
    
    --color-info: #3b82f6;
    --color-info-light: #dbeafe;
    
    /* Neutral Colors */
    --color-bg-primary: #ffffff;
    --color-bg-secondary: #f8fafc;
    --color-bg-tertiary: #f1f5f9;
    
    --color-text-primary: #0f172a;
    --color-text-secondary: #475569;
    --color-text-tertiary: #94a3b8;
    
    --color-border: #e2e8f0;
    --color-border-hover: #cbd5e1;
    
    /* Spacing */
    --space-xs: 0.25rem;   /* 4px */
    --space-sm: 0.5rem;    /* 8px */
    --space-md: 1rem;      /* 16px */
    --space-lg: 1.5rem;    /* 24px */
    --space-xl: 2rem;      /* 32px */
    --space-2xl: 3rem;    /* 48px */
    
    /* Typography */
    --font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    --font-size-xs: 0.75rem;   /* 12px */
    --font-size-sm: 0.875rem;  /* 14px */
    --font-size-md: 1rem;       /* 16px */
    --font-size-lg: 1.125rem;  /* 18px */
    --font-size-xl: 1.25rem;    /* 20px */
    --font-size-2xl: 1.5rem;   /* 24px */
    --font-size-3xl: 1.875rem; /* 30px */
    
    --font-weight-normal: 400;
    --font-weight-medium: 500;
    --font-weight-semibold: 600;
    --font-weight-bold: 700;
    
    --line-height-tight: 1.25;
    --line-height-normal: 1.5;
    --line-height-relaxed: 1.75;
    
    /* Border Radius */
    --radius-sm: 0.25rem;  /* 4px */
    --radius-md: 0.375rem; /* 6px */
    --radius-lg: 0.5rem;   /* 8px */
    --radius-xl: 0.75rem;  /* 12px */
    --radius-full: 9999px;
    
    /* Shadows */
    --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
    --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1);
    --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1);
    --shadow-xl: 0 20px 25px -5px rgb(0 0 0 / 0.1);
    
    /* Transitions */
    --transition-fast: 150ms ease;
    --transition-normal: 250ms ease;
    --transition-slow: 350ms ease;
    
    /* Z-Index */
    --z-dropdown: 100;
    --z-sticky: 200;
    --z-modal: 1000;
    --z-notification: 1100;
}

/* Dark Theme */
@media (prefers-color-scheme: dark) {
    :root {
        --color-bg-primary: #0f172a;
        --color-bg-secondary: #1e293b;
        --color-bg-tertiary: #334155;
        
        --color-text-primary: #f8fafc;
        --color-text-secondary: #cbd5e1;
        --color-text-tertiary: #64748b;
        
        --color-border: #334155;
        --color-border-hover: #475569;
    }
}

[data-theme="dark"] {
    --color-bg-primary: #0f172a;
    --color-bg-secondary: #1e293b;
    --color-bg-tertiary: #334155;
    
    --color-text-primary: #f8fafc;
    --color-text-secondary: #cbd5e1;
    --color-text-tertiary: #64748b;
    
    --color-border: #334155;
    --color-border-hover: #475569;
}
```

**Verification:**
- [ ] All variables defined
- [ ] Dark theme works
- [ ] Values are consistent

---

### Step 5: Create Layout CSS (css/layout.css)

Implement the main layout system:

```css
/* Main Layout Structure */

#app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

/* Loading Screen */
.loading-screen {
    position: fixed;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: var(--color-bg-primary);
    z-index: var(--z-modal);
}

.spinner {
    width: 48px;
    height: 48px;
    border: 4px solid var(--color-border);
    border-top-color: var(--color-primary);
    border-radius: 50%;
    animation: spin 1s linear infinite;
}

@keyframes spin {
    to { transform: rotate(360deg); }
}

/* Error Screen */
.error-screen {
    position: fixed;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: var(--color-bg-primary);
    z-index: var(--z-modal);
    padding: var(--space-lg);
    text-align: center;
}

/* Header */
.app-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-md) var(--space-lg);
    background: var(--color-bg-primary);
    border-bottom: 1px solid var(--color-border);
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
}

.header-brand {
    display: flex;
    align-items: center;
    gap: var(--space-md);
}

.logo-icon {
    font-size: var(--font-size-2xl);
}

.header-brand h1 {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
}

.header-actions {
    display: flex;
    gap: var(--space-sm);
}

/* Navigation */
.app-nav {
    display: flex;
    gap: var(--space-xs);
    padding: var(--space-md) var(--space-lg);
    background: var(--color-bg-secondary);
    border-bottom: 1px solid var(--color-border);
    overflow-x: auto;
}

.nav-item {
    display: flex;
    align-items: center;
    gap: var(--space-sm);
    padding: var(--space-sm) var(--space-md);
    background: transparent;
    border-radius: var(--radius-md);
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    white-space: nowrap;
    transition: all var(--transition-fast);
}

.nav-item:hover {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
}

.nav-item.active {
    background: var(--color-primary);
    color: white;
}

.nav-icon {
    font-size: var(--font-size-lg);
}

/* Main Content */
.app-content {
    flex: 1;
    padding: var(--space-lg);
    background: var(--color-bg-secondary);
}

#viewContainer {
    max-width: 1400px;
    margin: 0 auto;
}

/* Footer */
.app-footer {
    padding: var(--space-md) var(--space-lg);
    background: var(--color-bg-primary);
    border-top: 1px solid var(--color-border);
    text-align: center;
    color: var(--color-text-tertiary);
    font-size: var(--font-size-sm);
}

/* Notification Container */
.notification-container {
    position: fixed;
    top: var(--space-lg);
    right: var(--space-lg);
    z-index: var(--z-notification);
    display: flex;
    flex-direction: column;
    gap: var(--space-sm);
    pointer-events: none;
}

.notification-container > * {
    pointer-events: auto;
}

/* Modal Container */
.modal-container {
    position: fixed;
    inset: 0;
    z-index: var(--z-modal);
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
}

.modal-container > * {
    pointer-events: auto;
}
```

**Verification:**
- [ ] Layout renders correctly
- [ ] Sticky header works
- [ ] Navigation scrolls on mobile
- [ ] Footer stays at bottom

---

### Step 6: Create Components CSS (css/components.css)

Define reusable UI components:

```css
/* Buttons */
.btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-sm);
    padding: var(--space-sm) var(--space-lg);
    border-radius: var(--radius-md);
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    transition: all var(--transition-fast);
    cursor: pointer;
    border: none;
}

.btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
}

.btn-primary {
    background: var(--color-primary);
    color: white;
}

.btn-primary:hover:not(:disabled) {
    background: var(--color-primary-hover);
}

.btn-secondary {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
}

.btn-secondary:hover:not(:disabled) {
    background: var(--color-border);
}

.btn-danger {
    background: var(--color-danger);
    color: white;
}

.btn-danger:hover:not(:disabled) {
    background: #dc2626;
}

.btn-success {
    background: var(--color-success);
    color: white;
}

.btn-success:hover:not(:disabled) {
    background: #059669;
}

.btn-sm {
    padding: var(--space-xs) var(--space-md);
    font-size: var(--font-size-xs);
}

.btn-lg {
    padding: var(--space-md) var(--space-xl);
    font-size: var(--font-size-md);
}

.btn-icon {
    padding: var(--space-sm);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-secondary);
    transition: all var(--transition-fast);
}

.btn-icon:hover {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
}

/* Badges */
.badge {
    display: inline-flex;
    align-items: center;
    padding: var(--space-xs) var(--space-sm);
    border-radius: var(--radius-full);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
}

.badge-success {
    background: var(--color-success-light);
    color: #065f46;
}

.badge-warning {
    background: var(--color-warning-light);
    color: #92400e;
}

.badge-danger {
    background: var(--color-danger-light);
    color: #991b1b;
}

.badge-info {
    background: var(--color-info-light);
    color: #1e40af;
}

/* Status Indicators */
.status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    display: inline-block;
}

.status-dot.healthy {
    background: var(--color-success);
}

.status-dot.unhealthy {
    background: var(--color-danger);
}

.status-dot.pending {
    background: var(--color-warning);
}

/* Cards */
.card {
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    padding: var(--space-lg);
    box-shadow: var(--shadow-sm);
}

.card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-md);
}

.card-title {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
}

.card-body {
    color: var(--color-text-secondary);
}

.card-footer {
    margin-top: var(--space-lg);
    padding-top: var(--space-lg);
    border-top: 1px solid var(--color-border);
}
```

**Verification:**
- [ ] All button variants work
- [ ] Badges display correctly
- [ ] Status indicators visible
- [ ] Cards render properly

---

### Step 7: Create Forms CSS (css/forms.css)

Style form elements:

```css
/* Form Groups */
.form-group {
    margin-bottom: var(--space-lg);
}

.form-group label {
    display: block;
    margin-bottom: var(--space-sm);
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    color: var(--color-text-primary);
}

.form-group .help-text {
    margin-top: var(--space-xs);
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
}

/* Inputs */
.input {
    width: 100%;
    padding: var(--space-sm) var(--space-md);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    transition: border-color var(--transition-fast);
}

.input:focus {
    outline: none;
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-light);
}

.input:disabled {
    background: var(--color-bg-secondary);
    cursor: not-allowed;
}

.input.error {
    border-color: var(--color-danger);
}

.input.error:focus {
    box-shadow: 0 0 0 3px var(--color-danger-light);
}

/* Select */
.select {
    width: 100%;
    padding: var(--space-sm) var(--space-md);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    cursor: pointer;
    appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%2364748b' d='M4 6l4 4 4-4'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right var(--space-sm) center;
    padding-right: var(--space-2xl);
}

.select:focus {
    outline: none;
    border-color: var(--color-primary);
}

/* Textarea */
.textarea {
    width: 100%;
    padding: var(--space-sm) var(--space-md);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    font-family: var(--font-family);
    resize: vertical;
    min-height: 100px;
}

.textarea:focus {
    outline: none;
    border-color: var(--color-primary);
}

/* Checkbox */
.checkbox-group {
    display: flex;
    align-items: center;
    gap: var(--space-sm);
}

.checkbox {
    width: 18px;
    height: 18px;
    border: 2px solid var(--color-border);
    border-radius: var(--radius-sm);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
}

.checkbox.checked {
    background: var(--color-primary);
    border-color: var(--color-primary);
}

.checkbox.checked::after {
    content: '✓';
    color: white;
    font-size: 12px;
}

/* Form Grid */
.form-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: var(--space-lg);
}

/* Form Actions */
.form-actions {
    display: flex;
    gap: var(--space-md);
    margin-top: var(--space-xl);
    justify-content: flex-end;
}

/* Validation Messages */
.validation-message {
    margin-top: var(--space-xs);
    font-size: var(--font-size-xs);
    color: var(--color-danger);
}

.validation-message.hidden {
    display: none;
}
```

**Verification:**
- [ ] All input types styled
- [ ] Focus states visible
- [ ] Error states work
- [ ] Form grid responsive

---

### Step 8: Create Tables CSS (css/tables.css)

Style data tables:

```css
/* Table Container */
.table-container {
    overflow-x: auto;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    background: var(--color-bg-primary);
}

/* Table */
.table {
    width: 100%;
    border-collapse: collapse;
}

.table th,
.table td {
    padding: var(--space-md);
    text-align: left;
    border-bottom: 1px solid var(--color-border);
}

.table th {
    background: var(--color-bg-secondary);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    position: sticky;
    top: 0;
}

.table td {
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
}

.table tr:last-child td {
    border-bottom: none;
}

.table tbody tr:hover {
    background: var(--color-bg-secondary);
}

/* Table Actions */
.table-actions {
    display: flex;
    gap: var(--space-xs);
}

/* Table Empty State */
.table-empty {
    padding: var(--space-2xl);
    text-align: center;
    color: var(--color-text-tertiary);
}

.table-empty-icon {
    font-size: var(--font-size-3xl);
    margin-bottom: var(--space-md);
}

/* Table Sortable */
.table-sortable {
    cursor: pointer;
    user-select: none;
}

.table-sortable:hover {
    background: var(--color-bg-tertiary);
}

.table-sortable::after {
    content: '↕';
    margin-left: var(--space-xs);
    opacity: 0.3;
}

.table-sortable.asc::after {
    content: '↑';
    opacity: 1;
}

.table-sortable.desc::after {
    content: '↓';
    opacity: 1;
}
```

**Verification:**
- [ ] Table renders correctly
- [ ] Headers sticky
- [ ] Hover states work
- [ ] Responsive scrolling

---

### Step 9: Create Modals CSS (css/modals.css)

Style modal dialogs:

```css
/* Modal Overlay */
.modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-lg);
    animation: fadeIn var(--transition-normal);
}

@keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
}

/* Modal */
.modal {
    background: var(--color-bg-primary);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-xl);
    max-width: 500px;
    width: 100%;
    max-height: 90vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    animation: slideUp var(--transition-normal);
}

@keyframes slideUp {
    from {
        transform: translateY(20px);
        opacity: 0;
    }
    to {
        transform: translateY(0);
        opacity: 1;
    }
}

.modal-lg {
    max-width: 700px;
}

.modal-xl {
    max-width: 900px;
}

/* Modal Header */
.modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-lg);
    border-bottom: 1px solid var(--color-border);
}

.modal-title {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
}

.modal-close {
    background: transparent;
    border: none;
    color: var(--color-text-secondary);
    cursor: pointer;
    padding: var(--space-sm);
    border-radius: var(--radius-md);
    transition: all var(--transition-fast);
}

.modal-close:hover {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
}

/* Modal Body */
.modal-body {
    padding: var(--space-lg);
    overflow-y: auto;
    flex: 1;
}

/* Modal Footer */
.modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-md);
    padding: var(--space-lg);
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
}
```

**Verification:**
- [ ] Modal opens/closes smoothly
- [ ] Overlay blocks interaction
- [ ] Content scrolls when needed
- [ ] Size variants work

---

### Step 10: Create Notifications CSS (css/notifications.css)

Style toast notifications:

```css
/* Toast Notification */
.toast {
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    padding: var(--space-md) var(--space-lg);
    box-shadow: var(--shadow-lg);
    display: flex;
    align-items: flex-start;
    gap: var(--space-md);
    min-width: 300px;
    max-width: 400px;
    animation: slideInRight var(--transition-normal);
}

@keyframes slideInRight {
    from {
        transform: translateX(100%);
        opacity: 0;
    }
    to {
        transform: translateX(0);
        opacity: 1;
    }
}

.toast.hiding {
    animation: slideOutRight var(--transition-normal) forwards;
}

@keyframes slideOutRight {
    from {
        transform: translateX(0);
        opacity: 1;
    }
    to {
        transform: translateX(100%);
        opacity: 0;
    }
}

/* Toast Types */
.toast-success {
    border-left: 4px solid var(--color-success);
}

.toast-error {
    border-left: 4px solid var(--color-danger);
}

.toast-warning {
    border-left: 4px solid var(--color-warning);
}

.toast-info {
    border-left: 4px solid var(--color-info);
}

/* Toast Icon */
.toast-icon {
    font-size: var(--font-size-xl);
    flex-shrink: 0;
}

/* Toast Content */
.toast-content {
    flex: 1;
}

.toast-title {
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
    margin-bottom: var(--space-xs);
}

.toast-message {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
}

/* Toast Close */
.toast-close {
    background: transparent;
    border: none;
    color: var(--color-text-tertiary);
    cursor: pointer;
    padding: var(--space-xs);
    flex-shrink: 0;
}

.toast-close:hover {
    color: var(--color-text-primary);
}
```

**Verification:**
- [ ] Toasts animate in/out
- [ ] Different types styled
- [ ] Close button works
- [ ] Multiple toasts stack

---

### Step 11: Create Responsive CSS (css/responsive.css)

Add responsive breakpoints:

```css
/* Mobile First - Base styles are mobile */

/* Tablet (768px and up) */
@media (min-width: 768px) {
    .app-header {
        padding: var(--space-md) var(--space-xl);
    }
    
    .app-nav {
        padding: var(--space-md) var(--space-xl);
    }
    
    .app-content {
        padding: var(--space-xl);
    }
}

/* Desktop (1024px and up) */
@media (min-width: 1024px) {
    .app-content {
        padding: var(--space-2xl);
    }
}

/* Large Desktop (1280px and up) */
@media (min-width: 1280px) {
    .app-header {
        padding: var(--space-lg) var(--space-2xl);
    }
    
    .app-nav {
        padding: var(--space-lg) var(--space-2xl);
    }
}

/* Print Styles */
@media print {
    .app-nav,
    .app-footer,
    .header-actions,
    .notification-container,
    .modal-container {
        display: none;
    }
    
    .app-content {
        padding: 0;
    }
}
```

**Verification:**
- [ ] Mobile layout works
- [ ] Tablet layout works
- [ ] Desktop layout works
- [ ] Print styles correct

---

## Testing Strategy

### Visual Testing
- [ ] All CSS files load without errors
- [ ] No visual regressions across browsers
- [ ] Dark/light theme toggles correctly

### Cross-Browser Testing
- [ ] Chrome/Edge (Chromium)
- [ ] Firefox
- [ ] Safari

### Responsive Testing
- [ ] Mobile (320px - 480px)
- [ ] Tablet (768px - 1024px)
- [ ] Desktop (1024px+)

### Accessibility Testing
- [ ] Keyboard navigation works
- [ ] Screen reader announces elements
- [ ] Color contrast meets WCAG AA

---

## Verification Checklist

- [ ] All directories created
- [ ] index.html created and validates
- [ ] All CSS files created
- [ ] CSS variables work
- [ ] Dark theme works
- [ ] Layout renders correctly
- [ ] Components styled properly
- [ ] Forms styled properly
- [ ] Tables styled properly
- [ ] Modals styled properly
- [ ] Notifications styled properly
- [ ] Responsive design works
- [ ] Cross-browser compatible
- [ ] Accessibility compliant

---

## Next Steps

After completing Phase 1, proceed to **Phase 2: API Client & State Management** to implement the JavaScript infrastructure for API communication and state management.

---

**Last Updated:** 2026-03-19  
**Version:** 1.0
