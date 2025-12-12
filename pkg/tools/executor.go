package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SecurityChecker interface for security validation
type SecurityChecker interface {
	Check(call ToolCall, def *ToolDefinition) error
	RequiresApproval(call ToolCall, def *ToolDefinition, planMode bool) bool
}

// Executor manages tool execution
type Executor struct {
	registry        *Registry
	securityChecker SecurityChecker
	maxConcurrent   int
	defaultTimeout  time.Duration
}

// NewExecutor creates a new tool executor
func NewExecutor(registry *Registry, securityChecker SecurityChecker) *Executor {
	return &Executor{
		registry:        registry,
		securityChecker: securityChecker,
		maxConcurrent:   5,
		defaultTimeout:  2 * time.Minute,
	}
}

// ExecuteBatch executes multiple tool calls
func (e *Executor) ExecuteBatch(ctx context.Context, calls []ToolCall, opts ExecuteOptions) []ToolResult {
	results := make([]ToolResult, len(calls))

	// Categorize calls into parallel and sequential
	parallel, sequential := e.categorizeCalls(calls)

	// Execute parallel calls concurrently
	if len(parallel) > 0 {
		var wg sync.WaitGroup
		resultChan := make(chan struct {
			index  int
			result ToolResult
		}, len(parallel))

		// Limit concurrency
		sem := make(chan struct{}, e.maxConcurrent)

		for _, pc := range parallel {
			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire semaphore
				defer func() { <-sem }() // Release semaphore

				result := e.ExecuteSingle(ctx, call, opts)
				resultChan <- struct {
					index  int
					result ToolResult
				}{idx, result}
			}(pc.index, pc.call)
		}

		go func() {
			wg.Wait()
			close(resultChan)
		}()

		for r := range resultChan {
			results[r.index] = r.result
		}
	}

	// Execute sequential calls in order
	for _, sc := range sequential {
		results[sc.index] = e.ExecuteSingle(ctx, sc.call, opts)
	}

	return results
}

// ExecuteSingle executes a single tool call
func (e *Executor) ExecuteSingle(ctx context.Context, call ToolCall, opts ExecuteOptions) ToolResult {
	start := time.Now()

	result := ToolResult{
		CallID:   call.ID,
		ToolName: call.Name,
	}

	// Get tool
	tool, ok := e.registry.Get(call.Name)
	if !ok {
		result.Error = &ToolError{
			Code:    ErrCodeToolNotFound,
			Message: fmt.Sprintf("Unknown tool: %s", call.Name),
		}
		result.Duration = time.Since(start)
		return result
	}

	// Validate parameters
	if err := tool.Validate(call.Parameters); err != nil {
		result.Error = &ToolError{
			Code:    ErrCodeInvalidParams,
			Message: err.Error(),
		}
		result.Duration = time.Since(start)
		return result
	}

	// Security check
	if e.securityChecker != nil {
		def, _ := e.registry.GetDefinition(call.Name)
		if err := e.securityChecker.Check(call, def); err != nil {
			result.Error = &ToolError{
				Code:    ErrCodeSecurityViolation,
				Message: err.Error(),
			}
			result.Duration = time.Since(start)
			return result
		}
	}

	// Dry run - don't actually execute
	if opts.DryRun {
		result.Success = true
		result.Output = "[DRY RUN] Would execute tool"
		result.Duration = time.Since(start)
		return result
	}

	// Execute with timeout
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execResult, err := tool.Execute(execCtx, call.Parameters)
	if err != nil {
		// Check if it was a context timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result.Error = &ToolError{
				Code:    ErrCodeTimeout,
				Message: fmt.Sprintf("Tool execution timed out after %v", timeout),
			}
		} else {
			result.Error = &ToolError{
				Code:    ErrCodeExecutionError,
				Message: err.Error(),
			}
		}
	} else if execResult != nil {
		result.Success = execResult.Success
		result.Output = execResult.Output
		result.Error = execResult.Error
		result.Truncated = execResult.Truncated
	}

	result.Duration = time.Since(start)
	return result
}

// CheckApprovals returns tool calls that need user approval
func (e *Executor) CheckApprovals(calls []ToolCall, planMode bool) []*ApprovalRequest {
	if e.securityChecker == nil {
		return nil
	}

	var needsApproval []*ApprovalRequest

	for _, call := range calls {
		def, _ := e.registry.GetDefinition(call.Name)
		if e.securityChecker.RequiresApproval(call, def, planMode) {
			risk := RiskMedium
			if call.Name == "bash" {
				risk = RiskHigh
			} else if call.Name == "write_file" || call.Name == "edit_file" {
				risk = RiskMedium
			}

			needsApproval = append(needsApproval, &ApprovalRequest{
				ID:          call.ID,
				ToolCall:    call,
				Reason:      fmt.Sprintf("Tool %s requires approval", call.Name),
				Risk:        risk,
				RequestedAt: time.Now(),
			})
		}
	}

	return needsApproval
}

// indexedCall pairs a call with its original index
type indexedCall struct {
	index int
	call  ToolCall
}

// categorizeCalls separates calls into parallel and sequential execution
func (e *Executor) categorizeCalls(calls []ToolCall) (parallel, sequential []indexedCall) {
	// Tools that modify state should be sequential
	modifyingTools := map[string]bool{
		"write_file": true,
		"edit_file":  true,
		"bash":       true,
	}

	for i, call := range calls {
		if modifyingTools[call.Name] {
			sequential = append(sequential, indexedCall{i, call})
		} else {
			parallel = append(parallel, indexedCall{i, call})
		}
	}

	return parallel, sequential
}

// SetMaxConcurrent sets the maximum concurrent tool executions
func (e *Executor) SetMaxConcurrent(n int) {
	if n > 0 {
		e.maxConcurrent = n
	}
}

// SetDefaultTimeout sets the default execution timeout
func (e *Executor) SetDefaultTimeout(d time.Duration) {
	if d > 0 {
		e.defaultTimeout = d
	}
}
