package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
)

// AsyncExecutor handles asynchronous handoffs via Kafka
type AsyncExecutor struct {
	bus            kafka.MessageBus
	executor       *Executor
	pendingResults map[string]chan *HandoffResult
	mu             sync.RWMutex
	subscribed     bool
}

// AsyncConfig configures the async executor
type AsyncConfig struct {
	ResponseTimeout time.Duration
}

// DefaultAsyncConfig returns sensible defaults
func DefaultAsyncConfig() AsyncConfig {
	return AsyncConfig{
		ResponseTimeout: 5 * time.Minute,
	}
}

// NewAsyncExecutor creates an async executor
func NewAsyncExecutor(bus kafka.MessageBus, executor *Executor) *AsyncExecutor {
	return &AsyncExecutor{
		bus:            bus,
		executor:       executor,
		pendingResults: make(map[string]chan *HandoffResult),
	}
}

// Start initializes the async executor and subscribes to response topics
func (a *AsyncExecutor) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.subscribed {
		return nil
	}

	// Subscribe to task completion topic for handoff results
	if err := a.bus.Subscribe(ctx, kafka.TopicTaskComplete, a.handleTaskComplete); err != nil {
		return fmt.Errorf("failed to subscribe to task complete topic: %w", err)
	}

	a.subscribed = true
	return nil
}

// Stop shuts down the async executor
func (a *AsyncExecutor) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.subscribed {
		return nil
	}

	if err := a.bus.Unsubscribe(ctx, kafka.TopicTaskComplete); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	// Close all pending result channels
	for id, ch := range a.pendingResults {
		close(ch)
		delete(a.pendingResults, id)
	}

	a.subscribed = false
	return nil
}

// ExecuteAsync sends a handoff request via Kafka (fire-and-forget)
func (a *AsyncExecutor) ExecuteAsync(ctx context.Context, intent *HandoffIntent) (string, error) {
	handoffID := uuid.New().String()

	// Create task message
	msg := models.NewMessage(
		models.MsgTaskAssignment,
		"sntr",
		intent.Target,
		map[string]interface{}{
			"handoff_id": handoffID,
			"action":     string(intent.Action),
			"task":       intent.Task,
			"context":    intent.Context,
			"priority":   string(intent.Priority),
		},
	)
	msg.CorrelationID = handoffID

	// Publish to task assignment topic
	if err := a.bus.Publish(ctx, kafka.TopicTaskAssignment, msg); err != nil {
		return "", fmt.Errorf("failed to publish handoff: %w", err)
	}

	return handoffID, nil
}

// ExecuteWithResponse sends a handoff and waits for the response
func (a *AsyncExecutor) ExecuteWithResponse(ctx context.Context, intent *HandoffIntent, timeout time.Duration) (*HandoffResult, error) {
	handoffID := uuid.New().String()

	// Create result channel
	resultChan := make(chan *HandoffResult, 1)
	a.mu.Lock()
	a.pendingResults[handoffID] = resultChan
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.pendingResults, handoffID)
		a.mu.Unlock()
	}()

	// Create and send task message
	msg := models.NewMessage(
		models.MsgTaskAssignment,
		"sntr",
		intent.Target,
		map[string]interface{}{
			"handoff_id":      handoffID,
			"action":          string(intent.Action),
			"task":            intent.Task,
			"context":         intent.Context,
			"priority":        string(intent.Priority),
			"wait_for_result": true,
		},
	)
	msg.CorrelationID = handoffID

	if err := a.bus.Publish(ctx, kafka.TopicTaskAssignment, msg); err != nil {
		return nil, fmt.Errorf("failed to publish handoff: %w", err)
	}

	// Wait for response
	select {
	case result := <-resultChan:
		return result, nil
	case <-time.After(timeout):
		return &HandoffResult{
			ID:        handoffID,
			AgentName: intent.Target,
			Status:    ResultTimeout,
			Error:     "handoff timed out waiting for response",
			Timestamp: time.Now(),
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// handleTaskComplete processes incoming task completion messages
func (a *AsyncExecutor) handleTaskComplete(ctx context.Context, msg models.Message) error {
	// Extract handoff ID from correlation
	handoffID := msg.CorrelationID
	if handoffID == "" {
		// Try to get from payload
		if id, ok := msg.Payload["handoff_id"].(string); ok {
			handoffID = id
		}
	}

	if handoffID == "" {
		return nil // Not a handoff result, ignore
	}

	// Check if we're waiting for this result
	a.mu.RLock()
	resultChan, waiting := a.pendingResults[handoffID]
	a.mu.RUnlock()

	if !waiting {
		return nil // No one waiting for this result
	}

	// Parse result from payload
	result := &HandoffResult{
		ID:        handoffID,
		AgentName: msg.Source,
		Timestamp: msg.Timestamp,
	}

	// Extract result data
	if r, ok := msg.Payload["result"]; ok {
		result.Result = r
		result.Status = ResultSuccess
	}

	if e, ok := msg.Payload["error"].(string); ok && e != "" {
		result.Error = e
		result.Status = ResultError
	}

	if d, ok := msg.Payload["duration"].(float64); ok {
		result.Duration = time.Duration(d) * time.Millisecond
	}

	// Send result to waiting channel
	select {
	case resultChan <- result:
	default:
		// Channel full or closed, ignore
	}

	return nil
}

// ExecutePlanAsync executes a plan with steps distributed via Kafka
func (a *AsyncExecutor) ExecutePlanAsync(ctx context.Context, plan *ExecutionPlan) (*PlanExecution, error) {
	execution := &PlanExecution{
		Plan:        plan,
		Status:      PlanExecuting,
		CurrentStep: 0,
		StepResults: make(map[int]*HandoffResult),
		StartedAt:   time.Now(),
	}

	// Execute each step, some may be parallelizable
	for i, step := range plan.Steps {
		// Check dependencies
		canExecute := true
		for _, dep := range step.DependsOn {
			if result, ok := execution.StepResults[dep]; !ok || result.Status != ResultSuccess {
				canExecute = false
				break
			}
		}

		if !canExecute {
			execution.Status = PlanFailed
			execution.Error = fmt.Sprintf("dependency not met for step %d", i+1)
			return execution, fmt.Errorf("dependency not met for step %d", i+1)
		}

		execution.CurrentStep = i + 1

		// Create intent for this step
		intent := &HandoffIntent{
			Action:        ActionDelegate,
			Target:        step.Agent,
			Task:          step.Action,
			WaitForResult: true,
		}

		// Parse timeout if specified
		timeout := 5 * time.Minute
		if step.Estimated != "" {
			if d, err := time.ParseDuration(step.Estimated); err == nil {
				timeout = d * 2 // Double the estimate for safety
			}
		}

		// Execute via Kafka with response
		result, err := a.ExecuteWithResponse(ctx, intent, timeout)
		if err != nil {
			execution.Status = PlanFailed
			execution.Error = fmt.Sprintf("step %d failed: %v", i+1, err)
			return execution, err
		}

		execution.StepResults[step.StepNumber] = result

		if result.Status != ResultSuccess {
			execution.Status = PlanFailed
			execution.Error = fmt.Sprintf("step %d returned status %s", i+1, result.Status)
			return execution, fmt.Errorf("step %d failed", i+1)
		}
	}

	now := time.Now()
	execution.Status = PlanCompleted
	execution.CompletedAt = &now

	return execution, nil
}

// BroadcastIntent sends a handoff intent to all matching agents
func (a *AsyncExecutor) BroadcastIntent(ctx context.Context, intent *HandoffIntent) ([]string, error) {
	broadcastID := uuid.New().String()

	// Create broadcast message
	msg := models.NewMessage(
		models.MsgTaskAssignment,
		"sntr",
		"*", // Broadcast target
		map[string]interface{}{
			"broadcast_id": broadcastID,
			"action":       string(intent.Action),
			"task":         intent.Task,
			"context":      intent.Context,
			"priority":     string(intent.Priority),
			"broadcast":    true,
		},
	)
	msg.CorrelationID = broadcastID

	// Publish to service request topic for broadcast
	if err := a.bus.Publish(ctx, kafka.TopicServiceRequest, msg); err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	return []string{broadcastID}, nil
}

// IntentToJSON serializes a handoff intent to JSON
func IntentToJSON(intent *HandoffIntent) ([]byte, error) {
	return json.Marshal(intent)
}

// IntentFromJSON deserializes a handoff intent from JSON
func IntentFromJSON(data []byte) (*HandoffIntent, error) {
	var intent HandoffIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

// ResultToJSON serializes a handoff result to JSON
func ResultToJSON(result *HandoffResult) ([]byte, error) {
	return json.Marshal(result)
}

// ResultFromJSON deserializes a handoff result from JSON
func ResultFromJSON(data []byte) (*HandoffResult, error) {
	var result HandoffResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
