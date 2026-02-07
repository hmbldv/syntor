package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T071: TestFindProjectInstructions_NoFile - returns empty content when no SYNTOR.md
func TestFindProjectInstructions_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Override HOME so global rules also come from temp
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	pi, err := FindProjectInstructions(tmpDir)
	if err != nil {
		t.Fatalf("FindProjectInstructions() error = %v", err)
	}
	if pi.Content != "" {
		t.Errorf("Content = %q, want empty string when no SYNTOR.md", pi.Content)
	}
	if pi.Path != "" {
		t.Errorf("Path = %q, want empty string when no SYNTOR.md", pi.Path)
	}
}

// T072: TestFindProjectInstructions_WithFile - finds and reads SYNTOR.md
func TestFindProjectInstructions_WithFile(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	syntorContent := "# My Project\nThis is the project description."
	os.WriteFile(filepath.Join(tmpDir, "SYNTOR.md"), []byte(syntorContent), 0644)

	pi, err := FindProjectInstructions(tmpDir)
	if err != nil {
		t.Fatalf("FindProjectInstructions() error = %v", err)
	}
	if pi.Content != syntorContent {
		t.Errorf("Content = %q, want %q", pi.Content, syntorContent)
	}
	if pi.Path == "" {
		t.Error("Path should not be empty when SYNTOR.md exists")
	}
	if !strings.HasSuffix(pi.Path, "SYNTOR.md") {
		t.Errorf("Path = %q, should end with SYNTOR.md", pi.Path)
	}
}

// T073: TestFindProjectInstructions_ParentSearch - finds SYNTOR.md in parent dir
func TestFindProjectInstructions_ParentSearch(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Put SYNTOR.md in root
	os.WriteFile(filepath.Join(tmpDir, "SYNTOR.md"), []byte("# Parent Project"), 0644)

	// Create a child directory to search from
	childDir := filepath.Join(tmpDir, "subdir", "nested")
	os.MkdirAll(childDir, 0755)

	pi, err := FindProjectInstructions(childDir)
	if err != nil {
		t.Fatalf("FindProjectInstructions() error = %v", err)
	}
	if !strings.Contains(pi.Content, "Parent Project") {
		t.Errorf("Content = %q, should contain 'Parent Project'", pi.Content)
	}
	// Path should point to the parent's SYNTOR.md
	expectedPath := filepath.Join(tmpDir, "SYNTOR.md")
	if pi.Path != expectedPath {
		t.Errorf("Path = %q, want %q", pi.Path, expectedPath)
	}
}

// T074: TestLoadRulesDir_Empty - returns nil for empty/nonexistent dir
func TestLoadRulesDir_Empty(t *testing.T) {
	t.Run("nonexistent directory", func(t *testing.T) {
		rules := LoadRulesDir("/nonexistent/path/that/should/not/exist")
		if rules != nil {
			t.Errorf("LoadRulesDir() = %v, want nil for nonexistent dir", rules)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		rules := LoadRulesDir(tmpDir)
		if rules != nil {
			t.Errorf("LoadRulesDir() = %v, want nil for empty dir", rules)
		}
	})
}

// T075: TestLoadRulesDir_MultipleFiles - loads and sorts .md files
func TestLoadRulesDir_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rules files in non-alphabetical order
	files := map[string]string{
		"charlie.md": "# Charlie Rule",
		"alpha.md":   "# Alpha Rule",
		"bravo.md":   "# Bravo Rule",
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644)
	}

	rules := LoadRulesDir(tmpDir)
	if len(rules) != 3 {
		t.Fatalf("LoadRulesDir() returned %d rules, want 3", len(rules))
	}

	// Verify sorted order
	expectedOrder := []string{"alpha", "bravo", "charlie"}
	for i, name := range expectedOrder {
		if rules[i].Name != name {
			t.Errorf("rules[%d].Name = %q, want %q", i, rules[i].Name, name)
		}
	}

	// Verify content
	if !strings.Contains(rules[0].Content, "Alpha Rule") {
		t.Errorf("rules[0].Content = %q, want to contain 'Alpha Rule'", rules[0].Content)
	}
}

// T076: TestResolveReferences - @.syntor/rules/foo.md expands to file content
func TestResolveReferences(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .syntor/rules/ with a rule file
	rulesDir := filepath.Join(tmpDir, ".syntor", "rules")
	os.MkdirAll(rulesDir, 0755)
	os.WriteFile(filepath.Join(rulesDir, "security.md"), []byte("Always validate inputs."), 0644)

	input := "# Project\n\n@.syntor/rules/security.md\n\nMore content."
	result := resolveReferences(input, tmpDir)

	if !strings.Contains(result, "Always validate inputs.") {
		t.Errorf("resolveReferences() = %q, should contain expanded content", result)
	}
	if strings.Contains(result, "@.syntor/rules/security.md") {
		t.Errorf("resolveReferences() = %q, should not contain the @-reference after expansion", result)
	}
}

// T077: TestResolveReferences_Missing - missing ref file leaves @-reference as-is
func TestResolveReferences_Missing(t *testing.T) {
	tmpDir := t.TempDir()

	input := "# Project\n\n@.syntor/rules/nonexistent.md\n\nMore content."
	result := resolveReferences(input, tmpDir)

	if !strings.Contains(result, "@.syntor/rules/nonexistent.md") {
		t.Errorf("resolveReferences() = %q, should preserve missing @-reference as-is", result)
	}
}

// T078: TestMergeRules_ProjectOverrides - project rules override global by name
func TestMergeRules_ProjectOverrides(t *testing.T) {
	global := []RuleFile{
		{Name: "auth", Path: "/global/auth.md", Content: "global auth rule"},
		{Name: "logging", Path: "/global/logging.md", Content: "global logging rule"},
	}
	project := []RuleFile{
		{Name: "auth", Path: "/project/auth.md", Content: "project auth rule"},
		{Name: "deploy", Path: "/project/deploy.md", Content: "project deploy rule"},
	}

	merged := mergeRules(global, project)

	// Should have 3 rules: auth (project), deploy (project), logging (global)
	if len(merged) != 3 {
		t.Fatalf("mergeRules() returned %d rules, want 3", len(merged))
	}

	// Find auth rule - should be the project version
	var authRule *RuleFile
	for i := range merged {
		if merged[i].Name == "auth" {
			authRule = &merged[i]
			break
		}
	}
	if authRule == nil {
		t.Fatal("merged rules should contain 'auth'")
	}
	if authRule.Content != "project auth rule" {
		t.Errorf("auth rule content = %q, want 'project auth rule' (project should override global)", authRule.Content)
	}
	if authRule.Path != "/project/auth.md" {
		t.Errorf("auth rule path = %q, want '/project/auth.md'", authRule.Path)
	}

	// Verify logging (global, not overridden) is present
	found := false
	for _, r := range merged {
		if r.Name == "logging" && r.Content == "global logging rule" {
			found = true
			break
		}
	}
	if !found {
		t.Error("merged rules should contain global 'logging' rule")
	}

	// Verify sorted order
	for i := 1; i < len(merged); i++ {
		if merged[i].Name < merged[i-1].Name {
			t.Errorf("merged rules not sorted: %q comes after %q", merged[i].Name, merged[i-1].Name)
		}
	}
}

// T079: TestFormatProjectInstructions - formats with project-instructions and rules tags
func TestFormatProjectInstructions(t *testing.T) {
	pi := &ProjectInstructions{
		Content: "# My Project\nDescription here.",
		Path:    "/path/to/SYNTOR.md",
		Rules: []RuleFile{
			{Name: "security", Path: "/project/rules/security.md", Content: "Validate all inputs."},
		},
		GlobalRules: []RuleFile{
			{Name: "style", Path: "/global/rules/style.md", Content: "Use tabs."},
		},
	}

	formatted := FormatProjectInstructions(pi)

	// Should contain project-instructions tags
	if !strings.Contains(formatted, "<project-instructions>") {
		t.Error("formatted output should contain <project-instructions> tag")
	}
	if !strings.Contains(formatted, "</project-instructions>") {
		t.Error("formatted output should contain </project-instructions> tag")
	}

	// Should contain rules tags
	if !strings.Contains(formatted, "<rules>") {
		t.Error("formatted output should contain <rules> tag")
	}
	if !strings.Contains(formatted, "</rules>") {
		t.Error("formatted output should contain </rules> tag")
	}

	// Should contain content
	if !strings.Contains(formatted, "My Project") {
		t.Error("formatted output should contain project content")
	}
	if !strings.Contains(formatted, "Validate all inputs.") {
		t.Error("formatted output should contain rule content")
	}
	if !strings.Contains(formatted, "Use tabs.") {
		t.Error("formatted output should contain global rule content")
	}

	// Should contain source path
	if !strings.Contains(formatted, "/path/to/SYNTOR.md") {
		t.Error("formatted output should contain source path")
	}

	// Nil input should return empty string
	if result := FormatProjectInstructions(nil); result != "" {
		t.Errorf("FormatProjectInstructions(nil) = %q, want empty string", result)
	}

	// Empty content should not have project-instructions tags
	emptyPI := &ProjectInstructions{}
	emptyResult := FormatProjectInstructions(emptyPI)
	if strings.Contains(emptyResult, "<project-instructions>") {
		t.Error("empty ProjectInstructions should not produce <project-instructions> tag")
	}
}

// T080: TestLoadRulesDir_IgnoresNonMd - non-.md files are ignored
func TestLoadRulesDir_IgnoresNonMd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mix of .md and non-.md files
	os.WriteFile(filepath.Join(tmpDir, "valid.md"), []byte("# Valid Rule"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("Not a rule"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("key: value"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "script.sh"), []byte("#!/bin/bash"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "subdir.md"), 0755) // directory with .md name

	rules := LoadRulesDir(tmpDir)
	if len(rules) != 1 {
		t.Fatalf("LoadRulesDir() returned %d rules, want 1 (only .md files)", len(rules))
	}
	if rules[0].Name != "valid" {
		t.Errorf("rules[0].Name = %q, want 'valid'", rules[0].Name)
	}
}
