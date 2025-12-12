package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/syntor/syntor/pkg/config"
)

// Command represents a slash command with metadata
type Command struct {
	Name        string
	Description string
	Category    string // "agent", "system", "custom"
	IsAgent     bool   // true if this is an agent-switching command
}

// CommandRegistry manages available slash commands
type CommandRegistry struct {
	commands map[string]Command
}

// NewCommandRegistry creates a new command registry with built-in commands
func NewCommandRegistry() *CommandRegistry {
	r := &CommandRegistry{
		commands: make(map[string]Command),
	}
	r.registerBuiltinCommands()
	r.loadCustomCommands()
	return r
}

// registerBuiltinCommands adds the built-in slash commands
func (r *CommandRegistry) registerBuiltinCommands() {
	// Agent commands
	r.commands["coordination"] = Command{
		Name:        "coordination",
		Description: "Switch to coordination agent",
		Category:    "agent",
		IsAgent:     true,
	}
	r.commands["docs"] = Command{
		Name:        "docs",
		Description: "Switch to documentation agent",
		Category:    "agent",
		IsAgent:     true,
	}
	r.commands["git"] = Command{
		Name:        "git",
		Description: "Switch to git agent",
		Category:    "agent",
		IsAgent:     true,
	}
	r.commands["worker"] = Command{
		Name:        "worker",
		Description: "Switch to worker agent",
		Category:    "agent",
		IsAgent:     true,
	}
	r.commands["code"] = Command{
		Name:        "code",
		Description: "Switch to code worker agent",
		Category:    "agent",
		IsAgent:     true,
	}

	// System commands
	r.commands["help"] = Command{
		Name:        "help",
		Description: "Show available commands",
		Category:    "system",
	}
	r.commands["status"] = Command{
		Name:        "status",
		Description: "Show current agent and model",
		Category:    "system",
	}
	r.commands["models"] = Command{
		Name:        "models",
		Description: "List available models",
		Category:    "system",
	}
	r.commands["config"] = Command{
		Name:        "config",
		Description: "Show configuration",
		Category:    "system",
	}
	r.commands["clear"] = Command{
		Name:        "clear",
		Description: "Clear the screen",
		Category:    "system",
	}
	r.commands["quit"] = Command{
		Name:        "quit",
		Description: "Exit SYNTOR",
		Category:    "system",
	}
	r.commands["exit"] = Command{
		Name:        "exit",
		Description: "Exit SYNTOR",
		Category:    "system",
	}
	r.commands["copy"] = Command{
		Name:        "copy",
		Description: "Copy code block to clipboard (/copy [n])",
		Category:    "system",
	}
}

// loadCustomCommands loads custom commands from config directories
func (r *CommandRegistry) loadCustomCommands() {
	globalDir, projectDir := config.ConfigPaths()

	// Load from global commands directory
	r.loadCommandsFromDir(filepath.Join(globalDir, "commands"))

	// Load from project commands directory (overrides global)
	r.loadCommandsFromDir(filepath.Join(projectDir, "commands"))
}

// loadCommandsFromDir loads commands from markdown files in a directory
func (r *CommandRegistry) loadCommandsFromDir(dir string) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".md")
		// Read first line for description
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		description := "Custom command"
		lines := strings.Split(string(content), "\n")
		if len(lines) > 0 {
			firstLine := strings.TrimSpace(lines[0])
			// If first line is a comment or header, use it as description
			if strings.HasPrefix(firstLine, "#") {
				description = strings.TrimSpace(strings.TrimPrefix(firstLine, "#"))
			} else if len(firstLine) < 60 {
				description = firstLine
			}
		}

		r.commands[name] = Command{
			Name:        name,
			Description: description,
			Category:    "custom",
		}
	}
}

// GetCommand returns a command by name
func (r *CommandRegistry) GetCommand(name string) (Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// GetAllCommands returns all registered commands
func (r *CommandRegistry) GetAllCommands() []Command {
	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		// Sort by category first, then by name
		if cmds[i].Category != cmds[j].Category {
			categoryOrder := map[string]int{"agent": 0, "system": 1, "custom": 2}
			return categoryOrder[cmds[i].Category] < categoryOrder[cmds[j].Category]
		}
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// GetAgentCommands returns only agent-switching commands
func (r *CommandRegistry) GetAgentCommands() []Command {
	cmds := make([]Command, 0)
	for _, cmd := range r.commands {
		if cmd.IsAgent {
			cmds = append(cmds, cmd)
		}
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// FilterCommands returns commands matching a prefix
func (r *CommandRegistry) FilterCommands(prefix string) []Command {
	prefix = strings.ToLower(prefix)
	cmds := make([]Command, 0)
	for _, cmd := range r.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			cmds = append(cmds, cmd)
		}
	}
	sort.Slice(cmds, func(i, j int) bool {
		// Exact matches first
		if cmds[i].Name == prefix {
			return true
		}
		if cmds[j].Name == prefix {
			return false
		}
		// Then by category
		if cmds[i].Category != cmds[j].Category {
			categoryOrder := map[string]int{"agent": 0, "system": 1, "custom": 2}
			return categoryOrder[cmds[i].Category] < categoryOrder[cmds[j].Category]
		}
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// CommandExists checks if a command exists
func (r *CommandRegistry) CommandExists(name string) bool {
	_, ok := r.commands[name]
	return ok
}
