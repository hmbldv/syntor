package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// ManifestEvent represents a change to a manifest
type ManifestEvent struct {
	Type     EventType       // "created", "updated", "deleted"
	Name     string          // Agent name
	Manifest *AgentManifest  // The manifest (nil for deleted)
	Path     string          // File path
}

// EventType defines the type of manifest event
type EventType string

const (
	EventCreated EventType = "created"
	EventUpdated EventType = "updated"
	EventDeleted EventType = "deleted"
)

// ManifestStore manages agent manifests with hot-reload
type ManifestStore struct {
	manifests map[string]*AgentManifest
	paths     []string
	filePaths map[string]string // agent name -> file path
	watcher   *fsnotify.Watcher
	mu        sync.RWMutex
	callbacks []func(event ManifestEvent)
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManifestStore creates a new manifest store
// paths should include directories like ~/.syntor/agents/ and .syntor/agents/
func NewManifestStore(paths []string) (*ManifestStore, error) {
	ctx, cancel := context.WithCancel(context.Background())

	store := &ManifestStore{
		manifests: make(map[string]*AgentManifest),
		paths:     paths,
		filePaths: make(map[string]string),
		callbacks: make([]func(event ManifestEvent), 0),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Load all manifests from paths
	for _, path := range paths {
		if err := store.loadFromDirectory(path); err != nil {
			// Directory might not exist, which is okay
			continue
		}
	}

	return store, nil
}

// loadFromDirectory loads all YAML files from a directory
func (s *ManifestStore) loadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if err := s.LoadManifest(path); err != nil {
			// Log error but continue loading other manifests
			fmt.Fprintf(os.Stderr, "warning: failed to load manifest %s: %v\n", path, err)
		}
	}

	return nil
}

// LoadManifest loads a single manifest file
func (s *ManifestStore) LoadManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest AgentManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if err := ValidateManifest(&manifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Set timestamps if not present
	now := time.Now()
	if manifest.Metadata.CreatedAt.IsZero() {
		manifest.Metadata.CreatedAt = now
	}
	manifest.Metadata.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this is an update
	_, exists := s.manifests[manifest.Metadata.Name]

	s.manifests[manifest.Metadata.Name] = &manifest
	s.filePaths[manifest.Metadata.Name] = path

	// Notify callbacks
	eventType := EventCreated
	if exists {
		eventType = EventUpdated
	}
	s.notifyCallbacks(ManifestEvent{
		Type:     eventType,
		Name:     manifest.Metadata.Name,
		Manifest: &manifest,
		Path:     path,
	})

	return nil
}

// GetManifest returns a manifest by agent name
func (s *ManifestStore) GetManifest(name string) (*AgentManifest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.manifests[name]
	return m, ok
}

// ListManifests returns all loaded manifests
func (s *ManifestStore) ListManifests() []*AgentManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	manifests := make([]*AgentManifest, 0, len(s.manifests))
	for _, m := range s.manifests {
		manifests = append(manifests, m)
	}
	return manifests
}

// ListByType returns manifests filtered by agent type
func (s *ManifestStore) ListByType(agentType AgentType) []*AgentManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	manifests := make([]*AgentManifest, 0)
	for _, m := range s.manifests {
		if m.Spec.Type == agentType {
			manifests = append(manifests, m)
		}
	}
	return manifests
}

// FindByCapability returns manifests that have a specific capability
func (s *ManifestStore) FindByCapability(capability string) []*AgentManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	manifests := make([]*AgentManifest, 0)
	for _, m := range s.manifests {
		if m.HasCapability(capability) {
			manifests = append(manifests, m)
		}
	}
	return manifests
}

// DeleteManifest removes a manifest from the store
func (s *ManifestStore) DeleteManifest(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	manifest, exists := s.manifests[name]
	if !exists {
		return fmt.Errorf("manifest not found: %s", name)
	}

	path := s.filePaths[name]
	delete(s.manifests, name)
	delete(s.filePaths, name)

	s.notifyCallbacks(ManifestEvent{
		Type:     EventDeleted,
		Name:     name,
		Manifest: manifest,
		Path:     path,
	})

	return nil
}

// SaveManifest saves a manifest to a file
func (s *ManifestStore) SaveManifest(manifest *AgentManifest, path string) error {
	manifest.Metadata.UpdatedAt = time.Now()
	if manifest.Metadata.CreatedAt.IsZero() {
		manifest.Metadata.CreatedAt = manifest.Metadata.UpdatedAt
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Load into store
	return s.LoadManifest(path)
}

// StartWatching enables hot-reload via fsnotify
func (s *ManifestStore) StartWatching(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	s.watcher = watcher

	// Add directories to watch
	for _, path := range s.paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if err := watcher.Add(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to watch %s: %v\n", path, err)
		}
	}

	// Start watching in goroutine
	go s.watchLoop(ctx)

	return nil
}

// watchLoop handles file system events
func (s *ManifestStore) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			s.handleFSEvent(event)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}

// handleFSEvent processes a file system event
func (s *ManifestStore) handleFSEvent(event fsnotify.Event) {
	ext := filepath.Ext(event.Name)
	if ext != ".yaml" && ext != ".yml" {
		return
	}

	switch {
	case event.Op&fsnotify.Write == fsnotify.Write:
		fallthrough
	case event.Op&fsnotify.Create == fsnotify.Create:
		if err := s.LoadManifest(event.Name); err != nil {
			fmt.Fprintf(os.Stderr, "failed to reload manifest %s: %v\n", event.Name, err)
		}
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		// Find and remove the manifest for this file
		s.mu.Lock()
		for name, path := range s.filePaths {
			if path == event.Name {
				manifest := s.manifests[name]
				delete(s.manifests, name)
				delete(s.filePaths, name)
				s.notifyCallbacks(ManifestEvent{
					Type:     EventDeleted,
					Name:     name,
					Manifest: manifest,
					Path:     path,
				})
				break
			}
		}
		s.mu.Unlock()
	}
}

// OnChange registers a callback for manifest changes
func (s *ManifestStore) OnChange(callback func(ManifestEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// notifyCallbacks sends event to all registered callbacks
func (s *ManifestStore) notifyCallbacks(event ManifestEvent) {
	for _, cb := range s.callbacks {
		go cb(event)
	}
}

// Close stops the watcher and cleans up
func (s *ManifestStore) Close() error {
	s.cancel()
	if s.watcher != nil {
		return s.watcher.Close()
	}
	return nil
}

// GetDefaultPaths returns the default manifest search paths
func GetDefaultPaths() []string {
	paths := make([]string, 0, 3)

	// Global user config
	homeDir, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths, filepath.Join(homeDir, ".syntor", "agents"))
	}

	// Project-specific
	cwd, err := os.Getwd()
	if err == nil {
		paths = append(paths, filepath.Join(cwd, ".syntor", "agents"))
	}

	// Built-in defaults (relative to binary or configs dir)
	paths = append(paths, "configs/agents")

	return paths
}
