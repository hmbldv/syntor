package prompt

import (
	"context"
	"testing"

	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/tools"
)

// createTestStore creates a manifest store for testing
func createTestStore() *manifest.ManifestStore {
	store, _ := manifest.NewManifestStore([]string{})
	return store
}

func TestContextGatherer(t *testing.T) {
	t.Run("NewContextGatherer", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp/test")

		if gatherer == nil {
			t.Fatal("Expected non-nil gatherer")
		}
		if gatherer.GetProjectPath() != "/tmp/test" {
			t.Errorf("Expected /tmp/test, got %s", gatherer.GetProjectPath())
		}
	})

	t.Run("SetProjectPath", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/initial")

		gatherer.SetProjectPath("/updated")
		if gatherer.GetProjectPath() != "/updated" {
			t.Error("Project path not updated")
		}
	})

	t.Run("GatherAgentContext", func(t *testing.T) {
		// Use the actual configs/agents directory if available
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp")
		ctx := context.Background()

		// This will return empty list if no agents are loaded
		agents, err := gatherer.GatherAgentContext(ctx)
		if err != nil {
			t.Fatalf("GatherAgentContext failed: %v", err)
		}
		// Just verify it returns without error
		_ = agents
	})

	t.Run("GatherFullContext", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp")
		ctx := context.Background()

		promptCtx, err := gatherer.GatherFullContext(ctx, true)
		if err != nil {
			t.Fatalf("GatherFullContext failed: %v", err)
		}
		if !promptCtx.PlanMode {
			t.Error("Expected PlanMode to be true")
		}
	})
}

func TestToolContextGathering(t *testing.T) {
	t.Run("GatherToolContext Without Registry", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp")

		result := gatherer.GatherToolContext(nil)
		if result != "" {
			t.Error("Expected empty string without registry")
		}
	})

	t.Run("GatherToolContext With Registry", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp")

		// Create and register tool
		registry := tools.NewRegistry()
		mockTool := &mockTool{name: "test_tool", desc: "A test tool"}
		toolDef := &tools.ToolDefinition{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters: []tools.ToolParameter{
				{Name: "param1", Type: "string", Required: true},
			},
		}
		registry.Register(mockTool, toolDef)

		gatherer.SetToolRegistry(registry)

		result := gatherer.GatherToolContext([]string{"test_tool"})
		if result == "" {
			t.Error("Expected non-empty tool context")
		}
		if !contains(result, "test_tool") {
			t.Error("Tool context should contain tool name")
		}
	})

	t.Run("GatherToolContextStructured", func(t *testing.T) {
		store := createTestStore()
		gatherer := NewContextGatherer(store, "/tmp")

		registry := tools.NewRegistry()
		mockTool := &mockTool{name: "read_file", desc: "Read a file"}
		toolDef := &tools.ToolDefinition{
			Name:        "read_file",
			Description: "Read a file",
			Category:    tools.ToolCategoryFilesystem,
			Parameters: []tools.ToolParameter{
				{Name: "path", Type: "string", Required: true, Description: "File path"},
			},
		}
		registry.Register(mockTool, toolDef)

		gatherer.SetToolRegistry(registry)

		structured := gatherer.GatherToolContextStructured(nil)
		if len(structured) != 1 {
			t.Fatalf("Expected 1 tool context, got %d", len(structured))
		}
		if structured[0].Name != "read_file" {
			t.Error("Tool name mismatch")
		}
		if len(structured[0].Parameters) != 1 {
			t.Error("Expected 1 parameter")
		}
	})
}

func TestInMemoryStore(t *testing.T) {
	t.Run("Set and Get", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		store.Set("key1", MemoryItem{
			Key:   "key1",
			Value: "value1",
		})

		val, err := store.Get(ctx, "key1")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if val != "value1" {
			t.Errorf("Expected value1, got %s", val)
		}
	})

	t.Run("Get Non-Existent", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		val, err := store.Get(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if val != "" {
			t.Error("Expected empty string for non-existent key")
		}
	})

	t.Run("Query", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		store.Set("user:1", MemoryItem{Key: "user:1", Value: "alice"})
		store.Set("user:2", MemoryItem{Key: "user:2", Value: "bob"})
		store.Set("task:1", MemoryItem{Key: "task:1", Value: "task"})

		items, err := store.Query(ctx, "user:", 10)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(items))
		}
	})

	t.Run("Query With Limit", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		for i := 0; i < 10; i++ {
			store.Set(string(rune('a'+i)), MemoryItem{Key: string(rune('a' + i)), Value: "val"})
		}

		items, err := store.Query(ctx, "", 5)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(items) != 5 {
			t.Errorf("Expected 5 items (limit), got %d", len(items))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		store.Set("key", MemoryItem{Key: "key", Value: "value"})
		store.Delete("key")

		val, _ := store.Get(ctx, "key")
		if val != "" {
			t.Error("Expected empty after delete")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		store := NewInMemoryStore()
		ctx := context.Background()

		store.Set("key1", MemoryItem{Key: "key1", Value: "value1"})
		store.Set("key2", MemoryItem{Key: "key2", Value: "value2"})
		store.Clear()

		items, _ := store.Query(ctx, "", 100)
		if len(items) != 0 {
			t.Error("Expected empty store after clear")
		}
	})
}

func TestPromptContext(t *testing.T) {
	t.Run("Default Values", func(t *testing.T) {
		ctx := &PromptContext{}

		if ctx.PlanMode {
			t.Error("PlanMode should default to false")
		}
		if ctx.Agents != nil && len(ctx.Agents) > 0 {
			t.Error("Agents should be nil/empty by default")
		}
	})

	t.Run("With Values", func(t *testing.T) {
		ctx := &PromptContext{
			PlanMode: true,
			Agents: []AgentContext{
				{Name: "test", Description: "Test agent"},
			},
			ToolPrompt: "Available tools...",
			Custom: map[string]interface{}{
				"key": "value",
			},
		}

		if !ctx.PlanMode {
			t.Error("PlanMode should be true")
		}
		if len(ctx.Agents) != 1 {
			t.Error("Expected 1 agent")
		}
		if ctx.ToolPrompt == "" {
			t.Error("ToolPrompt should be set")
		}
		if ctx.Custom["key"] != "value" {
			t.Error("Custom context mismatch")
		}
	})
}

func TestBuildOptions(t *testing.T) {
	t.Run("Default Options", func(t *testing.T) {
		opts := BuildOptions{}

		if opts.IncludeAgents {
			t.Error("IncludeAgents should default to false")
		}
		if opts.IncludeProject {
			t.Error("IncludeProject should default to false")
		}
		if opts.IncludeTools {
			t.Error("IncludeTools should default to false")
		}
	})

	t.Run("Full Options", func(t *testing.T) {
		opts := BuildOptions{
			IncludeAgents:  true,
			IncludeProject: true,
			IncludeMemory:  true,
			IncludeTools:   true,
			ToolNames:      []string{"read_file", "write_file"},
			PlanMode:       true,
			CustomContext:  map[string]interface{}{"key": "value"},
		}

		if !opts.IncludeAgents {
			t.Error("IncludeAgents should be true")
		}
		if len(opts.ToolNames) != 2 {
			t.Error("Expected 2 tool names")
		}
	})
}

func TestAgentContext(t *testing.T) {
	t.Run("Fields", func(t *testing.T) {
		agent := AgentContext{
			Name:         "sntr",
			Type:         "coordination",
			Description:  "Main orchestrator",
			Capabilities: []string{"planning", "delegation"},
			Status:       "healthy",
			Load:         0.5,
		}

		if agent.Name != "sntr" {
			t.Error("Name mismatch")
		}
		if len(agent.Capabilities) != 2 {
			t.Error("Expected 2 capabilities")
		}
		if agent.Load != 0.5 {
			t.Error("Load mismatch")
		}
	})
}

func TestProjectContext(t *testing.T) {
	t.Run("Fields", func(t *testing.T) {
		project := ProjectContext{
			Name:        "syntor",
			Description: "Multi-agent AI system",
			Path:        "/home/user/syntor",
			Values:      []string{"quality", "performance"},
			Goals:       []string{"v1.0 release"},
			Conventions: map[string]string{
				"language": "Go",
			},
		}

		if project.Name != "syntor" {
			t.Error("Name mismatch")
		}
		if len(project.Values) != 2 {
			t.Error("Expected 2 values")
		}
		if project.Conventions["language"] != "Go" {
			t.Error("Convention mismatch")
		}
	})
}

func TestMemoryItem(t *testing.T) {
	t.Run("Fields", func(t *testing.T) {
		item := MemoryItem{
			Key:       "context:session",
			Value:     "User prefers detailed responses",
			Source:    "sntr",
			Timestamp: 1702400000,
		}

		if item.Key != "context:session" {
			t.Error("Key mismatch")
		}
		if item.Timestamp != 1702400000 {
			t.Error("Timestamp mismatch")
		}
	})
}

// Mock tool for testing
type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.desc }
func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	return &tools.ToolResult{Success: true}, nil
}
func (m *mockTool) Validate(params map[string]interface{}) error { return nil }

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
