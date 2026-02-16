# Dynamic Model Management Implementation Plan

## Overview

This plan implements dynamic model management to replace hardcoded model lists. The current implementation has models hardcoded in each provider's Go file, requiring code changes and redeployment when providers add/remove models.

## Problem Statement

**Current State**:
- Models are hardcoded in provider files (e.g., `provider/gemini/gemini.go:26-34`)
- Each provider has a `SupportedModels` array
- Adding/removing models requires:
  1. Editing provider file
  2. Recompiling application
  3. Redeploying server
- Dashboard has no model management UI

**Existing Infrastructure** (Partial):
- Provider interface has `ListModels()` and `SupportsModel()` methods
- Factory has `PopulateModelProviders()` and `RefreshProviderModels()` methods
- Some providers (Antigravity) have `cachedModels` for dynamic discovery
- Dashboard has provider settings but no model management

## Solution Architecture

### Hybrid Approach

```
┌─────────────────────────────────────────────────────────────────┐
│  Model Sources                                        │
├─────────────────────────────────────────────────────────────────┤
│  1. Hardcoded Defaults (fallback)                     │
│  2. Provider API Discovery (auto)                    │
│  3. Custom User Overrides (manual)                      │
└─────────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│  Model Storage Layer                                  │
│  - JSON file or database                              │
│  - Per-provider model lists                             │
│  - Enabled/disabled flags                               │
│  - Custom metadata (display name, description)              │
└─────────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│  API Layer                                           │
│  - GET    /api/models                                │
│  - POST   /api/models                                │
│  - DELETE /api/models/{id}                             │
│  - PUT    /api/models/{id}                             │
│  - POST   /api/providers/{id}/refresh-models             │
└─────────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│  Dashboard UI                                         │
│  - Models tab                                          │
│  - Per-provider model lists                            │
│  - Add/remove models                                    │
│  - Enable/disable models                                 │
│  - Refresh from provider API                             │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Steps

### Step 1: Create Model Storage Layer

#### 1.1 Define Model Data Structures

**File**: `internal/model/model.go`

```go
package model

import (
    "time"
)

// Model represents a model configuration
type Model struct {
    ID          string    `json:"id"`           // Unique model identifier (e.g., "gemini-2.5-flash")
    ProviderID  string    `json:"provider_id"`  // Provider that supports this model
    DisplayName string    `json:"display_name"` // User-friendly name
    Description string    `json:"description"` // Model description
    Enabled     bool      `json:"enabled"`      // Whether model is enabled
    Source      string    `json:"source"`       // "default", "discovered", "custom"
    CreatedAt   time.Time `json:"created_at"`   // When model was added
    UpdatedAt   time.Time `json:"updated_at"`   // When model was last updated
    Metadata    map[string]interface{} `json:"metadata,omitempty"` // Additional provider-specific data
}

// ModelList represents a list of models for a provider
type ModelList struct {
    ProviderID string  `json:"provider_id"`
    Models     []Model `json:"models"`
}

// ModelStore defines the interface for model storage
type ModelStore interface {
    // GetModels returns all models for a provider
    GetModels(providerID string) ([]Model, error)
    
    // GetModel returns a specific model by ID
    GetModel(providerID, modelID string) (*Model, error)
    
    // AddModel adds a new model
    AddModel(providerID string, model Model) error
    
    // RemoveModel removes a model
    RemoveModel(providerID, modelID string) error
    
    // UpdateModel updates an existing model
    UpdateModel(providerID string, model Model) error
    
    // EnableModel enables a model
    EnableModel(providerID, modelID string) error
    
    // DisableModel disables a model
    DisableModel(providerID, modelID string) error
    
    // ClearModels removes all models for a provider
    ClearModels(providerID string) error
}
```

#### 1.2 Implement JSON File-Based Storage

**File**: `internal/model/store.go`

```go
package model

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
)

const (
    // ModelDir is the directory for model storage
    ModelDir = ".models"
    // ModelFile is the filename for provider models
    ModelFile = "models.json"
)

// JSONModelStore implements ModelStore using JSON files
type JSONModelStore struct {
    mu      sync.RWMutex
    logger  logging.Logger
    baseDir string
}

// NewJSONModelStore creates a new JSON-based model store
func NewJSONModelStore(logger logging.Logger) *JSONModelStore {
    return &JSONModelStore{
        logger:  logger,
        baseDir: ModelDir,
    }
}

// GetModels returns all models for a provider
func (s *JSONModelStore) GetModels(providerID string) ([]Model, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    providerPath := filepath.Join(s.baseDir, providerID, ModelFile)
    data, err := os.ReadFile(providerPath)
    if err != nil {
        if os.IsNotExist(err) {
            // No models file yet, return empty list
            return []Model{}, nil
        }
        return nil, fmt.Errorf("failed to read models file: %w", err)
    }
    
    var modelList ModelList
    if err := json.Unmarshal(data, &modelList); err != nil {
        return nil, fmt.Errorf("failed to parse models file: %w", err)
    }
    
    return modelList.Models, nil
}

// GetModel returns a specific model by ID
func (s *JSONModelStore) GetModel(providerID, modelID string) (*Model, error) {
    models, err := s.GetModels(providerID)
    if err != nil {
        return nil, err
    }
    
    for _, model := range models {
        if model.ID == modelID {
            return &model, nil
        }
    }
    
    return nil, fmt.Errorf("model not found: %s", modelID)
}

// AddModel adds a new model
func (s *JSONModelStore) AddModel(providerID string, model Model) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get existing models
    models, err := s.GetModels(providerID)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Check if model already exists
    for _, m := range models {
        if m.ID == model.ID {
            return fmt.Errorf("model already exists: %s", model.ID)
        }
    }
    
    // Add new model
    models = append(models, model)
    
    // Save
    return s.saveModels(providerID, models)
}

// RemoveModel removes a model
func (s *JSONModelStore) RemoveModel(providerID, modelID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get existing models
    models, err := s.GetModels(providerID)
    if err != nil {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Find and remove model
    found := false
    updated := make([]Model, 0, len(models)-1)
    for _, m := range models {
        if m.ID == modelID {
            found = true
            continue
        }
        updated = append(updated, m)
    }
    
    if !found {
        return fmt.Errorf("model not found: %s", modelID)
    }
    
    // Save
    return s.saveModels(providerID, updated)
}

// UpdateModel updates an existing model
func (s *JSONModelStore) UpdateModel(providerID string, model Model) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get existing models
    models, err := s.GetModels(providerID)
    if err != nil {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Find and update model
    found := false
    for i, m := range models {
        if m.ID == model.ID {
            models[i] = model
            found = true
            break
        }
    }
    
    if !found {
        return fmt.Errorf("model not found: %s", model.ID)
    }
    
    // Save
    return s.saveModels(providerID, models)
}

// EnableModel enables a model
func (s *JSONModelStore) EnableModel(providerID, modelID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get existing models
    models, err := s.GetModels(providerID)
    if err != nil {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Find and enable model
    found := false
    for i, m := range models {
        if m.ID == modelID {
            models[i].Enabled = true
            models[i].UpdatedAt = time.Now()
            found = true
            break
        }
    }
    
    if !found {
        return fmt.Errorf("model not found: %s", modelID)
    }
    
    // Save
    return s.saveModels(providerID, models)
}

// DisableModel disables a model
func (s *JSONModelStore) DisableModel(providerID, modelID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Get existing models
    models, err := s.GetModels(providerID)
    if err != nil {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Find and disable model
    found := false
    for i, m := range models {
        if m.ID == modelID {
            models[i].Enabled = false
            models[i].UpdatedAt = time.Now()
            found = true
            break
        }
    }
    
    if !found {
        return fmt.Errorf("model not found: %s", modelID)
    }
    
    // Save
    return s.saveModels(providerID, models)
}

// ClearModels removes all models for a provider
func (s *JSONModelStore) ClearModels(providerID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    providerPath := filepath.Join(s.baseDir, providerID, ModelFile)
    
    // Remove the file
    if err := os.Remove(providerPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to clear models: %w", err)
    }
    
    return nil
}

// saveModels saves models to JSON file
func (s *JSONModelStore) saveModels(providerID string, models []Model) error {
    // Create provider directory
    providerPath := filepath.Join(s.baseDir, providerID)
    if err := os.MkdirAll(providerPath, 0755); err != nil {
        return fmt.Errorf("failed to create provider directory: %w", err)
    }
    
    filePath := filepath.Join(providerPath, ModelFile)
    
    // Marshal to JSON
    data, err := json.MarshalIndent(ModelList{
        ProviderID: providerID,
        Models:     models,
    }, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal models: %w", err)
    }
    
    // Write to file
    if err := os.WriteFile(filePath, data, 0644); err != nil {
        return fmt.Errorf("failed to write models file: %w", err)
    }
    
    s.logger.DebugLog("[ModelStore] Saved %d models for provider %s", len(models), providerID)
    return nil
}
```

#### 1.3 Initialize Default Models

**File**: `internal/model/defaults.go`

```go
package model

import (
    "time"
)

// DefaultModels returns the default models for each provider
// These are used as fallback when no custom models are configured
func DefaultModels() map[string][]Model {
    return map[string][]Model{
        "gemini-cli": {
            {
                ID:          "gemini-2.5-flash",
                DisplayName:  "Gemini 2.5 Flash",
                Description:  "Fast, efficient model for quick responses",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            {
                ID:          "gemini-2.5-flash-lite",
                DisplayName:  "Gemini 2.5 Flash Lite",
                Description:  "Lightweight version of Flash",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            {
                ID:          "gemini-2.5-pro",
                DisplayName:  "Gemini 2.5 Pro",
                Description:  "Advanced model for complex tasks",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            // ... more default models
        },
        "kiro": {
            {
                ID:          "claude-opus-4-5",
                DisplayName:  "Claude Opus 4.5",
                Description:  "Most capable Claude model",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            // ... more default models
        },
        "qwen": {
            {
                ID:          "qwen3-coder-plus",
                DisplayName:  "Qwen 3 Coder Plus",
                Description:  "Code-focused model",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            // ... more default models
        },
        "antigravity": {
            {
                ID:          "gemini-3-pro-preview",
                DisplayName:  "Gemini 3 Pro Preview",
                Description:  "Latest Gemini model via Antigravity",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            // ... more default models
        },
        "iflow": {
            {
                ID:          "glm-4.6",
                DisplayName:  "GLM 4.6",
                Description:  "General Language Model",
                Enabled:     true,
                Source:      "default",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            },
            // ... more default models
        },
    }
}

// InitializeDefaults initializes default models for all providers
func InitializeDefaults(store ModelStore) error {
    defaults := DefaultModels()
    
    for providerID, models := range defaults {
        for _, model := range models {
            if err := store.AddModel(providerID, model); err != nil {
                // Model might already exist, that's okay
                // We just want to ensure defaults are present
                continue
            }
        }
    }
    
    return nil
}
```

### Step 2: Create Model Manager

**File**: `internal/model/manager.go`

```go
package model

import (
    "context"
    "fmt"
    "sync"
    
    "github.com/sunbankio/qwencoder-proxy/logging"
    "github.com/sunbankio/qwencoder-proxy/provider"
)

// Manager manages model discovery and storage
type Manager struct {
    store    ModelStore
    providers map[string]provider.Provider
    mu        sync.RWMutex
    logger    logging.Logger
}

// NewManager creates a new model manager
func NewManager(store ModelStore, logger logging.Logger) *Manager {
    return &Manager{
        store:    store,
        providers: make(map[string]provider.Provider),
        logger:    logger,
    }
}

// RegisterProvider registers a provider for model discovery
func (m *Manager) RegisterProvider(providerID string, p provider.Provider) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.providers[providerID] = p
    m.logger.InfoLog("[ModelManager] Registered provider: %s", providerID)
}

// GetModels returns all enabled models for a provider
func (m *Manager) GetModels(providerID string) ([]Model, error) {
    models, err := m.store.GetModels(providerID)
    if err != nil {
        return nil, fmt.Errorf("failed to get models: %w", err)
    }
    
    // Filter enabled models
    enabled := make([]Model, 0, len(models))
    for _, model := range models {
        if model.Enabled {
            enabled = append(enabled, model)
        }
    }
    
    return enabled, nil
}

// GetAllModels returns all models (including disabled) for a provider
func (m *Manager) GetAllModels(providerID string) ([]Model, error) {
    return m.store.GetModels(providerID)
}

// AddModel adds a custom model
func (m *Manager) AddModel(providerID string, model Model) error {
    model.Source = "custom"
    model.CreatedAt = time.Now()
    model.UpdatedAt = time.Now()
    
    return m.store.AddModel(providerID, model)
}

// RemoveModel removes a model
func (m *Manager) RemoveModel(providerID, modelID string) error {
    return m.store.RemoveModel(providerID, modelID)
}

// UpdateModel updates a model
func (m *Manager) UpdateModel(providerID string, model Model) error {
    model.UpdatedAt = time.Now()
    return m.store.UpdateModel(providerID, model)
}

// EnableModel enables a model
func (m *Manager) EnableModel(providerID, modelID string) error {
    return m.store.EnableModel(providerID, modelID)
}

// DisableModel disables a model
func (m *Manager) DisableModel(providerID, modelID string) error {
    return m.store.DisableModel(providerID, modelID)
}

// RefreshFromProvider discovers models from provider API
func (m *Manager) RefreshFromProvider(ctx context.Context, providerID string) error {
    m.mu.RLock()
    p, ok := m.providers[providerID]
    m.mu.RUnlock()
    
    if !ok {
        return fmt.Errorf("provider not found: %s", providerID)
    }
    
    // Discover models from provider
    modelsData, err := p.ListModels(ctx)
    if err != nil {
        return fmt.Errorf("failed to discover models from provider: %w", err)
    }
    
    // Extract model names
    modelNames := m.extractModelNames(modelsData)
    
    // Get existing models
    existingModels, err := m.store.GetModels(providerID)
    if err != nil {
        return fmt.Errorf("failed to get existing models: %w", err)
    }
    
    // Create map of existing models
    existingMap := make(map[string]bool)
    for _, model := range existingModels {
        existingMap[model.ID] = true
    }
    
    // Add discovered models
    addedCount := 0
    for _, modelName := range modelNames {
        if !existingMap[modelName] {
            model := Model{
                ID:          modelName,
                DisplayName:  modelName,
                Description:  fmt.Sprintf("Discovered from %s API", providerID),
                Enabled:     true,
                Source:      "discovered",
                CreatedAt:   time.Now(),
                UpdatedAt:   time.Now(),
            }
            
            if err := m.store.AddModel(providerID, model); err != nil {
                m.logger.WarnLog("[ModelManager] Failed to add discovered model %s: %v", modelName, err)
            } else {
                addedCount++
                m.logger.DebugLog("[ModelManager] Added discovered model: %s", modelName)
            }
        }
    }
    
    m.logger.InfoLog("[ModelManager] Refreshed %d new models from provider %s", addedCount, providerID)
    return nil
}

// extractModelNames extracts model names from provider response
func (m *Manager) extractModelNames(data interface{}) []string {
    // This needs to handle different response formats
    // Based on provider/factory.go:extractModelNames implementation
    
    // For now, return empty slice
    // Each provider should implement its own extraction
    return []string{}
}

// SupportsModel checks if a provider supports a model
func (m *Manager) SupportsModel(providerID, modelID string) bool {
    models, err := m.GetModels(providerID)
    if err != nil {
        return false
    }
    
    for _, model := range models {
        if model.ID == modelID {
            return true
        }
    }
    
    return false
}
```

### Step 3: Add API Endpoints

**File**: `restapi/model_api.go`

```go
package restapi

import (
    "encoding/json"
    "net/http"
    "strconv"
    
    "github.com/sunbankio/qwencoder-proxy/internal/model"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

// ModelAPI handles model management endpoints
type ModelAPI struct {
    server    *Server
    modelMgr  *model.Manager
    logger     logging.Logger
}

// NewModelAPI creates a new model API handler
func NewModelAPI(server *Server, modelMgr *model.Manager, logger logging.Logger) *ModelAPI {
    return &ModelAPI{
        server:   server,
        modelMgr: modelMgr,
        logger:    logger,
    }
}

// RegisterModelRoutes registers model management routes
func (api *ModelAPI) RegisterModelRoutes(mux *http.ServeMux) {
    // Get all models for a provider
    mux.HandleFunc("/api/models", api.handleGetModels)
    
    // Add a custom model
    mux.HandleFunc("/api/models", api.handleAddModel).Methods("POST")
    
    // Get a specific model
    mux.HandleFunc("/api/models/", api.handleGetModel)
    
    // Update a model
    mux.HandleFunc("/api/models/", api.handleUpdateModel).Methods("PUT")
    
    // Delete a model
    mux.HandleFunc("/api/models/", api.handleDeleteModel).Methods("DELETE")
    
    // Refresh models from provider
    mux.HandleFunc("/api/providers/", api.handleRefreshProviderModels).Methods("POST")
}

// handleGetModels returns all models for a provider
func (api *ModelAPI) handleGetModels(w http.ResponseWriter, r *http.Request) {
    // Get provider ID from query
    providerID := r.URL.Query().Get("provider")
    if providerID == "" {
        // Return all models for all providers
        api.handleGetAllModels(w, r)
        return
    }
    
    models, err := api.modelMgr.GetModels(providerID)
    if err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to get models: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get models")
        return
    }
    
    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "provider_id": providerID,
        "models":     models,
        "count":       len(models),
    })
}

// handleGetAllModels returns all models for all providers
func (api *ModelAPI) handleGetAllModels(w http.ResponseWriter, r *http.Request) {
    // Get all providers
    providers := api.server.registry.ListProviders()
    
    result := make(map[string][]model.Model)
    
    for _, provider := range providers {
        models, err := api.modelMgr.GetAllModels(provider.ID)
        if err != nil {
            api.logger.WarnLog("[ModelAPI] Failed to get models for %s: %v", provider.ID, err)
            continue
        }
        result[provider.ID] = models
    }
    
    WriteJSON(w, http.StatusOK, result)
}

// handleGetModel returns a specific model
func (api *ModelAPI) handleGetModel(w http.ResponseWriter, r *http.Request) {
    // Extract provider ID and model ID from path
    // Path format: /api/models/{providerID}/{modelID}
    parts := splitPath(r.URL.Path)
    if len(parts) < 3 {
        WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid path format")
        return
    }
    
    providerID := parts[1]
    modelID := parts[2]
    
    model, err := api.modelMgr.GetAllModels(providerID)
    if err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to get models: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get models")
        return
    }
    
    // Find the specific model
    var found *model.Model
    for i, m := range models {
        if m.ID == modelID {
            found = &models[i]
            break
        }
    }
    
    if found == nil {
        WriteError(w, http.StatusNotFound, "model_not_found", "Model not found")
        return
    }
    
    WriteJSON(w, http.StatusOK, found)
}

// handleAddModel adds a new model
func (api *ModelAPI) handleAddModel(w http.ResponseWriter, r *http.Request) {
    var req struct {
        ProviderID  string          `json:"provider_id"`
        ID          string          `json:"id"`
        DisplayName  string          `json:"display_name"`
        Description string          `json:"description"`
    }
    
    if err := ParseJSON(r, &req); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to parse request: %v", err)
        WriteError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request")
        return
    }
    
    // Validate provider
    if !api.server.registry.HasProvider(req.ProviderID) {
        WriteError(w, http.StatusBadRequest, "invalid_provider", "Invalid provider")
        return
    }
    
    // Validate required fields
    if req.ID == "" {
        WriteError(w, http.StatusBadRequest, "invalid_request", "Model ID is required")
        return
    }
    
    // Create model
    newModel := model.Model{
        ID:          req.ID,
        DisplayName:  req.DisplayName,
        Description:  req.Description,
        Enabled:     true,
        Source:      "custom",
    }
    
    if err := api.modelMgr.AddModel(req.ProviderID, newModel); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to add model: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
        return
    }
    
    api.logger.InfoLog("[ModelAPI] Added model %s for provider %s", req.ID, req.ProviderID)
    WriteJSON(w, http.StatusCreated, newModel)
}

// handleUpdateModel updates an existing model
func (api *ModelAPI) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
    // Extract provider ID and model ID from path
    parts := splitPath(r.URL.Path)
    if len(parts) < 3 {
        WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid path format")
        return
    }
    
    providerID := parts[1]
    modelID := parts[2]
    
    var req struct {
        DisplayName string `json:"display_name"`
        Description string `json:"description"`
        Enabled    *bool  `json:"enabled"`
    }
    
    if err := ParseJSON(r, &req); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to parse request: %v", err)
        WriteError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request")
        return
    }
    
    // Get existing model
    models, err := api.modelMgr.GetAllModels(providerID)
    if err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to get models: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get models")
        return
    }
    
    // Find the model
    var found *model.Model
    var index int
    for i, m := range models {
        if m.ID == modelID {
            found = &models[i]
            index = i
            break
        }
    }
    
    if found == nil {
        WriteError(w, http.StatusNotFound, "model_not_found", "Model not found")
        return
    }
    
    // Update fields
    if req.DisplayName != "" {
        found.DisplayName = req.DisplayName
    }
    if req.Description != "" {
        found.Description = req.Description
    }
    if req.Enabled != nil {
        found.Enabled = *req.Enabled
    }
    
    if err := api.modelMgr.UpdateModel(providerID, *found); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to update model: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
        return
    }
    
    api.logger.InfoLog("[ModelAPI] Updated model %s for provider %s", modelID, providerID)
    WriteJSON(w, http.StatusOK, found)
}

// handleDeleteModel removes a model
func (api *ModelAPI) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
    // Extract provider ID and model ID from path
    parts := splitPath(r.URL.Path)
    if len(parts) < 3 {
        WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid path format")
        return
    }
    
    providerID := parts[1]
    modelID := parts[2]
    
    // Delete model
    if err := api.modelMgr.RemoveModel(providerID, modelID); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to delete model: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
        return
    }
    
    api.logger.InfoLog("[ModelAPI] Deleted model %s for provider %s", modelID, providerID)
    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "message": "Model deleted successfully",
    })
}

// handleRefreshProviderModels refreshes models from provider API
func (api *ModelAPI) handleRefreshProviderModels(w http.ResponseWriter, r *http.Request) {
    // Extract provider ID from path
    // Path format: /api/providers/{providerID}/refresh-models
    parts := splitPath(r.URL.Path)
    if len(parts) < 3 {
        WriteError(w, http.StatusBadRequest, "invalid_path", "Invalid path format")
        return
    }
    
    providerID := parts[1]
    
    // Validate provider
    if !api.server.registry.HasProvider(providerID) {
        WriteError(w, http.StatusBadRequest, "invalid_provider", "Invalid provider")
        return
    }
    
    // Refresh from provider
    if err := api.modelMgr.RefreshFromProvider(r.Context(), providerID); err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to refresh models: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
        return
    }
    
    // Get updated models
    models, err := api.modelMgr.GetModels(providerID)
    if err != nil {
        api.logger.ErrorLog("[ModelAPI] Failed to get models: %v", err)
        WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get models")
        return
    }
    
    api.logger.InfoLog("[ModelAPI] Refreshed models for provider %s, count: %d", providerID, len(models))
    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "provider_id": providerID,
        "models":     models,
        "count":       len(models),
    })
}

// splitPath splits a URL path into parts
func splitPath(path string) []string {
    // Remove leading slash and split
    path = path[1:]
    if path == "" {
        return []string{}
    }
    return []string{}
}
```

### Step 4: Update Provider Logic

#### 4.1 Update Provider Interface

**File**: `provider/provider.go`

Add new method to provider interface:

```go
// Provider represents an AI model provider
type Provider interface {
    // ... existing methods ...
    
    // GetModelManager returns the model manager for this provider
    GetModelManager() interface{} // Will be *model.Manager
}
```

#### 4.2 Update Provider Factory

**File**: `provider/factory.go`

Add model manager to factory:

```go
// Factory manages provider instances
type Factory struct {
    providers      map[ProviderType]Provider
    lastSuccess    map[string]ProviderType
    modelProviders ModelProviderMap
    modelManager   *model.Manager // New field
    mu             sync.RWMutex
    rng            *rand.Rand
}

// NewFactory creates a new provider factory
func NewFactory() *Factory {
    return &Factory{
        providers:      make(map[ProviderType]Provider),
        lastSuccess:    make(map[string]ProviderType),
        modelProviders: make(ModelProviderMap),
        modelManager:   nil, // Will be set later
        rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

// SetModelManager sets the model manager
func (f *Factory) SetModelManager(modelMgr *model.Manager) {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    f.modelManager = modelMgr
}

// Update PopulateModelProviders to use model manager
func (f *Factory) PopulateModelProviders(ctx context.Context) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    // Clear existing mappings
    f.modelProviders = make(ModelProviderMap)
    
    // Fetch models from model manager for each provider
    for providerType, provider := range f.providers {
        providerID := string(providerType)
        
        // Try to get models from model manager
        if f.modelManager != nil {
            models, err := f.modelManager.GetModels(providerID)
            if err == nil {
                // Add this provider to each model it supports
                for _, model := range models {
                    if _, exists := f.modelProviders[model.ID]; !exists {
                        f.modelProviders[model.ID] = []Provider{provider}
                    } else {
                        f.modelProviders[model.ID] = append(f.modelProviders[model.ID], provider)
                    }
                }
            }
        }
    }
    
    return nil
}

// Update SupportsModel to check model manager
func (f *Factory) SupportsModel(model string) bool {
    f.mu.RLock()
    defer f.mu.RUnlock()
    
    // Check model manager first
    if f.modelManager != nil {
        // Check all providers
        for providerType := range f.providers {
            providerID := string(providerType)
            if f.modelManager.SupportsModel(providerID, model) {
                return true
            }
        }
    }
    
    // Fall back to existing logic
    candidates, exists := f.modelProviders[model]
    if !exists || len(candidates) == 0 {
        return false
    }
    
    return true
}
```

#### 4.3 Update Individual Providers

**File**: `provider/gemini/gemini.go`

Update to use model manager:

```go
// Provider implements the provider.Provider interface for Gemini CLI
type Provider struct {
    *provider.BaseProvider
    baseURL                string
    authenticator          *Authenticator
    projectID              string
    projectInitError       error
    modelManager           *model.Manager // New field
}

// NewProvider creates a new Gemini provider
func NewProvider(authenticator *Authenticator) *Provider {
    if authenticator == nil {
        authenticator = NewAuthenticator(nil)
    }
    return &Provider{
        BaseProvider:  provider.NewBaseProvider(logging.NewLogger(), 5*time.Minute),
        baseURL:       DefaultBaseURL,
        authenticator:   authenticator,
        modelManager:   nil, // Will be set by factory
    }
}

// GetModelManager returns the model manager
func (p *Provider) GetModelManager() interface{} {
    return p.modelManager
}

// SupportedModels returns list of supported model IDs
func (p *Provider) SupportedModels() []string {
    // Try model manager first
    if p.modelManager != nil {
        models, err := p.modelManager.GetModels("gemini-cli")
        if err == nil && len(models) > 0 {
            // Return models from manager
            modelIDs := make([]string, len(models))
            for _, model := range models {
                modelIDs = append(modelIDs, model.ID)
            }
            return modelIDs
        }
    }
    
    // Fall back to hardcoded list
    return SupportedModels
}

// SupportsModel checks if the provider supports the given model
func (p *Provider) SupportsModel(model string) bool {
    // Try model manager first
    if p.modelManager != nil {
        if p.modelManager.SupportsModel("gemini-cli", model) {
            return true
        }
    }
    
    // Fall back to existing logic
    modelLower := strings.ToLower(model)
    if strings.HasPrefix(modelLower, "gemini-") {
        return true
    }
    for _, m := range SupportedModels {
        if strings.EqualFold(m, model) {
            return true
        }
    }
    return false
}
```

### Step 5: Update Dashboard UI

#### 5.1 Add Models Tab

**File**: `web/dashboard/templates/tabs.html`

Add new tab template:

```html
<!-- Models Tab -->
<div class="tab-content" id="modelsTab" style="display: none;">
    <div class="tab-header">
        <h2>Models</h2>
        <div class="tab-controls">
            <select id="modelProviderFilter">
                <option value="all">All Providers</option>
                <!-- Provider options will be dynamically inserted -->
            </select>
            <button id="refreshModelsBtn" class="btn btn-secondary">
                <span class="btn-icon">🔄</span> Refresh from Provider
            </button>
            <button id="addModelBtn" class="btn btn-primary">
                <span class="btn-icon">➕</span> Add Custom Model
            </button>
        </div>
    </div>
    
    <div class="providers-grid" id="modelsGrid">
        <!-- Provider model cards will be dynamically inserted -->
    </div>
</div>
```

#### 5.2 Add Model Modal

**File**: `web/dashboard/templates/modals.html`

Add model management modal:

```html
<!-- Add Model Modal -->
<div class="modal-overlay" id="addModelModal">
    <div class="modal">
        <div class="modal-header">
            <h3 id="addModelTitle">Add Model</h3>
            <button class="modal-close" id="addModelClose">&times;</button>
        </div>
        <div class="modal-body">
            <div class="form-group">
                <label for="modelProvider">Provider</label>
                <select id="modelProvider" required>
                    <option value="">Select Provider</option>
                    <!-- Provider options will be dynamically inserted -->
                </select>
            </div>
            <div class="form-group">
                <label for="modelId">Model ID *</label>
                <input type="text" id="modelId" required
                       placeholder="e.g., gemini-2.5-flash"
                       pattern="[a-z0-9-._-]+"
                       title="Alphanumeric, dots, underscores, and hyphens only">
            </div>
            <div class="form-group">
                <label for="modelDisplayName">Display Name</label>
                <input type="text" id="modelDisplayName"
                       placeholder="e.g., Gemini 2.5 Flash">
            </div>
            <div class="form-group">
                <label for="modelDescription">Description</label>
                <textarea id="modelDescription" rows="3"
                          placeholder="Brief description of the model"></textarea>
            </div>
        </div>
        <div class="modal-footer">
            <button class="btn btn-secondary" id="addModelCancel">Cancel</button>
            <button class="btn btn-primary" id="addModelSave">Add Model</button>
        </div>
    </div>
</div>
```

#### 5.3 Update Dashboard JavaScript

**File**: `web/dashboard/js/dashboard.js`

Add model management methods:

```javascript
// In Dashboard class constructor
constructor() {
    // ... existing fields ...
    this.models = []; // New field
    this.providers = [];
    this.currentModelProvider = null;
}

// In loadData method
async loadData() {
    try {
        this.loading.show();
        
        // Load providers
        const providersData = await this.api.getProviders();
        this.providers = providersData.providers || [];
        this.state.setProviders(this.providers);
        
        // Load models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Check server status
        await this.checkServerStatus();
    } catch (error) {
        this.log(`Failed to load data: ${error.message}`, 'error');
        this.toast.show('Failed to load data', 'error');
    } finally {
        this.loading.hide();
    }
}

// In render method
render() {
    // ... existing tab rendering ...
    
    // Render models tab
    this.renderModelsTab();
}

// New method: renderModelsTab
renderModelsTab() {
    const container = document.getElementById('modelsGrid');
    
    if (this.providers.length === 0) {
        container.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <div class="empty-state-icon">📦</div>
                <p>No providers available</p>
            </div>
        `;
        return;
    }
    
    container.innerHTML = this.providers.map(provider => {
        const providerModels = this.models[provider.id] || [];
        const enabledModels = providerModels.filter(m => m.enabled);
        const disabledModels = providerModels.filter(m => !m.enabled);
        
        return `
            <div class="provider-card">
                <div class="provider-header">
                    <h3>${provider.name}</h3>
                    <div class="provider-actions">
                        <button class="btn btn-sm btn-secondary"
                                onclick="dashboard.refreshProviderModels('${provider.id}')"
                                title="Refresh from provider API">
                            🔄
                        </button>
                    </div>
                </div>
                <div class="provider-body">
                    <div class="model-section">
                        <h4>Enabled Models (${enabledModels.length})</h4>
                        <div class="model-list">
                            ${enabledModels.map(model => `
                                <div class="model-item">
                                    <span class="model-id">${model.id}</span>
                                    <span class="model-name">${model.display_name || model.id}</span>
                                    <div class="model-actions">
                                        <button class="btn btn-xs btn-secondary"
                                                onclick="dashboard.disableModel('${provider.id}', '${model.id}')"
                                                title="Disable model">
                                            🚫
                                        </button>
                                        ${model.source === 'custom' ? `
                                            <button class="btn btn-xs btn-danger"
                                                        onclick="dashboard.removeModel('${provider.id}', '${model.id}')"
                                                        title="Remove model">
                                                🗑
                                            </button>
                                        ` : ''}
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                        ${disabledModels.length > 0 ? `
                            <div class="model-section">
                                <h4>Disabled Models (${disabledModels.length})</h4>
                                <div class="model-list">
                                    ${disabledModels.map(model => `
                                        <div class="model-item disabled">
                                            <span class="model-id">${model.id}</span>
                                            <span class="model-name">${model.display_name || model.id}</span>
                                            <div class="model-actions">
                                                <button class="btn btn-xs btn-secondary"
                                                        onclick="dashboard.enableModel('${provider.id}', '${model.id}')"
                                                        title="Enable model">
                                                    ✅
                                                </button>
                                                ${model.source === 'custom' ? `
                                                    <button class="btn btn-xs btn-danger"
                                                                onclick="dashboard.removeModel('${provider.id}', '${model.id}')"
                                                                title="Remove model">
                                                        🗑
                                                    </button>
                                                ` : ''}
                                            </div>
                                        </div>
                                    `).join('')}
                                </div>
                            </div>
                        ` : ''}
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

// New method: refreshProviderModels
async refreshProviderModels(providerId) {
    try {
        this.log(`Refreshing models for provider ${providerId}`, 'info');
        
        await this.api.refreshProviderModels(providerId);
        
        // Reload models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Re-render models tab
        this.renderModelsTab();
        
        this.toast.show(`Models refreshed for ${providerId}`, 'success');
    } catch (error) {
        this.log(`Failed to refresh models: ${error.message}`, 'error');
        this.toast.show('Failed to refresh models', 'error');
    }
}

// New method: showAddModelModal
showAddModelModal() {
    const modal = document.getElementById('addModelModal');
    const title = document.getElementById('addModelTitle');
    
    title.textContent = 'Add Custom Model';
    
    // Populate provider dropdown
    const providerSelect = document.getElementById('modelProvider');
    providerSelect.innerHTML = '<option value="">Select Provider</option>' + 
        this.providers.map(p => `<option value="${p.id}">${p.name}</option>`).join('');
    
    // Clear form
    document.getElementById('modelId').value = '';
    document.getElementById('modelDisplayName').value = '';
    document.getElementById('modelDescription').value = '';
    
    modal.classList.add('active');
}

// New method: hideAddModelModal
hideAddModelModal() {
    document.getElementById('addModelModal').classList.remove('active');
}

// New method: addModel
async addModel() {
    const providerId = document.getElementById('modelProvider').value;
    const modelId = document.getElementById('modelId').value.trim();
    const displayName = document.getElementById('modelDisplayName').value.trim();
    const description = document.getElementById('modelDescription').value.trim();
    
    // Validate
    if (!providerId) {
        this.toast.show('Please select a provider', 'warning');
        return;
    }
    
    if (!modelId) {
        this.toast.show('Model ID is required', 'warning');
        return;
    }
    
    try {
        this.log(`Adding model ${modelId} for provider ${providerId}`, 'info');
        
        await this.api.addModel({
            provider_id: providerId,
            id: modelId,
            display_name: displayName || modelId,
            description: description || ''
        });
        
        // Reload models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Re-render models tab
        this.renderModelsTab();
        
        this.hideAddModelModal();
        this.toast.show('Model added successfully', 'success');
    } catch (error) {
        this.log(`Failed to add model: ${error.message}`, 'error');
        this.toast.show('Failed to add model', 'error');
    }
}

// New method: removeModel
async removeModel(providerId, modelId) {
    if (!confirm(`Are you sure you want to remove model ${modelId}?`)) {
        return;
    }
    
    try {
        this.log(`Removing model ${modelId} from provider ${providerId}`, 'info');
        
        await this.api.removeModel(providerId, modelId);
        
        // Reload models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Re-render models tab
        this.renderModelsTab();
        
        this.toast.show('Model removed successfully', 'success');
    } catch (error) {
        this.log(`Failed to remove model: ${error.message}`, 'error');
        this.toast.show('Failed to remove model', 'error');
    }
}

// New method: enableModel
async enableModel(providerId, modelId) {
    try {
        this.log(`Enabling model ${modelId} for provider ${providerId}`, 'info');
        
        await this.api.updateModel(providerId, modelId, { enabled: true });
        
        // Reload models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Re-render models tab
        this.renderModelsTab();
        
        this.toast.show('Model enabled', 'success');
    } catch (error) {
        this.log(`Failed to enable model: ${error.message}`, 'error');
        this.toast.show('Failed to enable model', 'error');
    }
}

// New method: disableModel
async disableModel(providerId, modelId) {
    try {
        this.log(`Disabling model ${modelId} for provider ${providerId}`, 'info');
        
        await this.api.updateModel(providerId, modelId, { enabled: false });
        
        // Reload models
        const modelsData = await this.api.getAllModels();
        this.models = modelsData || {};
        this.state.setModels(this.models);
        
        // Re-render models tab
        this.renderModelsTab();
        
        this.toast.show('Model disabled', 'success');
    } catch (error) {
        this.log(`Failed to disable model: ${error.message}`, 'error');
        this.toast.show('Failed to disable model', 'error');
    }
}
```

#### 5.4 Update API Client

**File**: `web/dashboard/js/api/endpoints.js`

Add model management endpoints:

```javascript
export const ENDPOINTS = {
    // ... existing endpoints ...
    
    // Model management
    GET_MODELS: '/api/models',
    GET_ALL_MODELS: '/api/models?all=true',
    GET_MODEL: (providerId, modelId) => `/api/models/${providerId}/${modelId}`,
    ADD_MODEL: '/api/models',
    UPDATE_MODEL: (providerId, modelId) => `/api/models/${providerId}/${modelId}`,
    DELETE_MODEL: (providerId, modelId) => `/api/models/${providerId}/${modelId}`,
    REFRESH_PROVIDER_MODELS: (providerId) => `/api/providers/${providerId}/refresh-models`,
};
```

**File**: `web/dashboard/js/api/client.js`

Add model management methods:

```javascript
// In APIClient class

// Get all models
async getAllModels() {
    return this.get(ENDPOINTS.GET_ALL_MODELS);
}

// Get models for a specific provider
async getModels(providerId) {
    return this.get(`${ENDPOINTS.GET_MODELS}?provider=${providerId}`);
}

// Get a specific model
async getModel(providerId, modelId) {
    return this.get(ENDPOINTS.GET_MODEL(providerId, modelId));
}

// Add a custom model
async addModel(modelData) {
    return this.post(ENDPOINTS.ADD_MODEL, modelData);
}

// Update a model
async updateModel(providerId, modelId, updates) {
    return this.put(ENDPOINTS.UPDATE_MODEL(providerId, modelId), updates);
}

// Remove a model
async removeModel(providerId, modelId) {
    return this.delete(ENDPOINTS.DELETE_MODEL(providerId, modelId));
}

// Refresh models from provider API
async refreshProviderModels(providerId) {
    return this.post(ENDPOINTS.REFRESH_PROVIDER_MODELS(providerId));
}
```

### Step 6: Integration

#### 6.1 Update Server Initialization

**File**: `restapi/rest_api.go`

Add model manager initialization:

```go
// Server represents the OAuth REST API server
type Server struct {
    config            *Config
    registry          *ProviderRegistry
    stateManager      *StateManager
    logger            logging.Logger
    httpClient        *http.Client
    tokenStores       map[string]*tokpkg.MultiTokenStore
    tokenManagers     map[string]*tokpkg.TokenManager
    multiTokenManager *tokpkg.MultiTokenManager
    modelManager      *model.Manager // New field
    modelAPI          *ModelAPI // New field
}

// NewServer creates a new OAuth REST API server
func NewServer(config *Config, logger logging.Logger) *Server {
    if config == nil {
        config = DefaultConfig()
    }
    if logger == nil {
        logger = logging.NewLogger()
    }

    // Create multi-token manager
    multiTokenManager := tokpkg.NewMultiTokenManager(logger)
    if err := multiTokenManager.Initialize(); err != nil {
        logger.ErrorLog("Failed to initialize multi-token manager: %v", err)
    }
    if err := multiTokenManager.Start(); err != nil {
        logger.ErrorLog("Failed to start multi-token manager: %v", err)
    }

    // Create model store and manager
    modelStore := model.NewJSONModelStore(logger)
    modelManager := model.NewManager(modelStore, logger)
    
    // Initialize default models
    if err := model.InitializeDefaults(modelStore); err != nil {
        logger.ErrorLog("Failed to initialize default models: %v", err)
    }

    server := &Server{
        config:            config,
        registry:          NewProviderRegistry(),
        stateManager:      NewStateManager(),
        logger:            logger,
        httpClient:        &http.Client{Timeout: 30 * time.Second},
        tokenStores:       make(map[string]*tokpkg.MultiTokenStore),
        tokenManagers:     make(map[string]*tokpkg.TokenManager),
        multiTokenManager: multiTokenManager,
        modelManager:      modelManager,
        modelAPI:          NewModelAPI(server, modelManager, logger),
    }
    
    // Register provider refreshers
    if err := server.registerProviderRefreshers(); err != nil {
        logger.ErrorLog("Failed to register provider refreshers: %v", err)
    }

    return server
}

// In registerRoutes method, add model routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
    // ... existing routes ...
    
    // Register model management routes
    s.modelAPI.RegisterModelRoutes(mux)
}
```

#### 6.2 Update Provider Registration

When registering providers with factory, also register with model manager:

```go
// In provider registration code
factory.SetModelManager(modelManager)
factory.Register(provider)
```

### Step 7: Testing

#### 7.1 Unit Tests

**File**: `internal/model/store_test.go`

```go
package model

import (
    "testing"
    "time"
)

func TestJSONModelStore(t *testing.T) {
    logger := logging.NewLogger()
    store := NewJSONModelStore(logger)
    
    t.Run("AddModel", func(t *testing.T) {
        model := Model{
            ID:          "test-model",
            DisplayName:  "Test Model",
            Description:  "A test model",
            Enabled:     true,
            Source:      "custom",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        }
        
        err := store.AddModel("test-provider", model)
        if err != nil {
            t.Fatalf("Failed to add model: %v", err)
        }
        
        // Verify model was added
        models, err := store.GetModels("test-provider")
        if err != nil {
            t.Fatalf("Failed to get models: %v", err)
        }
        
        if len(models) != 1 {
            t.Fatalf("Expected 1 model, got %d", len(models))
        }
        
        if models[0].ID != "test-model" {
            t.Fatalf("Model ID mismatch: expected test-model, got %s", models[0].ID)
        }
    })
    
    t.Run("RemoveModel", func(t *testing.T) {
        // Add a model first
        model := Model{
            ID:          "test-model",
            DisplayName:  "Test Model",
            Description:  "A test model",
            Enabled:     true,
            Source:      "custom",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        }
        
        store.AddModel("test-provider", model)
        
        // Remove the model
        err := store.RemoveModel("test-provider", "test-model")
        if err != nil {
            t.Fatalf("Failed to remove model: %v", err)
        }
        
        // Verify model was removed
        models, err := store.GetModels("test-provider")
        if err != nil {
            t.Fatalf("Failed to get models: %v", err)
        }
        
        if len(models) != 0 {
            t.Fatalf("Expected 0 models, got %d", len(models))
        }
    })
    
    t.Run("EnableModel", func(t *testing.T) {
        // Add a disabled model
        model := Model{
            ID:          "test-model",
            DisplayName:  "Test Model",
            Description:  "A test model",
            Enabled:     false,
            Source:      "custom",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        }
        
        store.AddModel("test-provider", model)
        
        // Enable the model
        err := store.EnableModel("test-provider", "test-model")
        if err != nil {
            t.Fatalf("Failed to enable model: %v", err)
        }
        
        // Verify model was enabled
        models, err := store.GetModels("test-provider")
        if err != nil {
            t.Fatalf("Failed to get models: %v", err)
        }
        
        if !models[0].Enabled {
            t.Fatalf("Expected model to be enabled")
        }
    })
    
    t.Run("DisableModel", func(t *testing.T) {
        // Add an enabled model
        model := Model{
            ID:          "test-model",
            DisplayName:  "Test Model",
            Description:  "A test model",
            Enabled:     true,
            Source:      "custom",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        }
        
        store.AddModel("test-provider", model)
        
        // Disable the model
        err := store.DisableModel("test-provider", "test-model")
        if err != nil {
            t.Fatalf("Failed to disable model: %v", err)
        }
        
        // Verify model was disabled
        models, err := store.GetModels("test-provider")
        if err != nil {
            t.Fatalf("Failed to get models: %v", err)
        }
        
        if models[0].Enabled {
            t.Fatalf("Expected model to be disabled")
        }
    })
}
```

#### 7.2 Integration Tests

**File**: `restapi/model_api_test.go`

```go
package restapi

import (
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/sunbankio/qwencoder-proxy/internal/model"
    "github.com/sunbankio/qwencoder-proxy/logging"
)

func TestModelAPI(t *testing.T) {
    logger := logging.NewLogger()
    
    // Create test server
    modelStore := model.NewJSONModelStore(logger)
    modelMgr := model.NewManager(modelStore, logger)
    
    server := NewServer(DefaultConfig(), logger)
    server.modelManager = modelMgr
    
    // Create test mux
    mux := http.NewServeMux()
    server.modelAPI.RegisterModelRoutes(mux)
    
    // Create test server
    testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mux.ServeHTTP(w, r)
    }))
    defer testServer.Close()
    
    t.Run("GetModels", func(t *testing.T) {
        // Add test model
        testModel := model.Model{
            ID:          "test-model",
            DisplayName:  "Test Model",
            Description:  "A test model",
            Enabled:     true,
            Source:      "custom",
        }
        modelStore.AddModel("test-provider", testModel)
        
        // Make request
        req, _ := http.NewRequest("GET", testServer.URL+"/api/models?provider=test-provider", nil)
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            t.Fatalf("Failed to make request: %v", err)
        }
        defer resp.Body.Close()
        
        // Verify response
        if resp.StatusCode != http.StatusOK {
            t.Fatalf("Expected status 200, got %d", resp.StatusCode)
        }
    })
    
    t.Run("AddModel", func(t *testing.T) {
        // Prepare request
        reqBody := `{
            "provider_id": "test-provider",
            "id": "new-model",
            "display_name": "New Model",
            "description": "A new test model"
        }`
        
        req, _ := http.NewRequest("POST", testServer.URL+"/api/models", strings.NewReader(reqBody))
        req.Header.Set("Content-Type", "application/json")
        
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            t.Fatalf("Failed to make request: %v", err)
        }
        defer resp.Body.Close()
        
        // Verify response
        if resp.StatusCode != http.StatusCreated {
            t.Fatalf("Expected status 201, got %d", resp.StatusCode)
        }
    })
}
```

#### 7.3 Manual Testing

1. Start server with model manager initialized
2. Navigate to dashboard Models tab
3. Verify default models are displayed
4. Add a custom model via UI
5. Verify custom model appears in list
6. Disable a model via UI
7. Verify model is disabled
8. Enable the model via UI
9. Verify model is enabled
10. Remove a custom model via UI
11. Verify model is removed
12. Refresh models from provider API
13. Verify discovered models appear
14. Restart server
15. Verify models persist across restarts

## Definition of Done

### Backend
- [ ] Model storage layer implemented (ModelStore interface)
- [ ] JSON file-based storage implemented
- [ ] Default models defined and initialized
- [ ] Model manager created with CRUD operations
- [ ] Model API endpoints implemented
- [ ] Provider interface updated with GetModelManager()
- [ ] Factory updated to use model manager
- [ ] All providers updated to use model manager
- [ ] Unit tests for model storage
- [ ] Integration tests for model API
- [ ] Models persist across server restarts

### Frontend
- [ ] Models tab added to dashboard
- [ ] Model cards display enabled/disabled models
- [ ] Add model modal implemented
- [ ] Refresh from provider button added
- [ ] Enable/disable model functionality
- [ ] Remove model functionality
- [ ] API client methods added
- [ ] Models display correctly
- [ ] Model operations work correctly

### Integration
- [ ] Model manager initialized in server
- [ ] Model API routes registered
- [ ] Providers registered with model manager
- [ ] Factory uses model manager
- [ ] Default models loaded on startup
- [ ] Custom models can be added via UI
- [ ] Models can be enabled/disabled
- [ ] Models can be removed
- [ ] Models can be refreshed from provider API
- [ ] Models persist across server restarts

## Rollback Plan

If issues arise:

1. Disable model manager by commenting out initialization
2. System falls back to hardcoded model lists
3. Investigate logs for model-related errors
4. Fix individual issues (storage, API, UI)
5. Re-enable model manager one feature at a time

## Migration Guide

### For Existing Deployments

1. Backup current `.credentials` directory
2. Deploy new version with model manager
3. On first startup, default models will be initialized
4. Existing tokens will continue to work
5. Custom models can be added via dashboard

### For New Deployments

1. Default models are automatically initialized
2. No migration needed
3. Custom models can be added immediately

## References

- [`internal/model/model.go`](../internal/model/model.go) - Model data structures
- [`internal/model/store.go`](../internal/model/store.go) - Model storage implementation
- [`internal/model/defaults.go`](../internal/model/defaults.go) - Default model definitions
- [`internal/model/manager.go`](../internal/model/manager.go) - Model manager
- [`restapi/model_api.go`](../restapi/model_api.go) - Model API endpoints
- [`provider/provider.go`](../provider/provider.go) - Provider interface
- [`provider/factory.go`](../provider/factory.go) - Provider factory
- [`web/dashboard/js/dashboard.js`](../web/dashboard/js/dashboard.js) - Dashboard JavaScript
- [`web/dashboard/js/api/endpoints.js`](../web/dashboard/js/api/endpoints.js) - API endpoints
- [`web/dashboard/js/api/client.js`](../web/dashboard/js/api/client.js) - API client
