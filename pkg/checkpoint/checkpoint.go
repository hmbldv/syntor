package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager handles checkpoint creation, storage, and restoration.
type Manager struct {
	config  StorageConfig
	policy  PolicyConfig
	storage Storage

	// Session tracking
	sessions   map[string]*SessionState
	sessionsMu sync.RWMutex

	// Cleanup
	done chan struct{}
}

// SessionState tracks checkpoint state for a session.
type SessionState struct {
	SessionID       string
	LastCheckpoint  time.Time
	CheckpointCount int
	TotalSize       int64
}

// Storage defines the interface for checkpoint persistence.
type Storage interface {
	Save(ctx context.Context, checkpoint *Checkpoint) error
	Load(ctx context.Context, id string) (*Checkpoint, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, sessionID string) ([]CheckpointSummary, error)
	GetSize(ctx context.Context, sessionID string) (int64, error)
}

// NewManager creates a checkpoint manager.
func NewManager(config StorageConfig, policy PolicyConfig) (*Manager, error) {
	// Expand home directory
	baseDir := expandPath(config.BaseDir)
	config.BaseDir = baseDir

	// Create storage directory
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create checkpoint directory: %w", err)
	}

	storage, err := NewFileStorage(baseDir)
	if err != nil {
		return nil, fmt.Errorf("create storage: %w", err)
	}

	m := &Manager{
		config:   config,
		policy:   policy,
		storage:  storage,
		sessions: make(map[string]*SessionState),
		done:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go m.cleanupLoop()

	return m, nil
}

// Create creates a new checkpoint.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*Checkpoint, error) {
	// Check if we can create a checkpoint
	state := m.getSessionState(req.SessionID)
	if m.policy.MinIntervalBetween > 0 && time.Since(state.LastCheckpoint) < m.policy.MinIntervalBetween {
		return nil, fmt.Errorf("too soon since last checkpoint (min interval: %s)", m.policy.MinIntervalBetween)
	}

	// Create checkpoint
	checkpoint := &Checkpoint{
		ID:          uuid.New().String()[:8],
		SessionID:   req.SessionID,
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		CreatedAt:   time.Now(),
		Messages:    req.Messages,
		Variables:   req.Variables,
		ToolHistory: req.ToolHistory,
		Metadata:    make(map[string]string),
		Restorable:  true,
	}

	if req.ExpiresIn > 0 {
		checkpoint.ExpiresAt = checkpoint.CreatedAt.Add(req.ExpiresIn)
	}

	// Capture file snapshots
	if len(req.IncludeFiles) > 0 {
		for _, path := range req.IncludeFiles {
			snapshot, err := m.snapshotFile(path, req.MaxFileSize, req.Compress)
			if err != nil {
				// Log but don't fail
				continue
			}
			checkpoint.Snapshots = append(checkpoint.Snapshots, *snapshot)
		}
	}

	if req.IncludeDir != "" {
		snapshots, err := m.snapshotDirectory(req.IncludeDir, req.MaxFileSize, req.Compress)
		if err != nil {
			return nil, fmt.Errorf("snapshot directory: %w", err)
		}
		checkpoint.Snapshots = append(checkpoint.Snapshots, snapshots...)
	}

	// Calculate total size
	for _, snap := range checkpoint.Snapshots {
		checkpoint.Size += snap.Size
	}

	// Save checkpoint
	if err := m.storage.Save(ctx, checkpoint); err != nil {
		return nil, fmt.Errorf("save checkpoint: %w", err)
	}

	// Update session state
	m.updateSessionState(req.SessionID, checkpoint)

	// Enforce limits
	go m.enforceLimits(ctx, req.SessionID)

	return checkpoint, nil
}

// Restore restores state from a checkpoint.
func (m *Manager) Restore(ctx context.Context, req RestoreRequest) (*RestoreResult, error) {
	start := time.Now()

	checkpoint, err := m.storage.Load(ctx, req.CheckpointID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	if !checkpoint.Restorable {
		return nil, fmt.Errorf("checkpoint is not restorable")
	}

	result := &RestoreResult{
		CheckpointID: req.CheckpointID,
		Success:      true,
	}

	// Create backup of current state if requested
	if req.CreateBackup {
		backupReq := CreateRequest{
			SessionID:   checkpoint.SessionID,
			Type:        TypeAuto,
			Description: fmt.Sprintf("Backup before restore from %s", req.CheckpointID),
			IncludeFiles: func() []string {
				var paths []string
				for _, snap := range checkpoint.Snapshots {
					paths = append(paths, snap.Path)
				}
				return paths
			}(),
		}
		_, err := m.Create(ctx, backupReq)
		if err != nil {
			// Log but continue
		}
	}

	// Restore files
	if req.RestoreFiles {
		for _, snap := range checkpoint.Snapshots {
			// Check if this file should be restored
			if len(req.FilesToRestore) > 0 {
				found := false
				for _, f := range req.FilesToRestore {
					if f == snap.Path {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Check if file exists and we should overwrite
			if !req.OverwriteExisting {
				if _, err := os.Stat(snap.Path); err == nil {
					result.FailedFiles = append(result.FailedFiles, RestoreError{
						Path:   snap.Path,
						Reason: "file exists and overwrite not enabled",
					})
					continue
				}
			}

			// Restore the file
			if err := m.restoreFile(&snap); err != nil {
				result.FailedFiles = append(result.FailedFiles, RestoreError{
					Path:   snap.Path,
					Reason: err.Error(),
				})
				result.Success = false
			} else {
				result.RestoredFiles = append(result.RestoredFiles, snap.Path)
			}
		}
	}

	if req.RestoreMessages {
		result.MessagesRestored = len(checkpoint.Messages)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// Get retrieves a checkpoint by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Checkpoint, error) {
	return m.storage.Load(ctx, id)
}

// List returns checkpoints for a session.
func (m *Manager) List(ctx context.Context, sessionID string) ([]CheckpointSummary, error) {
	return m.storage.List(ctx, sessionID)
}

// Delete removes a checkpoint.
func (m *Manager) Delete(ctx context.Context, id string) error {
	return m.storage.Delete(ctx, id)
}

// Diff compares a checkpoint against current state or another checkpoint.
func (m *Manager) Diff(ctx context.Context, req DiffRequest) (*DiffResult, error) {
	checkpoint, err := m.storage.Load(ctx, req.CheckpointID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	result := &DiffResult{
		FromID: req.CheckpointID,
	}

	if req.CurrentState {
		// Compare against current file system state
		for _, snap := range checkpoint.Snapshots {
			diff, err := m.diffFile(&snap)
			if err != nil {
				continue
			}
			result.FileDiffs = append(result.FileDiffs, *diff)
		}
	} else if req.OtherID != "" {
		// Compare against another checkpoint
		other, err := m.storage.Load(ctx, req.OtherID)
		if err != nil {
			return nil, fmt.Errorf("load other checkpoint: %w", err)
		}
		result.ToID = req.OtherID

		// Build map of other checkpoint's files
		otherFiles := make(map[string]*FileSnapshot)
		for i := range other.Snapshots {
			otherFiles[other.Snapshots[i].Path] = &other.Snapshots[i]
		}

		// Compare files
		seenPaths := make(map[string]bool)
		for _, snap := range checkpoint.Snapshots {
			seenPaths[snap.Path] = true
			other, exists := otherFiles[snap.Path]

			var diff FileDiff
			diff.Path = snap.Path
			diff.OldHash = snap.Hash
			diff.OldSize = snap.Size

			if !exists {
				diff.ChangeType = ChangeDeleted
			} else {
				diff.NewHash = other.Hash
				diff.NewSize = other.Size
				if snap.Hash == other.Hash {
					diff.ChangeType = ChangeUnchanged
				} else {
					diff.ChangeType = ChangeModified
				}
			}
			result.FileDiffs = append(result.FileDiffs, diff)
		}

		// Check for new files in other checkpoint
		for path, other := range otherFiles {
			if !seenPaths[path] {
				result.FileDiffs = append(result.FileDiffs, FileDiff{
					Path:       path,
					ChangeType: ChangeAdded,
					NewHash:    other.Hash,
					NewSize:    other.Size,
				})
			}
		}

		// Message diff
		if len(checkpoint.Messages) > 0 || len(other.Messages) > 0 {
			result.MessageDiff = &MessageDiff{
				OldCount: len(checkpoint.Messages),
				NewCount: len(other.Messages),
			}
			if len(other.Messages) > len(checkpoint.Messages) {
				result.MessageDiff.Added = len(other.Messages) - len(checkpoint.Messages)
			} else {
				result.MessageDiff.Removed = len(checkpoint.Messages) - len(other.Messages)
			}
		}
	}

	return result, nil
}

// AutoCheckpoint creates an automatic checkpoint if policy allows.
func (m *Manager) AutoCheckpoint(ctx context.Context, sessionID string, trigger string, affectedFiles []string) (*Checkpoint, error) {
	// Check policy
	switch trigger {
	case "edit":
		if !m.policy.AutoBeforeEdit {
			return nil, nil
		}
	case "bash":
		if !m.policy.AutoBeforeBash {
			return nil, nil
		}
	}

	// Check interval
	state := m.getSessionState(sessionID)
	if time.Since(state.LastCheckpoint) < m.policy.MinIntervalBetween {
		return nil, nil
	}

	// Filter files based on exclude patterns
	var includedFiles []string
	for _, f := range affectedFiles {
		if !m.isExcluded(f) {
			includedFiles = append(includedFiles, f)
		}
	}

	if len(includedFiles) == 0 {
		return nil, nil
	}

	return m.Create(ctx, CreateRequest{
		SessionID:    sessionID,
		Type:         TypeAuto,
		Description:  fmt.Sprintf("Auto checkpoint before %s", trigger),
		IncludeFiles: includedFiles,
		Compress:     true,
	})
}

// Snapshot helpers

func (m *Manager) snapshotFile(path string, maxSize int64, compress bool) (*FileSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	snapshot := &FileSnapshot{
		Path:        path,
		ModTime:     info.ModTime(),
		Size:        info.Size(),
		IsDirectory: info.IsDir(),
		Permissions: info.Mode().String(),
	}

	if info.IsDir() {
		return snapshot, nil
	}

	// Check size limit
	if maxSize > 0 && info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d > %d", info.Size(), maxSize)
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Calculate hash
	hash := sha256.Sum256(content)
	snapshot.Hash = hex.EncodeToString(hash[:])

	// Store content (could be compressed or referenced for large files)
	if int64(len(content)) > m.config.LargeFileThreshold {
		// Store as reference (not implemented - would use blob storage)
		snapshot.ContentRef = snapshot.Hash
	} else {
		snapshot.Content = content
		snapshot.Compressed = compress
		// TODO: Actually compress if compress=true
	}

	return snapshot, nil
}

func (m *Manager) snapshotDirectory(dir string, maxSize int64, compress bool) ([]FileSnapshot, error) {
	var snapshots []FileSnapshot

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip excluded patterns
		if m.isExcluded(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		snapshot, err := m.snapshotFile(path, maxSize, compress)
		if err != nil {
			return nil // Skip files we can't snapshot
		}

		snapshots = append(snapshots, *snapshot)
		return nil
	})

	return snapshots, err
}

func (m *Manager) restoreFile(snap *FileSnapshot) error {
	if snap.IsDirectory {
		return os.MkdirAll(snap.Path, 0755)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(snap.Path), 0755); err != nil {
		return err
	}

	// Get content
	var content []byte
	if snap.ContentRef != "" {
		// Load from blob storage (not implemented)
		return fmt.Errorf("blob storage not implemented")
	} else {
		content = snap.Content
		// TODO: Decompress if compressed
	}

	// Write file
	return os.WriteFile(snap.Path, content, 0644)
}

func (m *Manager) diffFile(snap *FileSnapshot) (*FileDiff, error) {
	diff := &FileDiff{
		Path:    snap.Path,
		OldHash: snap.Hash,
		OldSize: snap.Size,
	}

	info, err := os.Stat(snap.Path)
	if os.IsNotExist(err) {
		diff.ChangeType = ChangeDeleted
		return diff, nil
	}
	if err != nil {
		return nil, err
	}

	diff.NewSize = info.Size()

	// Calculate current hash
	content, err := os.ReadFile(snap.Path)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(content)
	diff.NewHash = hex.EncodeToString(hash[:])

	if diff.OldHash == diff.NewHash {
		diff.ChangeType = ChangeUnchanged
	} else {
		diff.ChangeType = ChangeModified
	}

	return diff, nil
}

func (m *Manager) isExcluded(path string) bool {
	for _, pattern := range m.policy.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
		// Also check against full path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

// Session state management

func (m *Manager) getSessionState(sessionID string) *SessionState {
	m.sessionsMu.RLock()
	state, ok := m.sessions[sessionID]
	m.sessionsMu.RUnlock()

	if !ok {
		state = &SessionState{SessionID: sessionID}
		m.sessionsMu.Lock()
		m.sessions[sessionID] = state
		m.sessionsMu.Unlock()
	}

	return state
}

func (m *Manager) updateSessionState(sessionID string, checkpoint *Checkpoint) {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	state, ok := m.sessions[sessionID]
	if !ok {
		state = &SessionState{SessionID: sessionID}
		m.sessions[sessionID] = state
	}

	state.LastCheckpoint = checkpoint.CreatedAt
	state.CheckpointCount++
	state.TotalSize += checkpoint.Size
}

// Cleanup and limits

func (m *Manager) enforceLimits(ctx context.Context, sessionID string) {
	// Get all checkpoints for session
	summaries, err := m.storage.List(ctx, sessionID)
	if err != nil {
		return
	}

	// Sort by creation time (oldest first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.Before(summaries[j].CreatedAt)
	})

	// Enforce max count
	if m.config.MaxCheckpoints > 0 && len(summaries) > m.config.MaxCheckpoints {
		toDelete := len(summaries) - m.config.MaxCheckpoints
		for i := 0; i < toDelete; i++ {
			m.storage.Delete(ctx, summaries[i].ID)
		}
	}

	// Enforce max total size
	if m.config.MaxTotalSize > 0 {
		totalSize, _ := m.storage.GetSize(ctx, sessionID)
		for totalSize > m.config.MaxTotalSize && len(summaries) > 0 {
			m.storage.Delete(ctx, summaries[0].ID)
			totalSize -= summaries[0].Size
			summaries = summaries[1:]
		}
	}
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.cleanupExpired(context.Background())
		}
	}
}

func (m *Manager) cleanupExpired(ctx context.Context) {
	// For each session, cleanup expired checkpoints
	m.sessionsMu.RLock()
	sessions := make([]string, 0, len(m.sessions))
	for sid := range m.sessions {
		sessions = append(sessions, sid)
	}
	m.sessionsMu.RUnlock()

	for _, sessionID := range sessions {
		summaries, err := m.storage.List(ctx, sessionID)
		if err != nil {
			continue
		}

		now := time.Now()
		for _, summary := range summaries {
			// Check age
			if m.config.MaxAge > 0 && now.Sub(summary.CreatedAt) > m.config.MaxAge {
				m.storage.Delete(ctx, summary.ID)
			}
		}
	}
}

// Close shuts down the manager.
func (m *Manager) Close() error {
	close(m.done)
	return nil
}

// Helper functions

func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
