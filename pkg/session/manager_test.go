// Tests for session manager (Phases 0-10 UX parity).
// Batch 3: T021-T030
package session

import (
	"path/filepath"
	"testing"

	"github.com/syntor/syntor/pkg/inference"
)

// --- T021: Creates a session with short ID ---
func TestManager_Create(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	sess, err := mgr.Create("/tmp/work", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if sess.ID == "" {
		t.Error("Create: session ID is empty")
	}
	// UUID[:8] = 8 characters
	if len(sess.ID) != 8 {
		t.Errorf("Create: ID length got %d, want 8", len(sess.ID))
	}
	if sess.WorkingDir != "/tmp/work" {
		t.Errorf("WorkingDir: got %q, want %q", sess.WorkingDir, "/tmp/work")
	}
	if sess.AgentName != "test-agent" {
		t.Errorf("AgentName: got %q, want %q", sess.AgentName, "test-agent")
	}

	// Should be the current session
	if mgr.Current() == nil || mgr.Current().ID != sess.ID {
		t.Error("Current() should return the newly created session")
	}
}

// --- T022: Resumes by exact ID ---
func TestManager_Resume_ExactID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	created, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Append some messages
	history := []inference.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	if err := mgr.AppendMessages(history, "agent"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	// Create a new manager to simulate restart
	mgr2, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager2: %v", err)
	}

	sess, msgs, err := mgr2.Resume(created.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if sess.ID != created.ID {
		t.Errorf("Resume ID: got %q, want %q", sess.ID, created.ID)
	}
	if len(msgs) != 2 {
		t.Errorf("Resume messages: got %d, want 2", len(msgs))
	}
}

// --- T023: Resumes by ID prefix ---
func TestManager_Resume_Prefix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	created, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Resume by first 4 chars of ID
	prefix := created.ID[:4]
	mgr2, _ := NewManager(dir)
	sess, _, err := mgr2.Resume(prefix)
	if err != nil {
		t.Fatalf("Resume by prefix %q: %v", prefix, err)
	}

	if sess.ID != created.ID {
		t.Errorf("Resume by prefix: got %q, want %q", sess.ID, created.ID)
	}
}

// --- T024: Resumes by name prefix ---
func TestManager_Resume_NamePrefix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	created, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.SetName("my-debug-session"); err != nil {
		t.Fatalf("SetName: %v", err)
	}

	// Resume by name prefix
	mgr2, _ := NewManager(dir)
	sess, _, err := mgr2.Resume("my-debug")
	if err != nil {
		t.Fatalf("Resume by name prefix: %v", err)
	}

	if sess.ID != created.ID {
		t.Errorf("Resume by name: got %q, want %q", sess.ID, created.ID)
	}
}

// --- T025: Returns error for nonexistent ---
func TestManager_Resume_NotFound(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, _, err = mgr.Resume("nonexistent-id")
	if err == nil {
		t.Error("Resume nonexistent: expected error, got nil")
	}
}

// --- T026: Lists sessions up to limit ---
func TestManager_List(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		if _, err := mgr.Create("/tmp", "agent"); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	summaries, err := mgr.List(3)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(summaries) != 3 {
		t.Errorf("List(3): got %d, want 3", len(summaries))
	}

	// Without limit (0 defaults to 20)
	all, err := mgr.List(0)
	if err != nil {
		t.Fatalf("List(0): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("List(0): got %d, want 5", len(all))
	}
}

// --- T027: Forks session with messages ---
func TestManager_Fork(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	original, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Add messages to original
	history := []inference.Message{
		{Role: "user", Content: "message 1"},
		{Role: "assistant", Content: "reply 1"},
	}
	if err := mgr.AppendMessages(history, "agent"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	// Fork
	forked, err := mgr.Fork("forked-session")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	if forked.ID == original.ID {
		t.Error("Fork: forked session should have different ID")
	}
	if forked.Name != "forked-session" {
		t.Errorf("Fork Name: got %q, want %q", forked.Name, "forked-session")
	}
	if forked.Metadata["forked_from"] != original.ID {
		t.Errorf("Fork metadata: forked_from got %q, want %q",
			forked.Metadata["forked_from"], original.ID)
	}

	// Forked session should have the same messages
	msgs, err := mgr.store.LoadMessages(forked.ID)
	if err != nil {
		t.Fatalf("LoadMessages forked: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Forked messages: got %d, want 2", len(msgs))
	}

	// Current session should now be the forked one
	if mgr.Current().ID != forked.ID {
		t.Error("Current should be the forked session")
	}
}

// --- T028: Deletes session ---
func TestManager_Delete(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	sess, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if mgr.store.SessionExists(sess.ID) {
		t.Error("session should not exist after Delete")
	}
}

// --- T029: Sets session name ---
func TestManager_SetName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	sess, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.SetName("debug-session"); err != nil {
		t.Fatalf("SetName: %v", err)
	}

	if mgr.Current().Name != "debug-session" {
		t.Errorf("Name: got %q, want %q", mgr.Current().Name, "debug-session")
	}

	// Verify persistence by reloading
	mgr2, _ := NewManager(dir)
	loaded, _, err := mgr2.Resume(sess.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if loaded.Name != "debug-session" {
		t.Errorf("Persisted Name: got %q, want %q", loaded.Name, "debug-session")
	}
}

// --- T030: Incremental append works ---
func TestManager_AppendMessages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	sess, err := mgr.Create("/tmp", "agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// First append: 2 messages
	history1 := []inference.Message{
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "reply 1"},
	}
	if err := mgr.AppendMessages(history1, "agent"); err != nil {
		t.Fatalf("AppendMessages 1: %v", err)
	}

	// Second append: add 2 more messages (total history has 4)
	history2 := append(history1,
		inference.Message{Role: "user", Content: "msg 2"},
		inference.Message{Role: "assistant", Content: "reply 2"},
	)
	if err := mgr.AppendMessages(history2, "agent"); err != nil {
		t.Fatalf("AppendMessages 2: %v", err)
	}

	// Verify all 4 are stored
	mgr2, _ := NewManager(dir)
	_, msgs, err := mgr2.Resume(sess.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if len(msgs) != 4 {
		t.Fatalf("Total messages: got %d, want 4", len(msgs))
	}

	// Verify order
	expected := []string{"msg 1", "reply 1", "msg 2", "reply 2"}
	for i, want := range expected {
		if msgs[i].Content != want {
			t.Errorf("msg[%d]: got %q, want %q", i, msgs[i].Content, want)
		}
	}
}
