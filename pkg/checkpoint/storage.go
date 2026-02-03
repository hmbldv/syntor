package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// FileStorage implements Storage using the local filesystem.
type FileStorage struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStorage creates a new file-based storage.
func NewFileStorage(baseDir string) (*FileStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create base directory: %w", err)
	}

	return &FileStorage{
		baseDir: baseDir,
	}, nil
}

// Save stores a checkpoint to disk.
func (s *FileStorage) Save(ctx context.Context, checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create session directory
	sessionDir := filepath.Join(s.baseDir, checkpoint.SessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	// Serialize checkpoint
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	// Write to file
	checkpointFile := filepath.Join(sessionDir, checkpoint.ID+".json")
	if err := os.WriteFile(checkpointFile, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint file: %w", err)
	}

	// Write large file contents separately if needed
	for i, snap := range checkpoint.Snapshots {
		if snap.ContentRef != "" && len(snap.Content) > 0 {
			contentFile := filepath.Join(sessionDir, "blobs", snap.ContentRef)
			if err := os.MkdirAll(filepath.Dir(contentFile), 0755); err != nil {
				continue
			}
			if err := os.WriteFile(contentFile, snap.Content, 0644); err != nil {
				continue
			}
			// Clear inline content after saving to blob
			checkpoint.Snapshots[i].Content = nil
		}
	}

	return nil
}

// Load retrieves a checkpoint from disk.
func (s *FileStorage) Load(ctx context.Context, id string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Search all session directories for the checkpoint
	var checkpointFile string
	var sessionDir string

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("read base directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		candidate := filepath.Join(s.baseDir, entry.Name(), id+".json")
		if _, err := os.Stat(candidate); err == nil {
			checkpointFile = candidate
			sessionDir = filepath.Join(s.baseDir, entry.Name())
			break
		}
	}

	if checkpointFile == "" {
		return nil, fmt.Errorf("checkpoint not found: %s", id)
	}

	// Read checkpoint file
	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint file: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	// Load blob contents if needed
	for i, snap := range checkpoint.Snapshots {
		if snap.ContentRef != "" && len(snap.Content) == 0 {
			blobFile := filepath.Join(sessionDir, "blobs", snap.ContentRef)
			content, err := os.ReadFile(blobFile)
			if err == nil {
				checkpoint.Snapshots[i].Content = content
			}
		}
	}

	return &checkpoint, nil
}

// Delete removes a checkpoint from disk.
func (s *FileStorage) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and delete the checkpoint file
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return fmt.Errorf("read base directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		checkpointFile := filepath.Join(s.baseDir, entry.Name(), id+".json")
		if _, err := os.Stat(checkpointFile); err == nil {
			return os.Remove(checkpointFile)
		}
	}

	return nil // Not found is not an error for delete
}

// List returns summaries of all checkpoints for a session.
func (s *FileStorage) List(ctx context.Context, sessionID string) ([]CheckpointSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionDir := filepath.Join(s.baseDir, sessionID)
	entries, err := os.ReadDir(sessionDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session directory: %w", err)
	}

	var summaries []CheckpointSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		checkpointFile := filepath.Join(sessionDir, entry.Name())
		data, err := os.ReadFile(checkpointFile)
		if err != nil {
			continue
		}

		var checkpoint Checkpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			continue
		}

		summaries = append(summaries, CheckpointSummary{
			ID:          checkpoint.ID,
			SessionID:   checkpoint.SessionID,
			Name:        checkpoint.Name,
			Type:        checkpoint.Type,
			Description: checkpoint.Description,
			CreatedAt:   checkpoint.CreatedAt,
			Size:        checkpoint.Size,
			FileCount:   len(checkpoint.Snapshots),
			Restorable:  checkpoint.Restorable,
		})
	}

	// Sort by creation time (newest first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})

	return summaries, nil
}

// GetSize returns the total size of all checkpoints for a session.
func (s *FileStorage) GetSize(ctx context.Context, sessionID string) (int64, error) {
	summaries, err := s.List(ctx, sessionID)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, summary := range summaries {
		total += summary.Size
	}
	return total, nil
}

// ListAllSessions returns all session IDs with checkpoints.
func (s *FileStorage) ListAllSessions(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("read base directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "blobs" {
			sessions = append(sessions, entry.Name())
		}
	}

	return sessions, nil
}

// CleanupSession removes all checkpoints for a session.
func (s *FileStorage) CleanupSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionDir := filepath.Join(s.baseDir, sessionID)
	return os.RemoveAll(sessionDir)
}

// GetStats returns storage statistics.
func (s *FileStorage) GetStats(ctx context.Context) (*StorageStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &StorageStats{
		SessionStats: make(map[string]SessionStats),
	}

	sessions, err := s.ListAllSessions(ctx)
	if err != nil {
		return nil, err
	}

	for _, sessionID := range sessions {
		summaries, err := s.List(ctx, sessionID)
		if err != nil {
			continue
		}

		var sessionSize int64
		for _, summary := range summaries {
			sessionSize += summary.Size
		}

		stats.TotalCheckpoints += len(summaries)
		stats.TotalSize += sessionSize
		stats.SessionStats[sessionID] = SessionStats{
			CheckpointCount: len(summaries),
			TotalSize:       sessionSize,
		}
	}

	stats.SessionCount = len(sessions)
	return stats, nil
}

// StorageStats contains overall storage statistics.
type StorageStats struct {
	SessionCount     int                     `json:"session_count"`
	TotalCheckpoints int                     `json:"total_checkpoints"`
	TotalSize        int64                   `json:"total_size"`
	SessionStats     map[string]SessionStats `json:"session_stats"`
}

// SessionStats contains statistics for a single session.
type SessionStats struct {
	CheckpointCount int   `json:"checkpoint_count"`
	TotalSize       int64 `json:"total_size"`
}

// Verify FileStorage implements Storage
var _ Storage = (*FileStorage)(nil)
