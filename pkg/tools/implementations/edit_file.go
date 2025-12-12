package implementations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syntor/syntor/pkg/tools"
)

// EditFileTool performs find-and-replace edits in files
type EditFileTool struct{}

// NewEditFileTool creates a new edit file tool
func NewEditFileTool() *EditFileTool {
	return &EditFileTool{}
}

// Name returns the tool's name
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Description returns the tool's description
func (t *EditFileTool) Description() string {
	return "Performs exact string replacement in a file. The old_string must be unique in the file for the edit to succeed. Use replace_all=true to replace all occurrences."
}

// Execute performs the edit
func (t *EditFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
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

	// Get old_string
	oldString, ok := params["old_string"].(string)
	if !ok {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "old_string is required",
		}
		return result, nil
	}

	// Get new_string
	newString, ok := params["new_string"].(string)
	if !ok {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "new_string is required",
		}
		return result, nil
	}

	// Check if strings are different
	if oldString == newString {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "old_string and new_string must be different",
		}
		return result, nil
	}

	// Get replace_all flag
	replaceAll := false
	if v, ok := params["replace_all"].(bool); ok {
		replaceAll = v
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

	// Read file
	content, err := os.ReadFile(absPath)
	if os.IsNotExist(err) {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeFileNotFound,
			Message: fmt.Sprintf("file not found: %s", filePath),
		}
		return result, nil
	}
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("cannot read file: %s", err),
		}
		return result, nil
	}

	contentStr := string(content)

	// Count occurrences
	count := strings.Count(contentStr, oldString)

	if count == 0 {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: "old_string not found in file",
		}
		return result, nil
	}

	// If not replace_all and multiple occurrences, fail
	if !replaceAll && count > 1 {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("old_string appears %d times in file. Use replace_all=true to replace all occurrences, or provide more context to make it unique.", count),
		}
		return result, nil
	}

	// Perform replacement
	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(contentStr, oldString, newString)
	} else {
		newContent = strings.Replace(contentStr, oldString, newString, 1)
	}

	// Write back
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("cannot write file: %s", err),
		}
		return result, nil
	}

	result.Success = true
	if replaceAll {
		result.Output = fmt.Sprintf("Replaced %d occurrence(s) in %s", count, filePath)
	} else {
		result.Output = fmt.Sprintf("Replaced 1 occurrence in %s", filePath)
	}

	return result, nil
}

// Validate checks the parameters
func (t *EditFileTool) Validate(params map[string]interface{}) error {
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return fmt.Errorf("file_path is required")
	}

	if _, ok := params["old_string"].(string); !ok {
		return fmt.Errorf("old_string is required")
	}

	if _, ok := params["new_string"].(string); !ok {
		return fmt.Errorf("new_string is required")
	}

	return nil
}

// Definition returns the tool definition
func (t *EditFileTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategoryFilesystem,
		Parameters: []tools.ToolParameter{
			{
				Name:        "file_path",
				Type:        "string",
				Description: "The absolute path to the file to edit",
				Required:    true,
			},
			{
				Name:        "old_string",
				Type:        "string",
				Description: "The exact string to find and replace",
				Required:    true,
			},
			{
				Name:        "new_string",
				Type:        "string",
				Description: "The string to replace old_string with",
				Required:    true,
			},
			{
				Name:        "replace_all",
				Type:        "boolean",
				Description: "Replace all occurrences instead of requiring uniqueness",
				Required:    false,
				Default:     false,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "Success message with number of replacements",
		},
		Security: tools.ToolSecuritySpec{
			RequireApproval: true,
		},
	}
}
