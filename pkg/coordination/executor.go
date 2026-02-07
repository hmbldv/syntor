package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/falkordb"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/prompt"
)

// Executor handles handoff execution between agents
type Executor struct {
	registry       *inference.Registry
	manifestStore  *manifest.ManifestStore
	promptBuilder  *prompt.Builder
	falkorDB       *falkordb.Client // For dynamic agent model lookup
	activeHandoffs map[string]*HandoffStatus
	planExecutions map[string]*PlanExecution
	timeline       []TimelineEvent
	mu             sync.RWMutex
}

// ExecutorConfig configures the executor
type ExecutorConfig struct {
	MaxConcurrent int
	DefaultTimeout time.Duration
}

// NewExecutor creates a new handoff executor
func NewExecutor(registry *inference.Registry, store *manifest.ManifestStore, builder *prompt.Builder, falkorClient *falkordb.Client) *Executor {
	return &Executor{
		registry:       registry,
		manifestStore:  store,
		promptBuilder:  builder,
		falkorDB:       falkorClient,
		activeHandoffs: make(map[string]*HandoffStatus),
		planExecutions: make(map[string]*PlanExecution),
		timeline:       make([]TimelineEvent, 0),
	}
}

// Execute performs a handoff to another agent
func (e *Executor) Execute(ctx context.Context, intent *HandoffIntent) (*HandoffResult, error) {
	startTime := time.Now()
	handoffID := uuid.New().String()

	// Create handoff status
	status := &HandoffStatus{
		ID:        handoffID,
		FromAgent: "sntr",
		ToAgent:   intent.Target,
		Task:      intent.Task,
		Status:    HandoffExecuting,
		StartTime: startTime,
	}

	e.mu.Lock()
	e.activeHandoffs[handoffID] = status
	e.addTimelineEvent(TimelineEvent{
		Time:      startTime,
		Agent:     intent.Target,
		EventType: EventStarted,
		Message:   fmt.Sprintf("Handoff to %s started", intent.Target),
		Details:   intent.Task,
	})
	e.mu.Unlock()

	// Validate target agent exists
	agentManifest, ok := e.manifestStore.GetManifest(intent.Target)
	if !ok {
		return e.failHandoff(status, fmt.Errorf("target agent not found: %s", intent.Target))
	}

	// Parse timeout if specified
	timeout := 5 * time.Minute // Default
	if intent.Timeout != "" {
		if parsed, err := time.ParseDuration(intent.Timeout); err == nil {
			timeout = parsed
		}
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build system prompt for target agent
	systemPrompt, err := e.promptBuilder.Build(execCtx, intent.Target, prompt.BuildOptions{
		IncludeAgents:  true,
		IncludeProject: true,
	})
	if err != nil {
		// Fall back to manifest's raw system prompt
		systemPrompt = agentManifest.Spec.Prompt.System
	}

	// Get model for target agent - uses dynamic resolver (database) with static fallback
	modelID := e.registry.GetModelForAgentDynamic(execCtx, intent.Target)

	// Get provider for model
	providerName, found := e.registry.GetProviderForModel(modelID)
	if !found {
		providerName = "ollama" // Default fallback
	}

	provider, ok := e.registry.GetProvider(providerName)
	if !ok {
		return e.failHandoff(status, fmt.Errorf("provider not available: %s", providerName))
	}

	// Build messages for inference
	messages := []inference.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: e.formatTaskMessage(intent)},
	}

	// Execute inference
	request := inference.ChatRequest{
		Model:    modelID,
		Messages: messages,
	}

	response, err := provider.Chat(execCtx, request)
	if err != nil {
		return e.failHandoff(status, fmt.Errorf("inference failed: %w", err))
	}

	// Mark handoff complete
	endTime := time.Now()
	status.Status = HandoffCompleted
	status.EndTime = &endTime

	result := &HandoffResult{
		ID:        handoffID,
		AgentName: intent.Target,
		Status:    ResultSuccess,
		Result:    response.Message.Content,
		Duration:  endTime.Sub(startTime),
		Timestamp: endTime,
		Metadata: map[string]interface{}{
			"model":          modelID,
			"tokens_prompt":  response.Usage.PromptTokens,
			"tokens_total":   response.Usage.TotalTokens,
		},
	}

	status.Result = result

	e.mu.Lock()
	e.addTimelineEvent(TimelineEvent{
		Time:      endTime,
		Agent:     intent.Target,
		EventType: EventCompleted,
		Message:   fmt.Sprintf("Handoff to %s completed", intent.Target),
		Details:   fmt.Sprintf("Duration: %v", result.Duration),
	})
	e.mu.Unlock()

	return result, nil
}

// ExecutePlan executes a multi-step plan, running independent steps concurrently.
func (e *Executor) ExecutePlan(ctx context.Context, plan *ExecutionPlan) (*PlanExecution, error) {
	execution := &PlanExecution{
		Plan:        plan,
		Status:      PlanExecuting,
		CurrentStep: 0,
		StepResults: make(map[int]*HandoffResult),
		StartedAt:   time.Now(),
	}

	e.mu.Lock()
	e.planExecutions[plan.ID] = execution
	e.mu.Unlock()

	// Build a map of step number -> step for dependency lookup
	stepByNum := make(map[int]PlanStep, len(plan.Steps))
	for _, step := range plan.Steps {
		stepByNum[step.StepNumber] = step
	}

	// Track completed steps
	completed := make(map[int]bool)
	var completedMu sync.Mutex

	// Execute in waves: find ready steps, run them concurrently, repeat
	for len(completed) < len(plan.Steps) {
		// Find steps whose dependencies are all satisfied
		var ready []PlanStep
		for _, step := range plan.Steps {
			if completed[step.StepNumber] {
				continue
			}
			allDepsReady := true
			for _, dep := range step.DependsOn {
				if !completed[dep] {
					allDepsReady = false
					break
				}
			}
			if allDepsReady {
				ready = append(ready, step)
			}
		}

		if len(ready) == 0 {
			// No progress possible — unsatisfied deps (should not happen with valid plans)
			execution.Status = PlanFailed
			execution.Error = "no executable steps remaining; possible circular dependency"
			return execution, fmt.Errorf("plan stuck: no ready steps but %d incomplete", len(plan.Steps)-len(completed))
		}

		// Execute ready steps concurrently
		type stepResult struct {
			stepNum int
			result  *HandoffResult
			err     error
		}
		resultCh := make(chan stepResult, len(ready))

		for _, step := range ready {
			go func(s PlanStep) {
				// Build intent
				intent := &HandoffIntent{
					Action:        ActionDelegate,
					Target:        s.Agent,
					Task:          s.Action,
					WaitForResult: true,
				}

				// Add context from completed steps
				completedMu.Lock()
				if len(execution.StepResults) > 0 {
					intent.Context = map[string]interface{}{
						"previousResults": e.summarizePreviousResults(execution.StepResults),
					}
				}
				completedMu.Unlock()

				result, err := e.Execute(ctx, intent)
				resultCh <- stepResult{stepNum: s.StepNumber, result: result, err: err}
			}(step)
		}

		// Collect results for this wave
		for range ready {
			sr := <-resultCh
			completedMu.Lock()
			execution.StepResults[sr.stepNum] = sr.result
			completedMu.Unlock()

			if sr.err != nil {
				execution.Status = PlanFailed
				execution.Error = fmt.Sprintf("step %d failed: %v", sr.stepNum, sr.err)
				return execution, sr.err
			}
			if sr.result != nil && sr.result.Status != ResultSuccess {
				execution.Status = PlanFailed
				execution.Error = fmt.Sprintf("step %d returned status %s", sr.stepNum, sr.result.Status)
				return execution, fmt.Errorf("step %d failed with status: %s", sr.stepNum, sr.result.Status)
			}

			completedMu.Lock()
			completed[sr.stepNum] = true
			execution.CurrentStep = len(completed)
			completedMu.Unlock()
		}
	}

	// Mark plan as completed
	now := time.Now()
	execution.Status = PlanCompleted
	execution.CompletedAt = &now

	return execution, nil
}

// GetActiveHandoffs returns all active handoffs
func (e *Executor) GetActiveHandoffs() []*HandoffStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	handoffs := make([]*HandoffStatus, 0, len(e.activeHandoffs))
	for _, h := range e.activeHandoffs {
		handoffs = append(handoffs, h)
	}
	return handoffs
}

// GetHandoffStatus returns the status of a specific handoff
func (e *Executor) GetHandoffStatus(id string) (*HandoffStatus, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	h, ok := e.activeHandoffs[id]
	return h, ok
}

// GetTimeline returns recent timeline events
func (e *Executor) GetTimeline(limit int) []TimelineEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > len(e.timeline) {
		limit = len(e.timeline)
	}

	// Return most recent events
	start := len(e.timeline) - limit
	if start < 0 {
		start = 0
	}

	events := make([]TimelineEvent, limit)
	copy(events, e.timeline[start:])
	return events
}

// CancelHandoff cancels an active handoff
func (e *Executor) CancelHandoff(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	status, ok := e.activeHandoffs[id]
	if !ok {
		return fmt.Errorf("handoff not found: %s", id)
	}

	if status.Status != HandoffPending && status.Status != HandoffExecuting {
		return fmt.Errorf("handoff already completed")
	}

	now := time.Now()
	status.Status = HandoffFailed
	status.EndTime = &now

	e.addTimelineEvent(TimelineEvent{
		Time:      now,
		Agent:     status.ToAgent,
		EventType: EventError,
		Message:   "Handoff cancelled",
	})

	return nil
}

// failHandoff handles handoff failure
func (e *Executor) failHandoff(status *HandoffStatus, err error) (*HandoffResult, error) {
	endTime := time.Now()
	status.Status = HandoffFailed
	status.EndTime = &endTime

	result := &HandoffResult{
		ID:        status.ID,
		AgentName: status.ToAgent,
		Status:    ResultError,
		Error:     err.Error(),
		Duration:  endTime.Sub(status.StartTime),
		Timestamp: endTime,
	}

	status.Result = result

	e.mu.Lock()
	e.addTimelineEvent(TimelineEvent{
		Time:      endTime,
		Agent:     status.ToAgent,
		EventType: EventError,
		Message:   fmt.Sprintf("Handoff to %s failed", status.ToAgent),
		Details:   err.Error(),
	})
	e.mu.Unlock()

	return result, err
}

// mapAgentType is DEPRECATED - agents are now resolved dynamically from databases
// Kept for backward compatibility during migration
func (e *Executor) mapAgentType(agentName string) inference.AgentType {
	return inference.AgentType(agentName)
}

// formatTaskMessage formats the task for the target agent
func (e *Executor) formatTaskMessage(intent *HandoffIntent) string {
	var msg string

	msg = intent.Task

	// Add context if provided
	if len(intent.Context) > 0 {
		msg += "\n\n## Context\n"
		for k, v := range intent.Context {
			msg += fmt.Sprintf("- **%s**: %v\n", k, v)
		}
	}

	return msg
}

// summarizePreviousResults creates a summary of previous step results
func (e *Executor) summarizePreviousResults(results map[int]*HandoffResult) string {
	var summary string
	for step, result := range results {
		if result.Status == ResultSuccess {
			// Truncate long results
			content := fmt.Sprintf("%v", result.Result)
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			summary += fmt.Sprintf("Step %d (%s): %s\n", step, result.AgentName, content)
		}
	}
	return summary
}

// addTimelineEvent adds an event to the timeline (must be called with lock held)
func (e *Executor) addTimelineEvent(event TimelineEvent) {
	e.timeline = append(e.timeline, event)
	// Keep timeline bounded
	if len(e.timeline) > 1000 {
		e.timeline = e.timeline[100:]
	}
}

// CleanupOldHandoffs removes completed handoffs older than the specified duration
func (e *Executor) CleanupOldHandoffs(maxAge time.Duration) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, status := range e.activeHandoffs {
		if status.EndTime != nil && status.EndTime.Before(cutoff) {
			delete(e.activeHandoffs, id)
			removed++
		}
	}

	return removed
}
