package inference

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrProviderNotAvailable = errors.New("inference provider not available")
	ErrModelNotFound        = errors.New("model not found")
	ErrModelNotPulled       = errors.New("model not pulled")
	ErrInferenceTimeout     = errors.New("inference timeout")
	ErrInferenceFailed      = errors.New("inference failed")
)

// Provider defines the interface for AI inference providers
type Provider interface {
	// Name returns the provider name (e.g., "ollama", "anthropic", "deepseek")
	Name() string

	// IsAvailable checks if the provider is accessible
	IsAvailable(ctx context.Context) bool

	// ListModels returns available models from this provider
	ListModels(ctx context.Context) ([]Model, error)

	// HasModel checks if a specific model is available
	HasModel(ctx context.Context, modelID string) (bool, error)

	// PullModel downloads/prepares a model (for local providers)
	PullModel(ctx context.Context, modelID string, progress func(PullProgress)) error

	// Complete generates a completion for the given request
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// CompleteStream generates a streaming completion
	CompleteStream(ctx context.Context, req CompletionRequest) (CompletionStream, error)

	// Chat generates a chat completion
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// ChatStream generates a streaming chat completion
	ChatStream(ctx context.Context, req ChatRequest) (ChatStream, error)
}

// Model represents an available AI model
type Model struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Provider    string            `json:"provider" yaml:"provider"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Size        int64             `json:"size,omitempty" yaml:"size,omitempty"`         // Size in bytes
	Parameters  string            `json:"parameters,omitempty" yaml:"parameters,omitempty"` // e.g., "7B", "13B"
	Context     int               `json:"context,omitempty" yaml:"context,omitempty"`   // Context window size
	Capabilities []string         `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// CompletionRequest represents a text completion request
type CompletionRequest struct {
	Model       string            `json:"model"`
	Prompt      string            `json:"prompt"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	TopP        float64           `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Options     map[string]any    `json:"options,omitempty"`
}

// CompletionResponse represents a completion response
type CompletionResponse struct {
	ID         string    `json:"id"`
	Model      string    `json:"model"`
	Content    string    `json:"content"`
	StopReason string    `json:"stop_reason,omitempty"`
	Usage      Usage     `json:"usage"`
	CreatedAt  time.Time `json:"created_at"`
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	TopP        float64           `json:"top_p,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	System      string            `json:"system,omitempty"`
	Options     map[string]any    `json:"options,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID         string    `json:"id"`
	Model      string    `json:"model"`
	Message    Message   `json:"message"`
	StopReason string    `json:"stop_reason,omitempty"`
	Usage      Usage     `json:"usage"`
	CreatedAt  time.Time `json:"created_at"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionStream represents a streaming completion
type CompletionStream interface {
	// Next returns the next chunk, or io.EOF when done
	Next() (CompletionChunk, error)
	// Close closes the stream
	Close() error
}

// CompletionChunk represents a single chunk in a streaming response
type CompletionChunk struct {
	Content    string `json:"content"`
	Done       bool   `json:"done"`
	StopReason string `json:"stop_reason,omitempty"`
}

// ChatStream represents a streaming chat completion
type ChatStream interface {
	// Next returns the next chunk, or io.EOF when done
	Next() (ChatChunk, error)
	// Close closes the stream
	Close() error
}

// ChatChunk represents a single chunk in a streaming chat response
type ChatChunk struct {
	Content    string `json:"content"`
	Done       bool   `json:"done"`
	StopReason string `json:"stop_reason,omitempty"`
}

// PullProgress represents model pull progress
type PullProgress struct {
	Status    string  `json:"status"`
	Digest    string  `json:"digest,omitempty"`
	Total     int64   `json:"total,omitempty"`
	Completed int64   `json:"completed,omitempty"`
	Percent   float64 `json:"percent,omitempty"`
}

// StreamReader wraps an io.ReadCloser for streaming responses
type StreamReader struct {
	reader io.ReadCloser
}

func NewStreamReader(r io.ReadCloser) *StreamReader {
	return &StreamReader{reader: r}
}

func (s *StreamReader) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

func (s *StreamReader) Close() error {
	return s.reader.Close()
}
