package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileStore implements session storage using local JSONL files.
// Each session is stored as:
//   ~/.syntor/sessions/<id>/session.json   (metadata)
//   ~/.syntor/sessions/<id>/messages.jsonl  (append-only messages)
type FileStore struct {
	baseDir string
}

// NewFileStore creates a new file-based session store.
// If baseDir is empty, defaults to ~/.syntor/sessions.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".syntor", "sessions")
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	return &FileStore{baseDir: baseDir}, nil
}

// sessionDir returns the directory for a session.
func (s *FileStore) sessionDir(id string) string {
	return filepath.Join(s.baseDir, id)
}

// metadataPath returns the path to session metadata.
func (s *FileStore) metadataPath(id string) string {
	return filepath.Join(s.sessionDir(id), "session.json")
}

// messagesPath returns the path to session messages.
func (s *FileStore) messagesPath(id string) string {
	return filepath.Join(s.sessionDir(id), "messages.jsonl")
}

// SaveSession creates or updates session metadata.
func (s *FileStore) SaveSession(session *Session) error {
	dir := s.sessionDir(session.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	session.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return os.WriteFile(s.metadataPath(session.ID), data, 0644)
}

// LoadSession reads session metadata.
func (s *FileStore) LoadSession(id string) (*Session, error) {
	data, err := os.ReadFile(s.metadataPath(id))
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &session, nil
}

// AppendMessages appends new messages to the session's JSONL file.
func (s *FileStore) AppendMessages(sessionID string, messages []StoredMessage) error {
	dir := s.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	f, err := os.OpenFile(s.messagesPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open messages file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
	}

	return nil
}

// LoadMessages reads all messages from a session.
func (s *FileStore) LoadMessages(sessionID string) ([]StoredMessage, error) {
	f, err := os.Open(s.messagesPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open messages: %w", err)
	}
	defer f.Close()

	var messages []StoredMessage
	scanner := bufio.NewScanner(f)
	// Increase scanner buffer for large messages
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg StoredMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}
		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan messages: %w", err)
	}

	return messages, nil
}

// ListSessions returns summaries of all sessions, sorted by most recent first.
func (s *FileStore) ListSessions(limit int) ([]SessionSummary, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var summaries []SessionSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		session, err := s.LoadSession(entry.Name())
		if err != nil {
			continue
		}

		// Count messages
		messages, _ := s.LoadMessages(entry.Name())
		msgCount := len(messages)

		summaries = append(summaries, SessionSummary{
			ID:         session.ID,
			Name:       session.Name,
			CreatedAt:  session.CreatedAt,
			UpdatedAt:  session.UpdatedAt,
			WorkingDir: session.WorkingDir,
			AgentName:  session.AgentName,
			Messages:   msgCount,
			TokensUsed: session.TokensUsed,
		})
	}

	// Sort by most recent first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries, nil
}

// DeleteSession removes a session and all its data.
func (s *FileStore) DeleteSession(id string) error {
	return os.RemoveAll(s.sessionDir(id))
}

// SessionExists checks if a session exists.
func (s *FileStore) SessionExists(id string) bool {
	_, err := os.Stat(s.metadataPath(id))
	return err == nil
}
