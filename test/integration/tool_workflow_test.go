package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syntor/syntor/pkg/coordination"
	"github.com/syntor/syntor/pkg/tools"
	"github.com/syntor/syntor/pkg/tools/implementations"
	"github.com/syntor/syntor/pkg/tools/security"
)

// TestToolExecutionWorkflow tests the complete tool execution workflow
func TestToolExecutionWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "syntor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup tool registry
	registry := tools.NewRegistry()

	// Create security manager
	securityMgr := security.NewManager(tmpDir)

	// Register tools
	readFileTool := implementations.NewReadFileTool()
	writeFileTool := implementations.NewWriteFileTool()
	listDirTool := implementations.NewListDirectoryTool(tmpDir)
	globTool := implementations.NewGlobTool(tmpDir)
	grepTool := implementations.NewGrepTool(tmpDir)

	registry.Register(readFileTool, nil)
	registry.Register(writeFileTool, nil)
	registry.Register(listDirTool, nil)
	registry.Register(globTool, nil)
	registry.Register(grepTool, nil)

	// Create executor
	executor := tools.NewExecutor(registry, securityMgr)

	t.Run("Write and Read File", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.txt")
		testContent := "Hello, SYNTOR!"

		// Write file
		writeCall := tools.ToolCall{
			ID:   "write_001",
			Name: "write_file",
			Parameters: map[string]interface{}{
				"file_path": testFile,
				"content":   testContent,
			},
		}

		results := executor.ExecuteBatch(ctx, []tools.ToolCall{writeCall}, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Fatalf("Write failed: %v", results[0].Error)
		}

		// Read file back
		readCall := tools.ToolCall{
			ID:   "read_001",
			Name: "read_file",
			Parameters: map[string]interface{}{
				"file_path": testFile,
			},
		}

		results = executor.ExecuteBatch(ctx, []tools.ToolCall{readCall}, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Fatalf("Read failed: %v", results[0].Error)
		}

		output, ok := results[0].Output.(string)
		if !ok {
			t.Fatal("Expected string output")
		}
		if !contains(output, testContent) {
			t.Errorf("Content mismatch: expected %q in output", testContent)
		}
	})

	t.Run("List Directory", func(t *testing.T) {
		// Create some test files
		for i := 0; i < 3; i++ {
			f, _ := os.Create(filepath.Join(tmpDir, "file"+string(rune('a'+i))+".txt"))
			f.Close()
		}

		listCall := tools.ToolCall{
			ID:   "list_001",
			Name: "list_directory",
			Parameters: map[string]interface{}{
				"path": tmpDir,
			},
		}

		results := executor.ExecuteBatch(ctx, []tools.ToolCall{listCall}, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Fatalf("List failed: %v", results[0].Error)
		}

		output, ok := results[0].Output.(string)
		if !ok {
			t.Fatal("Expected string output")
		}
		if !contains(output, "filea.txt") {
			t.Error("Directory listing should contain filea.txt")
		}
	})

	t.Run("Glob Pattern Matching", func(t *testing.T) {
		// Create test files with different extensions
		os.Create(filepath.Join(tmpDir, "code.go"))
		os.Create(filepath.Join(tmpDir, "code.py"))
		os.Create(filepath.Join(tmpDir, "data.json"))

		globCall := tools.ToolCall{
			ID:   "glob_001",
			Name: "glob",
			Parameters: map[string]interface{}{
				"pattern": "*.go",
				"path":    tmpDir,
			},
		}

		results := executor.ExecuteBatch(ctx, []tools.ToolCall{globCall}, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Fatalf("Glob failed: %v", results[0].Error)
		}

		output, ok := results[0].Output.(string)
		if !ok {
			t.Fatal("Expected string output")
		}
		if !contains(output, "code.go") {
			t.Error("Glob should find code.go")
		}
		if contains(output, "code.py") {
			t.Error("Glob should not find code.py")
		}
	})

	t.Run("Grep Search", func(t *testing.T) {
		// Create a file with searchable content
		searchFile := filepath.Join(tmpDir, "search.txt")
		os.WriteFile(searchFile, []byte("line one\nTODO: fix this\nline three\n"), 0644)

		grepCall := tools.ToolCall{
			ID:   "grep_001",
			Name: "grep",
			Parameters: map[string]interface{}{
				"pattern": "TODO",
				"path":    tmpDir,
			},
		}

		results := executor.ExecuteBatch(ctx, []tools.ToolCall{grepCall}, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Fatalf("Grep failed: %v", results[0].Error)
		}

		output, ok := results[0].Output.(string)
		if !ok {
			t.Fatal("Expected string output")
		}
		if !contains(output, "TODO") {
			t.Error("Grep should find TODO")
		}
	})

	t.Run("Batch Execution", func(t *testing.T) {
		// Execute multiple tools in a batch
		calls := []tools.ToolCall{
			{ID: "batch_001", Name: "list_directory", Parameters: map[string]interface{}{"path": tmpDir}},
			{ID: "batch_002", Name: "glob", Parameters: map[string]interface{}{"pattern": "*", "path": tmpDir}},
		}

		results := executor.ExecuteBatch(ctx, calls, tools.ExecuteOptions{
			WorkingDir: tmpDir,
		})

		if len(results) != 2 {
			t.Fatalf("Expected 2 results, got %d", len(results))
		}

		for i, result := range results {
			if !result.Success {
				t.Errorf("Batch call %d failed: %v", i, result.Error)
			}
		}
	})
}

// TestToolParserIntegration tests parsing tool calls from LLM responses
func TestToolParserIntegration(t *testing.T) {
	parser := tools.NewParser()
	formatter := tools.NewFormatter()

	t.Run("Parse and Format Cycle", func(t *testing.T) {
		// Simulate LLM response with tool calls
		llmResponse := `I'll read the file for you.

` + "```json\n" + `{
  "tool_calls": [
    {"id": "call_001", "name": "read_file", "parameters": {"file_path": "/tmp/test.txt"}},
    {"id": "call_002", "name": "list_directory", "parameters": {"path": "/tmp"}}
  ]
}` + "\n```\n" + `

Let me process these.`

		// Parse the response
		parseResult, err := parser.Parse(llmResponse)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if !parseResult.HasTools {
			t.Error("Expected HasTools to be true")
		}
		if len(parseResult.ToolCalls) != 2 {
			t.Errorf("Expected 2 tool calls, got %d", len(parseResult.ToolCalls))
		}

		// Simulate tool execution results
		results := []tools.ToolResult{
			{CallID: "call_001", ToolName: "read_file", Success: true, Output: "File content here"},
			{CallID: "call_002", ToolName: "list_directory", Success: true, Output: "file1.txt\nfile2.txt"},
		}

		// Format results for LLM
		formatted := formatter.FormatResults(results)
		if formatted == "" {
			t.Error("Expected non-empty formatted output")
		}
		if !contains(formatted, "call_001") {
			t.Error("Formatted output should contain call IDs")
		}
		if !contains(formatted, "tool_results") {
			t.Error("Formatted output should have tool_results tags")
		}
	})

	t.Run("ContainsToolCalls Check", func(t *testing.T) {
		withTools := "```json\n{\"tool_calls\": [{\"name\": \"test\"}]}\n```"
		withoutTools := "This is just a regular response."

		if !parser.ContainsToolCalls(withTools) {
			t.Error("Should detect tool calls")
		}
		if parser.ContainsToolCalls(withoutTools) {
			t.Error("Should not detect tool calls in plain text")
		}
	})
}

// TestCoordinationIntegration tests coordination protocol parsing
func TestCoordinationIntegration(t *testing.T) {
	parser := coordination.NewParser()

	t.Run("Parse Delegation Intent", func(t *testing.T) {
		response := `I'll delegate this to the documentation agent.

` + "```json\n" + `{
  "action": "delegate",
  "target": "docs",
  "task": "Generate API documentation for the tools package",
  "context": {
    "reason": "Documentation specialist",
    "files": ["pkg/tools/types.go", "pkg/tools/registry.go"]
  },
  "priority": "normal"
}` + "\n```\n" + `

The docs agent is better suited for this task.`

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if !result.HasIntent {
			t.Error("Expected HasIntent to be true")
		}

		intent := result.GetFirstIntent()
		if intent == nil {
			t.Fatal("Expected non-nil intent")
		}
		if intent.Action != coordination.ActionDelegate {
			t.Errorf("Expected delegate action, got %s", intent.Action)
		}
		if intent.Target != "docs" {
			t.Errorf("Expected target docs, got %s", intent.Target)
		}
		if intent.Priority != coordination.PriorityNormal {
			t.Errorf("Expected normal priority, got %s", intent.Priority)
		}
	})

	t.Run("Parse Execution Plan", func(t *testing.T) {
		response := `Here's my plan for implementing this feature:

` + "```json\n" + `{
  "action": "plan",
  "summary": "Implement user authentication",
  "steps": [
    {"step": 1, "agent": "sntr", "action": "Read existing auth code"},
    {"step": 2, "agent": "code", "action": "Implement new auth module", "dependsOn": [1]},
    {"step": 3, "agent": "sntr", "action": "Write tests", "dependsOn": [2]},
    {"step": 4, "agent": "git", "action": "Commit changes", "dependsOn": [3]}
  ],
  "requiresApproval": true
}` + "\n```\n" + `

This plan requires your approval before execution.`

		result, err := parser.ParseResponse(response)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		if !result.HasPlan {
			t.Error("Expected HasPlan to be true")
		}

		plan := result.Plan
		if plan == nil {
			t.Fatal("Expected non-nil plan")
		}
		if len(plan.Steps) != 4 {
			t.Errorf("Expected 4 steps, got %d", len(plan.Steps))
		}
		if !plan.RequiresApproval {
			t.Error("Expected RequiresApproval to be true")
		}

		// Check dependencies
		step2 := plan.Steps[1]
		if len(step2.DependsOn) != 1 || step2.DependsOn[0] != 1 {
			t.Error("Step 2 should depend on step 1")
		}
	})
}

// TestSecurityValidation tests security manager validation
func TestSecurityValidation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "syntor-security-test-*")
	defer os.RemoveAll(tmpDir)

	securityMgr := security.NewManager(tmpDir)
	pathValidator := securityMgr.GetPathValidator()
	cmdValidator := securityMgr.GetCommandValidator()

	t.Run("Allow Valid Path", func(t *testing.T) {
		validPath := filepath.Join(tmpDir, "test.txt")
		err := pathValidator.ValidatePath(validPath, false)
		if err != nil {
			t.Errorf("Should allow path within working dir: %v", err)
		}
	})

	t.Run("Deny Sensitive Path", func(t *testing.T) {
		err := pathValidator.ValidatePath("/etc/passwd", false)
		if err == nil {
			t.Error("Should deny access to /etc/passwd")
		}
	})

	t.Run("Deny Path Traversal", func(t *testing.T) {
		traversalPath := filepath.Join(tmpDir, "..", "..", "etc", "passwd")
		err := pathValidator.ValidatePath(traversalPath, false)
		if err == nil {
			t.Error("Should deny path traversal attempts")
		}
	})

	t.Run("Validate Safe Command", func(t *testing.T) {
		err := cmdValidator.ValidateCommand("ls -la")
		if err != nil {
			t.Errorf("Should allow ls command: %v", err)
		}
	})

	t.Run("Deny Dangerous Command", func(t *testing.T) {
		err := cmdValidator.ValidateCommand("rm -rf /")
		if err == nil {
			t.Error("Should deny dangerous rm command")
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
