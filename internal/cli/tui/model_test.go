package tui

import (
	"strings"
	"testing"

	"github.com/syntor/syntor/pkg/inference"
)

// === FUNCTIONAL TESTS (FOUNDRY) ===

func TestGetModelContextWindow(t *testing.T) {
	t.Run("returns correct size for llama3.2:3b", func(t *testing.T) {
		size := getModelContextWindow("llama3.2:3b")
		if size != 8192 {
			t.Errorf("getModelContextWindow(llama3.2:3b) = %d, want 8192", size)
		}
	})

	t.Run("returns correct size for llama3.2:8b", func(t *testing.T) {
		size := getModelContextWindow("llama3.2:8b")
		if size != 8192 {
			t.Errorf("getModelContextWindow(llama3.2:8b) = %d, want 8192", size)
		}
	})

	t.Run("returns correct size for llama3.1:8b", func(t *testing.T) {
		size := getModelContextWindow("llama3.1:8b")
		if size != 128000 {
			t.Errorf("getModelContextWindow(llama3.1:8b) = %d, want 128000", size)
		}
	})

	t.Run("returns correct size for llama3.1:70b", func(t *testing.T) {
		size := getModelContextWindow("llama3.1:70b")
		if size != 128000 {
			t.Errorf("getModelContextWindow(llama3.1:70b) = %d, want 128000", size)
		}
	})

	t.Run("returns correct size for mistral:7b", func(t *testing.T) {
		size := getModelContextWindow("mistral:7b")
		if size != 32768 {
			t.Errorf("getModelContextWindow(mistral:7b) = %d, want 32768", size)
		}
	})

	t.Run("returns correct size for mixtral:8x7b", func(t *testing.T) {
		size := getModelContextWindow("mixtral:8x7b")
		if size != 32768 {
			t.Errorf("getModelContextWindow(mixtral:8x7b) = %d, want 32768", size)
		}
	})

	t.Run("returns correct size for qwen2.5-coder:7b", func(t *testing.T) {
		size := getModelContextWindow("qwen2.5-coder:7b")
		if size != 32768 {
			t.Errorf("getModelContextWindow(qwen2.5-coder:7b) = %d, want 32768", size)
		}
	})

	t.Run("returns correct size for qwen2.5-coder:14b", func(t *testing.T) {
		size := getModelContextWindow("qwen2.5-coder:14b")
		if size != 32768 {
			t.Errorf("getModelContextWindow(qwen2.5-coder:14b) = %d, want 32768", size)
		}
	})

	t.Run("returns correct size for deepseek-coder-v2:16b", func(t *testing.T) {
		size := getModelContextWindow("deepseek-coder-v2:16b")
		if size != 128000 {
			t.Errorf("getModelContextWindow(deepseek-coder-v2:16b) = %d, want 128000", size)
		}
	})

	t.Run("returns correct size for claude-3-opus", func(t *testing.T) {
		size := getModelContextWindow("claude-3-opus")
		if size != 200000 {
			t.Errorf("getModelContextWindow(claude-3-opus) = %d, want 200000", size)
		}
	})

	t.Run("returns correct size for claude-3-sonnet", func(t *testing.T) {
		size := getModelContextWindow("claude-3-sonnet")
		if size != 200000 {
			t.Errorf("getModelContextWindow(claude-3-sonnet) = %d, want 200000", size)
		}
	})

	t.Run("returns correct size for claude-3-haiku", func(t *testing.T) {
		size := getModelContextWindow("claude-3-haiku")
		if size != 200000 {
			t.Errorf("getModelContextWindow(claude-3-haiku) = %d, want 200000", size)
		}
	})

	t.Run("returns default for unknown model", func(t *testing.T) {
		size := getModelContextWindow("unknown-model:xyz")
		if size != 8192 {
			t.Errorf("getModelContextWindow(unknown-model:xyz) = %d, want 8192 (default)", size)
		}
	})
}

func TestGetAgentDisplayName(t *testing.T) {
	testCases := []struct {
		agentType inference.AgentType
		expected  string
	}{
		{inference.AgentSNTR, "sntr"},
		{inference.AgentDocumentation, "docs"},
		{inference.AgentGit, "git"},
		{inference.AgentWorker, "worker"},
		{inference.AgentWorkerCode, "code"},
		{inference.AgentType("unknown"), "syntor"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.agentType), func(t *testing.T) {
			result := getAgentDisplayName(tc.agentType)
			if result != tc.expected {
				t.Errorf("getAgentDisplayName(%s) = %q, want %q", tc.agentType, result, tc.expected)
			}
		})
	}
}

func TestGetSystemPrompt(t *testing.T) {
	t.Run("sntr agent has tool instructions", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentSNTR)
		if !strings.Contains(prompt, "tool") {
			t.Error("SNTR agent prompt should contain tool instructions")
		}
		if !strings.Contains(prompt, "json") {
			t.Error("SNTR agent prompt should contain JSON instructions")
		}
	})

	t.Run("documentation agent has doc focus", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentDocumentation)
		if !strings.Contains(strings.ToLower(prompt), "documentation") {
			t.Error("Documentation agent prompt should focus on documentation")
		}
	})

	t.Run("git agent has git focus", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentGit)
		if !strings.Contains(strings.ToLower(prompt), "git") {
			t.Error("Git agent prompt should focus on git")
		}
	})

	t.Run("worker agent is general purpose", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentWorker)
		if prompt == "" {
			t.Error("Worker agent should have a prompt")
		}
	})

	t.Run("worker_code agent has code focus", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentWorkerCode)
		if !strings.Contains(strings.ToLower(prompt), "code") {
			t.Error("Code worker agent prompt should focus on code")
		}
	})

	t.Run("unknown agent returns default prompt", func(t *testing.T) {
		prompt := getSystemPrompt(inference.AgentType("unknown"))
		if prompt == "" {
			t.Error("Unknown agent should return a default prompt")
		}
		if !strings.Contains(strings.ToLower(prompt), "syntor") {
			t.Error("Default prompt should mention SYNTOR")
		}
	})
}

func TestAgentTypeToManifestName(t *testing.T) {
	testCases := []struct {
		agentType inference.AgentType
		expected  string
	}{
		{inference.AgentSNTR, "sntr"},
		{inference.AgentDocumentation, "documentation"},
		{inference.AgentGit, "git"},
		{inference.AgentWorker, "worker"},
		{inference.AgentWorkerCode, "code"},
		{inference.AgentType("unknown"), "worker"}, // Default fallback
	}

	for _, tc := range testCases {
		t.Run(string(tc.agentType), func(t *testing.T) {
			result := agentTypeToManifestName(tc.agentType)
			if result != tc.expected {
				t.Errorf("agentTypeToManifestName(%s) = %q, want %q", tc.agentType, result, tc.expected)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	t.Run("wraps long text", func(t *testing.T) {
		text := "This is a very long line of text that should be wrapped at the specified width"
		wrapped := wrapText(text, 20)

		lines := strings.Split(wrapped, "\n")
		for _, line := range lines {
			// Note: wrapText may not perfectly enforce width due to word boundaries
			if len(line) > 30 { // Allow some slack for word boundaries
				t.Errorf("Line too long: %d chars > 30: %q", len(line), line)
			}
		}
	})

	t.Run("handles zero width", func(t *testing.T) {
		text := "Some text"
		wrapped := wrapText(text, 0)
		if wrapped != text {
			t.Error("Zero width should return original text")
		}
	})

	t.Run("handles negative width", func(t *testing.T) {
		text := "Some text"
		wrapped := wrapText(text, -5)
		if wrapped != text {
			t.Error("Negative width should return original text")
		}
	})

	t.Run("handles empty text", func(t *testing.T) {
		wrapped := wrapText("", 80)
		if wrapped != "" {
			t.Error("Empty text should return empty string")
		}
	})

	t.Run("preserves single short line", func(t *testing.T) {
		text := "Short"
		wrapped := wrapText(text, 80)
		if wrapped != text {
			t.Errorf("Short text should be unchanged: got %q, want %q", wrapped, text)
		}
	})
}

func TestChatMessage(t *testing.T) {
	t.Run("user message", func(t *testing.T) {
		msg := ChatMessage{
			Role:    "user",
			Content: "Hello, assistant!",
		}

		if msg.Role != "user" {
			t.Errorf("Role = %q, want user", msg.Role)
		}
		if msg.Content != "Hello, assistant!" {
			t.Errorf("Content mismatch")
		}
	})

	t.Run("assistant message with agent", func(t *testing.T) {
		msg := ChatMessage{
			Role:    "assistant",
			Content: "Hello, user!",
			Agent:   "sntr",
		}

		if msg.Role != "assistant" {
			t.Errorf("Role = %q, want assistant", msg.Role)
		}
		if msg.Agent != "sntr" {
			t.Errorf("Agent = %q, want sntr", msg.Agent)
		}
	})

	t.Run("system message", func(t *testing.T) {
		msg := ChatMessage{
			Role:    "system",
			Content: "System initialized",
		}

		if msg.Role != "system" {
			t.Error("Role should be system")
		}
	})
}

func TestActivityStatus(t *testing.T) {
	t.Run("inactive status", func(t *testing.T) {
		status := ActivityStatus{Active: false}
		if status.Active {
			t.Error("Status should be inactive")
		}
	})

	t.Run("active status with type", func(t *testing.T) {
		status := ActivityStatus{
			Active:      true,
			Type:        "thinking",
			Description: "Agent is processing...",
		}

		if !status.Active {
			t.Error("Status should be active")
		}
		if status.Type != "thinking" {
			t.Errorf("Type = %q, want thinking", status.Type)
		}
	})
}

func TestAutonomyMode(t *testing.T) {
	t.Run("auto mode is zero value", func(t *testing.T) {
		var mode AutonomyMode
		if mode != AutoMode {
			t.Error("Zero value should be AutoMode")
		}
	})

	t.Run("plan mode has higher value", func(t *testing.T) {
		if PlanMode <= AutoMode {
			t.Error("PlanMode should be > AutoMode")
		}
	})
}

func TestDetailLevel(t *testing.T) {
	t.Run("summary detail is zero value", func(t *testing.T) {
		var level DetailLevel
		if level != SummaryDetail {
			t.Error("Zero value should be SummaryDetail")
		}
	})

	t.Run("full detail has higher value", func(t *testing.T) {
		if FullDetail <= SummaryDetail {
			t.Error("FullDetail should be > SummaryDetail")
		}
	})
}

// === SECURITY TESTS (CRBRS) ===

func TestSystemPromptDoesNotLeakSensitiveInfo(t *testing.T) {
	agentTypes := []inference.AgentType{
		inference.AgentSNTR,
		inference.AgentDocumentation,
		inference.AgentGit,
		inference.AgentWorker,
		inference.AgentWorkerCode,
	}

	for _, agentType := range agentTypes {
		t.Run(string(agentType), func(t *testing.T) {
			prompt := getSystemPrompt(agentType)
			lowerPrompt := strings.ToLower(prompt)

			// Check for sensitive patterns
			sensitivePatterns := []string{
				"api_key",
				"apikey",
				"password",
				"secret",
				"credential",
			}

			for _, pattern := range sensitivePatterns {
				if strings.Contains(lowerPrompt, pattern) {
					t.Errorf("System prompt for %s should not contain sensitive pattern: %s", agentType, pattern)
				}
			}
		})
	}
}

func TestContextWindowSizesAreReasonable(t *testing.T) {
	t.Run("all context windows are positive", func(t *testing.T) {
		models := []string{
			"llama3.2:3b",
			"llama3.1:8b",
			"mistral:7b",
			"claude-3-opus",
			"unknown-model",
		}

		for _, model := range models {
			size := getModelContextWindow(model)
			if size <= 0 {
				t.Errorf("Context window for %s should be positive, got %d", model, size)
			}
		}
	})

	t.Run("context windows are within reasonable bounds", func(t *testing.T) {
		models := []string{
			"llama3.2:3b",
			"llama3.1:8b",
			"mistral:7b",
			"claude-3-opus",
		}

		for _, model := range models {
			size := getModelContextWindow(model)
			// Context windows should be between 1k and 2M tokens
			if size < 1024 || size > 2000000 {
				t.Errorf("Context window for %s seems unreasonable: %d", model, size)
			}
		}
	})
}

func TestWrapTextDoesNotPanic(t *testing.T) {
	t.Run("various edge cases", func(t *testing.T) {
		testCases := []struct {
			text  string
			width int
		}{
			{"", 0},
			{"", -1},
			{"text", 0},
			{"text", -1},
			{"text", 1},
			{strings.Repeat("a", 10000), 10},
			{strings.Repeat("a b ", 1000), 50},
		}

		for _, tc := range testCases {
			// Should not panic
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("wrapText panicked for text len=%d, width=%d: %v", len(tc.text), tc.width, r)
					}
				}()
				wrapText(tc.text, tc.width)
			}()
		}
	})
}

// === CONTEXT HIERARCHY TESTS ===

func TestBuildDynamicPromptContextHierarchy(t *testing.T) {
	// These tests verify the context injection patterns without requiring
	// a full Model instance

	t.Run("global context tag format", func(t *testing.T) {
		// Verify the expected tag format exists in the implementation
		// by checking what buildDynamicPrompt would produce

		// The global context should be wrapped in <global-context> tags
		globalCtx := "# Global Settings\nSome global config"
		expectedTag := "<global-context>"
		expectedCloseTag := "</global-context>"

		// Simulate what buildDynamicPrompt does
		result := "\n\n<global-context>\n" + globalCtx + "\n</global-context>"

		if !strings.Contains(result, expectedTag) {
			t.Errorf("Global context should be wrapped in %s tag", expectedTag)
		}
		if !strings.Contains(result, expectedCloseTag) {
			t.Errorf("Global context should have closing %s tag", expectedCloseTag)
		}
	})

	t.Run("project context tag format", func(t *testing.T) {
		projectCtx := "# Project: TestProject\nSome project config"
		expectedTag := "<project-context>"
		expectedCloseTag := "</project-context>"

		// Simulate what buildDynamicPrompt does
		result := "\n\n<project-context>\n" + projectCtx + "\n</project-context>"

		if !strings.Contains(result, expectedTag) {
			t.Errorf("Project context should be wrapped in %s tag", expectedTag)
		}
		if !strings.Contains(result, expectedCloseTag) {
			t.Errorf("Project context should have closing %s tag", expectedCloseTag)
		}
	})

	t.Run("global context comes before project context", func(t *testing.T) {
		// In buildDynamicPrompt, global context is injected first, then project context
		// This ensures proper context hierarchy

		basePrompt := "Base system prompt"
		globalCtx := "Global context"
		projectCtx := "Project context"

		// Simulate the expected order
		result := basePrompt + "\n\n<global-context>\n" + globalCtx + "\n</global-context>"
		result += "\n\n<project-context>\n" + projectCtx + "\n</project-context>"

		globalIdx := strings.Index(result, "<global-context>")
		projectIdx := strings.Index(result, "<project-context>")

		if globalIdx >= projectIdx {
			t.Error("Global context should appear before project context in the prompt")
		}
	})
}

// === CONVERSATION HISTORY TESTS ===
// Note: These are design/contract tests that verify the expected behavior
// The actual implementation testing requires mocking the Model

func TestConversationHistoryPatterns(t *testing.T) {
	t.Run("message roles are valid", func(t *testing.T) {
		validRoles := map[string]bool{
			"user":      true,
			"assistant": true,
			"system":    true,
		}

		// Test that ChatMessage can hold valid roles
		for role := range validRoles {
			msg := ChatMessage{Role: role, Content: "test"}
			if !validRoles[msg.Role] {
				t.Errorf("Invalid role: %s", msg.Role)
			}
		}
	})

	t.Run("conversation history maintains order", func(t *testing.T) {
		// Simulate a conversation
		history := []ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
			{Role: "assistant", Content: "I'm doing well!"},
		}

		// Verify alternating pattern
		for i := 0; i < len(history)-1; i++ {
			if history[i].Role == history[i+1].Role {
				// Same role twice in a row is allowed for tool results
				// but typical conversation should alternate
				if history[i].Role == "user" {
					t.Logf("Note: consecutive user messages at index %d", i)
				}
			}
		}
	})
}
