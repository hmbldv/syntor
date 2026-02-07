package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// T051: IsGitCommand returns true for "git push".
func TestIsGitCommand_True(t *testing.T) {
	start := time.Now()

	assert.True(t, IsGitCommand("git push"), "git push should be recognized as a git command")
	assert.True(t, IsGitCommand("git status"), "git status should be recognized as a git command")
	assert.True(t, IsGitCommand("  git diff"), "leading whitespace should not matter")

	dur := time.Since(start)
	appendResult(t, 6, "T051", "TestIsGitCommand_True", "pass", dur, "git commands detected")
}

// T052: IsGitCommand returns false for "ls -la".
func TestIsGitCommand_False(t *testing.T) {
	start := time.Now()

	assert.False(t, IsGitCommand("ls -la"), "ls should not be a git command")
	assert.False(t, IsGitCommand("echo git"), "echo git should not be a git command")
	assert.False(t, IsGitCommand("gitignore"), "gitignore should not be a git command")

	dur := time.Since(start)
	appendResult(t, 6, "T052", "TestIsGitCommand_False", "pass", dur, "non-git commands rejected")
}

// T053: ValidateGitCommand returns nil for safe commands.
func TestValidateGitCommand_Safe(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git commit -m test")
	assert.Nil(t, result, "git commit should be safe")

	result = ValidateGitCommand("git add .")
	assert.Nil(t, result, "git add should be safe")

	result = ValidateGitCommand("git log --oneline")
	assert.Nil(t, result, "git log should be safe")

	dur := time.Since(start)
	appendResult(t, 6, "T053", "TestValidateGitCommand_Safe", "pass", dur, "safe commands return nil")
}

// T054: ValidateGitCommand blocks "git push --force".
func TestValidateGitCommand_ForcePush(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git push --force")

	assert.NotNil(t, result, "git push --force should be caught")
	assert.Equal(t, GitActionBlock, result.Action)

	dur := time.Since(start)
	appendResult(t, 6, "T054", "TestValidateGitCommand_ForcePush", "pass", dur, "force push blocked")
}

// T055: ValidateGitCommand blocks "git push -f".
func TestValidateGitCommand_ForcePushF(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git push -f")

	assert.NotNil(t, result, "git push -f should be caught")
	assert.Equal(t, GitActionBlock, result.Action)

	dur := time.Since(start)
	appendResult(t, 6, "T055", "TestValidateGitCommand_ForcePushF", "pass", dur, "short force push blocked")
}

// T056: ValidateGitCommand blocks "git reset --hard".
func TestValidateGitCommand_ResetHard(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git reset --hard")

	assert.NotNil(t, result, "git reset --hard should be caught")
	assert.Equal(t, GitActionBlock, result.Action)

	dur := time.Since(start)
	appendResult(t, 6, "T056", "TestValidateGitCommand_ResetHard", "pass", dur, "reset hard blocked")
}

// T057: ValidateGitCommand blocks "git clean -fd".
func TestValidateGitCommand_CleanF(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git clean -fd")

	assert.NotNil(t, result, "git clean -fd should be caught")
	assert.Equal(t, GitActionBlock, result.Action)

	dur := time.Since(start)
	appendResult(t, 6, "T057", "TestValidateGitCommand_CleanF", "pass", dur, "clean -fd blocked")
}

// T058: ValidateGitCommand returns Confirm for "git push origin main".
func TestValidateGitCommand_PushMain(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git push origin main")

	assert.NotNil(t, result, "git push origin main should require confirmation")
	assert.Equal(t, GitActionConfirm, result.Action)
	assert.Contains(t, result.Reason, "main")

	dur := time.Since(start)
	appendResult(t, 6, "T058", "TestValidateGitCommand_PushMain", "pass", dur, "push main requires confirm")
}

// T059: ValidateGitCommand returns Confirm for "git rebase main".
func TestValidateGitCommand_Rebase(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git rebase main")

	assert.NotNil(t, result, "git rebase should require confirmation")
	assert.Equal(t, GitActionConfirm, result.Action)
	assert.Contains(t, result.Reason, "Rebase", "reason should mention rebase")

	dur := time.Since(start)
	appendResult(t, 6, "T059", "TestValidateGitCommand_Rebase", "pass", dur, "rebase requires confirm")
}

// T060: ValidateGitCommand blocks "git push --force-with-lease".
func TestValidateGitCommand_ForceWithLease(t *testing.T) {
	start := time.Now()

	result := ValidateGitCommand("git push --force-with-lease")

	assert.NotNil(t, result, "git push --force-with-lease should be caught")
	assert.Equal(t, GitActionBlock, result.Action)

	dur := time.Since(start)
	appendResult(t, 6, "T060", "TestValidateGitCommand_ForceWithLease", "pass", dur, "force-with-lease blocked")
}
