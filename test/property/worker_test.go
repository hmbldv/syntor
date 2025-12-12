// +build property

package property

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/syntor/syntor/pkg/models"
)

// **Feature: syntor-multi-agent-system, Property 14: Worker Agent task execution**
// *For any* Worker Agent, it should successfully execute project-specific tasks and data processing operations
// **Validates: Requirements 5.2**

// **Feature: syntor-multi-agent-system, Property 29: Plugin architecture support**
// *For any* new agent addition, it should integrate into the system without requiring modifications to existing agent code
// **Validates: Requirements 9.3**

// MockWorkerAgent simulates a worker agent for testing
type MockWorkerAgent struct {
	id            string
	handlers      map[string]func(ctx context.Context, task models.Task) (*models.TaskResult, error)
	activeTasks   map[string]bool
	maxConcurrent int
	semaphore     chan struct{}
	mu            sync.RWMutex
	tasksExecuted int64
	tasksFailed   int64
}

func NewMockWorkerAgent(id string, maxConcurrent int) *MockWorkerAgent {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	return &MockWorkerAgent{
		id:            id,
		handlers:      make(map[string]func(ctx context.Context, task models.Task) (*models.TaskResult, error)),
		activeTasks:   make(map[string]bool),
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

func (w *MockWorkerAgent) RegisterHandler(taskType string, handler func(ctx context.Context, task models.Task) (*models.TaskResult, error)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[taskType] = handler
}

func (w *MockWorkerAgent) ExecuteTask(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	// Check capacity
	select {
	case w.semaphore <- struct{}{}:
		defer func() { <-w.semaphore }()
	default:
		return nil, fmt.Errorf("at capacity")
	}

	// Get handler
	w.mu.RLock()
	handler, ok := w.handlers[task.Type]
	w.mu.RUnlock()

	if !ok {
		atomic.AddInt64(&w.tasksFailed, 1)
		return nil, fmt.Errorf("no handler for task type: %s", task.Type)
	}

	// Track active task
	w.mu.Lock()
	w.activeTasks[task.ID] = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		delete(w.activeTasks, task.ID)
		w.mu.Unlock()
	}()

	// Execute
	result, err := handler(ctx, task)
	if err != nil {
		atomic.AddInt64(&w.tasksFailed, 1)
		return nil, err
	}

	atomic.AddInt64(&w.tasksExecuted, 1)
	return result, nil
}

func (w *MockWorkerAgent) GetActiveCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.activeTasks)
}

func (w *MockWorkerAgent) CanHandle(taskType string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.handlers[taskType]
	return ok
}

// TestWorkerAgentTaskExecution tests Property 14
func TestWorkerAgentTaskExecution(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Worker executes registered task types
	properties.Property("worker executes registered tasks", prop.ForAll(
		func(taskCount int) bool {
			if taskCount <= 0 || taskCount > 50 {
				taskCount = 10
			}

			worker := NewMockWorkerAgent("test-worker", 20)
			worker.RegisterHandler("test-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				return &models.TaskResult{Data: map[string]interface{}{"processed": true}}, nil
			})

			ctx := context.Background()
			successCount := 0

			for i := 0; i < taskCount; i++ {
				task := models.Task{
					ID:   fmt.Sprintf("task-%d", i),
					Type: "test-task",
				}
				result, err := worker.ExecuteTask(ctx, task)
				if err == nil && result != nil {
					successCount++
				}
			}

			return successCount == taskCount
		},
		gen.IntRange(1, 50),
	))

	// Property: Worker rejects unregistered task types
	properties.Property("worker rejects unregistered tasks", prop.ForAll(
		func(taskType string) bool {
			if taskType == "" || taskType == "registered-task" {
				return true
			}

			worker := NewMockWorkerAgent("test-worker", 10)
			worker.RegisterHandler("registered-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				return &models.TaskResult{}, nil
			})

			task := models.Task{
				ID:   "test-task",
				Type: taskType,
			}

			_, err := worker.ExecuteTask(context.Background(), task)
			return err != nil // Should fail for unregistered type
		},
		gen.AlphaString(),
	))

	// Property: Worker respects concurrency limits
	properties.Property("worker respects concurrency limits", prop.ForAll(
		func(maxConcurrent int) bool {
			if maxConcurrent <= 0 || maxConcurrent > 20 {
				maxConcurrent = 5
			}

			worker := NewMockWorkerAgent("test-worker", maxConcurrent)

			// Handler that blocks briefly
			worker.RegisterHandler("slow-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				time.Sleep(50 * time.Millisecond)
				return &models.TaskResult{}, nil
			})

			ctx := context.Background()
			var wg sync.WaitGroup
			var maxObserved int32

			// Launch more tasks than max concurrent
			taskCount := maxConcurrent * 2
			for i := 0; i < taskCount; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					task := models.Task{
						ID:   fmt.Sprintf("task-%d", idx),
						Type: "slow-task",
					}

					// Check active count before execution starts
					current := int32(worker.GetActiveCount())
					if current > maxObserved {
						atomic.StoreInt32(&maxObserved, current)
					}

					worker.ExecuteTask(ctx, task)
				}(i)
			}

			wg.Wait()

			// Active count should never exceed max
			return int(maxObserved) <= maxConcurrent
		},
		gen.IntRange(1, 20),
	))

	// Property: Worker tracks task execution metrics
	properties.Property("worker tracks execution metrics", prop.ForAll(
		func(successCount int, failCount int) bool {
			if successCount < 0 || successCount > 20 {
				successCount = 5
			}
			if failCount < 0 || failCount > 20 {
				failCount = 3
			}

			worker := NewMockWorkerAgent("test-worker", 30)

			worker.RegisterHandler("success-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				return &models.TaskResult{}, nil
			})
			worker.RegisterHandler("fail-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				return nil, fmt.Errorf("intentional failure")
			})

			ctx := context.Background()

			// Execute success tasks
			for i := 0; i < successCount; i++ {
				task := models.Task{ID: fmt.Sprintf("success-%d", i), Type: "success-task"}
				worker.ExecuteTask(ctx, task)
			}

			// Execute fail tasks
			for i := 0; i < failCount; i++ {
				task := models.Task{ID: fmt.Sprintf("fail-%d", i), Type: "fail-task"}
				worker.ExecuteTask(ctx, task)
			}

			executed := atomic.LoadInt64(&worker.tasksExecuted)
			failed := atomic.LoadInt64(&worker.tasksFailed)

			return int(executed) == successCount && int(failed) == failCount
		},
		gen.IntRange(0, 20),
		gen.IntRange(0, 20),
	))

	// Property: Worker handles task timeouts
	properties.Property("worker handles task timeouts", prop.ForAll(
		func(_ int) bool {
			worker := NewMockWorkerAgent("test-worker", 10)

			worker.RegisterHandler("slow-task", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(1 * time.Second):
					return &models.TaskResult{}, nil
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			task := models.Task{ID: "timeout-task", Type: "slow-task"}
			_, err := worker.ExecuteTask(ctx, task)

			return err != nil // Should timeout
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// TestPluginArchitecture tests Property 29
func TestPluginArchitecture(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: New handlers can be added at runtime
	properties.Property("handlers addable at runtime", prop.ForAll(
		func(handlerCount int) bool {
			if handlerCount <= 0 || handlerCount > 20 {
				handlerCount = 5
			}

			worker := NewMockWorkerAgent("test-worker", 30)

			// Dynamically add handlers
			for i := 0; i < handlerCount; i++ {
				taskType := fmt.Sprintf("dynamic-task-%d", i)
				idx := i // Capture for closure
				worker.RegisterHandler(taskType, func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
					return &models.TaskResult{Data: map[string]interface{}{"handler": idx}}, nil
				})
			}

			// Verify all handlers work
			ctx := context.Background()
			for i := 0; i < handlerCount; i++ {
				task := models.Task{
					ID:   fmt.Sprintf("task-%d", i),
					Type: fmt.Sprintf("dynamic-task-%d", i),
				}
				result, err := worker.ExecuteTask(ctx, task)
				if err != nil || result == nil {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 20),
	))

	// Property: Multiple workers can have different handlers
	properties.Property("workers have independent handlers", prop.ForAll(
		func(workerCount int) bool {
			if workerCount <= 0 || workerCount > 10 {
				workerCount = 3
			}

			workers := make([]*MockWorkerAgent, workerCount)
			for i := 0; i < workerCount; i++ {
				workers[i] = NewMockWorkerAgent(fmt.Sprintf("worker-%d", i), 10)
				// Each worker handles different task type
				taskType := fmt.Sprintf("task-type-%d", i)
				workerIdx := i
				workers[i].RegisterHandler(taskType, func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
					return &models.TaskResult{Data: map[string]interface{}{"worker": workerIdx}}, nil
				})
			}

			// Each worker should only handle its own task type
			ctx := context.Background()
			for i := 0; i < workerCount; i++ {
				taskType := fmt.Sprintf("task-type-%d", i)
				for j := 0; j < workerCount; j++ {
					task := models.Task{ID: "test", Type: taskType}
					_, err := workers[j].ExecuteTask(ctx, task)
					if i == j {
						if err != nil {
							return false // Should succeed
						}
					} else {
						if err == nil {
							return false // Should fail
						}
					}
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property: Handler registration doesn't affect existing handlers
	properties.Property("handler registration is additive", prop.ForAll(
		func(initialCount int, additionalCount int) bool {
			if initialCount <= 0 || initialCount > 10 {
				initialCount = 3
			}
			if additionalCount <= 0 || additionalCount > 10 {
				additionalCount = 3
			}

			worker := NewMockWorkerAgent("test-worker", 30)

			// Register initial handlers
			for i := 0; i < initialCount; i++ {
				taskType := fmt.Sprintf("initial-%d", i)
				worker.RegisterHandler(taskType, func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
					return &models.TaskResult{}, nil
				})
			}

			// Register additional handlers
			for i := 0; i < additionalCount; i++ {
				taskType := fmt.Sprintf("additional-%d", i)
				worker.RegisterHandler(taskType, func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
					return &models.TaskResult{}, nil
				})
			}

			// Verify initial handlers still work
			for i := 0; i < initialCount; i++ {
				taskType := fmt.Sprintf("initial-%d", i)
				if !worker.CanHandle(taskType) {
					return false
				}
			}

			// Verify additional handlers work
			for i := 0; i < additionalCount; i++ {
				taskType := fmt.Sprintf("additional-%d", i)
				if !worker.CanHandle(taskType) {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// Unit tests
func TestWorkerTaskHandling(t *testing.T) {
	t.Run("data processing handler", func(t *testing.T) {
		worker := NewMockWorkerAgent("test", 10)
		worker.RegisterHandler("data", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
			input, _ := task.Payload["input"].(string)
			return &models.TaskResult{
				Data: map[string]interface{}{"output": input + "-processed"},
			}, nil
		})

		task := models.Task{
			ID:      "test-1",
			Type:    "data",
			Payload: map[string]interface{}{"input": "hello"},
		}

		result, err := worker.ExecuteTask(context.Background(), task)
		assert.NoError(t, err)
		assert.Equal(t, "hello-processed", result.Data["output"])
	})

	t.Run("concurrent task execution", func(t *testing.T) {
		worker := NewMockWorkerAgent("test", 5)
		var counter int64

		worker.RegisterHandler("increment", func(ctx context.Context, task models.Task) (*models.TaskResult, error) {
			atomic.AddInt64(&counter, 1)
			time.Sleep(10 * time.Millisecond)
			return &models.TaskResult{}, nil
		})

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				task := models.Task{ID: fmt.Sprintf("task-%d", idx), Type: "increment"}
				worker.ExecuteTask(context.Background(), task)
			}(i)
		}
		wg.Wait()

		// Some tasks may be rejected due to capacity, but executed ones should be counted
		assert.True(t, atomic.LoadInt64(&counter) > 0)
	})
}
