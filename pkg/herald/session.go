package herald

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// CreateSession creates a new Herald session.
func (c *Client) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	var session Session
	if err := c.doRequest(ctx, "POST", "/api/v1/sessions", req, &session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session, nil
}

// GetSession retrieves a session by ID.
func (c *Client) GetSessionByID(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/sessions/%s", sessionID), nil, &session); err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &session, nil
}

// ListSessions lists sessions with optional filtering.
func (c *Client) ListSessions(ctx context.Context, filter ListSessionsFilter) ([]Session, error) {
	path := "/api/v1/sessions"

	// Build query string
	params := url.Values{}
	if filter.Status != "" {
		params.Set("status", string(filter.Status))
	}
	if filter.Type != "" {
		params.Set("type", string(filter.Type))
	}
	if filter.AgentName != "" {
		params.Set("agent_name", filter.AgentName)
	}
	if filter.Limit > 0 {
		params.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		params.Set("offset", strconv.Itoa(filter.Offset))
	}

	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var sessions []Session
	if err := c.doRequest(ctx, "GET", path, nil, &sessions); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

// UpdateSession updates an existing session.
func (c *Client) UpdateSession(ctx context.Context, sessionID string, req UpdateSessionRequest) (*Session, error) {
	var session Session
	if err := c.doRequest(ctx, "PATCH", fmt.Sprintf("/api/v1/sessions/%s", sessionID), req, &session); err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	return &session, nil
}

// TerminateSession terminates a session.
func (c *Client) TerminateSession(ctx context.Context, sessionID string) error {
	if err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v1/sessions/%s", sessionID), nil, nil); err != nil {
		return fmt.Errorf("terminate session: %w", err)
	}
	return nil
}

// PeekSession returns recent output from a session without attaching.
func (c *Client) PeekSession(ctx context.Context, sessionID string, lines int) (string, error) {
	path := fmt.Sprintf("/api/v1/sessions/%s/peek", sessionID)
	if lines > 0 {
		path += fmt.Sprintf("?lines=%d", lines)
	}

	var result struct {
		Output string `json:"output"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return "", fmt.Errorf("peek session: %w", err)
	}
	return result.Output, nil
}

// AttachSession attaches to a session for interactive use.
// Returns a session attachment handle that can be used for streaming I/O.
func (c *Client) AttachSession(ctx context.Context, sessionID string) (*SessionAttachment, error) {
	session, err := c.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return &SessionAttachment{
		client:  c,
		session: session,
		ctx:     ctx,
	}, nil
}

// ForkSession creates a new session based on an existing one.
func (c *Client) ForkSession(ctx context.Context, sessionID string, name string) (*Session, error) {
	req := CreateSessionRequest{
		Name:     name,
		ParentID: sessionID,
	}

	var session Session
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/sessions/%s/fork", sessionID), req, &session); err != nil {
		return nil, fmt.Errorf("fork session: %w", err)
	}
	return &session, nil
}

// SessionAttachment represents an attached session for streaming I/O.
type SessionAttachment struct {
	client  *Client
	session *Session
	ctx     context.Context
}

// Session returns the attached session.
func (a *SessionAttachment) Session() *Session {
	return a.session
}

// SendMessage sends a message to the attached session.
func (a *SessionAttachment) SendMessage(ctx context.Context, message string) error {
	body := struct {
		Message string `json:"message"`
	}{Message: message}

	return a.client.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/sessions/%s/send", a.session.ID), body, nil)
}

// ReceiveOutput returns recent output since last read.
func (a *SessionAttachment) ReceiveOutput(ctx context.Context) (string, error) {
	return a.client.PeekSession(ctx, a.session.ID, 0)
}

// Detach detaches from the session.
func (a *SessionAttachment) Detach() error {
	// Update session status back to idle
	_, err := a.client.UpdateSession(a.ctx, a.session.ID, UpdateSessionRequest{
		Status: SessionStatusIdle,
	})
	return err
}

// SessionExists checks if a session exists by ID or name.
func (c *Client) SessionExists(ctx context.Context, idOrName string) (bool, error) {
	sessions, err := c.ListSessions(ctx, ListSessionsFilter{})
	if err != nil {
		return false, err
	}

	for _, s := range sessions {
		if s.ID == idOrName || s.Name == idOrName {
			return true, nil
		}
	}
	return false, nil
}

// FindSessionByName finds a session by name.
func (c *Client) FindSessionByName(ctx context.Context, name string) (*Session, error) {
	sessions, err := c.ListSessions(ctx, ListSessionsFilter{})
	if err != nil {
		return nil, err
	}

	for _, s := range sessions {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, &Error{
		Code:    ErrCodeNotFound,
		Message: fmt.Sprintf("session not found: %s", name),
	}
}

// ResumeSession finds and activates a session by ID or name.
func (c *Client) ResumeSession(ctx context.Context, idOrName string) (*Session, error) {
	// Try by ID first
	session, err := c.GetSessionByID(ctx, idOrName)
	if err == nil {
		// Update status to active
		session, err = c.UpdateSession(ctx, session.ID, UpdateSessionRequest{
			Status: SessionStatusActive,
		})
		if err != nil {
			return nil, fmt.Errorf("activate session: %w", err)
		}
		c.SetSession(session.ID)
		return session, nil
	}

	// Try by name
	session, err = c.FindSessionByName(ctx, idOrName)
	if err != nil {
		return nil, err
	}

	// Update status to active
	session, err = c.UpdateSession(ctx, session.ID, UpdateSessionRequest{
		Status: SessionStatusActive,
	})
	if err != nil {
		return nil, fmt.Errorf("activate session: %w", err)
	}
	c.SetSession(session.ID)
	return session, nil
}

// GetActiveSessions returns all currently active sessions.
func (c *Client) GetActiveSessions(ctx context.Context) ([]Session, error) {
	return c.ListSessions(ctx, ListSessionsFilter{
		Status: SessionStatusActive,
	})
}
