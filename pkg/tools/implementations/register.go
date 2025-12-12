package implementations

import (
	"github.com/syntor/syntor/pkg/tools"
)

// ToolWithDefinition is a tool that can provide its own definition
type ToolWithDefinition interface {
	tools.Tool
	Definition() *tools.ToolDefinition
}

// RegisterAll registers all built-in tools with the registry
func RegisterAll(registry *tools.Registry, workingDir string) error {
	// Create tools
	toolList := []ToolWithDefinition{
		NewReadFileTool(),
		NewWriteFileTool(),
		NewEditFileTool(),
		NewBashTool(workingDir),
		NewGlobTool(workingDir),
		NewGrepTool(workingDir),
		NewListDirectoryTool(workingDir),
	}

	// Register each tool
	for _, tool := range toolList {
		if err := registry.Register(tool, tool.Definition()); err != nil {
			return err
		}
	}

	return nil
}

// DefaultToolNames returns the names of all default tools
func DefaultToolNames() []string {
	return []string{
		"read_file",
		"write_file",
		"edit_file",
		"bash",
		"glob",
		"grep",
		"list_directory",
	}
}

// UpdateWorkingDir updates the working directory for all applicable tools
func UpdateWorkingDir(registry *tools.Registry, workingDir string) {
	// Tools that have SetWorkingDir method
	if tool, ok := registry.Get("bash"); ok {
		if bt, ok := tool.(*BashTool); ok {
			bt.SetWorkingDir(workingDir)
		}
	}

	if tool, ok := registry.Get("glob"); ok {
		if gt, ok := tool.(*GlobTool); ok {
			gt.SetWorkingDir(workingDir)
		}
	}

	if tool, ok := registry.Get("grep"); ok {
		if gt, ok := tool.(*GrepTool); ok {
			gt.SetWorkingDir(workingDir)
		}
	}

	if tool, ok := registry.Get("list_directory"); ok {
		if lt, ok := tool.(*ListDirectoryTool); ok {
			lt.SetWorkingDir(workingDir)
		}
	}
}
