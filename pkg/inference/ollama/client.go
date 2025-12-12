package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/syntor/syntor/pkg/inference"
)

const (
	defaultBaseURL = "http://localhost:11434"
	defaultTimeout = 5 * time.Minute
)

// Client implements the inference.Provider interface for Ollama
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// ClientConfig holds configuration for the Ollama client
type ClientConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewClient creates a new Ollama client
func NewClient(config ClientConfig) *Client {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}

	// Optimized transport for streaming with connection reuse
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // Faster for streaming
	}

	return &Client{
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
	}
}

// Name returns the provider name
func (c *Client) Name() string {
	return "ollama"
}

// IsAvailable checks if Ollama is accessible
func (c *Client) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// ListModels returns available models from Ollama
func (c *Client) ListModels(ctx context.Context) ([]inference.Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
			Details    struct {
				Format            string `json:"format"`
				Family            string `json:"family"`
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	models := make([]inference.Model, len(result.Models))
	for i, m := range result.Models {
		models[i] = inference.Model{
			ID:         m.Name,
			Name:       m.Name,
			Provider:   "ollama",
			Size:       m.Size,
			Parameters: m.Details.ParameterSize,
		}
	}

	return models, nil
}

// HasModel checks if a specific model is available locally
func (c *Client) HasModel(ctx context.Context, modelID string) (bool, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return false, err
	}

	for _, m := range models {
		if m.ID == modelID || m.Name == modelID {
			return true, nil
		}
	}

	return false, nil
}

// PullModel downloads a model
func (c *Client) PullModel(ctx context.Context, modelID string, progress func(inference.PullProgress)) error {
	body := map[string]interface{}{
		"name":   modelID,
		"stream": true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/pull", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a client without timeout for pulls (they can take a while)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Stream the progress
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var p struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &p); err != nil {
			continue
		}

		if progress != nil {
			var percent float64
			if p.Total > 0 {
				percent = float64(p.Completed) / float64(p.Total) * 100
			}

			progress(inference.PullProgress{
				Status:    p.Status,
				Digest:    p.Digest,
				Total:     p.Total,
				Completed: p.Completed,
				Percent:   percent,
			})
		}
	}

	return scanner.Err()
}

// Complete generates a completion
func (c *Client) Complete(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": false,
	}

	if req.MaxTokens > 0 {
		body["num_predict"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}

	// Merge additional options
	for k, v := range req.Options {
		body[k] = v
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("completion request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Model              string    `json:"model"`
		Response           string    `json:"response"`
		Done               bool      `json:"done"`
		DoneReason         string    `json:"done_reason"`
		PromptEvalCount    int       `json:"prompt_eval_count"`
		EvalCount          int       `json:"eval_count"`
		CreatedAt          time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &inference.CompletionResponse{
		ID:         fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
		Model:      result.Model,
		Content:    result.Response,
		StopReason: result.DoneReason,
		Usage: inference.Usage{
			PromptTokens:     result.PromptEvalCount,
			CompletionTokens: result.EvalCount,
			TotalTokens:      result.PromptEvalCount + result.EvalCount,
		},
		CreatedAt: result.CreatedAt,
	}, nil
}

// CompleteStream generates a streaming completion
func (c *Client) CompleteStream(ctx context.Context, req inference.CompletionRequest) (inference.CompletionStream, error) {
	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": true,
	}

	if req.MaxTokens > 0 {
		body["num_predict"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}

	for k, v := range req.Options {
		body[k] = v
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("completion request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return &completionStream{
		scanner: bufio.NewScanner(resp.Body),
		body:    resp.Body,
	}, nil
}

type completionStream struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
}

func (s *completionStream) Next() (inference.CompletionChunk, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return inference.CompletionChunk{}, err
		}
		return inference.CompletionChunk{}, io.EOF
	}

	var result struct {
		Response   string `json:"response"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
	}

	if err := json.Unmarshal(s.scanner.Bytes(), &result); err != nil {
		return inference.CompletionChunk{}, err
	}

	return inference.CompletionChunk{
		Content:    result.Response,
		Done:       result.Done,
		StopReason: result.DoneReason,
	}, nil
}

func (s *completionStream) Close() error {
	return s.body.Close()
}

// Chat generates a chat completion
func (c *Client) Chat(ctx context.Context, req inference.ChatRequest) (*inference.ChatResponse, error) {
	messages := make([]map[string]string, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]string{
			"role":    m.Role,
			"content": m.Content,
		}
	}

	body := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   false,
	}

	if req.MaxTokens > 0 {
		body["num_predict"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}
	if req.System != "" {
		body["system"] = req.System
	}

	for k, v := range req.Options {
		body[k] = v
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Model   string `json:"model"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done            bool      `json:"done"`
		DoneReason      string    `json:"done_reason"`
		PromptEvalCount int       `json:"prompt_eval_count"`
		EvalCount       int       `json:"eval_count"`
		CreatedAt       time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &inference.ChatResponse{
		ID:    fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
		Model: result.Model,
		Message: inference.Message{
			Role:    result.Message.Role,
			Content: result.Message.Content,
		},
		StopReason: result.DoneReason,
		Usage: inference.Usage{
			PromptTokens:     result.PromptEvalCount,
			CompletionTokens: result.EvalCount,
			TotalTokens:      result.PromptEvalCount + result.EvalCount,
		},
		CreatedAt: result.CreatedAt,
	}, nil
}

// ChatStream generates a streaming chat completion
func (c *Client) ChatStream(ctx context.Context, req inference.ChatRequest) (inference.ChatStream, error) {
	messages := make([]map[string]string, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]string{
			"role":    m.Role,
			"content": m.Content,
		}
	}

	body := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   true,
	}

	if req.MaxTokens > 0 {
		body["num_predict"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}
	if req.System != "" {
		body["system"] = req.System
	}

	for k, v := range req.Options {
		body[k] = v
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Use larger buffer for faster streaming reads
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max

	return &chatStream{
		scanner: scanner,
		body:    resp.Body,
	}, nil
}

type chatStream struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
}

func (s *chatStream) Next() (inference.ChatChunk, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return inference.ChatChunk{}, err
		}
		return inference.ChatChunk{}, io.EOF
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
	}

	if err := json.Unmarshal(s.scanner.Bytes(), &result); err != nil {
		return inference.ChatChunk{}, err
	}

	return inference.ChatChunk{
		Content:    result.Message.Content,
		Done:       result.Done,
		StopReason: result.DoneReason,
	}, nil
}

func (s *chatStream) Close() error {
	return s.body.Close()
}
