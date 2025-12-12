package implementations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syntor/syntor/pkg/tools"
)

// ListDirectoryTool lists directory contents
type ListDirectoryTool struct {
	workingDir string
}

// NewListDirectoryTool creates a new list directory tool
func NewListDirectoryTool(workingDir string) *ListDirectoryTool {
	return &ListDirectoryTool{
		workingDir: workingDir,
	}
}

// Name returns the tool's name
func (t *ListDirectoryTool) Name() string {
	return "list_directory"
}

// Description returns the tool's description
func (t *ListDirectoryTool) Description() string {
	return "Lists the contents of a directory. Shows files and subdirectories with size information. Optionally recursive."
}

// Execute lists the directory
func (t *ListDirectoryTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	result := &tools.ToolResult{
		ToolName: t.Name(),
	}

	// Get path
	dirPath, ok := params["path"].(string)
	if !ok || dirPath == "" {
		dirPath = t.workingDir
	}

	// Expand home directory
	if strings.HasPrefix(dirPath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			dirPath = filepath.Join(home, dirPath[1:])
		}
	}

	// Make path absolute
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: fmt.Sprintf("invalid path: %s", err),
		}
		return result, nil
	}

	// Verify path exists and is a directory
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeFileNotFound,
			Message: fmt.Sprintf("directory not found: %s", dirPath),
		}
		return result, nil
	}
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("cannot access path: %s", err),
		}
		return result, nil
	}
	if !info.IsDir() {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "path is not a directory",
		}
		return result, nil
	}

	// Get recursive flag
	recursive := false
	if v, ok := params["recursive"].(bool); ok {
		recursive = v
	}

	// Get max depth
	maxDepth := 3
	if v, ok := params["max_depth"].(float64); ok && v > 0 {
		maxDepth = int(v)
	}

	// Get show hidden flag
	showHidden := false
	if v, ok := params["show_hidden"].(bool); ok {
		showHidden = v
	}

	// List entries
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory: %s\n\n", absPath))

	if recursive {
		err = t.listRecursive(ctx, &sb, absPath, "", 0, maxDepth, showHidden)
	} else {
		err = t.listFlat(&sb, absPath, showHidden)
	}

	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("error listing directory: %s", err),
		}
		return result, nil
	}

	result.Success = true
	result.Output = sb.String()

	return result, nil
}

func (t *ListDirectoryTool) listFlat(sb *strings.Builder, path string, showHidden bool) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// Separate directories and files
	var dirs, files []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	// Sort alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name() < dirs[j].Name()
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	// Write directories first
	for _, entry := range dirs {
		sb.WriteString(fmt.Sprintf("📁 %s/\n", entry.Name()))
	}

	// Then files with sizes
	for _, entry := range files {
		info, err := entry.Info()
		if err != nil {
			sb.WriteString(fmt.Sprintf("📄 %s\n", entry.Name()))
			continue
		}
		sb.WriteString(fmt.Sprintf("📄 %s (%s)\n", entry.Name(), formatSize(info.Size())))
	}

	sb.WriteString(fmt.Sprintf("\n%d directories, %d files\n", len(dirs), len(files)))

	return nil
}

func (t *ListDirectoryTool) listRecursive(ctx context.Context, sb *strings.Builder, basePath, prefix string, depth, maxDepth int, showHidden bool) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if depth >= maxDepth {
		return nil
	}

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil // Skip unreadable directories
	}

	// Filter and sort entries
	var filtered []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		// Skip common large directories
		if entry.IsDir() && (name == "node_modules" || name == "vendor" || name == "__pycache__" || name == ".git") {
			continue
		}
		filtered = append(filtered, entry)
	}

	// Sort: directories first, then files
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return filtered[i].Name() < filtered[j].Name()
	})

	for i, entry := range filtered {
		isLast := i == len(filtered)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, connector, entry.Name()))
			t.listRecursive(ctx, sb, filepath.Join(basePath, entry.Name()), prefix+childPrefix, depth+1, maxDepth, showHidden)
		} else {
			info, err := entry.Info()
			sizeStr := ""
			if err == nil {
				sizeStr = fmt.Sprintf(" (%s)", formatSize(info.Size()))
			}
			sb.WriteString(fmt.Sprintf("%s%s%s%s\n", prefix, connector, entry.Name(), sizeStr))
		}
	}

	return nil
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// Validate checks the parameters
func (t *ListDirectoryTool) Validate(params map[string]interface{}) error {
	// Path is optional, defaults to working directory
	return nil
}

// SetWorkingDir updates the working directory
func (t *ListDirectoryTool) SetWorkingDir(dir string) {
	t.workingDir = dir
}

// Definition returns the tool definition
func (t *ListDirectoryTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategoryFilesystem,
		Parameters: []tools.ToolParameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "Directory path to list (defaults to working directory)",
				Required:    false,
			},
			{
				Name:        "recursive",
				Type:        "boolean",
				Description: "List contents recursively",
				Required:    false,
				Default:     false,
			},
			{
				Name:        "max_depth",
				Type:        "integer",
				Description: "Maximum depth for recursive listing (default 3)",
				Required:    false,
				Default:     3,
			},
			{
				Name:        "show_hidden",
				Type:        "boolean",
				Description: "Show hidden files and directories",
				Required:    false,
				Default:     false,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "Directory listing with file sizes",
		},
	}
}
