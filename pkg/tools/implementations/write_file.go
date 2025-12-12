package implementations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syntor/syntor/pkg/tools"
)

// WriteFileTool writes content to a file
type WriteFileTool struct{}

// NewWriteFileTool creates a new write file tool
func NewWriteFileTool() *WriteFileTool {
	return &WriteFileTool{}
}

// Name returns the tool's name
func (t *WriteFileTool) Name() string {
	return "write_file"
}

// Description returns the tool's description
func (t *WriteFileTool) Description() string {
	return "Writes content to a file. Creates the file if it doesn't exist, or overwrites if it does. Creates parent directories as needed."
}

// Execute writes to the file
func (t *WriteFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	result := &tools.ToolResult{
		ToolName: t.Name(),
	}

	// Get file path
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "file_path is required",
		}
		return result, nil
	}

	// Get content
	content, ok := params["content"].(string)
	if !ok {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "content is required",
		}
		return result, nil
	}

	// Expand home directory
	if strings.HasPrefix(filePath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = filepath.Join(home, filePath[1:])
		}
	}

	// Make path absolute
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: fmt.Sprintf("invalid path: %s", err),
		}
		return result, nil
	}

	// Create parent directories if needed
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodePermissionDenied,
			Message: fmt.Sprintf("cannot create directory: %s", err),
		}
		return result, nil
	}

	// Check if file exists (for reporting)
	_, existErr := os.Stat(absPath)
	fileExisted := existErr == nil

	// Write file
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("cannot write file: %s", err),
		}
		return result, nil
	}

	result.Success = true
	if fileExisted {
		result.Output = fmt.Sprintf("File overwritten: %s (%d bytes)", filePath, len(content))
	} else {
		result.Output = fmt.Sprintf("File created: %s (%d bytes)", filePath, len(content))
	}

	return result, nil
}

// Validate checks the parameters
func (t *WriteFileTool) Validate(params map[string]interface{}) error {
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return fmt.Errorf("file_path is required")
	}

	if _, ok := params["content"].(string); !ok {
		return fmt.Errorf("content is required")
	}

	return nil
}

// Definition returns the tool definition
func (t *WriteFileTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategoryFilesystem,
		Parameters: []tools.ToolParameter{
			{
				Name:        "file_path",
				Type:        "string",
				Description: "The absolute path to the file to write",
				Required:    true,
			},
			{
				Name:        "content",
				Type:        "string",
				Description: "The content to write to the file",
				Required:    true,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "Success message with file path and bytes written",
		},
		Security: tools.ToolSecuritySpec{
			RequireApproval: true,
		},
	}
}
