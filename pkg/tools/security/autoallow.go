package security

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AutoAllowRule defines a pattern for auto-allowing tool execution.
type AutoAllowRule struct {
	Tool         string   `yaml:"tool"`           // Tool name or glob pattern
	PathPatterns []string `yaml:"path_patterns"`  // Allowed path globs
	DenyPatterns []string `yaml:"deny_patterns"`  // Denied path globs (override allow)
	Commands     []string `yaml:"commands"`       // For bash: allowed command prefixes
	DenyCommands []string `yaml:"deny_commands"`  // For bash: denied command prefixes
}

// autoAllowConfig is the on-disk YAML structure.
type autoAllowConfig struct {
	Rules []AutoAllowRule `yaml:"rules"`
}

// AutoAllowPolicy evaluates whether a tool call should be auto-approved.
type AutoAllowPolicy struct {
	rules []AutoAllowRule
}

// defaultRules returns the built-in rules used when no config file exists.
func defaultRules() []AutoAllowRule {
	return []AutoAllowRule{
		{
			Tool: "read_file",
		},
		{
			Tool: "glob",
		},
		{
			Tool: "grep",
		},
		{
			Tool: "list_directory",
		},
		{
			Tool:         "bash",
			Commands:     []string{"git status", "git diff", "git log", "ls", "pwd", "cat"},
			DenyCommands: []string{"git push", "git reset", "rm -rf", "sudo"},
		},
	}
}

// LoadAutoAllowPolicy loads rules from project and global permissions files.
// Project rules can only RESTRICT global rules, not expand them.
func LoadAutoAllowPolicy(projectDir string) (*AutoAllowPolicy, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &AutoAllowPolicy{rules: defaultRules()}, nil
	}

	globalPath := filepath.Join(home, ".syntor", "permissions.yaml")
	projectPath := filepath.Join(projectDir, ".syntor", "permissions.yaml")

	globalRules, globalErr := loadRulesFile(globalPath)
	projectRules, projectErr := loadRulesFile(projectPath)

	// Neither file exists — use defaults
	if globalErr != nil && projectErr != nil {
		return &AutoAllowPolicy{rules: defaultRules()}, nil
	}

	// Only global exists
	if projectErr != nil {
		return &AutoAllowPolicy{rules: globalRules}, nil
	}

	// Only project exists — project can only restrict defaults
	if globalErr != nil {
		merged := restrictRules(defaultRules(), projectRules)
		return &AutoAllowPolicy{rules: merged}, nil
	}

	// Both exist — project restricts global
	merged := restrictRules(globalRules, projectRules)
	return &AutoAllowPolicy{rules: merged}, nil
}

// loadRulesFile reads and parses a permissions YAML file.
func loadRulesFile(path string) ([]AutoAllowRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg autoAllowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Rules, nil
}

// restrictRules returns only the global rules that also appear in the project
// set (matched by tool name). For matching rules, deny patterns from the
// project are appended so the project can further tighten access.
func restrictRules(global, project []AutoAllowRule) []AutoAllowRule {
	projectByTool := make(map[string]AutoAllowRule)
	for _, r := range project {
		projectByTool[r.Tool] = r
	}

	var merged []AutoAllowRule
	for _, g := range global {
		pr, ok := projectByTool[g.Tool]
		if !ok {
			// Project doesn't list this tool — not auto-allowed at project level
			continue
		}
		// Append project deny patterns to global rule
		r := g
		r.DenyPatterns = append(r.DenyPatterns, pr.DenyPatterns...)
		r.DenyCommands = append(r.DenyCommands, pr.DenyCommands...)
		merged = append(merged, r)
	}
	return merged
}

// Evaluate checks whether a tool call matches auto-allow rules.
// Returns allowed=true with a reason if the call is auto-approved,
// or allowed=false with a reason if it is not.
func (p *AutoAllowPolicy) Evaluate(toolName string, params map[string]any) (allowed bool, reason string) {
	rule, found := p.findRule(toolName)
	if !found {
		return false, "no auto-allow rule for tool: " + toolName
	}

	// Check path-based parameters
	pathParam := extractPathParam(toolName, params)
	if pathParam != "" {
		if matchesAny(pathParam, rule.DenyPatterns) {
			return false, "path matches deny pattern: " + pathParam
		}
		if len(rule.PathPatterns) > 0 && !matchesAny(pathParam, rule.PathPatterns) {
			return false, "path does not match any allow pattern: " + pathParam
		}
	}

	// Check command-based parameters (bash tool)
	if toolName == "bash" {
		cmd, _ := params["command"].(string)
		if cmd == "" {
			return false, "empty bash command"
		}

		// Deny commands always take priority
		if matchesCommandPrefix(cmd, rule.DenyCommands) {
			return false, "command matches deny prefix: " + cmd
		}

		// If allow commands are specified, command must match one
		if len(rule.Commands) > 0 && !matchesCommandPrefix(cmd, rule.Commands) {
			return false, "command does not match any allowed prefix: " + cmd
		}
	}

	return true, "auto-allowed by rule for tool: " + toolName
}

// findRule locates the first matching rule for a tool name, supporting globs.
func (p *AutoAllowPolicy) findRule(toolName string) (AutoAllowRule, bool) {
	for _, r := range p.rules {
		if r.Tool == toolName {
			return r, true
		}
		if matched, _ := filepath.Match(r.Tool, toolName); matched {
			return r, true
		}
	}
	return AutoAllowRule{}, false
}

// extractPathParam pulls the relevant path parameter for a given tool.
func extractPathParam(toolName string, params map[string]any) string {
	switch toolName {
	case "read_file", "write_file", "edit_file":
		if p, ok := params["file_path"].(string); ok {
			return p
		}
	case "glob", "grep", "list_directory":
		if p, ok := params["path"].(string); ok {
			return p
		}
	}
	return ""
}

// matchesAny checks if the value matches any of the glob patterns.
func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, value); matched {
			return true
		}
		// Also check if pattern matches the base name
		if matched, _ := filepath.Match(pattern, filepath.Base(value)); matched {
			return true
		}
	}
	return false
}

// matchesCommandPrefix checks if cmd starts with any of the given prefixes.
func matchesCommandPrefix(cmd string, prefixes []string) bool {
	trimmed := strings.TrimSpace(cmd)
	for _, prefix := range prefixes {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" ") || strings.HasPrefix(trimmed, prefix+"\t") {
			return true
		}
	}
	return false
}
