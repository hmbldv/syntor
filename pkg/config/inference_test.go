package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// === FUNCTIONAL TESTS (FOUNDRY) ===

func TestGlobalContextPath(t *testing.T) {
	t.Run("returns correct path", func(t *testing.T) {
		path := GlobalContextPath()
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".syntor", "CENTAUR.md")
		if path != expected {
			t.Errorf("GlobalContextPath() = %q, want %q", path, expected)
		}
	})

	t.Run("path is absolute", func(t *testing.T) {
		path := GlobalContextPath()
		if !filepath.IsAbs(path) {
			t.Errorf("GlobalContextPath() should return absolute path, got %q", path)
		}
	})

	t.Run("path contains syntor directory", func(t *testing.T) {
		path := GlobalContextPath()
		if !strings.Contains(path, ".syntor") {
			t.Errorf("GlobalContextPath() should contain .syntor, got %q", path)
		}
	})
}

func TestLoadGlobalContext(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		syntorDir := filepath.Join(tmpDir, ".syntor")
		os.MkdirAll(syntorDir, 0755)
		testFile := filepath.Join(syntorDir, "CENTAUR.md")
		os.WriteFile(testFile, []byte("# Test Global Context\nContent here."), 0644)

		// Temporarily override HOME
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		content, err := LoadGlobalContext()
		if err != nil {
			t.Fatalf("LoadGlobalContext() error = %v", err)
		}
		if !strings.Contains(content, "Test Global Context") {
			t.Errorf("LoadGlobalContext() content missing expected text")
		}
	})

	t.Run("file not exists returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		content, err := LoadGlobalContext()
		if err != nil {
			t.Fatalf("LoadGlobalContext() should not error when file missing, got %v", err)
		}
		if content != "" {
			t.Errorf("LoadGlobalContext() = %q, want empty string", content)
		}
	})
}

func TestGetGlobalContext(t *testing.T) {
	t.Run("returns content when file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		syntorDir := filepath.Join(tmpDir, ".syntor")
		os.MkdirAll(syntorDir, 0755)
		testFile := filepath.Join(syntorDir, "CENTAUR.md")
		os.WriteFile(testFile, []byte("# Wrapper Test"), 0644)

		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		content, err := GetGlobalContext()
		if err != nil {
			t.Fatalf("GetGlobalContext() error = %v", err)
		}
		if !strings.Contains(content, "Wrapper Test") {
			t.Errorf("GetGlobalContext() missing expected content")
		}
	})

	t.Run("returns empty string when file not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		content, err := GetGlobalContext()
		if err != nil {
			t.Fatalf("GetGlobalContext() should not error, got %v", err)
		}
		if content != "" {
			t.Errorf("GetGlobalContext() = %q, want empty string", content)
		}
	})
}

func TestCreateDefaultGlobalContext(t *testing.T) {
	t.Run("creates file with content", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		err := CreateDefaultGlobalContext()
		if err != nil {
			t.Fatalf("CreateDefaultGlobalContext() error = %v", err)
		}

		path := GlobalContextPath()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read created file: %v", err)
		}
		if !strings.Contains(string(content), "CENTAUR") {
			t.Error("Created file should contain CENTAUR content")
		}
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		err := CreateDefaultGlobalContext()
		if err != nil {
			t.Fatalf("CreateDefaultGlobalContext() error = %v", err)
		}

		syntorDir := filepath.Join(tmpDir, ".syntor")
		info, err := os.Stat(syntorDir)
		if err != nil {
			t.Fatalf("Directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("Expected directory, got file")
		}
	})

	t.Run("file contains agent routing protocol", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		err := CreateDefaultGlobalContext()
		if err != nil {
			t.Fatalf("CreateDefaultGlobalContext() error = %v", err)
		}

		path := GlobalContextPath()
		content, _ := os.ReadFile(path)
		if !strings.Contains(string(content), "Agent Routing Protocol") {
			t.Error("Created file should contain Agent Routing Protocol section")
		}
		if !strings.Contains(string(content), "FalkorDB") {
			t.Error("Created file should mention FalkorDB for routing")
		}
	})
}

func TestGlobalContextExists(t *testing.T) {
	t.Run("returns true when file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		syntorDir := filepath.Join(tmpDir, ".syntor")
		os.MkdirAll(syntorDir, 0755)
		testFile := filepath.Join(syntorDir, "CENTAUR.md")
		os.WriteFile(testFile, []byte("test"), 0644)

		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		if !GlobalContextExists() {
			t.Error("GlobalContextExists() = false, want true")
		}
	})

	t.Run("returns false when file not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		if GlobalContextExists() {
			t.Error("GlobalContextExists() = true, want false")
		}
	})

	t.Run("returns false when directory exists but file does not", func(t *testing.T) {
		tmpDir := t.TempDir()
		syntorDir := filepath.Join(tmpDir, ".syntor")
		os.MkdirAll(syntorDir, 0755)

		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		if GlobalContextExists() {
			t.Error("GlobalContextExists() = true when only directory exists, want false")
		}
	})
}

func TestProjectMarkdownPath(t *testing.T) {
	t.Run("returns SYNTOR.md", func(t *testing.T) {
		path := ProjectMarkdownPath()
		if path != "SYNTOR.md" {
			t.Errorf("ProjectMarkdownPath() = %q, want SYNTOR.md", path)
		}
	})
}

func TestCreateProjectMarkdown(t *testing.T) {
	t.Run("creates file with project info", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Change to temp directory
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(tmpDir)

		err := CreateProjectMarkdown("TestProject", "A test project description")
		if err != nil {
			t.Fatalf("CreateProjectMarkdown() error = %v", err)
		}

		content, err := os.ReadFile("SYNTOR.md")
		if err != nil {
			t.Fatalf("Failed to read SYNTOR.md: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "TestProject") {
			t.Error("SYNTOR.md should contain project name")
		}
		if !strings.Contains(contentStr, "A test project description") {
			t.Error("SYNTOR.md should contain project description")
		}
	})
}

func TestConfigPaths(t *testing.T) {
	t.Run("returns correct paths", func(t *testing.T) {
		globalDir, projectDir := ConfigPaths()

		home, _ := os.UserHomeDir()
		expectedGlobal := filepath.Join(home, ".syntor")
		if globalDir != expectedGlobal {
			t.Errorf("global dir = %q, want %q", globalDir, expectedGlobal)
		}
		if projectDir != ".syntor" {
			t.Errorf("project dir = %q, want .syntor", projectDir)
		}
	})
}

func TestInferenceConfigGetModelForAgent(t *testing.T) {
	t.Run("returns specific model for coordination", func(t *testing.T) {
		cfg := &InferenceConfig{
			DefaultModel: "default:model",
			Models: AgentModels{
				Coordination: "coordination:model",
			},
		}

		model := cfg.GetModelForAgent("coordination")
		if model != "coordination:model" {
			t.Errorf("GetModelForAgent(coordination) = %q, want coordination:model", model)
		}
	})

	t.Run("returns default model for unknown agent", func(t *testing.T) {
		cfg := &InferenceConfig{
			DefaultModel: "default:model",
		}

		model := cfg.GetModelForAgent("unknown")
		if model != "default:model" {
			t.Errorf("GetModelForAgent(unknown) = %q, want default:model", model)
		}
	})

	t.Run("returns default when agent model is empty", func(t *testing.T) {
		cfg := &InferenceConfig{
			DefaultModel: "default:model",
			Models: AgentModels{
				Coordination: "", // Empty
			},
		}

		model := cfg.GetModelForAgent("coordination")
		if model != "default:model" {
			t.Errorf("GetModelForAgent(coordination) = %q, want default:model when agent model is empty", model)
		}
	})
}

func TestInferenceConfigGetAllAssignedModels(t *testing.T) {
	t.Run("returns unique models", func(t *testing.T) {
		cfg := &InferenceConfig{
			DefaultModel: "default:model",
			Models: AgentModels{
				Coordination:  "model:a",
				Documentation: "model:b",
				Git:           "model:a", // Duplicate
				Worker:        "model:c",
				WorkerCode:    "model:c", // Duplicate
			},
		}

		models := cfg.GetAllAssignedModels()

		// Should have 4 unique models: a, b, c, default
		if len(models) != 4 {
			t.Errorf("GetAllAssignedModels() returned %d models, want 4", len(models))
		}

		// Verify all expected models are present
		expected := map[string]bool{"model:a": false, "model:b": false, "model:c": false, "default:model": false}
		for _, m := range models {
			if _, ok := expected[m]; !ok {
				t.Errorf("Unexpected model: %s", m)
			}
			expected[m] = true
		}
		for m, found := range expected {
			if !found {
				t.Errorf("Missing expected model: %s", m)
			}
		}
	})
}

// === SECURITY TESTS (CRBRS) ===

func TestGlobalContextPath_NoTraversal(t *testing.T) {
	t.Run("path does not contain traversal sequences", func(t *testing.T) {
		path := GlobalContextPath()
		if strings.Contains(path, "..") {
			t.Errorf("Path should not contain traversal sequences: %s", path)
		}
	})

	t.Run("HOME manipulation does not escape syntor dir", func(t *testing.T) {
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)

		os.Setenv("HOME", "/tmp/../etc")
		path := GlobalContextPath()
		cleanPath := filepath.Clean(path)

		// Should still be under a syntor directory
		if !strings.Contains(cleanPath, ".syntor") {
			t.Errorf("Path should contain .syntor even with manipulated HOME: %s", cleanPath)
		}
	})

	t.Run("path is under home directory", func(t *testing.T) {
		path := GlobalContextPath()
		home, _ := os.UserHomeDir()

		if !strings.HasPrefix(path, home) {
			t.Errorf("Path should be under home directory: path=%s, home=%s", path, home)
		}
	})
}

func TestCreateDefaultGlobalContext_SafeWrite(t *testing.T) {
	t.Run("file created with secure permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		err := CreateDefaultGlobalContext()
		if err != nil {
			t.Fatalf("CreateDefaultGlobalContext() error = %v", err)
		}

		path := GlobalContextPath()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}

		mode := info.Mode().Perm()
		// File should not be world-writable
		if mode&0002 != 0 {
			t.Errorf("File should not be world-writable, mode = %o", mode)
		}
		// File should be at most 0644
		if mode > 0644 {
			t.Errorf("File permissions too permissive: %o (max 0644)", mode)
		}
	})

	t.Run("directory created with secure permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		err := CreateDefaultGlobalContext()
		if err != nil {
			t.Fatalf("CreateDefaultGlobalContext() error = %v", err)
		}

		syntorDir := filepath.Join(tmpDir, ".syntor")
		info, err := os.Stat(syntorDir)
		if err != nil {
			t.Fatalf("Failed to stat directory: %v", err)
		}

		mode := info.Mode().Perm()
		if mode > 0755 {
			t.Errorf("Directory permissions too permissive: %o (max 0755)", mode)
		}
	})
}

func TestLoadGlobalContext_SecurityBounds(t *testing.T) {
	t.Run("does not read outside syntor directory", func(t *testing.T) {
		// Create a file outside .syntor
		tmpDir := t.TempDir()
		outsideFile := filepath.Join(tmpDir, "CENTAUR.md")
		os.WriteFile(outsideFile, []byte("SHOULD NOT READ"), 0644)

		originalHome := os.Getenv("HOME")
		defer os.Setenv("HOME", originalHome)
		os.Setenv("HOME", tmpDir)

		// LoadGlobalContext should only look in .syntor/CENTAUR.md
		content, _ := LoadGlobalContext()
		if strings.Contains(content, "SHOULD NOT READ") {
			t.Error("LoadGlobalContext read file outside .syntor directory")
		}
	})
}

func TestLoadProjectMarkdown_SearchBounds(t *testing.T) {
	t.Run("does not search beyond 5 parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a deep directory structure (7 levels deep)
		deepPath := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "f", "g")
		os.MkdirAll(deepPath, 0755)

		// Put SYNTOR.md at the root (7 levels up from deepPath)
		os.WriteFile(filepath.Join(tmpDir, "SYNTOR.md"), []byte("ROOT_SYNTOR"), 0644)

		// Change to deep directory
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(deepPath)

		// Should not find it because it's 7 levels up
		_, _, err := LoadProjectMarkdown()
		if err == nil {
			t.Error("LoadProjectMarkdown should not search beyond 5 parent directories")
		}
	})

	t.Run("finds SYNTOR.md within 5 parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a 5-level deep directory structure
		deepPath := filepath.Join(tmpDir, "a", "b", "c", "d", "e")
		os.MkdirAll(deepPath, 0755)

		// Put SYNTOR.md at the root (5 levels up)
		os.WriteFile(filepath.Join(tmpDir, "SYNTOR.md"), []byte("FOUND_SYNTOR"), 0644)

		// Change to deep directory
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(deepPath)

		// Should find it
		content, _, err := LoadProjectMarkdown()
		if err != nil {
			t.Fatalf("LoadProjectMarkdown should find file within 5 parents: %v", err)
		}
		if !strings.Contains(content, "FOUND_SYNTOR") {
			t.Error("LoadProjectMarkdown returned wrong content")
		}
	})
}

func TestDefaultConfigs(t *testing.T) {
	t.Run("DefaultInferenceConfig has sane defaults", func(t *testing.T) {
		cfg := DefaultInferenceConfig()

		if cfg.Provider == "" {
			t.Error("DefaultInferenceConfig should have a provider")
		}
		if cfg.DefaultModel == "" {
			t.Error("DefaultInferenceConfig should have a default model")
		}
		if cfg.OllamaHost == "" {
			t.Error("DefaultInferenceConfig should have an Ollama host")
		}
	})

	t.Run("DefaultCLIConfig has sane defaults", func(t *testing.T) {
		cfg := DefaultCLIConfig()

		if cfg.Theme == "" {
			t.Error("DefaultCLIConfig should have a theme")
		}
		if cfg.Editor == "" {
			t.Error("DefaultCLIConfig should have an editor")
		}
	})

	t.Run("DefaultSyntorConfig combines defaults", func(t *testing.T) {
		cfg := DefaultSyntorConfig()

		if cfg.Inference.Provider == "" {
			t.Error("DefaultSyntorConfig should have inference provider")
		}
		if cfg.CLI.Theme == "" {
			t.Error("DefaultSyntorConfig should have CLI theme")
		}
	})
}

func TestGetEnv(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv("TEST_VAR_123", "test_value")
		defer os.Unsetenv("TEST_VAR_123")

		result := GetEnv("TEST_VAR_123", "default")
		if result != "test_value" {
			t.Errorf("GetEnv = %q, want test_value", result)
		}
	})

	t.Run("returns default when not set", func(t *testing.T) {
		result := GetEnv("NONEXISTENT_VAR_456", "default_value")
		if result != "default_value" {
			t.Errorf("GetEnv = %q, want default_value", result)
		}
	})
}
