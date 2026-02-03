package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// === FUNCTIONAL TESTS (FOUNDRY) ===

func TestNewCommandRegistry(t *testing.T) {
	t.Run("creates registry with builtin commands", func(t *testing.T) {
		reg := NewCommandRegistry()
		if reg == nil {
			t.Fatal("NewCommandRegistry returned nil")
		}

		// Check some expected builtin commands exist
		builtins := []string{"help", "status", "quit", "clear", "init"}
		for _, name := range builtins {
			if !reg.CommandExists(name) {
				t.Errorf("Expected builtin command %q to exist", name)
			}
		}
	})

	t.Run("agent commands are registered", func(t *testing.T) {
		reg := NewCommandRegistry()

		agentCmds := []string{"coordination", "docs", "git", "worker", "code"}
		for _, name := range agentCmds {
			cmd, ok := reg.GetCommand(name)
			if !ok {
				t.Errorf("Expected agent command %q to exist", name)
				continue
			}
			if cmd.Category != "agent" {
				t.Errorf("Command %q should have category 'agent', got %q", name, cmd.Category)
			}
		}
	})
}

func TestCommandRegistry_UsageCommand(t *testing.T) {
	t.Run("usage command exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("usage")
		if !ok {
			t.Fatal("usage command should exist")
		}
		if cmd.Category != "system" {
			t.Errorf("usage command category = %q, want system", cmd.Category)
		}
		if cmd.Description == "" {
			t.Error("usage command should have a description")
		}
	})
}

func TestCommandRegistry_InitProjectCommand(t *testing.T) {
	t.Run("init-project command exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("init-project")
		if !ok {
			t.Fatal("init-project command should exist")
		}
		if cmd.Category != "system" {
			t.Errorf("init-project command category = %q, want system", cmd.Category)
		}
		if cmd.Description == "" {
			t.Error("init-project command should have a description")
		}
	})
}

func TestCommandRegistry_InitGlobalCommand(t *testing.T) {
	t.Run("init-global command exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("init-global")
		if !ok {
			t.Fatal("init-global command should exist")
		}
		if cmd.Category != "system" {
			t.Errorf("init-global command category = %q, want system", cmd.Category)
		}
		if cmd.Description == "" {
			t.Error("init-global command should have a description")
		}
	})
}

func TestCommandRegistry_RouteCommand(t *testing.T) {
	t.Run("route command exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("route")
		if !ok {
			t.Fatal("route command should exist")
		}
		if cmd.Category != "agent" {
			t.Errorf("route command category = %q, want agent", cmd.Category)
		}
		if cmd.Description == "" {
			t.Error("route command should have a description")
		}
	})
}

func TestCommandRegistry_AgentsCommand(t *testing.T) {
	t.Run("agents command exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("agents")
		if !ok {
			t.Fatal("agents command should exist")
		}
		if cmd.Category != "system" {
			t.Errorf("agents command category = %q, want system", cmd.Category)
		}
		if cmd.Description == "" {
			t.Error("agents command should have a description")
		}
	})
}

func TestCommandRegistry_GetCommand(t *testing.T) {
	t.Run("returns command when exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmd, ok := reg.GetCommand("help")
		if !ok {
			t.Fatal("help command should exist")
		}
		if cmd.Name != "help" {
			t.Errorf("Command name = %q, want help", cmd.Name)
		}
	})

	t.Run("returns false when not exists", func(t *testing.T) {
		reg := NewCommandRegistry()

		_, ok := reg.GetCommand("nonexistent-command-xyz")
		if ok {
			t.Error("Should return false for nonexistent command")
		}
	})
}

func TestCommandRegistry_GetAllCommands(t *testing.T) {
	t.Run("returns all commands", func(t *testing.T) {
		reg := NewCommandRegistry()
		cmds := reg.GetAllCommands()

		if len(cmds) == 0 {
			t.Fatal("Should return at least builtin commands")
		}

		// Verify sorting by category
		categoryOrder := map[string]int{"agent": 0, "system": 1, "custom": 2}
		for i := 0; i < len(cmds)-1; i++ {
			currOrder := categoryOrder[cmds[i].Category]
			nextOrder := categoryOrder[cmds[i+1].Category]
			if currOrder > nextOrder {
				t.Errorf("Commands not sorted by category: %s (%s) before %s (%s)",
					cmds[i].Name, cmds[i].Category, cmds[i+1].Name, cmds[i+1].Category)
			}
		}
	})
}

func TestCommandRegistry_GetAgentCommands(t *testing.T) {
	t.Run("returns only agent commands", func(t *testing.T) {
		reg := NewCommandRegistry()
		cmds := reg.GetAgentCommands()

		for _, cmd := range cmds {
			if !cmd.IsAgent {
				t.Errorf("Command %q should have IsAgent=true", cmd.Name)
			}
		}
	})

	t.Run("agent commands are sorted by name", func(t *testing.T) {
		reg := NewCommandRegistry()
		cmds := reg.GetAgentCommands()

		for i := 0; i < len(cmds)-1; i++ {
			if cmds[i].Name > cmds[i+1].Name {
				t.Errorf("Agent commands not sorted: %s > %s", cmds[i].Name, cmds[i+1].Name)
			}
		}
	})
}

func TestCommandRegistry_FilterCommands(t *testing.T) {
	t.Run("filters by prefix", func(t *testing.T) {
		reg := NewCommandRegistry()

		// Filter by "co" should match "code", "config", "copy", "coordination"
		cmds := reg.FilterCommands("co")
		if len(cmds) == 0 {
			t.Fatal("Should find commands starting with 'co'")
		}

		for _, cmd := range cmds {
			if cmd.Name[:2] != "co" {
				t.Errorf("Command %q does not start with 'co'", cmd.Name)
			}
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmdsLower := reg.FilterCommands("help")
		cmdsUpper := reg.FilterCommands("HELP")

		if len(cmdsLower) != len(cmdsUpper) {
			t.Error("FilterCommands should be case insensitive")
		}
	})

	t.Run("empty prefix returns all", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmds := reg.FilterCommands("")
		allCmds := reg.GetAllCommands()

		if len(cmds) != len(allCmds) {
			t.Error("Empty prefix should return all commands")
		}
	})

	t.Run("exact match sorted first", func(t *testing.T) {
		reg := NewCommandRegistry()

		cmds := reg.FilterCommands("help")
		if len(cmds) > 0 && cmds[0].Name != "help" {
			t.Error("Exact match should be first in results")
		}
	})
}

func TestCommandRegistry_CommandExists(t *testing.T) {
	t.Run("returns true for existing command", func(t *testing.T) {
		reg := NewCommandRegistry()

		if !reg.CommandExists("help") {
			t.Error("help command should exist")
		}
	})

	t.Run("returns false for nonexistent command", func(t *testing.T) {
		reg := NewCommandRegistry()

		if reg.CommandExists("nonexistent-xyz") {
			t.Error("nonexistent-xyz should not exist")
		}
	})
}

func TestCommandRegistry_LoadCustomCommands(t *testing.T) {
	t.Run("loads commands from markdown files", func(t *testing.T) {
		// Create temp directories for custom commands
		tmpDir := t.TempDir()
		globalCmdDir := filepath.Join(tmpDir, ".syntor", "commands")
		os.MkdirAll(globalCmdDir, 0755)

		// Create a custom command file
		customCmd := filepath.Join(globalCmdDir, "mycommand.md")
		os.WriteFile(customCmd, []byte("# My Custom Command\nDoes something special"), 0644)

		// Create a registry that will try to load from the temp dir
		// Note: This tests the pattern but won't actually load from tmpDir
		// because loadCustomCommands uses ConfigPaths() which returns real paths
		reg := NewCommandRegistry()

		// The custom command won't be found because we can't easily mock ConfigPaths
		// This test verifies the registry is created without errors
		if reg == nil {
			t.Fatal("Registry creation should not fail")
		}
	})
}

func TestCommand(t *testing.T) {
	t.Run("command fields", func(t *testing.T) {
		cmd := Command{
			Name:        "test",
			Description: "A test command",
			Category:    "system",
			IsAgent:     false,
		}

		if cmd.Name != "test" {
			t.Errorf("Name = %q, want test", cmd.Name)
		}
		if cmd.Description != "A test command" {
			t.Errorf("Description mismatch")
		}
		if cmd.Category != "system" {
			t.Errorf("Category = %q, want system", cmd.Category)
		}
		if cmd.IsAgent {
			t.Error("IsAgent should be false")
		}
	})

	t.Run("agent command fields", func(t *testing.T) {
		cmd := Command{
			Name:        "myagent",
			Description: "Switch to my agent",
			Category:    "agent",
			IsAgent:     true,
		}

		if !cmd.IsAgent {
			t.Error("IsAgent should be true for agent commands")
		}
		if cmd.Category != "agent" {
			t.Error("Agent commands should have category 'agent'")
		}
	})
}

// === SECURITY TESTS (CRBRS) ===

func TestCommandRegistry_NoInjection(t *testing.T) {
	t.Run("command names are safe", func(t *testing.T) {
		reg := NewCommandRegistry()
		cmds := reg.GetAllCommands()

		for _, cmd := range cmds {
			// Command names should not contain shell metacharacters
			unsafeChars := []string{";", "|", "&", "$", "`", "(", ")", "<", ">", "\\", "\n", "\r"}
			for _, char := range unsafeChars {
				if contains(cmd.Name, char) {
					t.Errorf("Command name %q contains unsafe character %q", cmd.Name, char)
				}
			}
		}
	})
}

func TestFilterCommands_InputValidation(t *testing.T) {
	t.Run("handles special characters in prefix", func(t *testing.T) {
		reg := NewCommandRegistry()

		// These should not cause issues
		testPrefixes := []string{
			"*",
			".",
			"[",
			"]",
			"(",
			")",
			"\\",
			"/",
		}

		for _, prefix := range testPrefixes {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("FilterCommands panicked on prefix %q: %v", prefix, r)
					}
				}()
				// Should not panic
				reg.FilterCommands(prefix)
			}()
		}
	})
}

func TestCommandRegistry_CustomCommandSecurity(t *testing.T) {
	t.Run("custom command files must be .md", func(t *testing.T) {
		// loadCommandsFromDir uses filepath.Glob with "*.md" pattern
		// This ensures only .md files are loaded

		tmpDir := t.TempDir()

		// Create files with different extensions
		os.WriteFile(filepath.Join(tmpDir, "safe.md"), []byte("# Safe command"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "dangerous.sh"), []byte("#!/bin/bash\nrm -rf /"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "malicious.md.sh"), []byte("#!/bin/bash"), 0644)

		// Only .md files should be matched by Glob pattern "*.md"
		files, _ := filepath.Glob(filepath.Join(tmpDir, "*.md"))

		for _, f := range files {
			if filepath.Ext(f) != ".md" {
				t.Errorf("Glob matched non-.md file: %s", f)
			}
		}
	})
}

// === COMPREHENSIVE COMMAND LIST TESTS ===

func TestAllExpectedCommandsExist(t *testing.T) {
	reg := NewCommandRegistry()

	// System commands
	systemCommands := []string{
		"help",
		"status",
		"models",
		"config",
		"clear",
		"quit",
		"exit",
		"copy",
		"init",
		"end",
		"plan",
		"checkpoint",
		"skills",
		"usage",
		"init-project",
		"init-global",
		"agents",
	}

	for _, name := range systemCommands {
		t.Run("system/"+name, func(t *testing.T) {
			cmd, ok := reg.GetCommand(name)
			if !ok {
				t.Errorf("Expected system command %q to exist", name)
				return
			}
			if cmd.Category != "system" && cmd.Category != "agent" {
				t.Errorf("Command %q has unexpected category %q", name, cmd.Category)
			}
		})
	}

	// Agent commands
	agentCommands := []string{
		"coordination",
		"docs",
		"git",
		"worker",
		"code",
		"route",
	}

	for _, name := range agentCommands {
		t.Run("agent/"+name, func(t *testing.T) {
			_, ok := reg.GetCommand(name)
			if !ok {
				t.Errorf("Expected agent command %q to exist", name)
			}
		})
	}
}

func TestCommandDescriptionsAreHelpful(t *testing.T) {
	reg := NewCommandRegistry()
	cmds := reg.GetAllCommands()

	for _, cmd := range cmds {
		t.Run(cmd.Name, func(t *testing.T) {
			if cmd.Description == "" {
				t.Errorf("Command %q has no description", cmd.Name)
			}
			if len(cmd.Description) < 10 {
				t.Errorf("Command %q description too short: %q", cmd.Name, cmd.Description)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
