package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestHotReloadCreate tests that new manifest files are detected and loaded
func TestHotReloadCreate(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create store with temp directory
	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Track events
	var events []ManifestEvent
	var mu sync.Mutex
	eventCh := make(chan struct{}, 10)

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		eventCh <- struct{}{}
	})

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a new manifest file
	manifestContent := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: test-agent
  description: "Test agent for hot reload"
spec:
  type: worker
  capabilities:
    - name: testing
      description: "Test capability"
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "You are a test agent"
`

	manifestPath := filepath.Join(tmpDir, "test-agent.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Wait for event
	select {
	case <-eventCh:
		// Event received
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for create event")
	}

	// Verify manifest was loaded
	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("No events received")
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Type != EventCreated {
		t.Errorf("Expected created event, got %s", lastEvent.Type)
	}
	if lastEvent.Name != "test-agent" {
		t.Errorf("Expected name test-agent, got %s", lastEvent.Name)
	}

	// Verify manifest is in store
	manifest, ok := store.GetManifest("test-agent")
	if !ok {
		t.Fatal("Manifest not found in store after hot reload")
	}
	if manifest.Metadata.Description != "Test agent for hot reload" {
		t.Error("Manifest data mismatch")
	}
}

// TestHotReloadUpdate tests that modified manifest files are reloaded
func TestHotReloadUpdate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create initial manifest
	manifestPath := filepath.Join(tmpDir, "update-test.yaml")
	initialContent := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: update-test
  description: "Initial description"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Initial prompt"
`
	if err := os.WriteFile(manifestPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial manifest: %v", err)
	}

	// Create store (will load initial manifest)
	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Verify initial load
	manifest, ok := store.GetManifest("update-test")
	if !ok {
		t.Fatal("Initial manifest not loaded")
	}
	if manifest.Metadata.Description != "Initial description" {
		t.Error("Initial description mismatch")
	}

	// Track events
	var events []ManifestEvent
	var mu sync.Mutex
	eventCh := make(chan struct{}, 10)

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		eventCh <- struct{}{}
	})

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Update the manifest
	updatedContent := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: update-test
  description: "Updated description"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Updated prompt"
`
	if err := os.WriteFile(manifestPath, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("Failed to write updated manifest: %v", err)
	}

	// Wait for event
	select {
	case <-eventCh:
		// Event received
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for update event")
	}

	// Verify update
	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("No events received")
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Type != EventUpdated {
		t.Errorf("Expected updated event, got %s", lastEvent.Type)
	}

	// Verify manifest was updated in store
	manifest, ok = store.GetManifest("update-test")
	if !ok {
		t.Fatal("Manifest not found after update")
	}
	if manifest.Metadata.Description != "Updated description" {
		t.Errorf("Expected 'Updated description', got '%s'", manifest.Metadata.Description)
	}
}

// TestHotReloadDelete tests that deleted manifest files are removed from store
func TestHotReloadDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create initial manifest
	manifestPath := filepath.Join(tmpDir, "delete-test.yaml")
	content := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: delete-test
  description: "To be deleted"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Test prompt"
`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create store
	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Verify initial load
	_, ok := store.GetManifest("delete-test")
	if !ok {
		t.Fatal("Initial manifest not loaded")
	}

	// Track events
	var events []ManifestEvent
	var mu sync.Mutex
	eventCh := make(chan struct{}, 10)

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		eventCh <- struct{}{}
	})

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Delete the manifest file
	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("Failed to delete manifest: %v", err)
	}

	// Wait for event
	select {
	case <-eventCh:
		// Event received
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for delete event")
	}

	// Verify deletion
	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("No events received")
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Type != EventDeleted {
		t.Errorf("Expected deleted event, got %s", lastEvent.Type)
	}
	if lastEvent.Name != "delete-test" {
		t.Errorf("Expected name delete-test, got %s", lastEvent.Name)
	}

	// Verify manifest was removed from store
	_, ok = store.GetManifest("delete-test")
	if ok {
		t.Error("Manifest should have been removed from store")
	}
}

// TestMultipleCallbacks tests that multiple callbacks receive events
func TestMultipleCallbacks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Register multiple callbacks
	var count1, count2 int
	var mu sync.Mutex
	done := make(chan struct{}, 2)

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		count1++
		mu.Unlock()
		done <- struct{}{}
	})

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		count2++
		mu.Unlock()
		done <- struct{}{}
	})

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create a manifest
	content := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: callback-test
  description: "Test"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Test"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "callback-test.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Wait for both callbacks
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("Timeout waiting for callback %d", i+1)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if count1 != 1 {
		t.Errorf("Callback 1 expected 1 call, got %d", count1)
	}
	if count2 != 1 {
		t.Errorf("Callback 2 expected 1 call, got %d", count2)
	}
}

// TestNonYAMLFilesIgnored tests that non-YAML files are ignored
func TestNonYAMLFilesIgnored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	var eventReceived bool
	var mu sync.Mutex

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		eventReceived = true
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create non-YAML files
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notes.md"), []byte("# Notes"), 0644)

	// Wait a bit
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if eventReceived {
		t.Error("Should not receive events for non-YAML files")
	}
}

// TestStoreClose tests that closing the store stops watching
func TestStoreClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Create a file after close - should not cause issues
	content := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: after-close
  description: "Test"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Test"
`
	os.WriteFile(filepath.Join(tmpDir, "after-close.yaml"), []byte(content), 0644)

	// Wait a bit and verify no panics or errors
	time.Sleep(200 * time.Millisecond)

	// Manifest should not be in store (watcher closed)
	_, ok := store.GetManifest("after-close")
	if ok {
		t.Error("Manifest should not have been loaded after close")
	}
}

// TestInvalidManifestIgnored tests that invalid YAML files don't crash the watcher
func TestInvalidManifestIgnored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	var events []ManifestEvent
	var mu sync.Mutex
	eventCh := make(chan struct{}, 10)

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		eventCh <- struct{}{}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create invalid YAML
	os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte("not: valid: yaml: [[["), 0644)

	// Wait a bit
	time.Sleep(500 * time.Millisecond)

	// Create a valid manifest after the invalid one
	validContent := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: valid-after-invalid
  description: "Valid manifest"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Test"
`
	os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(validContent), 0644)

	// Wait for valid manifest event
	select {
	case <-eventCh:
		// Event received
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for valid manifest event")
	}

	// Verify valid manifest was loaded
	_, ok := store.GetManifest("valid-after-invalid")
	if !ok {
		t.Error("Valid manifest should have been loaded after invalid one")
	}
}

// TestRapidFileChanges tests handling of rapid file changes (debouncing behavior)
func TestRapidFileChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "syntor-hotreload-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewManifestStore([]string{tmpDir})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	var eventCount int
	var mu sync.Mutex

	store.OnChange(func(e ManifestEvent) {
		mu.Lock()
		eventCount++
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.StartWatching(ctx); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create manifest
	manifestPath := filepath.Join(tmpDir, "rapid-test.yaml")
	baseContent := `apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: rapid-test
  description: "Version %d"
spec:
  type: worker
  capabilities:
    - name: testing
  model:
    default: claude-3-5-sonnet
  prompt:
    system: "Test"
`

	// Rapidly update the file multiple times
	for i := 1; i <= 5; i++ {
		content := []byte(fmt.Sprintf(baseContent, i))
		if err := os.WriteFile(manifestPath, content, 0644); err != nil {
			t.Fatalf("Failed to write manifest: %v", err)
		}
		time.Sleep(50 * time.Millisecond) // Small delay between writes
	}

	// Wait for events to process
	time.Sleep(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// We should have received at least one event
	if eventCount == 0 {
		t.Error("Expected at least one event")
	}

	// Verify final state
	manifest, ok := store.GetManifest("rapid-test")
	if !ok {
		t.Fatal("Manifest not found")
	}

	// Description should be "Version 5" (the last write)
	if manifest.Metadata.Description != "Version 5" {
		t.Errorf("Expected final version, got '%s'", manifest.Metadata.Description)
	}
}

// TestWatchNonExistentDirectory tests graceful handling of non-existent directories
func TestWatchNonExistentDirectory(t *testing.T) {
	nonExistentPath := "/tmp/syntor-does-not-exist-12345"

	store, err := NewManifestStore([]string{nonExistentPath})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should not error when watching non-existent directory
	if err := store.StartWatching(ctx); err != nil {
		t.Errorf("StartWatching should handle non-existent directories gracefully: %v", err)
	}
}

