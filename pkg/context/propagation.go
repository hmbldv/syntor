package context

import (
	"encoding/json"
	"time"
)

// TaskContext carries context between agents during handoffs
type TaskContext struct {
	// Task identification
	TaskID          string `json:"taskId"`
	ParentTaskID    string `json:"parentTaskId,omitempty"`
	OriginalRequest string `json:"originalRequest"`
	CurrentTask     string `json:"currentTask"`

	// User intent and context
	UserIntent string            `json:"userIntent,omitempty"`
	UserGoals  []string          `json:"userGoals,omitempty"`
	Constraints map[string]string `json:"constraints,omitempty"`

	// Project context (immutable during task)
	ProjectName   string            `json:"projectName,omitempty"`
	ProjectPath   string            `json:"projectPath,omitempty"`
	CoreValues    []string          `json:"coreValues,omitempty"`
	Conventions   map[string]string `json:"conventions,omitempty"`

	// Agent execution context
	SourceAgent      string             `json:"sourceAgent"`
	TargetAgent      string             `json:"targetAgent"`
	AgentChain       []string           `json:"agentChain,omitempty"`       // Ordered list of agents involved
	PreviousResults  []AgentResult      `json:"previousResults,omitempty"` // Results from prior agents
	SharedMemory     map[string]string  `json:"sharedMemory,omitempty"`    // Data shared across agents

	// Execution metadata
	SessionID    string    `json:"sessionId"`
	Priority     string    `json:"priority,omitempty"` // low, normal, high, critical
	Deadline     *time.Time `json:"deadline,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	MaxDepth     int       `json:"maxDepth,omitempty"`     // Max handoff depth (prevent infinite loops)
	CurrentDepth int       `json:"currentDepth,omitempty"` // Current handoff depth
}

// AgentResult captures the result of an agent's work
type AgentResult struct {
	AgentName   string                 `json:"agentName"`
	Task        string                 `json:"task"`
	Output      string                 `json:"output,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	CompletedAt time.Time              `json:"completedAt"`
}

// NewTaskContext creates a new task context for an initial request
func NewTaskContext(sessionID, taskID, request string) *TaskContext {
	return &TaskContext{
		TaskID:          taskID,
		OriginalRequest: request,
		CurrentTask:     request,
		SessionID:       sessionID,
		AgentChain:      make([]string, 0),
		PreviousResults: make([]AgentResult, 0),
		SharedMemory:    make(map[string]string),
		Priority:        "normal",
		CreatedAt:       time.Now(),
		MaxDepth:        10, // Default max handoff depth
		CurrentDepth:    0,
	}
}

// Fork creates a child context for delegation to another agent
func (tc *TaskContext) Fork(targetAgent, task string) *TaskContext {
	child := &TaskContext{
		TaskID:          generateTaskID(),
		ParentTaskID:    tc.TaskID,
		OriginalRequest: tc.OriginalRequest,
		CurrentTask:     task,
		UserIntent:      tc.UserIntent,
		UserGoals:       tc.UserGoals,
		Constraints:     copyMap(tc.Constraints),
		ProjectName:     tc.ProjectName,
		ProjectPath:     tc.ProjectPath,
		CoreValues:      tc.CoreValues,
		Conventions:     copyMap(tc.Conventions),
		SourceAgent:     tc.TargetAgent, // Current agent becomes source
		TargetAgent:     targetAgent,
		AgentChain:      append(tc.AgentChain, tc.TargetAgent),
		PreviousResults: tc.PreviousResults, // Share results
		SharedMemory:    tc.SharedMemory,    // Share memory
		SessionID:       tc.SessionID,
		Priority:        tc.Priority,
		Deadline:        tc.Deadline,
		CreatedAt:       time.Now(),
		MaxDepth:        tc.MaxDepth,
		CurrentDepth:    tc.CurrentDepth + 1,
	}
	return child
}

// AddResult adds an agent result to the context
func (tc *TaskContext) AddResult(result AgentResult) {
	tc.PreviousResults = append(tc.PreviousResults, result)
}

// SetSharedData stores data in shared memory
func (tc *TaskContext) SetSharedData(key, value string) {
	if tc.SharedMemory == nil {
		tc.SharedMemory = make(map[string]string)
	}
	tc.SharedMemory[key] = value
}

// GetSharedData retrieves data from shared memory
func (tc *TaskContext) GetSharedData(key string) (string, bool) {
	if tc.SharedMemory == nil {
		return "", false
	}
	val, ok := tc.SharedMemory[key]
	return val, ok
}

// CanDelegate checks if further delegation is allowed
func (tc *TaskContext) CanDelegate() bool {
	return tc.CurrentDepth < tc.MaxDepth
}

// HasVisitedAgent checks if an agent was already in the chain
func (tc *TaskContext) HasVisitedAgent(agentName string) bool {
	for _, agent := range tc.AgentChain {
		if agent == agentName {
			return true
		}
	}
	return false
}

// GetLastResult returns the most recent agent result
func (tc *TaskContext) GetLastResult() *AgentResult {
	if len(tc.PreviousResults) == 0 {
		return nil
	}
	return &tc.PreviousResults[len(tc.PreviousResults)-1]
}

// ToJSON serializes the context for transmission
func (tc *TaskContext) ToJSON() (string, error) {
	data, err := json.Marshal(tc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON deserializes a context from JSON
func FromJSON(data string) (*TaskContext, error) {
	var tc TaskContext
	if err := json.Unmarshal([]byte(data), &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

// BuildPromptContext creates a summary suitable for inclusion in prompts
func (tc *TaskContext) BuildPromptContext() string {
	var summary string

	summary += "## Task Context\n"
	summary += "Original Request: " + tc.OriginalRequest + "\n"
	if tc.CurrentTask != tc.OriginalRequest {
		summary += "Current Task: " + tc.CurrentTask + "\n"
	}
	if tc.UserIntent != "" {
		summary += "User Intent: " + tc.UserIntent + "\n"
	}

	if tc.ProjectName != "" {
		summary += "\n## Project\n"
		summary += "Name: " + tc.ProjectName + "\n"
		if len(tc.CoreValues) > 0 {
			summary += "Core Values:\n"
			for _, v := range tc.CoreValues {
				summary += "  - " + v + "\n"
			}
		}
	}

	if len(tc.AgentChain) > 0 {
		summary += "\n## Agent Chain\n"
		for i, agent := range tc.AgentChain {
			summary += "  " + string(rune('1'+i)) + ". " + agent + "\n"
		}
	}

	if len(tc.PreviousResults) > 0 {
		summary += "\n## Previous Results\n"
		// Only include last 3 results to keep context manageable
		start := 0
		if len(tc.PreviousResults) > 3 {
			start = len(tc.PreviousResults) - 3
		}
		for _, r := range tc.PreviousResults[start:] {
			status := "success"
			if !r.Success {
				status = "failed"
			}
			summary += "- " + r.AgentName + " (" + status + "): " + r.Task + "\n"
			if r.Output != "" && len(r.Output) < 200 {
				summary += "  Output: " + r.Output + "\n"
			}
		}
	}

	return summary
}

// Helper functions

func generateTaskID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(6)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond) // Ensure different values
	}
	return string(b)
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
