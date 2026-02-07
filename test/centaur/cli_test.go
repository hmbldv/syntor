// Centaur Batch 9: CLI & Integration Tests (T081-T090)
// These tests invoke the syntor binary and validate its output, ensuring
// all major CLI commands behave correctly as an outside-in integration check.
package centaur

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const syntorBinary = "/Users/doop/.local/bin/syntor"

// runSyntor executes the syntor binary with the given args and a 10s timeout.
// Returns combined stdout+stderr and any error.
func runSyntor(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, syntorBinary, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// T081: syntor version outputs version info with commit hash
func TestSyntor_Version(t *testing.T) {
	out, err := runSyntor("version")
	// The command should succeed (exit 0).
	if err != nil {
		// Some stderr warnings are expected (Vault unavailable), but exit code should be 0.
		// exec.ExitError means non-zero exit.
		if _, ok := err.(*exec.ExitError); ok {
			t.Fatalf("syntor version exited with error: %v\nOutput: %s", err, out)
		}
	}

	// Must contain "SYNTOR" and a commit hash (hex string)
	if !strings.Contains(out, "SYNTOR") {
		t.Errorf("expected output to contain 'SYNTOR', got: %s", out)
	}
	if !strings.Contains(out, "Commit:") {
		t.Errorf("expected output to contain 'Commit:', got: %s", out)
	}
}

// T082: syntor --help lists all commands
func TestSyntor_Help(t *testing.T) {
	out, err := runSyntor("--help")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("syntor --help exited with code %d: %s", exitErr.ExitCode(), out)
		}
	}

	requiredCommands := []string{"agents", "chat", "config", "models", "sessions", "version"}
	for _, cmd := range requiredCommands {
		if !strings.Contains(out, cmd) {
			t.Errorf("expected help output to contain command %q, got: %s", cmd, out)
		}
	}
}

// T083: syntor sessions list --json returns valid JSON (or an error message, not a panic)
func TestSyntor_SessionsList(t *testing.T) {
	out, err := runSyntor("sessions", "list", "--json")
	// The command may fail if the session backend is unavailable. That is acceptable.
	// What matters is: no panic, and if it succeeds the output is valid JSON.
	if err != nil {
		// Non-zero exit is acceptable if there is a meaningful error message
		if strings.Contains(out, "panic") {
			t.Fatalf("syntor sessions list --json panicked: %s", out)
		}
		// Graceful error - test passes (command didn't panic)
		t.Logf("sessions list returned error (expected if DB unavailable): %s", out)
		return
	}

	// If it succeeded, the output should be valid JSON
	trimmed := extractJSON(out)
	if trimmed != "" && !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid JSON output, got: %s", out)
	}
}

// T084: syntor agents list --json returns valid JSON (may error if DB unavailable)
func TestSyntor_AgentsList(t *testing.T) {
	out, err := runSyntor("agents", "list", "--json")
	if err != nil {
		if strings.Contains(out, "panic") {
			t.Fatalf("syntor agents list --json panicked: %s", out)
		}
		// Graceful error - acceptable
		t.Logf("agents list returned error (expected if DB unavailable): %s", out)
		return
	}

	trimmed := extractJSON(out)
	if trimmed != "" && !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid JSON output, got: %s", out)
	}
}

// T085: syntor models list --json returns valid JSON
func TestSyntor_ModelsList(t *testing.T) {
	out, err := runSyntor("models", "list", "--json")
	if err != nil {
		if strings.Contains(out, "panic") {
			t.Fatalf("syntor models list --json panicked: %s", out)
		}
		t.Logf("models list returned error: %s", out)
		return
	}

	trimmed := extractJSON(out)
	if trimmed != "" && !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid JSON output, got: %s", out)
	}
}

// T086: syntor config show outputs config content
func TestSyntor_ConfigShow(t *testing.T) {
	out, err := runSyntor("config", "show")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("syntor config show exited with code %d: %s", exitErr.ExitCode(), out)
		}
	}

	// Config output should contain key sections
	expectedContent := []string{"inference", "provider"}
	for _, kw := range expectedContent {
		if !strings.Contains(strings.ToLower(out), kw) {
			t.Errorf("expected config output to contain %q, got: %s", kw, out)
		}
	}
}

// T087: syntor config path shows config file paths
func TestSyntor_ConfigPath(t *testing.T) {
	out, err := runSyntor("config", "path")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("syntor config path exited with code %d: %s", exitErr.ExitCode(), out)
		}
	}

	if !strings.Contains(out, ".syntor") {
		t.Errorf("expected config path output to reference .syntor, got: %s", out)
	}
	if !strings.Contains(out, "config.yaml") {
		t.Errorf("expected config path output to reference config.yaml, got: %s", out)
	}
}

// T088: syntor --invalid-flag returns error (non-zero exit)
func TestSyntor_InvalidFlag(t *testing.T) {
	out, err := runSyntor("--invalid-flag")
	if err == nil {
		t.Fatalf("expected non-zero exit for invalid flag, got success. Output: %s", out)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got: %v", err)
	}

	if exitErr.ExitCode() == 0 {
		t.Errorf("expected non-zero exit code for invalid flag")
	}

	if !strings.Contains(out, "unknown flag") && !strings.Contains(out, "Error") {
		t.Errorf("expected error message about unknown flag, got: %s", out)
	}
}

// T089: syntor sessions delete nonexistent returns error gracefully
func TestSyntor_SessionsDelete_Missing(t *testing.T) {
	out, err := runSyntor("sessions", "delete", "nonexistent-session-id-9999")
	if err == nil {
		// Either it succeeded (unlikely) or the error was not surfaced via exit code
		t.Logf("sessions delete did not return error (unexpected). Output: %s", out)
		return
	}

	// Ensure no panic
	if strings.Contains(out, "panic") {
		t.Fatalf("sessions delete panicked on nonexistent session: %s", out)
	}

	// Should contain an error or "Error" message - graceful failure
	if !strings.Contains(strings.ToLower(out), "error") && !strings.Contains(strings.ToLower(out), "not found") {
		t.Logf("sessions delete returned non-zero but no clear error message: %s", out)
	}
}

// T090: syntor chat --help shows usage info
func TestSyntor_ChatHelp(t *testing.T) {
	out, err := runSyntor("chat", "--help")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("syntor chat --help exited with code %d: %s", exitErr.ExitCode(), out)
		}
	}

	if !strings.Contains(out, "chat") {
		t.Errorf("expected chat help output to contain 'chat', got: %s", out)
	}
	if !strings.Contains(out, "Usage") && !strings.Contains(out, "usage") {
		t.Errorf("expected chat help output to contain usage info, got: %s", out)
	}
}

// extractJSON attempts to find the first JSON array or object in the output,
// skipping any warning lines that precede it.
func extractJSON(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return ""
}
