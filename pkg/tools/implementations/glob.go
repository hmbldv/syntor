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

// GlobTool finds files matching a pattern
type GlobTool struct {
	workingDir string
}

// NewGlobTool creates a new glob tool
func NewGlobTool(workingDir string) *GlobTool {
	return &GlobTool{
		workingDir: workingDir,
	}
}

// Name returns the tool's name
func (t *GlobTool) Name() string {
	return "glob"
}

// Description returns the tool's description
func (t *GlobTool) Description() string {
	return "Finds files matching a glob pattern. Supports patterns like '**/*.go', 'src/**/*.ts'. Returns file paths sorted by modification time (newest first)."
}

// Execute performs the glob search
func (t *GlobTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	result := &tools.ToolResult{
		ToolName: t.Name(),
	}

	// Get pattern
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "pattern is required",
		}
		return result, nil
	}

	// Get search path
	searchPath := t.workingDir
	if v, ok := params["path"].(string); ok && v != "" {
		searchPath = v
	}

	// Expand home directory
	if strings.HasPrefix(searchPath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			searchPath = filepath.Join(home, searchPath[1:])
		}
	}

	// Make path absolute
	absPath, err := filepath.Abs(searchPath)
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: fmt.Sprintf("invalid path: %s", err),
		}
		return result, nil
	}

	// Verify path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeFileNotFound,
			Message: fmt.Sprintf("path not found: %s", searchPath),
		}
		return result, nil
	}

	// Find matching files
	type fileInfo struct {
		path    string
		modTime int64
	}

	var matches []fileInfo

	// Handle ** patterns (recursive)
	if strings.Contains(pattern, "**") {
		err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if info.IsDir() {
				return nil
			}

			// Get relative path for matching
			relPath, err := filepath.Rel(absPath, path)
			if err != nil {
				return nil
			}

			// Match against pattern
			if matchGlobPattern(pattern, relPath) {
				matches = append(matches, fileInfo{
					path:    path,
					modTime: info.ModTime().Unix(),
				})
			}

			return nil
		})
	} else {
		// Simple glob without **
		fullPattern := filepath.Join(absPath, pattern)
		globMatches, err := filepath.Glob(fullPattern)
		if err == nil {
			for _, path := range globMatches {
				info, err := os.Stat(path)
				if err == nil && !info.IsDir() {
					matches = append(matches, fileInfo{
						path:    path,
						modTime: info.ModTime().Unix(),
					})
				}
			}
		}
	}

	if err != nil && err != context.Canceled {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("glob error: %s", err),
		}
		return result, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	// Build output
	var sb strings.Builder
	maxResults := 1000

	for i, m := range matches {
		if i >= maxResults {
			sb.WriteString(fmt.Sprintf("\n[... truncated, showing %d of %d matches]", maxResults, len(matches)))
			result.Truncated = true
			break
		}
		sb.WriteString(m.path)
		sb.WriteString("\n")
	}

	result.Success = true
	if len(matches) == 0 {
		result.Output = "No files found matching pattern"
	} else {
		result.Output = fmt.Sprintf("Found %d file(s):\n%s", len(matches), sb.String())
	}

	return result, nil
}

// matchGlobPattern matches a file path against a glob pattern with ** support
func matchGlobPattern(pattern, path string) bool {
	// Convert ** to a regex-like matching
	parts := strings.Split(pattern, "**")

	if len(parts) == 1 {
		// No **, use simple glob
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	// Handle ** patterns
	// **/*.go matches any .go file in any subdirectory
	// src/**/*.ts matches .ts files under src/

	pathParts := strings.Split(filepath.ToSlash(path), "/")

	// Check prefix (before **)
	prefix := strings.TrimSuffix(parts[0], "/")
	if prefix != "" {
		if !strings.HasPrefix(filepath.ToSlash(path), prefix+"/") && filepath.ToSlash(path) != prefix {
			return false
		}
	}

	// Check suffix (after **)
	suffix := strings.TrimPrefix(parts[len(parts)-1], "/")
	if suffix != "" {
		// Match the suffix pattern against the filename
		fileName := pathParts[len(pathParts)-1]
		matched, _ := filepath.Match(suffix, fileName)
		if !matched {
			return false
		}
	}

	return true
}

// Validate checks the parameters
func (t *GlobTool) Validate(params map[string]interface{}) error {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	return nil
}

// SetWorkingDir updates the working directory
func (t *GlobTool) SetWorkingDir(dir string) {
	t.workingDir = dir
}

// Definition returns the tool definition
func (t *GlobTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategorySearch,
		Parameters: []tools.ToolParameter{
			{
				Name:        "pattern",
				Type:        "string",
				Description: "Glob pattern to match files (e.g., '**/*.go', 'src/**/*.ts')",
				Required:    true,
			},
			{
				Name:        "path",
				Type:        "string",
				Description: "Base directory to search in (defaults to working directory)",
				Required:    false,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "List of matching file paths",
		},
	}
}
