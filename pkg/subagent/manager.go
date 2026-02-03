package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager handles sub-agent lifecycle and execution.
type Manager struct {
	// Configuration
	config Config

	// Active sub-agents
	agents   map[string]*SubAgent
	agentsMu sync.RWMutex

	// Trust tracking for promotion/demotion
	trustTracker map[string]*TrustRecord
	trustMu      sync.RWMutex

	// Event observers
	observers   []Observer
	observersMu sync.RWMutex

	// Dependencies
	executor Executor

	// Shutdown
	done chan struct{}
}

// Config holds manager configuration.
type Config struct {
	// MaxConcurrent limits parallel sub-agents
	MaxConcurrent int `yaml:"max_concurrent" json:"max_concurrent"`

	// DefaultTimeout for sub-agent operations
	DefaultTimeout time.Duration `yaml:"default_timeout" json:"default_timeout"`

	// PromotionCriteria for trust level advancement
	PromotionCriteria PromotionCriteria `yaml:"promotion_criteria" json:"promotion_criteria"`

	// AutoPromote enables automatic trust promotion
	AutoPromote bool `yaml:"auto_promote" json:"auto_promote"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxConcurrent:     5,
		DefaultTimeout:    10 * time.Minute,
		PromotionCriteria: DefaultPromotionCriteria(),
		AutoPromote:       true,
	}
}

// Executor defines the interface for executing sub-agent tasks.
type Executor interface {
	Execute(ctx context.Context, agent *SubAgent) (*Result, error)
}

// TrustRecord tracks execution history for trust management.
type TrustRecord struct {
	AgentType        string
	ConsecutiveSuccesses int
	TotalSuccesses   int
	TotalFailures    int
	UserOverrides    int
	LastExecution    time.Time
	CurrentTrust     TrustLevel
}

// NewManager creates a new sub-agent manager.
func NewManager(config Config, executor Executor) *Manager {
	return &Manager{
		config:       config,
		agents:       make(map[string]*SubAgent),
		trustTracker: make(map[string]*TrustRecord),
		observers:    nil,
		executor:     executor,
		done:         make(chan struct{}),
	}
}

// Spawn creates and optionally starts a new sub-agent.
func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (*SubAgent, error) {
	// Generate ID
	id := uuid.New().String()[:8]

	// Set defaults
	timeout := req.Timeout
	if timeout == 0 {
		timeout = m.config.DefaultTimeout
	}

	// Get trust level from tracker if available
	trustLevel := req.TrustLevel
	if record := m.getTrustRecord(req.AgentType); record != nil {
		trustLevel = record.CurrentTrust
	}

	agent := &SubAgent{
		ID:         id,
		Name:       req.Name,
		AgentType:  req.AgentType,
		TrustLevel: trustLevel,
		Timeout:    timeout,
		MaxRetries: req.MaxRetries,
		Status:     StatusPending,
		CreatedAt:  time.Now(),
		Task:       req.Task,
		Context: &Context{
			WorkingDir:   req.WorkingDir,
			Variables:    req.Variables,
			AllowedTools: req.AllowedTools,
			Constraints:  req.Constraints,
			Messages:     nil,
		},
	}

	// Register the agent
	m.agentsMu.Lock()
	m.agents[agent.ID] = agent
	m.agentsMu.Unlock()

	// Emit spawned event
	m.emit(ctx, Event{
		Type:      EventSpawned,
		AgentID:   agent.ID,
		Timestamp: time.Now(),
		Data:      agent,
	})

	// Start execution if not running in background with delayed start
	if !req.RunInBackground {
		go m.execute(ctx, agent)
	}

	return agent, nil
}

// SpawnAndWait spawns a sub-agent and waits for completion.
func (m *Manager) SpawnAndWait(ctx context.Context, req SpawnRequest) (*SubAgent, error) {
	agent, err := m.Spawn(ctx, req)
	if err != nil {
		return nil, err
	}

	return m.Wait(ctx, agent.ID)
}

// Wait waits for a sub-agent to complete.
func (m *Manager) Wait(ctx context.Context, agentID string) (*SubAgent, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			agent := m.Get(agentID)
			if agent == nil {
				return nil, fmt.Errorf("agent not found: %s", agentID)
			}

			switch agent.Status {
			case StatusCompleted, StatusFailed, StatusCancelled:
				return agent, nil
			}
		}
	}
}

// execute runs the sub-agent's task.
func (m *Manager) execute(ctx context.Context, agent *SubAgent) {
	// Set timeout context
	execCtx := ctx
	if agent.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, agent.Timeout)
		defer cancel()
	}

	// Update status
	m.updateStatus(agent, StatusRunning)
	now := time.Now()
	agent.StartedAt = &now

	m.emit(execCtx, Event{
		Type:      EventStarted,
		AgentID:   agent.ID,
		Timestamp: time.Now(),
	})

	// Execute with retries
	var result *Result
	var err error
	attempts := agent.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		result, err = m.executor.Execute(execCtx, agent)
		if err == nil && result.Success {
			break
		}

		// Check if we should retry
		if attempt < attempts-1 {
			select {
			case <-execCtx.Done():
				err = execCtx.Err()
				break
			case <-time.After(time.Second * time.Duration(attempt+1)):
				// Exponential backoff
			}
		}
	}

	// Record result
	completed := time.Now()
	agent.CompletedAt = &completed
	agent.Result = result

	if err != nil || (result != nil && !result.Success) {
		m.updateStatus(agent, StatusFailed)
		agent.FailureCount++
		m.recordExecution(agent.AgentType, false)

		m.emit(ctx, Event{
			Type:      EventFailed,
			AgentID:   agent.ID,
			Timestamp: time.Now(),
			Data:      result,
		})
	} else {
		m.updateStatus(agent, StatusCompleted)
		agent.SuccessCount++
		m.recordExecution(agent.AgentType, true)

		m.emit(ctx, Event{
			Type:      EventCompleted,
			AgentID:   agent.ID,
			Timestamp: time.Now(),
			Data:      result,
		})

		// Check for promotion
		if m.config.AutoPromote {
			m.checkPromotion(ctx, agent.AgentType)
		}
	}
}

// updateStatus updates the agent's status thread-safely.
func (m *Manager) updateStatus(agent *SubAgent, status Status) {
	m.agentsMu.Lock()
	defer m.agentsMu.Unlock()
	agent.Status = status
}

// Get retrieves a sub-agent by ID.
func (m *Manager) Get(agentID string) *SubAgent {
	m.agentsMu.RLock()
	defer m.agentsMu.RUnlock()
	return m.agents[agentID]
}

// List returns all sub-agents, optionally filtered by status.
func (m *Manager) List(status Status) []*SubAgent {
	m.agentsMu.RLock()
	defer m.agentsMu.RUnlock()

	var result []*SubAgent
	for _, agent := range m.agents {
		if status == "" || agent.Status == status {
			result = append(result, agent)
		}
	}
	return result
}

// Cancel cancels a running sub-agent.
func (m *Manager) Cancel(ctx context.Context, agentID string) error {
	agent := m.Get(agentID)
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	if agent.Status != StatusRunning && agent.Status != StatusPending && agent.Status != StatusWaiting {
		return fmt.Errorf("agent not in cancellable state: %s", agent.Status)
	}

	m.updateStatus(agent, StatusCancelled)
	now := time.Now()
	agent.CompletedAt = &now

	m.emit(ctx, Event{
		Type:      EventCancelled,
		AgentID:   agent.ID,
		Timestamp: time.Now(),
	})

	return nil
}

// Parallel spawns multiple sub-agents and runs them in parallel.
func (m *Manager) Parallel(ctx context.Context, req ParallelRequest) (*ParallelResult, error) {
	start := time.Now()

	// Set timeout
	execCtx := ctx
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Spawn all agents
	var wg sync.WaitGroup
	resultCh := make(chan struct {
		id     string
		result *Result
		err    error
	}, len(req.Agents))

	failFastCtx, failFastCancel := context.WithCancel(execCtx)
	defer failFastCancel()

	for _, spawnReq := range req.Agents {
		wg.Add(1)
		go func(sr SpawnRequest) {
			defer wg.Done()

			agent, err := m.SpawnAndWait(failFastCtx, sr)
			if err != nil {
				resultCh <- struct {
					id     string
					result *Result
					err    error
				}{sr.Name, nil, err}

				if req.FailFast {
					failFastCancel()
				}
				return
			}

			resultCh <- struct {
				id     string
				result *Result
				err    error
			}{agent.ID, agent.Result, nil}

			if req.FailFast && agent.Result != nil && !agent.Result.Success {
				failFastCancel()
			}
		}(spawnReq)
	}

	// Wait for all to complete
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	parallelResult := &ParallelResult{
		Results: make(map[string]*Result),
	}

	for r := range resultCh {
		if r.err != nil || (r.result != nil && !r.result.Success) {
			parallelResult.Failed = append(parallelResult.Failed, r.id)
		} else {
			parallelResult.Completed = append(parallelResult.Completed, r.id)
		}
		if r.result != nil {
			parallelResult.Results[r.id] = r.result
		}
	}

	parallelResult.Duration = time.Since(start)
	return parallelResult, nil
}

// Trust Management

func (m *Manager) getTrustRecord(agentType string) *TrustRecord {
	m.trustMu.RLock()
	defer m.trustMu.RUnlock()
	return m.trustTracker[agentType]
}

func (m *Manager) recordExecution(agentType string, success bool) {
	m.trustMu.Lock()
	defer m.trustMu.Unlock()

	record, ok := m.trustTracker[agentType]
	if !ok {
		record = &TrustRecord{
			AgentType:    agentType,
			CurrentTrust: Level0Visible,
		}
		m.trustTracker[agentType] = record
	}

	record.LastExecution = time.Now()

	if success {
		record.ConsecutiveSuccesses++
		record.TotalSuccesses++
	} else {
		record.ConsecutiveSuccesses = 0
		record.TotalFailures++
	}
}

func (m *Manager) checkPromotion(ctx context.Context, agentType string) {
	m.trustMu.Lock()
	defer m.trustMu.Unlock()

	record, ok := m.trustTracker[agentType]
	if !ok || record.CurrentTrust >= Level2Background {
		return
	}

	criteria := m.config.PromotionCriteria

	// Check demotion triggers
	if record.TotalFailures >= 2 {
		if record.CurrentTrust > Level0Visible {
			record.CurrentTrust--
			m.emit(ctx, Event{
				Type:      EventDemoted,
				AgentID:   agentType,
				Timestamp: time.Now(),
				Data:      record.CurrentTrust,
			})
		}
		return
	}

	// Check promotion criteria
	if record.ConsecutiveSuccesses >= criteria.MinSuccessCount &&
		record.UserOverrides == 0 {

		record.CurrentTrust++
		m.emit(ctx, Event{
			Type:      EventPromoted,
			AgentID:   agentType,
			Timestamp: time.Now(),
			Data:      record.CurrentTrust,
		})
	}
}

// RecordUserOverride records that a user overrode a sub-agent's action.
func (m *Manager) RecordUserOverride(agentType string) {
	m.trustMu.Lock()
	defer m.trustMu.Unlock()

	record, ok := m.trustTracker[agentType]
	if !ok {
		return
	}

	record.UserOverrides++
	record.ConsecutiveSuccesses = 0
}

// Demote forces a trust level demotion.
func (m *Manager) Demote(ctx context.Context, agentType string) {
	m.trustMu.Lock()
	defer m.trustMu.Unlock()

	record, ok := m.trustTracker[agentType]
	if !ok || record.CurrentTrust <= Level0Visible {
		return
	}

	record.CurrentTrust--
	m.emit(ctx, Event{
		Type:      EventDemoted,
		AgentID:   agentType,
		Timestamp: time.Now(),
		Data:      record.CurrentTrust,
	})
}

// Promote forces a trust level promotion.
func (m *Manager) Promote(ctx context.Context, agentType string) error {
	m.trustMu.Lock()
	defer m.trustMu.Unlock()

	record, ok := m.trustTracker[agentType]
	if !ok {
		record = &TrustRecord{
			AgentType:    agentType,
			CurrentTrust: Level0Visible,
		}
		m.trustTracker[agentType] = record
	}

	if record.CurrentTrust >= Level2Background {
		return fmt.Errorf("already at maximum trust level")
	}

	record.CurrentTrust++
	m.emit(ctx, Event{
		Type:      EventPromoted,
		AgentID:   agentType,
		Timestamp: time.Now(),
		Data:      record.CurrentTrust,
	})

	return nil
}

// GetTrustLevel returns the current trust level for an agent type.
func (m *Manager) GetTrustLevel(agentType string) TrustLevel {
	m.trustMu.RLock()
	defer m.trustMu.RUnlock()

	if record, ok := m.trustTracker[agentType]; ok {
		return record.CurrentTrust
	}
	return Level0Visible
}

// Observer Management

// AddObserver adds an event observer.
func (m *Manager) AddObserver(observer Observer) {
	m.observersMu.Lock()
	defer m.observersMu.Unlock()
	m.observers = append(m.observers, observer)
}

// RemoveObserver removes an event observer.
func (m *Manager) RemoveObserver(observer Observer) {
	m.observersMu.Lock()
	defer m.observersMu.Unlock()

	for i, o := range m.observers {
		if o == observer {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

func (m *Manager) emit(ctx context.Context, event Event) {
	m.observersMu.RLock()
	observers := make([]Observer, len(m.observers))
	copy(observers, m.observers)
	m.observersMu.RUnlock()

	for _, o := range observers {
		go o.OnEvent(ctx, event)
	}
}

// Cleanup removes completed/failed agents older than the given duration.
func (m *Manager) Cleanup(maxAge time.Duration) int {
	m.agentsMu.Lock()
	defer m.agentsMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, agent := range m.agents {
		if agent.Status == StatusCompleted || agent.Status == StatusFailed || agent.Status == StatusCancelled {
			if agent.CompletedAt != nil && agent.CompletedAt.Before(cutoff) {
				delete(m.agents, id)
				removed++
			}
		}
	}

	return removed
}

// Close shuts down the manager.
func (m *Manager) Close() error {
	close(m.done)
	return nil
}
