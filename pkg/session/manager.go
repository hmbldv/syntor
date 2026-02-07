package session

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/inference"
)

// Manager handles session lifecycle: create, resume, list, fork, delete.
type Manager struct {
	store          *FileStore
	currentSession *Session
	lastSavedIndex int // Index of last saved message for incremental appends
}

// NewManager creates a new session manager.
func NewManager(baseDir string) (*Manager, error) {
	store, err := NewFileStore(baseDir)
	if err != nil {
		return nil, err
	}

	return &Manager{
		store: store,
	}, nil
}

// Create starts a new session.
func (m *Manager) Create(workingDir, agentName string) (*Session, error) {
	session := &Session{
		ID:         uuid.New().String()[:8], // Short ID for usability
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		WorkingDir: workingDir,
		AgentName:  agentName,
		Metadata:   make(map[string]string),
	}

	if err := m.store.SaveSession(session); err != nil {
		return nil, fmt.Errorf("save new session: %w", err)
	}

	m.currentSession = session
	m.lastSavedIndex = 0
	return session, nil
}

// Resume loads an existing session by ID or name prefix.
func (m *Manager) Resume(idOrName string) (*Session, []inference.Message, error) {
	// Try exact ID match first
	if m.store.SessionExists(idOrName) {
		return m.loadSession(idOrName)
	}

	// Try prefix match on ID or name
	summaries, err := m.store.ListSessions(0)
	if err != nil {
		return nil, nil, err
	}

	var matches []SessionSummary
	for _, s := range summaries {
		if strings.HasPrefix(s.ID, idOrName) ||
			(s.Name != "" && strings.HasPrefix(strings.ToLower(s.Name), strings.ToLower(idOrName))) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, nil, fmt.Errorf("no session found matching %q", idOrName)
	case 1:
		return m.loadSession(matches[0].ID)
	default:
		var ids []string
		for _, s := range matches {
			ids = append(ids, fmt.Sprintf("%s (%s)", s.ID, s.Name))
		}
		return nil, nil, fmt.Errorf("ambiguous match for %q, candidates: %s",
			idOrName, strings.Join(ids, ", "))
	}
}

// loadSession loads a session and its messages.
func (m *Manager) loadSession(id string) (*Session, []inference.Message, error) {
	session, err := m.store.LoadSession(id)
	if err != nil {
		return nil, nil, err
	}

	storedMsgs, err := m.store.LoadMessages(id)
	if err != nil {
		return nil, nil, err
	}

	// Convert to inference messages
	messages := make([]inference.Message, len(storedMsgs))
	for i, sm := range storedMsgs {
		messages[i] = sm.ToInferenceMessage()
	}

	m.currentSession = session
	m.lastSavedIndex = len(storedMsgs)
	return session, messages, nil
}

// AppendMessages saves new messages incrementally.
// Only saves messages that haven't been saved yet (since lastSavedIndex).
func (m *Manager) AppendMessages(history []inference.Message, agentName string) error {
	if m.currentSession == nil {
		return nil // No active session
	}

	if len(history) <= m.lastSavedIndex {
		return nil // Nothing new to save
	}

	newMsgs := history[m.lastSavedIndex:]
	stored := make([]StoredMessage, len(newMsgs))
	for i, msg := range newMsgs {
		stored[i] = FromInferenceMessage(
			m.currentSession.ID,
			m.lastSavedIndex+i,
			msg,
			agentName,
		)
	}

	if err := m.store.AppendMessages(m.currentSession.ID, stored); err != nil {
		return err
	}

	m.lastSavedIndex = len(history)

	// Update session metadata
	m.currentSession.UpdatedAt = time.Now()
	return m.store.SaveSession(m.currentSession)
}

// List returns recent session summaries.
func (m *Manager) List(limit int) ([]SessionSummary, error) {
	if limit == 0 {
		limit = 20
	}
	return m.store.ListSessions(limit)
}

// Fork creates a new session that starts with the current session's messages.
func (m *Manager) Fork(newName string) (*Session, error) {
	if m.currentSession == nil {
		return nil, fmt.Errorf("no active session to fork")
	}

	// Load current messages
	messages, err := m.store.LoadMessages(m.currentSession.ID)
	if err != nil {
		return nil, err
	}

	// Create new session
	forked := &Session{
		ID:         uuid.New().String()[:8],
		Name:       newName,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		WorkingDir: m.currentSession.WorkingDir,
		AgentName:  m.currentSession.AgentName,
		Metadata: map[string]string{
			"forked_from": m.currentSession.ID,
		},
	}

	if err := m.store.SaveSession(forked); err != nil {
		return nil, err
	}

	// Copy messages to new session
	if len(messages) > 0 {
		for i := range messages {
			messages[i].SessionID = forked.ID
		}
		if err := m.store.AppendMessages(forked.ID, messages); err != nil {
			return nil, err
		}
	}

	m.currentSession = forked
	m.lastSavedIndex = len(messages)
	return forked, nil
}

// Delete removes a session.
func (m *Manager) Delete(id string) error {
	return m.store.DeleteSession(id)
}

// SetName sets the name of the current session.
func (m *Manager) SetName(name string) error {
	if m.currentSession == nil {
		return fmt.Errorf("no active session")
	}
	m.currentSession.Name = name
	return m.store.SaveSession(m.currentSession)
}

// UpdateTokensUsed updates the token count for the current session.
func (m *Manager) UpdateTokensUsed(tokens int64) {
	if m.currentSession != nil {
		m.currentSession.TokensUsed = tokens
	}
}

// Current returns the current active session, or nil.
func (m *Manager) Current() *Session {
	return m.currentSession
}

// BaseDir returns the session storage base directory.
func (m *Manager) BaseDir() string {
	return m.store.baseDir
}

// Flush saves any pending session state.
func (m *Manager) Flush() error {
	if m.currentSession == nil {
		return nil
	}
	return m.store.SaveSession(m.currentSession)
}

// FormatSessionList formats session summaries for display.
func FormatSessionList(summaries []SessionSummary) string {
	if len(summaries) == 0 {
		return "No sessions found."
	}

	var sb strings.Builder
	sb.WriteString("Recent Sessions:\n\n")

	for _, s := range summaries {
		name := s.ID
		if s.Name != "" {
			name = fmt.Sprintf("%s (%s)", s.ID, s.Name)
		}

		age := time.Since(s.UpdatedAt)
		ageStr := formatDuration(age)

		dir := s.WorkingDir
		if home, err := os.UserHomeDir(); err == nil {
			dir = strings.Replace(dir, home, "~", 1)
		}

		sb.WriteString(fmt.Sprintf("  %s  %d msgs  %s ago  %s\n",
			name, s.Messages, ageStr, dir))
	}

	return sb.String()
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
