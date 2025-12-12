package implementations

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syntor/syntor/pkg/tools"
)

// ReadFileTool reads file contents with line numbers
type ReadFileTool struct{}

// NewReadFileTool creates a new read file tool
func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{}
}

// Name returns the tool's name
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description returns the tool's description
func (t *ReadFileTool) Description() string {
	return "Reads a file from the filesystem and returns its contents with line numbers. Supports offset and limit parameters for reading portions of large files."
}

// Execute reads the file
func (t *ReadFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
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

	// Check if file exists
	info, err := os.Stat(absPath)
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
			Message: fmt.Sprintf("cannot access file: %s", err),
		}
		return result, nil
	}

	// Don't read directories
	if info.IsDir() {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "path is a directory, not a file",
		}
		return result, nil
	}

	// Get offset and limit
	offset := 0
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}

	limit := 2000 // Default limit
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	// Read file
	file, err := os.Open(absPath)
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodePermissionDenied,
			Message: fmt.Sprintf("cannot open file: %s", err),
		}
		return result, nil
	}
	defer file.Close()

	// Read lines with line numbers
	var sb strings.Builder
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long lines
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	lineNum := 0
	linesRead := 0
	const maxLineLength = 2000

	for scanner.Scan() {
		lineNum++

		// Skip lines before offset
		if lineNum <= offset {
			continue
		}

		// Stop at limit
		if linesRead >= limit {
			sb.WriteString(fmt.Sprintf("\n[... truncated after %d lines]", limit))
			result.Truncated = true
			break
		}

		line := scanner.Text()

		// Truncate very long lines
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}

		sb.WriteString(fmt.Sprintf("%6d\t%s\n", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("error reading file: %s", err),
		}
		return result, nil
	}

	result.Success = true
	result.Output = sb.String()

	return result, nil
}

// Validate checks the parameters
func (t *ReadFileTool) Validate(params map[string]interface{}) error {
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return fmt.Errorf("file_path is required")
	}

	if offset, ok := params["offset"].(float64); ok {
		if offset < 0 {
			return fmt.Errorf("offset must be non-negative")
		}
	}

	if limit, ok := params["limit"].(float64); ok {
		if limit <= 0 {
			return fmt.Errorf("limit must be positive")
		}
	}

	return nil
}

// Definition returns the tool definition
func (t *ReadFileTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategoryFilesystem,
		Parameters: []tools.ToolParameter{
			{
				Name:        "file_path",
				Type:        "string",
				Description: "The absolute path to the file to read",
				Required:    true,
			},
			{
				Name:        "offset",
				Type:        "integer",
				Description: "Line number to start reading from (0-indexed)",
				Required:    false,
				Default:     0,
			},
			{
				Name:        "limit",
				Type:        "integer",
				Description: "Maximum number of lines to read",
				Required:    false,
				Default:     2000,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "File contents with line numbers",
		},
	}
}
