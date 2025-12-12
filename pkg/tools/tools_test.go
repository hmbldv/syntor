package tools

import (
	"context"
	"testing"
	"time"
)

// MockTool implements Tool for testing
type MockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
	validateErr error
}

func (m *MockTool) Name() string        { return m.name }
func (m *MockTool) Description() string { return m.description }
func (m *MockTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return &ToolResult{
		CallID:   "test",
		ToolName: m.name,
		Success:  true,
		Output:   "mock output",
	}, nil
}
func (m *MockTool) Validate(params map[string]interface{}) error {
	return m.validateErr
}

func TestRegistry(t *testing.T) {
	t.Run("Register and Get", func(t *testing.T) {
		reg := NewRegistry()
		tool := &MockTool{name: "test_tool", description: "A test tool"}
		def := &ToolDefinition{
			Name:        "test_tool",
			Description: "A test tool",
			Category:    ToolCategoryFilesystem,
			Parameters: []ToolParameter{
				{Name: "path", Type: "string", Required: true},
			},
		}

		err := reg.Register(tool, def)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		// Get tool
		retrieved, ok := reg.Get("test_tool")
		if !ok {
			t.Fatal("Tool not found after registration")
		}
		if retrieved.Name() != "test_tool" {
			t.Errorf("Expected name test_tool, got %s", retrieved.Name())
		}

		// Get definition
		retrievedDef, ok := reg.GetDefinition("test_tool")
		if !ok {
			t.Fatal("Definition not found after registration")
		}
		if len(retrievedDef.Parameters) != 1 {
			t.Errorf("Expected 1 parameter, got %d", len(retrievedDef.Parameters))
		}
	})

	t.Run("Duplicate Registration", func(t *testing.T) {
		reg := NewRegistry()
		tool := &MockTool{name: "dupe_tool"}

		err := reg.Register(tool, nil)
		if err != nil {
			t.Fatalf("First registration failed: %v", err)
		}

		err = reg.Register(tool, nil)
		if err == nil {
			t.Error("Expected error on duplicate registration")
		}
	})

	t.Run("List Tools", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(&MockTool{name: "tool1"}, nil)
		reg.Register(&MockTool{name: "tool2"}, nil)
		reg.Register(&MockTool{name: "tool3"}, nil)

		names := reg.List()
		if len(names) != 3 {
			t.Errorf("Expected 3 tools, got %d", len(names))
		}
	})

	t.Run("GenerateToolPrompt", func(t *testing.T) {
		reg := NewRegistry()
		tool := &MockTool{name: "read_file", description: "Read a file"}
		def := &ToolDefinition{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: []ToolParameter{
				{Name: "path", Type: "string", Required: true, Description: "File path"},
				{Name: "limit", Type: "integer", Required: false, Description: "Line limit"},
			},
		}
		reg.Register(tool, def)

		prompt := reg.GenerateToolPrompt([]string{"read_file"})
		if prompt == "" {
			t.Error("Expected non-empty prompt")
		}
		if !contains(prompt, "read_file") {
			t.Error("Prompt should contain tool name")
		}
		if !contains(prompt, "path") {
			t.Error("Prompt should contain parameter name")
		}
	})
}

func TestParser(t *testing.T) {
	parser := NewParser()

	t.Run("Parse Valid Tool Call", func(t *testing.T) {
		response := `Let me read that file for you.

` + "```json\n" + `{
  "tool_calls": [
    {"id": "call_001", "name": "read_file", "parameters": {"path": "/test.txt"}}
  ]
}` + "\n```\n" + `

I'll process the results.`

		result, err := parser.Parse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if !result.HasTools {
			t.Error("Expected HasTools to be true")
		}
		if len(result.ToolCalls) != 1 {
			t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
		}
		if result.ToolCalls[0].Name != "read_file" {
			t.Errorf("Expected read_file, got %s", result.ToolCalls[0].Name)
		}
	})

	t.Run("Parse Multiple Tool Calls", func(t *testing.T) {
		response := "```json\n" + `{
  "tool_calls": [
    {"id": "call_001", "name": "glob", "parameters": {"pattern": "*.go"}},
    {"id": "call_002", "name": "read_file", "parameters": {"path": "main.go"}}
  ]
}` + "\n```"

		result, err := parser.Parse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(result.ToolCalls) != 2 {
			t.Fatalf("Expected 2 tool calls, got %d", len(result.ToolCalls))
		}
	})

	t.Run("Parse No Tool Calls", func(t *testing.T) {
		response := "This is just a regular response with no tool calls."

		result, err := parser.Parse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if result.HasTools {
			t.Error("Expected HasTools to be false")
		}
		if len(result.ToolCalls) != 0 {
			t.Errorf("Expected 0 tool calls, got %d", len(result.ToolCalls))
		}
	})

	t.Run("Extract Text Content", func(t *testing.T) {
		response := "Before the tool call.\n\n```json\n{\"tool_calls\": []}\n```\n\nAfter the tool call."

		result, err := parser.Parse(response)
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
}

func TestFormatter(t *testing.T) {
	formatter := NewFormatter()

	t.Run("Format Single Result", func(t *testing.T) {
		result := ToolResult{
			CallID:   "call_001",
			ToolName: "read_file",
			Success:  true,
			Output:   "Line 1\nLine 2",
			Duration: 50 * time.Millisecond,
		}

		formatted := formatter.FormatResults([]ToolResult{result})
		if formatted == "" {
			t.Error("Expected non-empty formatted output")
		}
		if !contains(formatted, "call_001") {
			t.Error("Formatted output should contain call ID")
		}
		if !contains(formatted, "read_file") {
			t.Error("Formatted output should contain tool name")
		}
		if !contains(formatted, "success") {
			t.Error("Formatted output should contain success status")
		}
	})

	t.Run("Format Error Result", func(t *testing.T) {
		result := ToolResult{
			CallID:   "call_002",
			ToolName: "bash",
			Success:  false,
			Error: &ToolError{
				Code:    ErrCodeExecutionError,
				Message: "command failed",
			},
		}

		formatted := formatter.FormatResults([]ToolResult{result})
		if !contains(formatted, "false") {
			t.Error("Formatted output should indicate failure")
		}
		if !contains(formatted, "EXECUTION_ERROR") {
			t.Error("Formatted output should contain error code")
		}
	})

	t.Run("Format Multiple Results", func(t *testing.T) {
		results := []ToolResult{
			{CallID: "call_001", ToolName: "glob", Success: true, Output: "file1.go\nfile2.go"},
			{CallID: "call_002", ToolName: "read_file", Success: true, Output: "content"},
		}

		formatted := formatter.FormatResults(results)
		if !contains(formatted, "call_001") {
			t.Error("Should contain first call ID")
		}
		if !contains(formatted, "call_002") {
			t.Error("Should contain second call ID")
		}
	})

	t.Run("Truncate Long Output", func(t *testing.T) {
		// Create output longer than max
		maxLen := 30000
		longOutput := make([]byte, maxLen+1000)
		for i := range longOutput {
			longOutput[i] = 'x'
		}

		result := ToolResult{
			CallID:   "call_003",
			ToolName: "cat",
			Success:  true,
			Output:   string(longOutput),
		}

		formatted := formatter.FormatResults([]ToolResult{result})
		if len(formatted) > maxLen+500 { // Allow for XML tags
			t.Error("Formatted output should be truncated")
		}
	})
}

func TestToolError(t *testing.T) {
	t.Run("Error Interface", func(t *testing.T) {
		err := &ToolError{
			Code:    ErrCodeFileNotFound,
			Message: "file not found",
		}

		errStr := err.Error()
		if !contains(errStr, ErrCodeFileNotFound) {
			t.Error("Error string should contain code")
		}
		if !contains(errStr, "file not found") {
			t.Error("Error string should contain message")
		}
	})

	t.Run("Error With Details", func(t *testing.T) {
		err := &ToolError{
			Code:    ErrCodeSecurityViolation,
			Message: "path denied",
			Details: "/etc/passwd",
		}

		errStr := err.Error()
		if !contains(errStr, "/etc/passwd") {
			t.Error("Error string should contain details")
		}
	})
}

func TestToolCall(t *testing.T) {
	t.Run("Basic Structure", func(t *testing.T) {
		call := ToolCall{
			ID:   "call_001",
			Name: "read_file",
			Parameters: map[string]interface{}{
				"path":  "/test.txt",
				"limit": 100,
			},
		}

		if call.ID != "call_001" {
			t.Error("ID mismatch")
		}
		if call.Name != "read_file" {
			t.Error("Name mismatch")
		}
		if call.Parameters["path"] != "/test.txt" {
			t.Error("Path parameter mismatch")
		}
	})
}

func TestToolResult(t *testing.T) {
	t.Run("Success Result", func(t *testing.T) {
		result := &ToolResult{
			CallID:    "call_001",
			ToolName:  "glob",
			Success:   true,
			Output:    "file1.go\nfile2.go",
			Duration:  25 * time.Millisecond,
			Truncated: false,
		}

		if !result.Success {
			t.Error("Result should be successful")
		}
		if result.Truncated {
			t.Error("Result should not be truncated")
		}
	})

	t.Run("Failed Result", func(t *testing.T) {
		result := &ToolResult{
			CallID:   "call_002",
			ToolName: "bash",
			Success:  false,
			Error: &ToolError{
				Code:    ErrCodeExecutionError,
				Message: "exit code 1",
			},
		}

		if result.Success {
			t.Error("Result should not be successful")
		}
		if result.Error == nil {
			t.Error("Error should be set")
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
