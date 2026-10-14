// Package plugin provides a plugin architecture for extending superfile.
// Supports both native Go plugins (.so) and WASM modules.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"sync"
)

// PluginType represents the type of plugin.
type PluginType string

const (
	PluginTypeStorage     PluginType = "storage"
	PluginTypeCategorizer PluginType = "categorizer"
	PluginTypePreview     PluginType = "preview"
	PluginTypeSearch      PluginType = "search"
	PluginTypeUI          PluginType = "ui"
)

// PluginManifest describes a plugin.
type PluginManifest struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Description   string                 `json:"description"`
	Author        string                 `json:"author"`
	License       string                 `json:"license"`
	Homepage      string                 `json:"homepage"`
	Type          PluginType             `json:"type"`
	EntryPoint    string                 `json:"entry_point"`  // For .so plugins: symbol name
	WASMModule    string                 `json:"wasm_module"`  // For WASM plugins: .wasm file
	Dependencies  []string               `json:"dependencies"` // Other plugin names
	Permissions   []string               `json:"permissions"`  // Required permissions
	Config        map[string]interface{} `json:"config"`       // Default config
	MinAPIVersion string                 `json:"min_api_version"`
	MaxAPIVersion string                 `json:"max_api_version"`
}

// PluginInterface is the interface all plugins must implement.
type PluginInterface interface {
	// Manifest returns the plugin's manifest.
	Manifest() PluginManifest

	// Init initializes the plugin with configuration.
	Init(ctx context.Context, config map[string]interface{}) error

	// Start starts the plugin (for long-running plugins).
	Start(ctx context.Context) error

	// Stop stops the plugin gracefully.
	Stop(ctx context.Context) error

	// HealthCheck returns the plugin's health status.
	HealthCheck(ctx context.Context) error
}

// StoragePlugin extends PluginInterface for storage providers.
type StoragePlugin interface {
	PluginInterface
	// CloudStorageProvider returns the cloud storage provider implementation.
	CloudStorageProvider() interface{} // Will be asserted to cloudstorage.CloudStorageProvider
}

// CategorizerPlugin extends PluginInterface for categorizers.
type CategorizerPlugin interface {
	PluginInterface
	// Categorize categorizes a file.
	Categorize(ctx context.Context, filePath string, content []byte) interface{} // CategorizationResult
	// GetCategories returns supported categories.
	GetCategories() []interface{} // FileCategory
}

// PreviewPlugin extends PluginInterface for file previews.
type PreviewPlugin interface {
	PluginInterface
	// CanPreview returns true if this plugin can preview the file.
	CanPreview(filePath string, mimeType string) bool
	// GeneratePreview generates a preview for the file.
	GeneratePreview(ctx context.Context, filePath string, maxWidth, maxHeight int) ([]byte, string, error) // data, mimeType, error
}

// SearchPlugin extends PluginInterface for search providers.
type SearchPlugin interface {
	PluginInterface
	// Search searches for files matching the query.
	Search(ctx context.Context, query string, options map[string]interface{}) ([]interface{}, error) // SearchResult
	// Index indexes a file or directory.
	Index(ctx context.Context, path string) error
}

// UIPlugin extends PluginInterface for UI extensions.
type UIPlugin interface {
	PluginInterface
	// GetComponents returns UI components provided by this plugin.
	GetComponents() map[string]interface{}
	// HandleKey handles a key press.
	HandleKey(key string) bool
}

// PluginLoader manages loading and lifecycle of plugins.
type PluginLoader struct {
	pluginsDir string
	plugins    map[string]PluginInstance
	mu         sync.RWMutex
}

// PluginInstance holds a loaded plugin with its metadata.
type PluginInstance struct {
	Manifest PluginManifest
	Plugin   PluginInterface
	Loaded   bool
	Error    error
	Config   map[string]interface{}
}

// NewPluginLoader creates a new plugin loader.
func NewPluginLoader(pluginsDir string) *PluginLoader {
	return &PluginLoader{
		pluginsDir: pluginsDir,
		plugins:    make(map[string]PluginInstance),
	}
}

// LoadPlugin loads a plugin from a .so file.
func (l *PluginLoader) LoadPlugin(ctx context.Context, pluginPath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Open the plugin
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	// Look up the Plugin symbol
	sym, err := p.Lookup("Plugin")
	if err != nil {
		return fmt.Errorf("plugin missing 'Plugin' symbol: %w", err)
	}

	// Assert to PluginInterface
	pluginInstance, ok := sym.(PluginInterface)
	if !ok {
		return fmt.Errorf("plugin does not implement PluginInterface")
	}

	// Get manifest
	manifest := pluginInstance.Manifest()
	if manifest.Name == "" {
		return fmt.Errorf("plugin manifest missing name")
	}

	// Initialize
	if err := pluginInstance.Init(ctx, manifest.Config); err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Store
	l.plugins[manifest.Name] = PluginInstance{
		Manifest: manifest,
		Plugin:   pluginInstance,
		Loaded:   true,
		Config:   manifest.Config,
	}

	return nil
}

// LoadPluginFromDir loads all plugins from the plugins directory.
func (l *PluginLoader) LoadPluginFromDir(ctx context.Context) error {
	entries, err := os.ReadDir(l.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No plugins directory is OK
		}
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".so" && ext != ".wasm" {
			continue
		}

		pluginPath := filepath.Join(l.pluginsDir, entry.Name())
		if err := l.LoadPlugin(ctx, pluginPath); err != nil {
			// Log error but continue loading other plugins
			fmt.Printf("Failed to load plugin %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

// GetPlugin retrieves a loaded plugin by name.
func (l *PluginLoader) GetPlugin(name string) (PluginInterface, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	instance, ok := l.plugins[name]
	if !ok || !instance.Loaded {
		return nil, false
	}
	return instance.Plugin, true
}

// GetPluginsByType returns all loaded plugins of a given type.
func (l *PluginLoader) GetPluginsByType(pluginType PluginType) []PluginInterface {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []PluginInterface
	for _, instance := range l.plugins {
		if instance.Loaded && instance.Manifest.Type == pluginType {
			result = append(result, instance.Plugin)
		}
	}
	return result
}

// UnloadPlugin unloads a plugin by name.
func (l *PluginLoader) UnloadPlugin(ctx context.Context, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	instance, ok := l.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}

	if instance.Loaded {
		if err := instance.Plugin.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop plugin: %w", err)
		}
	}

	delete(l.plugins, name)
	return nil
}

// ListPlugins returns all loaded plugin manifests.
func (l *PluginLoader) ListPlugins() []PluginManifest {
	l.mu.RLock()
	defer l.mu.RUnlock()

	manifests := make([]PluginManifest, 0, len(l.plugins))
	for _, instance := range l.plugins {
		if instance.Loaded {
			manifests = append(manifests, instance.Manifest)
		}
	}
	return manifests
}

// GetPluginConfig returns the configuration for a plugin.
func (l *PluginLoader) GetPluginConfig(name string) (map[string]interface{}, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	instance, ok := l.plugins[name]
	if !ok {
		return nil, false
	}
	return instance.Config, true
}

// SetPluginConfig updates the configuration for a plugin.
func (l *PluginLoader) SetPluginConfig(name string, config map[string]interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	instance, ok := l.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}

	instance.Config = config
	instance.Plugin.Init(context.Background(), config)
	l.plugins[name] = instance
	return nil
}

// ValidateManifest validates a plugin manifest file.
func ValidateManifest(manifestPath string) (*PluginManifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.Name == "" {
		return nil, fmt.Errorf("manifest missing name")
	}
	if manifest.Version == "" {
		return nil, fmt.Errorf("manifest missing version")
	}
	if manifest.Type == "" {
		return nil, fmt.Errorf("manifest missing type")
	}

	return &manifest, nil
}

// Example plugin implementation for reference
// This would be in a separate .so file:
//
// package main
//
// import (
// 	"context"
// 	"github.com/nakisisageorge/superfile-enhanced/src/plugin"
// )
//
// type MyStoragePlugin struct{}
// func (p *MyStoragePlugin) Manifest() plugin.PluginManifest { ... }
// func (p *MyStoragePlugin) Init(ctx context.Context, config map[string]interface{}) error { ... }
// func (p *MyStoragePlugin) Start(ctx context.Context) error { ... }
// func (p *MyStoragePlugin) Stop(ctx context.Context) error { ... }
// func (p *MyStoragePlugin) HealthCheck(ctx context.Context) error { ... }
// func (p *MyStoragePlugin) CloudStorageProvider() interface{} { ... }
//
// var Plugin plugin.PluginInterface = &MyStoragePlugin{}
