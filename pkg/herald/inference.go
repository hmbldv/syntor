package herald

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Complete sends a non-streaming inference request through Herald.
func (c *Client) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	req.Stream = false

	var resp InferenceResponse
	if err := c.doRequest(ctx, "POST", "/api/v1/inference/complete", req, &resp); err != nil {
		return nil, fmt.Errorf("inference complete: %w", err)
	}
	return &resp, nil
}

// CompleteStream sends a streaming inference request through Herald.
func (c *Client) CompleteStream(ctx context.Context, req InferenceRequest) (*InferenceStream, error) {
	req.Stream = true

	body, err := c.doStreamRequest(ctx, "/api/v1/inference/stream", req)
	if err != nil {
		return nil, fmt.Errorf("inference stream: %w", err)
	}

	return &InferenceStream{
		reader:   bufio.NewReader(body),
		body:     body,
		done:     false,
		response: &InferenceResponse{},
	}, nil
}

// Chat sends a chat request through Herald.
func (c *Client) Chat(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	req.Stream = false

	var resp InferenceResponse
	if err := c.doRequest(ctx, "POST", "/api/v1/inference/chat", req, &resp); err != nil {
		return nil, fmt.Errorf("inference chat: %w", err)
	}
	return &resp, nil
}

// ChatStream sends a streaming chat request through Herald.
func (c *Client) ChatStream(ctx context.Context, req InferenceRequest) (*InferenceStream, error) {
	req.Stream = true

	body, err := c.doStreamRequest(ctx, "/api/v1/inference/chat/stream", req)
	if err != nil {
		return nil, fmt.Errorf("inference chat stream: %w", err)
	}

	return &InferenceStream{
		reader:   bufio.NewReader(body),
		body:     body,
		done:     false,
		response: &InferenceResponse{},
	}, nil
}

// InferenceStream handles streaming responses from Herald.
type InferenceStream struct {
	reader   *bufio.Reader
	body     io.ReadCloser
	done     bool
	response *InferenceResponse
	buffer   strings.Builder
}

// Next reads the next chunk from the stream.
// Returns io.EOF when the stream is complete.
func (s *InferenceStream) Next() (*StreamChunk, error) {
	if s.done {
		return nil, io.EOF
	}

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.done = true
				return nil, io.EOF
			}
			return nil, fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle SSE format
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Check for done signal
			if data == "[DONE]" {
				s.done = true
				return nil, io.EOF
			}

			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// Try treating as plain text
				chunk.Delta = data
			}

			// Accumulate content
			s.buffer.WriteString(chunk.Delta)

			// Check for finish
			if chunk.FinishReason != "" {
				s.done = true
				s.response.Message.Content = s.buffer.String()
				s.response.FinishReason = chunk.FinishReason
			}

			return &chunk, nil
		}

		// Try parsing as raw JSON
		var chunk StreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err == nil {
			s.buffer.WriteString(chunk.Delta)
			if chunk.FinishReason != "" {
				s.done = true
				s.response.Message.Content = s.buffer.String()
				s.response.FinishReason = chunk.FinishReason
			}
			return &chunk, nil
		}
	}
}

// Response returns the accumulated response after streaming is complete.
func (s *InferenceStream) Response() *InferenceResponse {
	return s.response
}

// Content returns the accumulated content so far.
func (s *InferenceStream) Content() string {
	return s.buffer.String()
}

// Done returns true if the stream is complete.
func (s *InferenceStream) Done() bool {
	return s.done
}

// Close closes the stream.
func (s *InferenceStream) Close() error {
	s.done = true
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}

// ListModels returns available models from Herald.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var models []ModelInfo
	if err := c.doRequest(ctx, "GET", "/api/v1/inference/models", nil, &models); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	return models, nil
}

// ModelInfo describes an available model.
type ModelInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"` // ollama, anthropic, deepseek
	ContextWindow int      `json:"context_window"`
	MaxTokens     int      `json:"max_tokens"`
	Capabilities  []string `json:"capabilities"` // chat, completion, vision, tools
	Available     bool     `json:"available"`
}

// GetModel returns information about a specific model.
func (c *Client) GetModel(ctx context.Context, modelID string) (*ModelInfo, error) {
	var model ModelInfo
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/inference/models/%s", modelID), nil, &model); err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	return &model, nil
}

// SetPreferredModel sets the preferred model for the session.
func (c *Client) SetPreferredModel(ctx context.Context, sessionID, modelID string) error {
	body := struct {
		Model string `json:"model"`
	}{Model: modelID}

	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/sessions/%s/model", sessionID), body, nil); err != nil {
		return fmt.Errorf("set preferred model: %w", err)
	}
	return nil
}

// InferenceConfig holds inference-specific configuration.
type InferenceConfig struct {
	// DefaultModel is the default model to use
	DefaultModel string `yaml:"default_model" json:"default_model"`

	// FallbackModels are tried in order if the default fails
	FallbackModels []string `yaml:"fallback_models" json:"fallback_models"`

	// MaxTokens is the default max tokens for completions
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`

	// Temperature is the default temperature
	Temperature float64 `yaml:"temperature" json:"temperature"`

	// Timeout for inference requests
	Timeout int `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// DefaultInferenceConfig returns sensible defaults.
func DefaultInferenceConfig() InferenceConfig {
	return InferenceConfig{
		DefaultModel:   "qwen2.5-coder:32b",
		FallbackModels: []string{"llama3.3:70b", "mistral:latest"},
		MaxTokens:      4096,
		Temperature:    0.7,
		Timeout:        120,
	}
}
