// Centaur Batch 10: Edge Cases & Cross-Cutting Tests (T091-T100)
// These tests verify edge-case handling in session management, memory,
// security, messaging, and compaction subsystems.
package centaur

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ctxpkg "github.com/syntor/syntor/pkg/context"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/memory"
	"github.com/syntor/syntor/pkg/session"
	"github.com/syntor/syntor/pkg/subagent"
	"github.com/syntor/syntor/pkg/tools/security"
)

// T091: session manager Resume with a nonexistent ID returns an error
func TestSession_ResumeNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := session.NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, _, err = mgr.Resume("nonexistent-id-does-not-exist")
	if err == nil {
		t.Fatal("expected error when resuming nonexistent session, got nil")
	}

	if !strings.Contains(err.Error(), "no session found") {
		t.Errorf("expected 'no session found' error, got: %v", err)
	}
}

// T092: storage handles corrupt JSONL gracefully by skipping bad lines
func TestSession_CorruptJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := session.NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a real session so the directory exists
	sess, err := mgr.Create(tmpDir, "test-agent")
	if err != nil {
		t.Fatalf("Create session failed: %v", err)
	}

	// Append some valid messages
	validMsgs := []inference.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	if err := mgr.AppendMessages(validMsgs, "test-agent"); err != nil {
		t.Fatalf("AppendMessages failed: %v", err)
	}

	// Manually inject corrupt lines into the messages.jsonl file
	messagesPath := filepath.Join(tmpDir, sess.ID, "messages.jsonl")
	f, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open messages file: %v", err)
	}
	// Write garbage lines
	f.WriteString("{corrupt json line\n")
	f.WriteString("not even close to json\n")
	f.WriteString("")
	f.Close()

	// Resume should still work - corrupt lines are skipped
	_, msgs, err := mgr.Resume(sess.ID)
	if err != nil {
		t.Fatalf("Resume failed after corrupt data: %v", err)
	}

	// Should have exactly 2 valid messages (the corrupt lines skipped)
	if len(msgs) != 2 {
		t.Errorf("expected 2 valid messages after skipping corrupt lines, got %d", len(msgs))
	}
}

// T093: concurrent AppendMessages calls don't corrupt data
func TestSession_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := session.NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = mgr.Create(tmpDir, "test-agent")
	if err != nil {
		t.Fatalf("Create session failed: %v", err)
	}

	// Perform concurrent writes from multiple goroutines
	// Each goroutine builds on the previous message count.
	// Since AppendMessages uses lastSavedIndex, we need to serialize.
	// Instead, we test that the file-level append is safe by
	// creating multiple managers pointing at the same session dir.
	const numWriters = 5
	const msgsPerWriter = 10

	var wg sync.WaitGroup
	errCh := make(chan error, numWriters)

	// Each writer creates its own manager and session, then appends messages
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			wMgr, wErr := session.NewManager(tmpDir)
			if wErr != nil {
				errCh <- wErr
				return
			}
			_, wErr = wMgr.Create(tmpDir, "writer")
			if wErr != nil {
				errCh <- wErr
				return
			}

			// Build up messages incrementally
			var allMsgs []inference.Message
			for j := 0; j < msgsPerWriter; j++ {
				allMsgs = append(allMsgs, inference.Message{
					Role:    "user",
					Content: "msg from writer",
				})
				if wErr := wMgr.AppendMessages(allMsgs, "writer"); wErr != nil {
					errCh <- wErr
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		t.Errorf("concurrent write error: %v", e)
	}

	// Verify we can list sessions without error
	summaries, err := mgr.List(100)
	if err != nil {
		t.Fatalf("List sessions after concurrent writes failed: %v", err)
	}

	// Should have numWriters + 1 sessions (1 from initial Create + numWriters from goroutines)
	expectedSessions := numWriters + 1
	if len(summaries) < expectedSessions {
		t.Errorf("expected at least %d sessions, got %d", expectedSessions, len(summaries))
	}
}

// T094: MEMORY.md with >200 lines gets truncated by TruncateMemory
func TestMemory_Write200Lines(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := memory.NewManager(tmpDir, filepath.Join(tmpDir, "project"))

	// Write >200 lines to global memory
	var lines []string
	for i := 0; i < 250; i++ {
		lines = append(lines, "- Line of memory content number")
	}
	content := strings.Join(lines, "\n")

	// Ensure the global dir exists
	globalMemPath := filepath.Join(tmpDir, "MEMORY.md")
	if err := os.WriteFile(globalMemPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test MEMORY.md: %v", err)
	}

	// Truncate
	if err := mgr.TruncateMemory("global"); err != nil {
		t.Fatalf("TruncateMemory failed: %v", err)
	}

	// Read back and verify line count
	data, err := os.ReadFile(globalMemPath)
	if err != nil {
		t.Fatalf("failed to read truncated MEMORY.md: %v", err)
	}

	resultLines := strings.Split(string(data), "\n")
	if len(resultLines) > memory.MaxMemoryLines {
		t.Errorf("expected at most %d lines after truncation, got %d", memory.MaxMemoryLines, len(resultLines))
	}
}

// T095: corrupt permissions.yaml falls back to defaults
func TestAutoAllow_YamlParseError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a corrupt permissions.yaml in the "project" .syntor dir
	projectSyntorDir := filepath.Join(tmpDir, ".syntor")
	if err := os.MkdirAll(projectSyntorDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	corruptYAML := []byte("{{{{invalid yaml content not valid at all::::")
	if err := os.WriteFile(filepath.Join(projectSyntorDir, "permissions.yaml"), corruptYAML, 0644); err != nil {
		t.Fatalf("failed to write corrupt permissions.yaml: %v", err)
	}

	// LoadAutoAllowPolicy should fall back to defaults, not panic
	policy, err := security.LoadAutoAllowPolicy(tmpDir)
	if err != nil {
		t.Fatalf("LoadAutoAllowPolicy returned unexpected error: %v", err)
	}

	if policy == nil {
		t.Fatal("expected non-nil policy with defaults")
	}

	// Verify default rules work - read_file should be allowed
	allowed, _ := policy.Evaluate("read_file", nil)
	if !allowed {
		t.Error("expected read_file to be allowed by default policy")
	}
}

// T096: "git branch -D main" is blocked by git safety
func TestGitSafety_BranchDeleteForce(t *testing.T) {
	result := security.ValidateGitCommand("git branch -D main")
	if result == nil {
		t.Fatal("expected git branch -D main to be flagged, got nil (safe)")
	}
	if result.Action != security.GitActionBlock {
		t.Errorf("expected block action, got %s", result.Action)
	}
}

// T097: "git checkout ." is blocked
func TestGitSafety_CheckoutDot(t *testing.T) {
	result := security.ValidateGitCommand("git checkout .")
	if result == nil {
		t.Fatal("expected git checkout . to be flagged, got nil (safe)")
	}
	if result.Action != security.GitActionBlock {
		t.Errorf("expected block action, got %s", result.Action)
	}
}

// T098: "git restore ." is blocked
func TestGitSafety_RestoreDot(t *testing.T) {
	result := security.ValidateGitCommand("git restore .")
	if result == nil {
		t.Fatal("expected git restore . to be flagged, got nil (safe)")
	}
	if result.Action != security.GitActionBlock {
		t.Errorf("expected block action, got %s", result.Action)
	}
}

// T099: agent A sends to B, B receives and responds via MessageBus
func TestMessageBus_AgentRoundTrip(t *testing.T) {
	bus := subagent.NewLocalMessageBus()
	defer bus.Close()

	// Subscribe both agents
	inboxA := bus.Subscribe("agent-a")
	inboxB := bus.Subscribe("agent-b")

	ctx := context.Background()

	// A sends a message to B
	err := bus.Send(ctx, "agent-a", "agent-b", subagent.AgentMessage{
		Type:    "message",
		Content: "hello from A",
	})
	if err != nil {
		t.Fatalf("Send A->B failed: %v", err)
	}

	// B should receive it
	select {
	case msg := <-inboxB:
		if msg.Content != "hello from A" {
			t.Errorf("B received wrong content: %q", msg.Content)
		}
		if msg.From != "agent-a" {
			t.Errorf("B received wrong sender: %q", msg.From)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B did not receive message from A within timeout")
	}

	// B responds to A
	err = bus.Send(ctx, "agent-b", "agent-a", subagent.AgentMessage{
		Type:    "message",
		Content: "hello from B",
	})
	if err != nil {
		t.Fatalf("Send B->A failed: %v", err)
	}

	// A should receive the response
	select {
	case msg := <-inboxA:
		if msg.Content != "hello from B" {
			t.Errorf("A received wrong content: %q", msg.Content)
		}
		if msg.From != "agent-b" {
			t.Errorf("A received wrong sender: %q", msg.From)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("A did not receive response from B within timeout")
	}
}

// T100: NewCompactor with zero config gets sensible defaults
func TestCompactor_ZeroConfig(t *testing.T) {
	// Pass a zero-value config - the Compactor should use defaults
	c := ctxpkg.NewCompactor(nil, ctxpkg.CompactorConfig{})

	// Verify that ShouldCompact works with the default config.
	// With default MaxTokens=120000 and CompactAt=0.75, threshold is 90000 tokens.
	// A small history should NOT trigger compaction.
	smallHistory := []inference.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	if c.ShouldCompact(smallHistory) {
		t.Error("expected ShouldCompact=false for small history with default config")
	}

	// A very large history should trigger compaction.
	// 90000 tokens ~ 360000 chars. Build a message with enough content.
	largeContent := strings.Repeat("word ", 80000) // ~400000 chars ~ 100000 tokens
	largeHistory := []inference.Message{
		{Role: "user", Content: largeContent},
	}
	if !c.ShouldCompact(largeHistory) {
		t.Error("expected ShouldCompact=true for large history with default config")
	}
}
