package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSEConfig configures SSE transport.
type SSEConfig struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

// SSETransport implements MCP transport over Server-Sent Events.
type SSETransport struct {
	config     SSEConfig
	httpClient *http.Client

	// SSE connection
	sseResp   *http.Response
	sseCancel context.CancelFunc

	// Message endpoint (usually different from SSE endpoint)
	messageURL string

	// Response handling
	pending   map[int64]chan *JSONRPCMessage
	pendingMu sync.Mutex

	// Notification handler
	notifHandler func(*JSONRPCMessage)
	notifMu      sync.RWMutex

	// State
	connected bool
	done      chan struct{}
	mu        sync.Mutex
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport(config SSEConfig) *SSETransport {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &SSETransport{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		pending: make(map[int64]chan *JSONRPCMessage),
		done:    make(chan struct{}),
	}
}

// Start establishes the SSE connection.
func (t *SSETransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return fmt.Errorf("already connected")
	}

	// Create SSE request
	sseCtx, cancel := context.WithCancel(ctx)
	t.sseCancel = cancel

	req, err := http.NewRequestWithContext(sseCtx, "GET", t.config.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	t.sseResp = resp
	t.connected = true

	// Derive message URL from SSE URL (common pattern: /sse -> /message)
	t.messageURL = strings.Replace(t.config.URL, "/sse", "/message", 1)

	// Start SSE reader
	go t.readSSE()

	return nil
}

// Send sends a message and waits for a response.
func (t *SSETransport) Send(ctx context.Context, msg *JSONRPCMessage) (*JSONRPCMessage, error) {
	if !t.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if msg.ID == nil {
		return nil, fmt.Errorf("request must have an ID")
	}

	// Create response channel
	respCh := make(chan *JSONRPCMessage, 1)
	t.pendingMu.Lock()
	t.pending[*msg.ID] = respCh
	t.pendingMu.Unlock()

	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, *msg.ID)
		t.pendingMu.Unlock()
	}()

	// Send via HTTP POST
	if err := t.postMessage(ctx, msg); err != nil {
		return nil, err
	}

	// Wait for response via SSE
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(t.config.Timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	case resp := <-respCh:
		return resp, nil
	}
}

// SendNotification sends a notification.
func (t *SSETransport) SendNotification(ctx context.Context, msg *JSONRPCMessage) error {
	if !t.IsConnected() {
		return fmt.Errorf("not connected")
	}
	return t.postMessage(ctx, msg)
}

// OnNotification sets the notification handler.
func (t *SSETransport) OnNotification(handler func(*JSONRPCMessage)) {
	t.notifMu.Lock()
	defer t.notifMu.Unlock()
	t.notifHandler = handler
}

// Close terminates the connection.
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected {
		return nil
	}

	close(t.done)
	t.connected = false

	if t.sseCancel != nil {
		t.sseCancel()
	}
	if t.sseResp != nil {
		t.sseResp.Body.Close()
	}

	return nil
}

// IsConnected returns connection status.
func (t *SSETransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

func (t *SSETransport) postMessage(ctx context.Context, msg *JSONRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.messageURL, bytes.NewReader(data))
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

func (t *SSETransport) readSSE() {
	reader := bufio.NewReader(t.sseResp.Body)

	var eventData strings.Builder

	for {
		select {
		case <-t.done:
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				// Log error
			}
			t.Close()
			return
		}

		line = strings.TrimSpace(line)

		if line == "" {
			// Empty line = end of event
			if eventData.Len() > 0 {
				t.processSSEEvent(eventData.String())
				eventData.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			eventData.WriteString(data)
		}
		// Ignore event: and id: lines for now
	}
}

func (t *SSETransport) processSSEEvent(data string) {
	var msg JSONRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

	if msg.ID != nil {
		// Response to a request
		t.pendingMu.Lock()
		if ch, ok := t.pending[*msg.ID]; ok {
			ch <- &msg
		}
		t.pendingMu.Unlock()
	} else {
		// Notification
		t.notifMu.RLock()
		handler := t.notifHandler
		t.notifMu.RUnlock()

		if handler != nil {
			go handler(&msg)
		}
	}
}
