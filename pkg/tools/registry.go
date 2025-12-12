package tools

import (
	"fmt"
	"strings"
	"sync"
)

// Registry manages available tools
type Registry struct {
	tools       map[string]Tool
	definitions map[string]*ToolDefinition
	mu          sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools:       make(map[string]Tool),
		definitions: make(map[string]*ToolDefinition),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool, def *ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.tools[name] = tool
	if def != nil {
		r.definitions[name] = def
	}
	return nil
}

// Get returns a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// GetDefinition returns a tool definition by name
func (r *Registry) GetDefinition(name string) (*ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[name]
	return def, ok
}

// List returns all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetToolsForNames returns tools by their names
func (r *Registry) GetToolsForNames(names []string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(names))
	for _, name := range names {
		if tool, ok := r.tools[name]; ok {
			tools = append(tools, tool)
		}
	}
	return tools
}

// GenerateToolPrompt generates the tool description section for system prompts
func (r *Registry) GenerateToolPrompt(toolNames []string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("You have access to the following tools. To use a tool, output a JSON code block:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"tool_calls\": [\n")
	sb.WriteString("    {\"id\": \"unique_id\", \"name\": \"tool_name\", \"parameters\": {...}}\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")
	sb.WriteString("You can call multiple tools in a single response by adding multiple entries to the tool_calls array.\n\n")

	for _, name := range toolNames {
		tool, ok := r.tools[name]
		if !ok {
			continue
		}

		def, hasDef := r.definitions[name]

		sb.WriteString(fmt.Sprintf("### %s\n", name))
		sb.WriteString(fmt.Sprintf("%s\n\n", tool.Description()))

		if hasDef && len(def.Parameters) > 0 {
			sb.WriteString("**Parameters:**\n")
			for _, param := range def.Parameters {
				required := ""
				if param.Required {
					required = " (required)"
				}
				sb.WriteString(fmt.Sprintf("- `%s` (%s)%s: %s\n",
					param.Name, param.Type, required, param.Description))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// GenerateCompactToolList generates a shorter tool list for context-constrained prompts
func (r *Registry) GenerateCompactToolList(toolNames []string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Tools: ")

	names := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		if _, ok := r.tools[name]; ok {
			names = append(names, name)
		}
	}

	sb.WriteString(strings.Join(names, ", "))
	return sb.String()
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}
