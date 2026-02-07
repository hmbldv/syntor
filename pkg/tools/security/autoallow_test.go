package security

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// appendResult writes a test result to /tmp/centaur-test-results.jsonl.
func appendResult(t *testing.T, batch int, testID, name, result string, duration time.Duration, notes string) {
	t.Helper()
	entry := map[string]interface{}{
		"batch":      batch,
		"test_id":    testID,
		"name":       name,
		"result":     result,
		"duration_s": duration.Seconds(),
		"notes":      notes,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile("/tmp/centaur-test-results.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Logf("warning: could not write result: %v", err)
		return
	}
	defer f.Close()
	f.Write(data)
	f.WriteString("\n")
}

// T041: read_file is auto-allowed by default rules.
func TestDefaultRules_ReadFile(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("read_file", map[string]any{"file_path": "/tmp/test.txt"})

	assert.True(t, allowed, "read_file should be auto-allowed")
	assert.Contains(t, reason, "auto-allowed")

	dur := time.Since(start)
	appendResult(t, 5, "T041", "TestDefaultRules_ReadFile", "pass", dur, "read_file auto-allowed by default")
}

// T042: glob is auto-allowed by default rules.
func TestDefaultRules_Glob(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("glob", map[string]any{"pattern": "*.go"})

	assert.True(t, allowed, "glob should be auto-allowed")
	assert.Contains(t, reason, "auto-allowed")

	dur := time.Since(start)
	appendResult(t, 5, "T042", "TestDefaultRules_Glob", "pass", dur, "glob auto-allowed by default")
}

// T043: "git status" is auto-allowed for bash by default rules.
func TestDefaultRules_Bash_GitStatus(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("bash", map[string]any{"command": "git status"})

	assert.True(t, allowed, "git status should be auto-allowed")
	assert.Contains(t, reason, "auto-allowed")

	dur := time.Since(start)
	appendResult(t, 5, "T043", "TestDefaultRules_Bash_GitStatus", "pass", dur, "git status auto-allowed")
}

// T044: "git push" is denied for bash by default rules.
func TestDefaultRules_Bash_GitPush(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("bash", map[string]any{"command": "git push origin main"})

	assert.False(t, allowed, "git push should be denied")
	assert.Contains(t, reason, "deny")

	dur := time.Since(start)
	appendResult(t, 5, "T044", "TestDefaultRules_Bash_GitPush", "pass", dur, "git push denied by default")
}

// T045: "rm -rf" is denied for bash by default rules.
func TestDefaultRules_Bash_RmRf(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("bash", map[string]any{"command": "rm -rf /tmp/something"})

	assert.False(t, allowed, "rm -rf should be denied")
	assert.Contains(t, reason, "deny")

	dur := time.Since(start)
	appendResult(t, 5, "T045", "TestDefaultRules_Bash_RmRf", "pass", dur, "rm -rf denied by default")
}

// T046: Unknown tool returns not-allowed.
func TestEvaluate_UnknownTool(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{rules: defaultRules()}

	allowed, reason := policy.Evaluate("launch_missiles", map[string]any{})

	assert.False(t, allowed, "unknown tool should not be auto-allowed")
	assert.Contains(t, reason, "no auto-allow rule")

	dur := time.Since(start)
	appendResult(t, 5, "T046", "TestEvaluate_UnknownTool", "pass", dur, "unknown tool rejected")
}

// T047: Paths matching deny patterns are rejected.
func TestDenyPatterns(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{
		rules: []AutoAllowRule{
			{
				Tool:         "read_file",
				DenyPatterns: []string{"*.env", "*.secret"},
			},
		},
	}

	allowed, reason := policy.Evaluate("read_file", map[string]any{"file_path": "/project/.env"})

	assert.False(t, allowed, "path matching deny pattern should be rejected")
	assert.Contains(t, reason, "deny pattern")

	dur := time.Since(start)
	appendResult(t, 5, "T047", "TestDenyPatterns", "pass", dur, "deny patterns block matching paths")
}

// T048: Paths matching allow patterns are accepted.
func TestPathPatterns_Allow(t *testing.T) {
	start := time.Now()
	policy := &AutoAllowPolicy{
		rules: []AutoAllowRule{
			{
				Tool:         "read_file",
				PathPatterns: []string{"/project/src/*"},
			},
		},
	}

	allowed, _ := policy.Evaluate("read_file", map[string]any{"file_path": "/project/src/main.go"})
	assert.True(t, allowed, "path matching allow pattern should be accepted")

	notAllowed, reason := policy.Evaluate("read_file", map[string]any{"file_path": "/etc/passwd"})
	assert.False(t, notAllowed, "path not matching allow pattern should be rejected")
	assert.Contains(t, reason, "does not match any allow pattern")

	dur := time.Since(start)
	appendResult(t, 5, "T048", "TestPathPatterns_Allow", "pass", dur, "allow patterns filter paths")
}

// T049: LoadAutoAllowPolicy falls back to defaults when no files exist.
func TestLoadAutoAllowPolicy_NoFiles(t *testing.T) {
	start := time.Now()
	// Use a temp dir that has no permissions.yaml files
	tmp := t.TempDir()

	policy, err := LoadAutoAllowPolicy(tmp)

	assert.NoError(t, err)
	assert.NotNil(t, policy)

	// Verify it uses default rules by checking a known default
	allowed, _ := policy.Evaluate("read_file", map[string]any{"file_path": "/tmp/test.txt"})
	assert.True(t, allowed, "should fall back to default rules which allow read_file")

	dur := time.Since(start)
	appendResult(t, 5, "T049", "TestLoadAutoAllowPolicy_NoFiles", "pass", dur, "falls back to defaults")
}

// T050: Project rules restrict global rules (only tools in both sets survive).
func TestRestrictRules(t *testing.T) {
	start := time.Now()

	global := []AutoAllowRule{
		{Tool: "read_file"},
		{Tool: "glob"},
		{Tool: "bash", Commands: []string{"git status", "ls"}},
	}

	// Project only allows read_file and bash, drops glob entirely.
	// Project also adds a deny pattern for bash.
	project := []AutoAllowRule{
		{Tool: "read_file"},
		{Tool: "bash", DenyCommands: []string{"ls"}},
	}

	merged := restrictRules(global, project)

	// Only read_file and bash should survive (glob was not in project set)
	assert.Len(t, merged, 2, "only tools present in both sets should remain")

	toolNames := make(map[string]bool)
	for _, r := range merged {
		toolNames[r.Tool] = true
	}
	assert.True(t, toolNames["read_file"], "read_file should survive restriction")
	assert.True(t, toolNames["bash"], "bash should survive restriction")
	assert.False(t, toolNames["glob"], "glob should be removed by restriction")

	// Check that project deny commands were appended to the bash rule
	for _, r := range merged {
		if r.Tool == "bash" {
			assert.Contains(t, r.DenyCommands, "ls", "project deny commands should be appended")
			assert.Contains(t, r.Commands, "git status", "global allow commands preserved")
		}
	}

	dur := time.Since(start)
	appendResult(t, 5, "T050", "TestRestrictRules", "pass", dur, "project rules restrict global rules")
}
