package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Discovery manages tool discovery across MCP servers.
type Discovery struct {
	client *Client

	// Tool index for fast lookup
	toolsByName     map[string]*DiscoveredTool
	toolsByServer   map[string][]*DiscoveredTool
	toolsByCategory map[string][]*DiscoveredTool
	mu              sync.RWMutex
}

// NewDiscovery creates a tool discovery manager.
func NewDiscovery(client *Client) *Discovery {
	return &Discovery{
		client:          client,
		toolsByName:     make(map[string]*DiscoveredTool),
		toolsByServer:   make(map[string][]*DiscoveredTool),
		toolsByCategory: make(map[string][]*DiscoveredTool),
	}
}

// Refresh updates the tool index from all connected servers.
func (d *Discovery) Refresh(ctx context.Context) error {
	tools, err := d.client.ListAllTools(ctx)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Clear existing index
	d.toolsByName = make(map[string]*DiscoveredTool)
	d.toolsByServer = make(map[string][]*DiscoveredTool)
	d.toolsByCategory = make(map[string][]*DiscoveredTool)

	// Build index
	for i := range tools {
		tool := &tools[i]
		d.toolsByName[tool.FullName] = tool
		d.toolsByServer[tool.ServerName] = append(d.toolsByServer[tool.ServerName], tool)

		// Categorize by name patterns
		category := categorizeToolName(tool.Tool.Name)
		d.toolsByCategory[category] = append(d.toolsByCategory[category], tool)
	}

	return nil
}

// FindTool looks up a tool by full name.
func (d *Discovery) FindTool(fullName string) (*DiscoveredTool, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	tool, ok := d.toolsByName[fullName]
	return tool, ok
}

// SearchTools searches for tools matching keywords.
func (d *Discovery) SearchTools(keywords ...string) []*DiscoveredTool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []*DiscoveredTool
	for _, tool := range d.toolsByName {
		if matchesKeywords(tool, keywords) {
			results = append(results, tool)
		}
	}
	return results
}

// GetToolsByServer returns all tools from a specific server.
func (d *Discovery) GetToolsByServer(serverName string) []*DiscoveredTool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.toolsByServer[serverName]
}

// GetToolsByCategory returns tools in a category.
func (d *Discovery) GetToolsByCategory(category string) []*DiscoveredTool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.toolsByCategory[category]
}

// GetAllTools returns all discovered tools.
func (d *Discovery) GetAllTools() []*DiscoveredTool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tools := make([]*DiscoveredTool, 0, len(d.toolsByName))
	for _, tool := range d.toolsByName {
		tools = append(tools, tool)
	}
	return tools
}

// GetCategories returns all tool categories.
func (d *Discovery) GetCategories() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	categories := make([]string, 0, len(d.toolsByCategory))
	for cat := range d.toolsByCategory {
		categories = append(categories, cat)
	}
	return categories
}

// GetToolSchema returns the JSON Schema for a tool's input.
func (d *Discovery) GetToolSchema(fullName string) (*ToolInputSchema, error) {
	d.mu.RLock()
	tool, ok := d.toolsByName[fullName]
	d.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found: %s", fullName)
	}

	var schema ToolInputSchema
	if err := json.Unmarshal(tool.Tool.InputSchema, &schema); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	return &schema, nil
}

// GenerateToolDocs generates documentation for discovered tools.
func (d *Discovery) GenerateToolDocs() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("# Available MCP Tools\n\n")

	// Group by server
	for server, tools := range d.toolsByServer {
		sb.WriteString(fmt.Sprintf("## %s\n\n", server))

		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("### %s\n", tool.FullName))
			if tool.Tool.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", tool.Tool.Description))
			}

			var schema ToolInputSchema
			if json.Unmarshal(tool.Tool.InputSchema, &schema) == nil {
				if len(schema.Properties) > 0 {
					sb.WriteString("**Parameters:**\n")
					for name, prop := range schema.Properties {
						required := ""
						for _, req := range schema.Required {
							if req == name {
								required = " (required)"
								break
							}
						}
						sb.WriteString(fmt.Sprintf("- `%s`%s: %s\n", name, required, prop.Description))
					}
					sb.WriteString("\n")
				}
			}
		}
	}

	return sb.String()
}

// Helper functions

func categorizeToolName(name string) string {
	name = strings.ToLower(name)

	// Common categories based on tool name patterns
	patterns := map[string][]string{
		"database":   {"sql", "query", "database", "db", "postgres", "mysql", "mongo"},
		"filesystem": {"file", "read", "write", "directory", "path", "glob"},
		"git":        {"git", "commit", "branch", "merge", "pull", "push"},
		"web":        {"http", "fetch", "request", "api", "url", "web"},
		"search":     {"search", "find", "grep", "locate"},
		"shell":      {"bash", "shell", "exec", "command", "run"},
		"code":       {"code", "edit", "format", "lint", "compile"},
	}

	for category, keywords := range patterns {
		for _, keyword := range keywords {
			if strings.Contains(name, keyword) {
				return category
			}
		}
	}

	return "other"
}

func matchesKeywords(tool *DiscoveredTool, keywords []string) bool {
	searchText := strings.ToLower(tool.Tool.Name + " " + tool.Tool.Description + " " + tool.ServerName)

	for _, kw := range keywords {
		if !strings.Contains(searchText, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}
