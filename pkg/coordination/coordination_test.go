package coordination

import (
	"testing"
	"time"
)

func TestParser(t *testing.T) {
	parser := NewParser()

	t.Run("Parse Delegate Intent", func(t *testing.T) {
		response := `I'll delegate this to the docs agent.

` + "```json\n" + `{
  "action": "delegate",
  "target": "docs",
  "task": "Generate documentation for the API",
  "context": {"reason": "Documentation specialist"}
}` + "\n```"

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if !result.HasIntent {
			t.Error("Expected HasIntent to be true")
		}
		if len(result.Intents) != 1 {
			t.Fatalf("Expected 1 intent, got %d", len(result.Intents))
		}

		intent := result.GetFirstIntent()
		if intent.Action != ActionDelegate {
			t.Errorf("Expected delegate action, got %s", intent.Action)
		}
		if intent.Target != "docs" {
			t.Errorf("Expected target docs, got %s", intent.Target)
		}
	})

	t.Run("Parse Plan", func(t *testing.T) {
		response := `Here's my plan:

` + "```json\n" + `{
  "action": "plan",
  "summary": "Implement feature X",
  "steps": [
    {"step": 1, "agent": "sntr", "action": "Read existing code"},
    {"step": 2, "agent": "code", "action": "Implement changes"},
    {"step": 3, "agent": "git", "action": "Commit changes"}
  ],
  "requiresApproval": true
}` + "\n```"

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if !result.HasPlan {
			t.Error("Expected HasPlan to be true")
		}
		if result.Plan == nil {
			t.Fatal("Expected plan to be non-nil")
		}
		if len(result.Plan.Steps) != 3 {
			t.Errorf("Expected 3 steps, got %d", len(result.Plan.Steps))
		}
	})

	t.Run("Parse No Intent", func(t *testing.T) {
		response := "This is a regular response without any special actions."

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if result.HasIntent {
			t.Error("Expected HasIntent to be false")
		}
		if result.HasPlan {
			t.Error("Expected HasPlan to be false")
		}
	})

	t.Run("Extract Text Content", func(t *testing.T) {
		response := "Before.\n\n```json\n{\"action\": \"delegate\", \"target\": \"test\", \"task\": \"test\"}\n```\n\nAfter."

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if !contains(result.TextContent, "Before") {
			t.Error("TextContent should contain text before JSON")
		}
		if !contains(result.TextContent, "After") {
			t.Error("TextContent should contain text after JSON")
		}
	})

	t.Run("ContainsIntent Check", func(t *testing.T) {
		withIntent := "```json\n{\"action\": \"delegate\", \"target\": \"docs\", \"task\": \"test\"}\n```"
		withoutIntent := "Regular text response"

		if !parser.ContainsIntent(withIntent) {
			t.Error("Should detect intent")
		}
		if parser.ContainsIntent(withoutIntent) {
			t.Error("Should not detect intent in regular text")
		}
	})
}

func TestHandoffIntent(t *testing.T) {
	t.Run("Delegate Action", func(t *testing.T) {
		intent := &HandoffIntent{
			Action:   ActionDelegate,
			Target:   "docs",
			Task:     "Generate documentation",
			Priority: PriorityNormal,
		}

		if intent.Action != ActionDelegate {
			t.Error("Action mismatch")
		}
		if intent.Target != "docs" {
			t.Error("Target mismatch")
		}
	})

	t.Run("With Context", func(t *testing.T) {
		intent := &HandoffIntent{
			Action: ActionDelegate,
			Target: "worker",
			Task:   "Process data",
			Context: map[string]interface{}{
				"source": "sntr",
				"files":  []string{"a.txt", "b.txt"},
			},
		}

		if intent.Context["source"] != "sntr" {
			t.Error("Context source mismatch")
		}
	})
}

func TestExecutionPlan(t *testing.T) {
	t.Run("GetAgentNames", func(t *testing.T) {
		plan := &ExecutionPlan{
			Steps: []PlanStep{
				{StepNumber: 1, Agent: "sntr", Action: "Read"},
				{StepNumber: 2, Agent: "code", Action: "Write"},
				{StepNumber: 3, Agent: "sntr", Action: "Verify"},
				{StepNumber: 4, Agent: "git", Action: "Commit"},
			},
		}

		agents := plan.GetAgentNames()
		if len(agents) != 3 {
			t.Errorf("Expected 3 unique agents, got %d", len(agents))
		}

		// Check agents are present (order may vary)
		agentSet := make(map[string]bool)
		for _, a := range agents {
			agentSet[a] = true
		}
		if !agentSet["sntr"] {
			t.Error("Expected sntr in agents")
		}
		if !agentSet["code"] {
			t.Error("Expected code in agents")
		}
		if !agentSet["git"] {
			t.Error("Expected git in agents")
		}
	})
}

func TestPlanExecution(t *testing.T) {
	t.Run("IsComplete States", func(t *testing.T) {
		plan := &ExecutionPlan{Steps: []PlanStep{{StepNumber: 1}}}

		tests := []struct {
			status   PlanStatus
			expected bool
		}{
			{PlanPending, false},
			{PlanApproved, false},
			{PlanExecuting, false},
			{PlanCompleted, true},
			{PlanFailed, true},
			{PlanCancelled, true},
		}

		for _, tt := range tests {
			exec := &PlanExecution{Plan: plan, Status: tt.status}
			if exec.IsComplete() != tt.expected {
				t.Errorf("Status %s: expected IsComplete=%v, got %v",
					tt.status, tt.expected, exec.IsComplete())
			}
		}
	})

	t.Run("GetProgress", func(t *testing.T) {
		plan := &ExecutionPlan{
			Steps: []PlanStep{
				{StepNumber: 1},
				{StepNumber: 2},
				{StepNumber: 3},
				{StepNumber: 4},
			},
		}

		exec := &PlanExecution{
			Plan: plan,
			StepResults: map[int]*HandoffResult{
				1: {Status: ResultSuccess},
				2: {Status: ResultSuccess},
			},
		}

		progress := exec.GetProgress()
		if progress != 50.0 {
			t.Errorf("Expected 50%% progress, got %.1f%%", progress)
		}
	})

	t.Run("Empty Plan Progress", func(t *testing.T) {
		exec := &PlanExecution{
			Plan: &ExecutionPlan{Steps: []PlanStep{}},
		}

		progress := exec.GetProgress()
		if progress != 0 {
			t.Errorf("Expected 0%% progress for empty plan, got %.1f%%", progress)
		}
	})
}

func TestHandoffResult(t *testing.T) {
	t.Run("Success Result", func(t *testing.T) {
		result := &HandoffResult{
			ID:        "test-001",
			AgentName: "docs",
			Status:    ResultSuccess,
			Result:    "Documentation generated",
			Duration:  2 * time.Second,
			Timestamp: time.Now(),
		}

		if result.Status != ResultSuccess {
			t.Error("Expected success status")
		}
	})

	t.Run("Error Result", func(t *testing.T) {
		result := &HandoffResult{
			ID:        "test-002",
			AgentName: "code",
			Status:    ResultError,
			Error:     "Compilation failed",
			Timestamp: time.Now(),
		}

		if result.Status != ResultError {
			t.Error("Expected error status")
		}
		if result.Error == "" {
			t.Error("Expected error message")
		}
	})
}

func TestFormatFunctions(t *testing.T) {
	t.Run("FormatIntentForDisplay", func(t *testing.T) {
		intent := &HandoffIntent{
			Action:   ActionDelegate,
			Target:   "docs",
			Task:     "Generate docs",
			Priority: PriorityHigh,
			Context: map[string]interface{}{
				"reason": "Specialized task",
			},
		}

		display := FormatIntentForDisplay(intent)
		if !contains(display, "delegate") {
			t.Error("Should contain action")
		}
		if !contains(display, "docs") {
			t.Error("Should contain target")
		}
		if !contains(display, "high") {
			t.Error("Should contain priority")
		}
	})

	t.Run("FormatPlanForDisplay", func(t *testing.T) {
		plan := &ExecutionPlan{
			Summary:     "Test Plan",
			TotalAgents: 2,
			Estimated:   "5m",
			Steps: []PlanStep{
				{StepNumber: 1, Agent: "sntr", Action: "Read"},
				{StepNumber: 2, Agent: "code", Action: "Write"},
			},
		}

		// Brief display
		brief := FormatPlanForDisplay(plan, false)
		if !contains(brief, "Test Plan") {
			t.Error("Brief should contain summary")
		}

		// Detailed display
		detailed := FormatPlanForDisplay(plan, true)
		if !contains(detailed, "Steps:") {
			t.Error("Detailed should contain steps")
		}
		if !contains(detailed, "sntr") {
			t.Error("Detailed should contain agent names")
		}
	})
}

func TestHandoffStatus(t *testing.T) {
	t.Run("Status Transitions", func(t *testing.T) {
		status := &HandoffStatus{
			ID:        "handoff-001",
			FromAgent: "sntr",
			ToAgent:   "docs",
			Task:      "Generate docs",
			Status:    HandoffPending,
			StartTime: time.Now(),
		}

		if status.Status != HandoffPending {
			t.Error("Should start pending")
		}

		status.Status = HandoffExecuting
		if status.Status != HandoffExecuting {
			t.Error("Should be executing")
		}

		now := time.Now()
		status.Status = HandoffCompleted
		status.EndTime = &now
		if status.Status != HandoffCompleted {
			t.Error("Should be completed")
		}
		if status.EndTime == nil {
			t.Error("EndTime should be set")
		}
	})
}

func TestTimelineEvent(t *testing.T) {
	t.Run("Event Types", func(t *testing.T) {
		events := []TimelineEvent{
			{Time: time.Now(), Agent: "sntr", EventType: EventStarted, Message: "Started"},
			{Time: time.Now(), Agent: "docs", EventType: EventHandoff, Message: "Handoff"},
			{Time: time.Now(), Agent: "docs", EventType: EventCompleted, Message: "Done"},
		}

		if events[0].EventType != EventStarted {
			t.Error("First event should be started")
		}
		if events[1].EventType != EventHandoff {
			t.Error("Second event should be handoff")
		}
		if events[2].EventType != EventCompleted {
			t.Error("Third event should be completed")
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
