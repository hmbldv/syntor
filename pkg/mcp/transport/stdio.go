package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// JSONRPCMessage mirrors the type from parent package.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return e.Message
}

// StdioConfig configures stdio transport.
type StdioConfig struct {
	Command string
	Args    []string
	Env     map[string]string
	Timeout time.Duration
}

// StdioTransport implements MCP transport over stdin/stdout.
type StdioTransport struct {
	config StdioConfig

	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	reader   *bufio.Reader

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

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(config StdioConfig) *StdioTransport {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &StdioTransport{
		config:  config,
		pending: make(map[int64]chan *JSONRPCMessage),
		done:    make(chan struct{}),
	}
}

// Start launches the subprocess and begins communication.
func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return fmt.Errorf("already connected")
	}

	// Create command
	t.cmd = exec.CommandContext(ctx, t.config.Command, t.config.Args...)

	// Set environment
	if len(t.config.Env) > 0 {
		env := os.Environ()
		for k, v := range t.config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		t.cmd.Env = env
	}

	// Get pipes
	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("get stdin pipe: %w", err)
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("get stdout pipe: %w", err)
	}

	t.reader = bufio.NewReader(t.stdout)

	// Start process
	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	t.connected = true

	// Start response reader
	go t.readResponses()

	return nil
}

// Send sends a message and waits for a response.
func (t *StdioTransport) Send(ctx context.Context, msg *JSONRPCMessage) (*JSONRPCMessage, error) {
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

	// Send message
	if err := t.writeMessage(msg); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(t.config.Timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	case resp := <-respCh:
		return resp, nil
	}
}

// SendNotification sends a notification (no response expected).
func (t *StdioTransport) SendNotification(ctx context.Context, msg *JSONRPCMessage) error {
	if !t.IsConnected() {
		return fmt.Errorf("not connected")
	}
	return t.writeMessage(msg)
}

// OnNotification sets the notification handler.
func (t *StdioTransport) OnNotification(handler func(*JSONRPCMessage)) {
	t.notifMu.Lock()
	defer t.notifMu.Unlock()
	t.notifHandler = handler
}

// Close terminates the connection.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected {
		return nil
	}

	close(t.done)
	t.connected = false

	// Close pipes
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.stdout != nil {
		t.stdout.Close()
	}

	// Kill process
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}

	return nil
}

// IsConnected returns connection status.
func (t *StdioTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

func (t *StdioTransport) writeMessage(msg *JSONRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Write message followed by newline
	t.mu.Lock()
	_, err = t.stdin.Write(append(data, '\n'))
	t.mu.Unlock()

	return err
}

func (t *StdioTransport) readResponses() {
	for {
		select {
		case <-t.done:
			return
		default:
		}

		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				// Log error
			}
			t.Close()
			return
		}

		if len(line) == 0 {
			continue
		}

		var msg JSONRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Route message
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
}
