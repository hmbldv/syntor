package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// RuleFile represents a loaded rules file.
type RuleFile struct {
	Name    string // Filename without extension
	Path    string // Full path to the file
	Content string // File content
}

// ProjectInstructions holds loaded project instructions and rules.
type ProjectInstructions struct {
	// Content from SYNTOR.md (with @-references resolved)
	Content string
	// Path where SYNTOR.md was found
	Path string
	// Rules loaded from .syntor/rules/ directories
	Rules []RuleFile
	// Global rules from ~/.syntor/rules/
	GlobalRules []RuleFile
}

// refPattern matches @.syntor/rules/foo.md references in SYNTOR.md
var refPattern = regexp.MustCompile(`@(\.syntor/rules/[^\s]+\.md)`)

// FindProjectInstructions searches for SYNTOR.md starting from startDir,
// walking up to 6 parent directories. Returns the content, resolved path,
// and any error.
func FindProjectInstructions(startDir string) (*ProjectInstructions, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	instructions := &ProjectInstructions{}

	// Find SYNTOR.md
	content, mdPath, err := findSyntorMD(startDir)
	if err != nil {
		// No SYNTOR.md found, still load rules if available
		content = ""
		mdPath = ""
	}

	// Determine project root (where SYNTOR.md lives, or startDir)
	projectRoot := startDir
	if mdPath != "" {
		projectRoot = filepath.Dir(mdPath)
	}

	// Load project rules from .syntor/rules/
	projectRulesDir := filepath.Join(projectRoot, ".syntor", "rules")
	projectRules := LoadRulesDir(projectRulesDir)

	// Load global rules from ~/.syntor/rules/
	home, _ := os.UserHomeDir()
	globalRulesDir := filepath.Join(home, ".syntor", "rules")
	globalRules := LoadRulesDir(globalRulesDir)

	// Resolve @-references in SYNTOR.md content
	if content != "" {
		content = resolveReferences(content, projectRoot)
	}

	instructions.Content = content
	instructions.Path = mdPath
	instructions.Rules = projectRules
	instructions.GlobalRules = globalRules

	return instructions, nil
}

// findSyntorMD searches up from startDir for SYNTOR.md.
func findSyntorMD(startDir string) (string, string, error) {
	dir := startDir
	for i := 0; i < 6; i++ {
		mdPath := filepath.Join(dir, "SYNTOR.md")
		data, err := os.ReadFile(mdPath)
		if err == nil {
			return string(data), mdPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", os.ErrNotExist
}

// LoadRulesDir reads all .md files from a rules directory, sorted by name.
func LoadRulesDir(dir string) []RuleFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var rules []RuleFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		rules = append(rules, RuleFile{
			Name:    name,
			Path:    path,
			Content: string(data),
		})
	}

	// Sort by name for deterministic ordering
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Name < rules[j].Name
	})

	return rules
}

// resolveReferences replaces @.syntor/rules/foo.md references with file content.
func resolveReferences(content string, projectRoot string) string {
	return refPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the path after @
		relPath := strings.TrimPrefix(match, "@")
		fullPath := filepath.Join(projectRoot, relPath)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			// Leave the reference as-is if file not found
			return match
		}

		return string(data)
	})
}

// FormatProjectInstructions formats instructions and rules for injection
// into the system prompt.
func FormatProjectInstructions(pi *ProjectInstructions) string {
	if pi == nil {
		return ""
	}

	var sb strings.Builder

	// Project instructions from SYNTOR.md
	if pi.Content != "" {
		sb.WriteString("<project-instructions>\n")
		if pi.Path != "" {
			sb.WriteString(fmt.Sprintf("# Project Instructions (from %s)\n\n", pi.Path))
		}
		sb.WriteString(pi.Content)
		sb.WriteString("\n</project-instructions>\n\n")
	}

	// Combine global rules (lower priority) with project rules (higher priority)
	// Project rules override global rules with the same name
	allRules := mergeRules(pi.GlobalRules, pi.Rules)

	if len(allRules) > 0 {
		sb.WriteString("<rules>\n")
		for _, rule := range allRules {
			sb.WriteString(fmt.Sprintf("## Rule: %s\n", rule.Name))
			sb.WriteString(fmt.Sprintf("Source: %s\n\n", rule.Path))
			sb.WriteString(rule.Content)
			sb.WriteString("\n\n")
		}
		sb.WriteString("</rules>\n")
	}

	return sb.String()
}

// mergeRules combines global and project rules. Project rules with the same
// name override global rules.
func mergeRules(global, project []RuleFile) []RuleFile {
	seen := make(map[string]bool)
	var merged []RuleFile

	// Add project rules first (they take priority)
	for _, r := range project {
		seen[r.Name] = true
		merged = append(merged, r)
	}

	// Add global rules that aren't overridden by project rules
	for _, r := range global {
		if !seen[r.Name] {
			merged = append(merged, r)
		}
	}

	// Sort for consistent ordering
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Name < merged[j].Name
	})

	return merged
}

// RulesWatcher monitors .syntor/rules/ directories for changes and
// triggers callbacks to reload rules into the prompt builder.
type RulesWatcher struct {
	watcher   *fsnotify.Watcher
	dirs      []string
	callbacks []func()
	mu        sync.RWMutex
	cancel    context.CancelFunc
}

// NewRulesWatcher creates a watcher for rules directories.
// Watches both ~/.syntor/rules/ (global) and .syntor/rules/ (project).
func NewRulesWatcher(projectRoot string) (*RulesWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".syntor", "rules"),
		filepath.Join(projectRoot, ".syntor", "rules"),
	}

	rw := &RulesWatcher{
		watcher: watcher,
		dirs:    dirs,
	}

	// Add existing directories to watch
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			_ = watcher.Add(dir)
		}
	}

	return rw, nil
}

// OnChange registers a callback that fires when rules change.
func (rw *RulesWatcher) OnChange(cb func()) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.callbacks = append(rw.callbacks, cb)
}

// Start begins watching for file changes in a goroutine.
func (rw *RulesWatcher) Start(ctx context.Context) {
	ctx, rw.cancel = context.WithCancel(ctx)
	go rw.watchLoop(ctx)
}

// Stop stops the watcher.
func (rw *RulesWatcher) Stop() {
	if rw.cancel != nil {
		rw.cancel()
	}
	if rw.watcher != nil {
		rw.watcher.Close()
	}
}

// watchLoop handles fsnotify events.
func (rw *RulesWatcher) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-rw.watcher.Events:
			if !ok {
				return
			}
			// Only react to .md file changes
			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				rw.notifyCallbacks()
			}
		case _, ok := <-rw.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
		}
	}
}

// notifyCallbacks calls all registered callbacks.
func (rw *RulesWatcher) notifyCallbacks() {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	for _, cb := range rw.callbacks {
		go cb()
	}
}
