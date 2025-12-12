package implementations

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/syntor/syntor/pkg/tools"
)

// GrepTool searches file contents with regex
type GrepTool struct {
	workingDir string
}

// NewGrepTool creates a new grep tool
func NewGrepTool(workingDir string) *GrepTool {
	return &GrepTool{
		workingDir: workingDir,
	}
}

// Name returns the tool's name
func (t *GrepTool) Name() string {
	return "grep"
}

// Description returns the tool's description
func (t *GrepTool) Description() string {
	return "Searches file contents using regular expressions. Returns matching lines with file paths and line numbers. Supports context lines before/after matches."
}

// Execute performs the search
func (t *GrepTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
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

	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: fmt.Sprintf("invalid regex pattern: %s", err),
		}
		return result, nil
	}

	// Get case insensitivity
	if caseInsensitive, ok := params["case_insensitive"].(bool); ok && caseInsensitive {
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			result.Error = &tools.ToolError{
				Code:    tools.ErrCodeInvalidParams,
				Message: fmt.Sprintf("invalid regex pattern: %s", err),
			}
			return result, nil
		}
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

	// Get file filter
	filePattern := ""
	if v, ok := params["glob"].(string); ok {
		filePattern = v
	}

	// Get context lines
	contextBefore := 0
	if v, ok := params["context_before"].(float64); ok {
		contextBefore = int(v)
	}
	contextAfter := 0
	if v, ok := params["context_after"].(float64); ok {
		contextAfter = int(v)
	}

	// Get max results
	maxResults := 100
	if v, ok := params["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
		if maxResults > 1000 {
			maxResults = 1000
		}
	}

	// Search results
	type match struct {
		file    string
		lineNum int
		line    string
		context []string
	}

	var matches []match
	totalMatches := 0

	// Walk directory
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
			// Skip hidden directories and common non-code directories
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and large files
		if info.Size() > 1024*1024 { // 1MB
			return nil
		}

		// Apply file pattern filter
		if filePattern != "" {
			matched, _ := filepath.Match(filePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Skip binary files by extension
		ext := strings.ToLower(filepath.Ext(path))
		binaryExts := map[string]bool{
			".exe": true, ".dll": true, ".so": true, ".dylib": true,
			".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
			".zip": true, ".tar": true, ".gz": true, ".rar": true,
			".pdf": true, ".doc": true, ".docx": true,
			".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		}
		if binaryExts[ext] {
			return nil
		}

		// Search file
		fileMatches := searchFile(path, re, contextBefore, contextAfter, maxResults-totalMatches)
		for _, m := range fileMatches {
			matches = append(matches, m)
			totalMatches++
			if totalMatches >= maxResults {
				return fmt.Errorf("max results reached")
			}
		}

		return nil
	})

	if err != nil && err.Error() != "max results reached" && err != context.Canceled {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeExecutionError,
			Message: fmt.Sprintf("search error: %s", err),
		}
		return result, nil
	}

	// Build output
	var sb strings.Builder

	if len(matches) == 0 {
		result.Success = true
		result.Output = "No matches found"
		return result, nil
	}

	currentFile := ""
	for _, m := range matches {
		if m.file != currentFile {
			if currentFile != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("=== %s ===\n", m.file))
			currentFile = m.file
		}

		// Write context before
		for _, ctx := range m.context {
			sb.WriteString(ctx)
			sb.WriteString("\n")
		}

		// Write match line
		sb.WriteString(fmt.Sprintf("%d: %s\n", m.lineNum, m.line))
	}

	if totalMatches >= maxResults {
		sb.WriteString(fmt.Sprintf("\n[Results limited to %d matches]", maxResults))
		result.Truncated = true
	}

	result.Success = true
	result.Output = fmt.Sprintf("Found %d match(es):\n\n%s", len(matches), sb.String())

	return result, nil
}

// searchFile searches a single file for matches
func searchFile(path string, re *regexp.Regexp, contextBefore, contextAfter, maxMatches int) []struct {
	file    string
	lineNum int
	line    string
	context []string
} {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var matches []struct {
		file    string
		lineNum int
		line    string
		context []string
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer for long lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		lines = append(lines, line)

		if re.MatchString(line) {
			// Get context lines
			var context []string
			start := len(lines) - 1 - contextBefore
			if start < 0 {
				start = 0
			}
			for i := start; i < len(lines)-1; i++ {
				context = append(context, fmt.Sprintf("%d- %s", lineNum-(len(lines)-1-i), lines[i]))
			}

			matches = append(matches, struct {
				file    string
				lineNum int
				line    string
				context []string
			}{
				file:    path,
				lineNum: lineNum,
				line:    line,
				context: context,
			})

			if len(matches) >= maxMatches {
				break
			}
		}

		// Keep only necessary context lines in memory
		if len(lines) > contextBefore+1 {
			lines = lines[1:]
		}
	}

	return matches
}

// Validate checks the parameters
func (t *GrepTool) Validate(params map[string]interface{}) error {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return fmt.Errorf("pattern is required")
	}

	// Validate regex
	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %s", err)
	}

	return nil
}

// SetWorkingDir updates the working directory
func (t *GrepTool) SetWorkingDir(dir string) {
	t.workingDir = dir
}

// Definition returns the tool definition
func (t *GrepTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategorySearch,
		Parameters: []tools.ToolParameter{
			{
				Name:        "pattern",
				Type:        "string",
				Description: "Regular expression pattern to search for",
				Required:    true,
			},
			{
				Name:        "path",
				Type:        "string",
				Description: "Directory to search in (defaults to working directory)",
				Required:    false,
			},
			{
				Name:        "glob",
				Type:        "string",
				Description: "File pattern filter (e.g., '*.go', '*.ts')",
				Required:    false,
			},
			{
				Name:        "case_insensitive",
				Type:        "boolean",
				Description: "Perform case-insensitive search",
				Required:    false,
				Default:     false,
			},
			{
				Name:        "context_before",
				Type:        "integer",
				Description: "Number of lines to show before each match",
				Required:    false,
				Default:     0,
			},
			{
				Name:        "context_after",
				Type:        "integer",
				Description: "Number of lines to show after each match",
				Required:    false,
				Default:     0,
			},
			{
				Name:        "max_results",
				Type:        "integer",
				Description: "Maximum number of matches to return (default 100, max 1000)",
				Required:    false,
				Default:     100,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "Matching lines with file paths and line numbers",
		},
	}
}
