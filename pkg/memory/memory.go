// Package memory provides persistent cross-session memory for Syn Tor.
// Memory files (MEMORY.md) are loaded at startup and injected into prompts.
// Auto-memory extraction records insights from sessions automatically.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxMemoryLines is the maximum number of lines in MEMORY.md before truncation.
	MaxMemoryLines = 200

	// GlobalMemoryFile is the name of the global memory file.
	GlobalMemoryFile = "MEMORY.md"

	// MemoryDir is the subdirectory for topic-specific memory files.
	MemoryDir = "memory"
)

// Manager handles loading, injecting, and writing persistent memory.
type Manager struct {
	globalDir  string // ~/.syntor/
	projectDir string // .syntor/ (relative to project root)
}

// NewManager creates a new memory manager.
// If globalDir is empty, defaults to ~/.syntor.
// If projectDir is empty, uses current working directory.
func NewManager(globalDir, projectDir string) *Manager {
	if globalDir == "" {
		home, _ := os.UserHomeDir()
		globalDir = filepath.Join(home, ".syntor")
	}
	if projectDir == "" {
		projectDir = ".syntor"
	}
	// Bootstrap: ensure global memory dir exists
	os.MkdirAll(filepath.Join(globalDir, MemoryDir), 0755)

	return &Manager{
		globalDir:  globalDir,
		projectDir: projectDir,
	}
}

// LoadGlobal reads ~/.syntor/MEMORY.md.
func (m *Manager) LoadGlobal() (string, error) {
	path := filepath.Join(m.globalDir, GlobalMemoryFile)
	return readFileOrEmpty(path)
}

// LoadProject reads .syntor/MEMORY.md from the project directory.
func (m *Manager) LoadProject() (string, error) {
	path := filepath.Join(m.projectDir, GlobalMemoryFile)
	return readFileOrEmpty(path)
}

// LoadTopicFiles reads all .md files from the memory directory.
func (m *Manager) LoadTopicFiles(scope string) (map[string]string, error) {
	var dir string
	switch scope {
	case "global":
		dir = filepath.Join(m.globalDir, MemoryDir)
	case "project":
		dir = filepath.Join(m.projectDir, MemoryDir)
	default:
		return nil, fmt.Errorf("invalid scope: %s (use 'global' or 'project')", scope)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	topics := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		topics[name] = string(content)
	}

	return topics, nil
}

// FormatForPrompt formats memory content for injection into the system prompt.
func (m *Manager) FormatForPrompt() string {
	var sb strings.Builder

	// Load global memory
	globalMem, _ := m.LoadGlobal()
	if globalMem != "" {
		// Truncate to MaxMemoryLines
		lines := strings.Split(globalMem, "\n")
		if len(lines) > MaxMemoryLines {
			lines = lines[:MaxMemoryLines]
			lines = append(lines, "\n... (truncated)")
		}
		globalMem = strings.Join(lines, "\n")

		sb.WriteString("<memory scope=\"global\">\n")
		sb.WriteString(globalMem)
		sb.WriteString("\n</memory>\n\n")
	}

	// Load project memory
	projectMem, _ := m.LoadProject()
	if projectMem != "" {
		lines := strings.Split(projectMem, "\n")
		if len(lines) > MaxMemoryLines {
			lines = lines[:MaxMemoryLines]
			lines = append(lines, "\n... (truncated)")
		}
		projectMem = strings.Join(lines, "\n")

		sb.WriteString("<memory scope=\"project\">\n")
		sb.WriteString(projectMem)
		sb.WriteString("\n</memory>\n\n")
	}

	return sb.String()
}

// Write appends or updates a key-value entry in MEMORY.md.
func (m *Manager) Write(scope, content string) error {
	var path string
	switch scope {
	case "global":
		path = filepath.Join(m.globalDir, GlobalMemoryFile)
	case "project":
		path = filepath.Join(m.projectDir, GlobalMemoryFile)
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Append to file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n" + content + "\n")
	return err
}

// CreateTopicFile creates a new topic-specific memory file.
func (m *Manager) CreateTopicFile(scope, name, content string) error {
	var dir string
	switch scope {
	case "global":
		dir = filepath.Join(m.globalDir, MemoryDir)
	case "project":
		dir = filepath.Join(m.projectDir, MemoryDir)
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := name
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	path := filepath.Join(dir, filename)

	// Add header with timestamp
	header := fmt.Sprintf("# %s\n\nCreated: %s\n\n", name, time.Now().Format(time.RFC3339))
	return os.WriteFile(path, []byte(header+content), 0644)
}

// UpdateTopicFile appends content to an existing topic file.
func (m *Manager) UpdateTopicFile(scope, name, content string) error {
	var dir string
	switch scope {
	case "global":
		dir = filepath.Join(m.globalDir, MemoryDir)
	case "project":
		dir = filepath.Join(m.projectDir, MemoryDir)
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	filename := name
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	path := filepath.Join(dir, filename)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString("\n" + content + "\n")
	return err
}

// TruncateMemory ensures MEMORY.md doesn't exceed MaxMemoryLines.
func (m *Manager) TruncateMemory(scope string) error {
	var path string
	switch scope {
	case "global":
		path = filepath.Join(m.globalDir, GlobalMemoryFile)
	case "project":
		path = filepath.Join(m.projectDir, GlobalMemoryFile)
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) <= MaxMemoryLines {
		return nil // No truncation needed
	}

	truncated := strings.Join(lines[:MaxMemoryLines], "\n")
	return os.WriteFile(path, []byte(truncated), 0644)
}

// readFileOrEmpty reads a file, returning empty string if not found.
func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
