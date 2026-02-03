package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HTTPConfig configures HTTP transport.
type HTTPConfig struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

// HTTPTransport implements MCP transport over HTTP.
// Each request gets an immediate response (no long-polling or streaming).
type HTTPTransport struct {
	config     HTTPConfig
	httpClient *http.Client

	// Notification handler (HTTP transport may poll for notifications)
	notifHandler func(*JSONRPCMessage)
	notifMu      sync.RWMutex

	// State
	connected bool
	done      chan struct{}
	mu        sync.Mutex
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(config HTTPConfig) *HTTPTransport {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &HTTPTransport{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		done: make(chan struct{}),
	}
}

// Start marks the transport as ready.
func (t *HTTPTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return fmt.Errorf("already connected")
	}

	// Test connection with a simple request
	req, err := http.NewRequestWithContext(ctx, "OPTIONS", t.config.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		// Don't fail - server might not support OPTIONS
	} else {
		resp.Body.Close()
	}

	t.connected = true
	return nil
}

// Send sends a message and returns the response.
func (t *HTTPTransport) Send(ctx context.Context, msg *JSONRPCMessage) (*JSONRPCMessage, error) {
	if !t.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.config.URL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("error response: %d - %s", resp.StatusCode, string(body))
	}

	var response JSONRPCMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &response, nil
}

// SendNotification sends a notification.
func (t *HTTPTransport) SendNotification(ctx context.Context, msg *JSONRPCMessage) error {
	if !t.IsConnected() {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// OnNotification sets the notification handler.
// Note: HTTP transport typically doesn't receive push notifications.
func (t *HTTPTransport) OnNotification(handler func(*JSONRPCMessage)) {
	t.notifMu.Lock()
	defer t.notifMu.Unlock()
	t.notifHandler = handler
}

// Close terminates the connection.
func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected {
		return nil
	}

	close(t.done)
	t.connected = false
	t.httpClient.CloseIdleConnections()

	return nil
}

// IsConnected returns connection status.
func (t *HTTPTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}
