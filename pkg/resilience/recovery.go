package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrNoCheckpoint     = errors.New("no checkpoint available")
	ErrRecoveryFailed   = errors.New("recovery failed")
	ErrCheckpointFailed = errors.New("checkpoint failed")
)

// Checkpoint represents a saved state
type Checkpoint struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agent_id"`
	Timestamp time.Time              `json:"timestamp"`
	State     map[string]interface{} `json:"state"`
	Version   int                    `json:"version"`
}

// CheckpointStore interface for storing and retrieving checkpoints
type CheckpointStore interface {
	Save(ctx context.Context, checkpoint Checkpoint) error
	Load(ctx context.Context, agentID string) (*Checkpoint, error)
	Delete(ctx context.Context, agentID string) error
	List(ctx context.Context) ([]Checkpoint, error)
}

// InMemoryCheckpointStore implements CheckpointStore in memory
type InMemoryCheckpointStore struct {
	checkpoints map[string]Checkpoint
	mu          sync.RWMutex
}

// NewInMemoryCheckpointStore creates a new in-memory checkpoint store
func NewInMemoryCheckpointStore() *InMemoryCheckpointStore {
	return &InMemoryCheckpointStore{
		checkpoints: make(map[string]Checkpoint),
	}
}

func (s *InMemoryCheckpointStore) Save(ctx context.Context, checkpoint Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpoints[checkpoint.AgentID] = checkpoint
	return nil
}

func (s *InMemoryCheckpointStore) Load(ctx context.Context, agentID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp, exists := s.checkpoints[agentID]
	if !exists {
		return nil, ErrNoCheckpoint
	}

	return &cp, nil
}

func (s *InMemoryCheckpointStore) Delete(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checkpoints, agentID)
	return nil
}

func (s *InMemoryCheckpointStore) List(ctx context.Context) ([]Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Checkpoint, 0, len(s.checkpoints))
	for _, cp := range s.checkpoints {
		result = append(result, cp)
	}
	return result, nil
}

// RecoveryManager manages agent recovery with checkpointing
type RecoveryManager struct {
	store            CheckpointStore
	checkpointInterval time.Duration
	maxCheckpoints   int

	// State extractors/restorers per agent type
	stateExtractors map[string]func() map[string]interface{}
	stateRestorers  map[string]func(map[string]interface{}) error

	mu sync.RWMutex
}

// RecoveryManagerConfig holds configuration for the recovery manager
type RecoveryManagerConfig struct {
	Store              CheckpointStore
	CheckpointInterval time.Duration
	MaxCheckpoints     int
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(config RecoveryManagerConfig) *RecoveryManager {
	if config.Store == nil {
		config.Store = NewInMemoryCheckpointStore()
	}
	if config.CheckpointInterval <= 0 {
		config.CheckpointInterval = 30 * time.Second
	}
	if config.MaxCheckpoints <= 0 {
		config.MaxCheckpoints = 10
	}

	return &RecoveryManager{
		store:              config.Store,
		checkpointInterval: config.CheckpointInterval,
		maxCheckpoints:     config.MaxCheckpoints,
		stateExtractors:    make(map[string]func() map[string]interface{}),
		stateRestorers:     make(map[string]func(map[string]interface{}) error),
	}
}

// RegisterStateExtractor registers a function to extract state for checkpointing
func (rm *RecoveryManager) RegisterStateExtractor(agentID string, extractor func() map[string]interface{}) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.stateExtractors[agentID] = extractor
}

// RegisterStateRestorer registers a function to restore state from checkpoint
func (rm *RecoveryManager) RegisterStateRestorer(agentID string, restorer func(map[string]interface{}) error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.stateRestorers[agentID] = restorer
}

// CreateCheckpoint creates a checkpoint for an agent
func (rm *RecoveryManager) CreateCheckpoint(ctx context.Context, agentID string) (*Checkpoint, error) {
	rm.mu.RLock()
	extractor, exists := rm.stateExtractors[agentID]
	rm.mu.RUnlock()

	var state map[string]interface{}
	if exists {
		state = extractor()
	} else {
		state = make(map[string]interface{})
	}

	// Load existing to get version
	existing, _ := rm.store.Load(ctx, agentID)
	version := 1
	if existing != nil {
		version = existing.Version + 1
	}

	checkpoint := Checkpoint{
		ID:        generateCheckpointID(agentID, version),
		AgentID:   agentID,
		Timestamp: time.Now(),
		State:     state,
		Version:   version,
	}

	if err := rm.store.Save(ctx, checkpoint); err != nil {
		return nil, ErrCheckpointFailed
	}

	return &checkpoint, nil
}

// Recover attempts to recover an agent from the latest checkpoint
func (rm *RecoveryManager) Recover(ctx context.Context, agentID string) (*Checkpoint, error) {
	checkpoint, err := rm.store.Load(ctx, agentID)
	if err != nil {
		return nil, err
	}

	rm.mu.RLock()
	restorer, exists := rm.stateRestorers[agentID]
	rm.mu.RUnlock()

	if exists {
		if err := restorer(checkpoint.State); err != nil {
			return nil, ErrRecoveryFailed
		}
	}

	return checkpoint, nil
}

// HasCheckpoint checks if a checkpoint exists for an agent
func (rm *RecoveryManager) HasCheckpoint(ctx context.Context, agentID string) bool {
	_, err := rm.store.Load(ctx, agentID)
	return err == nil
}

// DeleteCheckpoint deletes the checkpoint for an agent
func (rm *RecoveryManager) DeleteCheckpoint(ctx context.Context, agentID string) error {
	return rm.store.Delete(ctx, agentID)
}

// StartPeriodicCheckpointing starts periodic checkpointing for an agent
func (rm *RecoveryManager) StartPeriodicCheckpointing(ctx context.Context, agentID string) {
	ticker := time.NewTicker(rm.checkpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.CreateCheckpoint(ctx, agentID)
		}
	}
}

// generateCheckpointID generates a unique checkpoint ID
func generateCheckpointID(agentID string, version int) string {
	return agentID + "-" + time.Now().Format("20060102150405") + "-v" + string(rune('0'+version%10))
}

// FailureDetector detects agent failures
type FailureDetector struct {
	heartbeatInterval time.Duration
	failureThreshold  time.Duration
	lastHeartbeats    map[string]time.Time
	onFailure         func(agentID string)
	mu                sync.RWMutex
}

// FailureDetectorConfig holds configuration for failure detection
type FailureDetectorConfig struct {
	HeartbeatInterval time.Duration
	FailureThreshold  time.Duration
	OnFailure         func(agentID string)
}

// NewFailureDetector creates a new failure detector
func NewFailureDetector(config FailureDetectorConfig) *FailureDetector {
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 10 * time.Second
	}
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 30 * time.Second
	}

	return &FailureDetector{
		heartbeatInterval: config.HeartbeatInterval,
		failureThreshold:  config.FailureThreshold,
		lastHeartbeats:    make(map[string]time.Time),
		onFailure:         config.OnFailure,
	}
}

// RecordHeartbeat records a heartbeat from an agent
func (fd *FailureDetector) RecordHeartbeat(agentID string) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.lastHeartbeats[agentID] = time.Now()
}

// IsAlive checks if an agent is considered alive
func (fd *FailureDetector) IsAlive(agentID string) bool {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	lastHeartbeat, exists := fd.lastHeartbeats[agentID]
	if !exists {
		return false
	}

	return time.Since(lastHeartbeat) < fd.failureThreshold
}

// RemoveAgent removes an agent from tracking
func (fd *FailureDetector) RemoveAgent(agentID string) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	delete(fd.lastHeartbeats, agentID)
}

// CheckFailures checks for failed agents and triggers callbacks
func (fd *FailureDetector) CheckFailures() []string {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	var failed []string
	now := time.Now()

	for agentID, lastHeartbeat := range fd.lastHeartbeats {
		if now.Sub(lastHeartbeat) >= fd.failureThreshold {
			failed = append(failed, agentID)
			if fd.onFailure != nil {
				fd.onFailure(agentID)
			}
		}
	}

	return failed
}

// StartMonitoring starts background failure monitoring
func (fd *FailureDetector) StartMonitoring(ctx context.Context) {
	ticker := time.NewTicker(fd.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fd.CheckFailures()
		}
	}
}

// GetTrackedAgents returns the list of tracked agent IDs
func (fd *FailureDetector) GetTrackedAgents() []string {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	agents := make([]string, 0, len(fd.lastHeartbeats))
	for agentID := range fd.lastHeartbeats {
		agents = append(agents, agentID)
	}
	return agents
}

// MarshalCheckpoint serializes a checkpoint to JSON
func MarshalCheckpoint(cp *Checkpoint) ([]byte, error) {
	return json.Marshal(cp)
}

// UnmarshalCheckpoint deserializes a checkpoint from JSON
func UnmarshalCheckpoint(data []byte) (*Checkpoint, error) {
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
