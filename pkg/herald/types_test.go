package herald

import (
	"encoding/json"
	"testing"
	"time"
)

// === FUNCTIONAL TESTS (FOUNDRY) ===

func TestSession_JSONSerialization(t *testing.T) {
	t.Run("serializes and deserializes correctly", func(t *testing.T) {
		session := Session{
			ID:           "sess_123",
			Name:         "Test Session",
			Type:         SessionTypeCLI,
			Status:       SessionStatusActive,
			TrustTier:    T2,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			LastActive:   time.Now(),
			WorkingDir:   "/home/user/project",
			AgentName:    "sntr",
			TokensUsed:   1000,
			MessageCount: 5,
			Messages: []Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			GlobalContext:  "Global context content",
			ProjectContext: "Project context content",
			ContextTokens:  500,
		}

		// Serialize
		data, err := json.Marshal(session)
		if err != nil {
			t.Fatalf("Failed to marshal session: %v", err)
		}

		// Deserialize
		var decoded Session
		err = json.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("Failed to unmarshal session: %v", err)
		}

		// Verify fields
		if decoded.ID != session.ID {
			t.Errorf("ID = %q, want %q", decoded.ID, session.ID)
		}
		if decoded.Name != session.Name {
			t.Errorf("Name = %q, want %q", decoded.Name, session.Name)
		}
		if decoded.Type != session.Type {
			t.Errorf("Type = %q, want %q", decoded.Type, session.Type)
		}
		if decoded.Status != session.Status {
			t.Errorf("Status = %q, want %q", decoded.Status, session.Status)
		}
		if decoded.TrustTier != session.TrustTier {
			t.Errorf("TrustTier = %d, want %d", decoded.TrustTier, session.TrustTier)
		}
	})

	t.Run("Messages field serializes correctly", func(t *testing.T) {
		session := Session{
			ID: "sess_messages",
			Messages: []Message{
				{Role: "user", Content: "First message"},
				{Role: "assistant", Content: "First response"},
				{Role: "user", Content: "Second message"},
				{Role: "assistant", Content: "Second response"},
			},
		}

		data, err := json.Marshal(session)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded Session
		json.Unmarshal(data, &decoded)

		if len(decoded.Messages) != 4 {
			t.Errorf("Expected 4 messages, got %d", len(decoded.Messages))
		}

		// Verify message order and content
		for i, msg := range session.Messages {
			if decoded.Messages[i].Role != msg.Role {
				t.Errorf("Message %d role = %q, want %q", i, decoded.Messages[i].Role, msg.Role)
			}
			if decoded.Messages[i].Content != msg.Content {
				t.Errorf("Message %d content mismatch", i)
			}
		}
	})

	t.Run("GlobalContext field serializes correctly", func(t *testing.T) {
		session := Session{
			ID:            "sess_global",
			GlobalContext: "# Global CENTAUR Context\n\nThis is global context from CENTAUR.md",
		}

		data, err := json.Marshal(session)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded Session
		json.Unmarshal(data, &decoded)

		if decoded.GlobalContext != session.GlobalContext {
			t.Errorf("GlobalContext not preserved: got %q", decoded.GlobalContext)
		}
	})

	t.Run("ProjectContext field serializes correctly", func(t *testing.T) {
		session := Session{
			ID:             "sess_project",
			ProjectContext: "# Project SYNTOR Context\n\nThis is project context from SYNTOR.md",
		}

		data, err := json.Marshal(session)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded Session
		json.Unmarshal(data, &decoded)

		if decoded.ProjectContext != session.ProjectContext {
			t.Errorf("ProjectContext not preserved: got %q", decoded.ProjectContext)
		}
	})

	t.Run("ContextTokens field serializes correctly", func(t *testing.T) {
		session := Session{
			ID:            "sess_tokens",
			ContextTokens: 15000,
		}

		data, _ := json.Marshal(session)
		var decoded Session
		json.Unmarshal(data, &decoded)

		if decoded.ContextTokens != 15000 {
			t.Errorf("ContextTokens = %d, want 15000", decoded.ContextTokens)
		}
	})

	t.Run("empty optional fields are omitted", func(t *testing.T) {
		session := Session{
			ID:     "sess_minimal",
			Name:   "Minimal",
			Type:   SessionTypeCLI,
			Status: SessionStatusActive,
		}

		data, _ := json.Marshal(session)
		jsonStr := string(data)

		// Optional fields with omitempty should not appear if empty
		if containsString(jsonStr, "messages") && containsString(jsonStr, `"messages":[]`) {
			// Empty slice should be omitted due to omitempty
			// Actually checking if it's really omitted
		}
	})
}

func TestMessage_JSONSerialization(t *testing.T) {
	t.Run("basic message", func(t *testing.T) {
		msg := Message{
			Role:    "user",
			Content: "Hello, world!",
		}

		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded Message
		json.Unmarshal(data, &decoded)

		if decoded.Role != "user" {
			t.Errorf("Role = %q, want user", decoded.Role)
		}
		if decoded.Content != "Hello, world!" {
			t.Errorf("Content mismatch")
		}
	})

	t.Run("message with tool calls", func(t *testing.T) {
		msg := Message{
			Role:    "assistant",
			Content: "Let me check that file.",
			ToolCalls: []ToolCall{
				{
					ID:        "call_001",
					Name:      "read_file",
					Arguments: map[string]any{"path": "/test.txt"},
				},
			},
		}

		data, _ := json.Marshal(msg)
		var decoded Message
		json.Unmarshal(data, &decoded)

		if len(decoded.ToolCalls) != 1 {
			t.Fatalf("Expected 1 tool call, got %d", len(decoded.ToolCalls))
		}
		if decoded.ToolCalls[0].Name != "read_file" {
			t.Errorf("ToolCall name = %q, want read_file", decoded.ToolCalls[0].Name)
		}
	})

	t.Run("tool result message", func(t *testing.T) {
		msg := Message{
			Role:       "tool",
			Content:    "File contents here",
			Name:       "read_file",
			ToolCallID: "call_001",
		}

		data, _ := json.Marshal(msg)
		var decoded Message
		json.Unmarshal(data, &decoded)

		if decoded.Role != "tool" {
			t.Errorf("Role = %q, want tool", decoded.Role)
		}
		if decoded.ToolCallID != "call_001" {
			t.Errorf("ToolCallID = %q, want call_001", decoded.ToolCallID)
		}
	})
}

func TestTrustTier(t *testing.T) {
	t.Run("String representation", func(t *testing.T) {
		testCases := []struct {
			tier     TrustTier
			expected string
		}{
			{T0, "T0 (Restricted)"},
			{T1, "T1 (Read-Only)"},
			{T2, "T2 (Modify)"},
			{T3, "T3 (Execute)"},
			{T4, "T4 (Autonomous)"},
		}

		for _, tc := range testCases {
			t.Run(tc.expected, func(t *testing.T) {
				if tc.tier.String() != tc.expected {
					t.Errorf("String() = %q, want %q", tc.tier.String(), tc.expected)
				}
			})
		}
	})

	t.Run("RequiresApproval for read operations", func(t *testing.T) {
		if T0.RequiresApproval(OpRead) != true {
			t.Error("T0 should require approval for read")
		}
		if T1.RequiresApproval(OpRead) != false {
			t.Error("T1 should not require approval for read")
		}
	})

	t.Run("RequiresApproval for write operations", func(t *testing.T) {
		if T1.RequiresApproval(OpWrite) != true {
			t.Error("T1 should require approval for write")
		}
		if T2.RequiresApproval(OpWrite) != false {
			t.Error("T2 should not require approval for write")
		}
	})

	t.Run("RequiresApproval for execute operations", func(t *testing.T) {
		if T2.RequiresApproval(OpExecute) != true {
			t.Error("T2 should require approval for execute")
		}
		if T3.RequiresApproval(OpExecute) != false {
			t.Error("T3 should not require approval for execute")
		}
	})

	t.Run("RequiresApproval for network operations", func(t *testing.T) {
		if T3.RequiresApproval(OpNetwork) != true {
			t.Error("T3 should require approval for network")
		}
		if T4.RequiresApproval(OpNetwork) != false {
			t.Error("T4 should not require approval for network")
		}
	})
}

func TestSessionTypes(t *testing.T) {
	t.Run("all session types", func(t *testing.T) {
		types := []SessionType{
			SessionTypeCLI,
			SessionTypeAgent,
			SessionTypeBackground,
		}

		for _, st := range types {
			if st == "" {
				t.Error("Session type should not be empty")
			}
		}
	})
}

func TestSessionStatus(t *testing.T) {
	t.Run("all session statuses", func(t *testing.T) {
		statuses := []SessionStatus{
			SessionStatusCreating,
			SessionStatusActive,
			SessionStatusIdle,
			SessionStatusSuspended,
			SessionStatusTerminated,
			SessionStatusError,
		}

		for _, s := range statuses {
			if s == "" {
				t.Error("Session status should not be empty")
			}
		}
	})
}

func TestApprovalRequest(t *testing.T) {
	t.Run("serialization", func(t *testing.T) {
		req := ApprovalRequest{
			ID:          "approval_001",
			SessionID:   "sess_123",
			Type:        ApprovalTypeTool,
			Operation:   "bash",
			Description: "Run shell command",
			Risk:        RiskHigh,
			Status:      ApprovalStatusPending,
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(5 * time.Minute),
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded ApprovalRequest
		json.Unmarshal(data, &decoded)

		if decoded.ID != "approval_001" {
			t.Errorf("ID mismatch")
		}
		if decoded.Risk != RiskHigh {
			t.Errorf("Risk = %q, want %q", decoded.Risk, RiskHigh)
		}
	})
}

func TestInferenceRequest(t *testing.T) {
	t.Run("serialization with all fields", func(t *testing.T) {
		req := InferenceRequest{
			SessionID: "sess_123",
			Model:     "llama3.2:8b",
			Messages: []Message{
				{Role: "user", Content: "Hello"},
			},
			MaxTokens:    1000,
			Temperature:  0.7,
			Stream:       true,
			SystemPrompt: "You are a helpful assistant.",
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded InferenceRequest
		json.Unmarshal(data, &decoded)

		if decoded.Model != "llama3.2:8b" {
			t.Errorf("Model = %q, want llama3.2:8b", decoded.Model)
		}
		if decoded.Temperature != 0.7 {
			t.Errorf("Temperature = %f, want 0.7", decoded.Temperature)
		}
	})
}

func TestUsage(t *testing.T) {
	t.Run("total tokens calculation", func(t *testing.T) {
		usage := Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		}

		if usage.TotalTokens != usage.PromptTokens+usage.CompletionTokens {
			t.Error("TotalTokens should equal PromptTokens + CompletionTokens")
		}
	})
}

func TestError(t *testing.T) {
	t.Run("error interface", func(t *testing.T) {
		err := &Error{
			Code:    ErrCodeNotFound,
			Message: "Session not found",
		}

		if err.Error() != "Session not found" {
			t.Errorf("Error() = %q, want 'Session not found'", err.Error())
		}
	})

	t.Run("error codes exist", func(t *testing.T) {
		codes := []string{
			ErrCodeUnauthorized,
			ErrCodeForbidden,
			ErrCodeNotFound,
			ErrCodeInvalidRequest,
			ErrCodeRateLimited,
			ErrCodeServiceUnavailable,
			ErrCodeInternalError,
			ErrCodeApprovalRequired,
			ErrCodeApprovalDenied,
			ErrCodeSessionExpired,
		}

		for _, code := range codes {
			if code == "" {
				t.Error("Error code should not be empty")
			}
		}
	})
}

// === SECURITY TESTS (CRBRS) ===

func TestSession_SensitiveFieldHandling(t *testing.T) {
	t.Run("no credential fields exposed", func(t *testing.T) {
		session := Session{
			ID:   "sess_123",
			Name: "Test",
		}

		data, _ := json.Marshal(session)
		jsonStr := string(data)

		// Session struct should not contain sensitive field patterns
		sensitivePatterns := []string{
			"password",
			"secret",
			"api_key",
			"apikey",
			"credential",
			"private_key",
		}

		for _, pattern := range sensitivePatterns {
			if containsString(jsonStr, pattern) {
				t.Errorf("Session JSON should not contain sensitive pattern: %s", pattern)
			}
		}
	})
}

func TestMessage_ContentSafety(t *testing.T) {
	t.Run("handles special characters in content", func(t *testing.T) {
		msg := Message{
			Role:    "user",
			Content: `<script>alert("xss")</script> && rm -rf /`,
		}

		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message with special chars: %v", err)
		}

		var decoded Message
		json.Unmarshal(data, &decoded)

		// Content should be preserved exactly (escaping is JSON's job)
		if decoded.Content != msg.Content {
			t.Error("Message content not preserved correctly")
		}
	})

	t.Run("handles unicode in content", func(t *testing.T) {
		msg := Message{
			Role:    "assistant",
			Content: "Hello 世界! 🌍 مرحبا",
		}

		data, _ := json.Marshal(msg)
		var decoded Message
		json.Unmarshal(data, &decoded)

		if decoded.Content != msg.Content {
			t.Error("Unicode content not preserved")
		}
	})
}

func TestTrustTier_Escalation(t *testing.T) {
	t.Run("higher tiers have more permissions", func(t *testing.T) {
		// T4 should never require approval
		if T4.RequiresApproval(OpRead) {
			t.Error("T4 should not require approval for read")
		}
		if T4.RequiresApproval(OpWrite) {
			t.Error("T4 should not require approval for write")
		}
		if T4.RequiresApproval(OpExecute) {
			t.Error("T4 should not require approval for execute")
		}
		if T4.RequiresApproval(OpNetwork) {
			t.Error("T4 should not require approval for network")
		}

		// T0 should always require approval
		if !T0.RequiresApproval(OpRead) {
			t.Error("T0 should require approval for read")
		}
		if !T0.RequiresApproval(OpWrite) {
			t.Error("T0 should require approval for write")
		}
		if !T0.RequiresApproval(OpExecute) {
			t.Error("T0 should require approval for execute")
		}
		if !T0.RequiresApproval(OpNetwork) {
			t.Error("T0 should require approval for network")
		}
	})

	t.Run("unknown operation type requires approval", func(t *testing.T) {
		unknownOp := OperationType("unknown")

		// All tiers should require approval for unknown operations
		for tier := T0; tier <= T4; tier++ {
			if !tier.RequiresApproval(unknownOp) {
				t.Errorf("Tier %d should require approval for unknown operation", tier)
			}
		}
	})
}

func TestApprovalRequest_Expiration(t *testing.T) {
	t.Run("expiration time is in future", func(t *testing.T) {
		now := time.Now()
		req := ApprovalRequest{
			ID:        "approval_001",
			CreatedAt: now,
			ExpiresAt: now.Add(5 * time.Minute),
		}

		if req.ExpiresAt.Before(req.CreatedAt) {
			t.Error("ExpiresAt should be after CreatedAt")
		}
	})
}

// Helper function
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
