package security

import (
	"regexp"
	"strings"
)

// GitAction represents the action to take for a git command.
type GitAction string

const (
	// GitActionBlock prevents the git command from executing.
	GitActionBlock GitAction = "block"

	// GitActionConfirm requires user confirmation before executing.
	GitActionConfirm GitAction = "confirm"
)

// GitValidationResult is the outcome of validating a git command.
type GitValidationResult struct {
	Action  GitAction
	Reason  string
	Message string
}

// Destructive git patterns that should always be blocked.
var destructiveGitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`git\s+push\s+.*--force\b`),
	regexp.MustCompile(`git\s+push\s+.*-f\b`),
	regexp.MustCompile(`git\s+push\s+--force\b`),
	regexp.MustCompile(`git\s+push\s+-f\b`),
	regexp.MustCompile(`git\s+push\s+.*--force-with-lease\b`),
	regexp.MustCompile(`git\s+push\s+--force-with-lease\b`),
	regexp.MustCompile(`git\s+reset\s+--hard\b`),
	regexp.MustCompile(`git\s+clean\s+.*-f`),
	regexp.MustCompile(`git\s+branch\s+.*-D\b`),
	regexp.MustCompile(`git\s+checkout\s+\.\s*$`),
	regexp.MustCompile(`git\s+restore\s+\.\s*$`),
}

// Risky git patterns that require user confirmation.
var riskyGitPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{
		pattern: regexp.MustCompile(`git\s+push\s+.*\b(main|master)\b`),
		reason:  "Pushing directly to main/master branch",
	},
	{
		pattern: regexp.MustCompile(`git\s+push\s+(origin\s+)?(main|master)\b`),
		reason:  "Pushing directly to main/master branch",
	},
	{
		pattern: regexp.MustCompile(`git\s+rebase\b`),
		reason:  "Rebase can rewrite commit history",
	},
}

// IsGitCommand returns true if the command is a git command.
func IsGitCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	return strings.HasPrefix(trimmed, "git ") || trimmed == "git"
}

// ValidateGitCommand checks a git command for safety.
// Returns nil if the command is safe, or a GitValidationResult with the action to take.
func ValidateGitCommand(cmd string) *GitValidationResult {
	cmd = strings.TrimSpace(cmd)

	// Check destructive patterns (block)
	for _, pattern := range destructiveGitPatterns {
		if pattern.MatchString(cmd) {
			return &GitValidationResult{
				Action:  GitActionBlock,
				Reason:  "Destructive git operation",
				Message: "Blocked destructive git command: " + truncateCmd(cmd, 100),
			}
		}
	}

	// Check risky patterns (confirm)
	for _, risky := range riskyGitPatterns {
		if risky.pattern.MatchString(cmd) {
			return &GitValidationResult{
				Action:  GitActionConfirm,
				Reason:  risky.reason,
				Message: "Requires confirmation: " + truncateCmd(cmd, 100),
			}
		}
	}

	return nil
}

func truncateCmd(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
