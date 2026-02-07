// Tests for session persistence (Phases 0-10 UX parity).
// Batch 2: T011-T020
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- T011: Creating FileStore makes the sessions directory ---
func TestNewFileStore_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	// dir does not exist yet

	_, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

// --- T012: Save then load roundtrip ---
func TestSaveAndLoadSession(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	original := &Session{
		ID:         "test1234",
		Name:       "my session",
		CreatedAt:  time.Now().Truncate(time.Second),
		UpdatedAt:  time.Now().Truncate(time.Second),
		WorkingDir: "/tmp/work",
		AgentName:  "syntor",
		TokensUsed: 42,
		Metadata:   map[string]string{"key": "value"},
	}

	if err := store.SaveSession(original); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := store.LoadSession("test1234")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if loaded.ID != original.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", loaded.Name, original.Name)
	}
	if loaded.WorkingDir != original.WorkingDir {
		t.Errorf("WorkingDir: got %q, want %q", loaded.WorkingDir, original.WorkingDir)
	}
	if loaded.AgentName != original.AgentName {
		t.Errorf("AgentName: got %q, want %q", loaded.AgentName, original.AgentName)
	}
	if loaded.TokensUsed != original.TokensUsed {
		t.Errorf("TokensUsed: got %d, want %d", loaded.TokensUsed, original.TokensUsed)
	}
	if loaded.Metadata["key"] != "value" {
		t.Errorf("Metadata[key]: got %q, want %q", loaded.Metadata["key"], "value")
	}
}

// --- T013: Append then load messages roundtrip ---
func TestAppendAndLoadMessages(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionID := "sess001"
	// Save session metadata first (creates the directory)
	if err := store.SaveSession(&Session{ID: sessionID, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	messages := []StoredMessage{
		{SessionID: sessionID, Index: 0, Timestamp: time.Now(), Role: "user", Content: "hello"},
		{SessionID: sessionID, Index: 1, Timestamp: time.Now(), Role: "assistant", Content: "hi"},
	}

	if err := store.AppendMessages(sessionID, messages); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	loaded, err := store.LoadMessages(sessionID)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("LoadMessages: got %d messages, want 2", len(loaded))
	}

	if loaded[0].Content != "hello" {
		t.Errorf("msg[0].Content: got %q, want %q", loaded[0].Content, "hello")
	}
	if loaded[1].Content != "hi" {
		t.Errorf("msg[1].Content: got %q, want %q", loaded[1].Content, "hi")
	}
}

// --- T014: Lists most recent first ---
func TestListSessions_SortedByRecent(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Create sessions with different timestamps (save order matters for UpdatedAt)
	older := &Session{
		ID:        "older111",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}
	newer := &Session{
		ID:        "newer222",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	// Save older first, then newer
	// Note: SaveSession updates UpdatedAt to time.Now(), so we save older first
	// and adjust by saving with a sleep or by relying on the struct values.
	// Since SaveSession overwrites UpdatedAt, we need to work around that.
	// Save them and then manually fix the metadata files.

	if err := store.SaveSession(older); err != nil {
		t.Fatalf("SaveSession older: %v", err)
	}
	// Manually rewrite with the desired timestamp
	older.UpdatedAt = time.Now().Add(-2 * time.Hour)
	data, _ := json.MarshalIndent(older, "", "  ")
	os.WriteFile(store.metadataPath(older.ID), data, 0644)

	if err := store.SaveSession(newer); err != nil {
		t.Fatalf("SaveSession newer: %v", err)
	}
	newer.UpdatedAt = time.Now().Add(-1 * time.Hour)
	data, _ = json.MarshalIndent(newer, "", "  ")
	os.WriteFile(store.metadataPath(newer.ID), data, 0644)

	summaries, err := store.ListSessions(0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("ListSessions: got %d, want 2", len(summaries))
	}

	// Most recent first
	if summaries[0].ID != "newer222" {
		t.Errorf("first session: got %q, want %q", summaries[0].ID, "newer222")
	}
	if summaries[1].ID != "older111" {
		t.Errorf("second session: got %q, want %q", summaries[1].ID, "older111")
	}
}

// --- T015: Removes session directory ---
func TestDeleteSession(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sess := &Session{ID: "del12345", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if !store.SessionExists("del12345") {
		t.Fatal("session should exist before delete")
	}

	if err := store.DeleteSession("del12345"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if store.SessionExists("del12345") {
		t.Error("session should not exist after delete")
	}
}

// --- T016: Returns true for existing, false for missing ---
func TestSessionExists(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if store.SessionExists("nonexist") {
		t.Error("SessionExists: should return false for missing session")
	}

	sess := &Session{ID: "exists01", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if !store.SessionExists("exists01") {
		t.Error("SessionExists: should return true for existing session")
	}
}

// --- T017: Corrupt JSONL lines are skipped ---
func TestLoadMessages_SkipsMalformedLines(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionID := "corrupt1"
	if err := store.SaveSession(&Session{ID: sessionID, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Write a JSONL file with one valid and one corrupt line
	msgPath := store.messagesPath(sessionID)
	validMsg := StoredMessage{SessionID: sessionID, Index: 0, Role: "user", Content: "valid", Timestamp: time.Now()}
	validLine, _ := json.Marshal(validMsg)

	content := string(validLine) + "\n" +
		"THIS IS NOT JSON\n" +
		"{broken json too\n"

	if err := os.WriteFile(msgPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.LoadMessages(sessionID)
	if err != nil {
		t.Fatalf("LoadMessages: unexpected error: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("LoadMessages: got %d messages, want 1 (corrupt lines skipped)", len(loaded))
	}

	if loaded[0].Content != "valid" {
		t.Errorf("msg[0].Content: got %q, want %q", loaded[0].Content, "valid")
	}
}

// --- T018: Empty messages file returns nil ---
func TestLoadMessages_EmptyFile(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionID := "empty001"
	if err := store.SaveSession(&Session{ID: sessionID, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Create an empty messages file
	msgPath := store.messagesPath(sessionID)
	if err := os.WriteFile(msgPath, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.LoadMessages(sessionID)
	if err != nil {
		t.Fatalf("LoadMessages: unexpected error: %v", err)
	}

	if loaded != nil {
		t.Errorf("LoadMessages empty: got %v, want nil", loaded)
	}
}

// --- T019: Respects limit parameter ---
func TestListSessions_Limit(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		sess := &Session{
			ID:        "limit" + string(rune('a'+i)) + "00",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.SaveSession(sess); err != nil {
			t.Fatalf("SaveSession[%d]: %v", i, err)
		}
	}

	summaries, err := store.ListSessions(3)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(summaries) != 3 {
		t.Errorf("ListSessions(3): got %d results, want 3", len(summaries))
	}
}

// --- T020: Appending to new session creates its directory ---
func TestAppendMessages_CreatesDir(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	sessionID := "newdir01"
	// Do NOT create session metadata first -- just append
	messages := []StoredMessage{
		{SessionID: sessionID, Index: 0, Timestamp: time.Now(), Role: "user", Content: "first"},
	}

	if err := store.AppendMessages(sessionID, messages); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	// The session directory should now exist
	dir := store.sessionDir(sessionID)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat session dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created by AppendMessages")
	}

	// And the messages should be loadable
	loaded, err := store.LoadMessages(sessionID)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("LoadMessages: got %d, want 1", len(loaded))
	}
}
