package herald

import (
	"context"
	"fmt"
	"io"
	"time"

	"syntor/pkg/inference"
)

// Provider implements inference.Provider using Herald as the backend.
// This is the primary inference provider when Herald is available.
type Provider struct {
	client *Client
	config InferenceConfig
}

// NewProvider creates a new Herald-backed inference provider.
func NewProvider(client *Client, config InferenceConfig) *Provider {
	return &Provider{
		client: client,
		config: config,
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "herald"
}

// IsAvailable checks if Herald inference is accessible.
func (p *Provider) IsAvailable(ctx context.Context) bool {
	return p.client.IsAvailable(ctx)
}

// ListModels returns available models from Herald.
func (p *Provider) ListModels(ctx context.Context) ([]inference.Model, error) {
	models, err := p.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]inference.Model, len(models))
	for i, m := range models {
		result[i] = inference.Model{
			ID:           m.ID,
			Name:         m.Name,
			Provider:     m.Provider,
			Context:      m.ContextWindow,
			Capabilities: m.Capabilities,
		}
	}
	return result, nil
}

// HasModel checks if a specific model is available via Herald.
func (p *Provider) HasModel(ctx context.Context, modelID string) (bool, error) {
	model, err := p.client.GetModel(ctx, modelID)
	if err != nil {
		// Check if it's a not found error
		if heraldErr, ok := err.(*Error); ok && heraldErr.Code == ErrCodeNotFound {
			return false, nil
		}
		return false, err
	}
	return model.Available, nil
}

// PullModel requests Herald to pull/prepare a model.
func (p *Provider) PullModel(ctx context.Context, modelID string, progress func(inference.PullProgress)) error {
	// Herald handles model management internally
	// This is a no-op or could trigger a pull request
	body := struct {
		Model string `json:"model"`
	}{Model: modelID}

	if err := p.client.doRequest(ctx, "POST", "/api/v1/inference/models/pull", body, nil); err != nil {
		return fmt.Errorf("pull model: %w", err)
	}

	if progress != nil {
		progress(inference.PullProgress{
			Status:  "complete",
			Percent: 100,
		})
	}
	return nil
}

// Complete generates a completion through Herald.
func (p *Provider) Complete(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
	// Build messages from prompt
	messages := []Message{
		{Role: "user", Content: req.Prompt},
	}

	heraldReq := InferenceRequest{
		SessionID:   p.client.GetSession(),
		Model:       p.selectModel(req.Model),
		Messages:    messages,
		MaxTokens:   p.selectMaxTokens(req.MaxTokens),
		Temperature: p.selectTemperature(req.Temperature),
		Stream:      false,
	}

	resp, err := p.client.Complete(ctx, heraldReq)
	if err != nil {
		return nil, err
	}

	return &inference.CompletionResponse{
		ID:         resp.ID,
		Model:      resp.Model,
		Content:    resp.Message.Content,
		StopReason: resp.FinishReason,
		Usage: inference.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		CreatedAt: resp.CreatedAt,
	}, nil
}

// CompleteStream generates a streaming completion through Herald.
func (p *Provider) CompleteStream(ctx context.Context, req inference.CompletionRequest) (inference.CompletionStream, error) {
	messages := []Message{
		{Role: "user", Content: req.Prompt},
	}

	heraldReq := InferenceRequest{
		SessionID:   p.client.GetSession(),
		Model:       p.selectModel(req.Model),
		Messages:    messages,
		MaxTokens:   p.selectMaxTokens(req.MaxTokens),
		Temperature: p.selectTemperature(req.Temperature),
		Stream:      true,
	}

	stream, err := p.client.CompleteStream(ctx, heraldReq)
	if err != nil {
		return nil, err
	}

	return &completionStreamAdapter{stream: stream}, nil
}

// Chat generates a chat completion through Herald.
func (p *Provider) Chat(ctx context.Context, req inference.ChatRequest) (*inference.ChatResponse, error) {
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	heraldReq := InferenceRequest{
		SessionID:    p.client.GetSession(),
		Model:        p.selectModel(req.Model),
		Messages:     messages,
		MaxTokens:    p.selectMaxTokens(req.MaxTokens),
		Temperature:  p.selectTemperature(req.Temperature),
		SystemPrompt: req.System,
		Stream:       false,
	}

	resp, err := p.client.Chat(ctx, heraldReq)
	if err != nil {
		return nil, err
	}

	return &inference.ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Message: inference.Message{
			Role:    resp.Message.Role,
			Content: resp.Message.Content,
		},
		StopReason: resp.FinishReason,
		Usage: inference.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		CreatedAt: resp.CreatedAt,
	}, nil
}

// ChatStream generates a streaming chat completion through Herald.
func (p *Provider) ChatStream(ctx context.Context, req inference.ChatRequest) (inference.ChatStream, error) {
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	heraldReq := InferenceRequest{
		SessionID:    p.client.GetSession(),
		Model:        p.selectModel(req.Model),
		Messages:     messages,
		MaxTokens:    p.selectMaxTokens(req.MaxTokens),
		Temperature:  p.selectTemperature(req.Temperature),
		SystemPrompt: req.System,
		Stream:       true,
	}

	stream, err := p.client.ChatStream(ctx, heraldReq)
	if err != nil {
		return nil, err
	}

	return &chatStreamAdapter{stream: stream}, nil
}

// selectModel returns the model to use, falling back to defaults.
func (p *Provider) selectModel(requested string) string {
	if requested != "" {
		return requested
	}
	return p.config.DefaultModel
}

// selectMaxTokens returns max tokens, falling back to defaults.
func (p *Provider) selectMaxTokens(requested int) int {
	if requested > 0 {
		return requested
	}
	return p.config.MaxTokens
}

// selectTemperature returns temperature, falling back to defaults.
func (p *Provider) selectTemperature(requested float64) float64 {
	if requested > 0 {
		return requested
	}
	return p.config.Temperature
}

// completionStreamAdapter adapts Herald's stream to inference.CompletionStream.
type completionStreamAdapter struct {
	stream *InferenceStream
}

func (a *completionStreamAdapter) Next() (inference.CompletionChunk, error) {
	chunk, err := a.stream.Next()
	if err != nil {
		if err == io.EOF {
			return inference.CompletionChunk{Done: true}, io.EOF
		}
		return inference.CompletionChunk{}, err
	}

	return inference.CompletionChunk{
		Content:    chunk.Delta,
		Done:       chunk.FinishReason != "",
		StopReason: chunk.FinishReason,
	}, nil
}

func (a *completionStreamAdapter) Close() error {
	return a.stream.Close()
}

// chatStreamAdapter adapts Herald's stream to inference.ChatStream.
type chatStreamAdapter struct {
	stream *InferenceStream
}

func (a *chatStreamAdapter) Next() (inference.ChatChunk, error) {
	chunk, err := a.stream.Next()
	if err != nil {
		if err == io.EOF {
			return inference.ChatChunk{Done: true}, io.EOF
		}
		return inference.ChatChunk{}, err
	}

	return inference.ChatChunk{
		Content:    chunk.Delta,
		Done:       chunk.FinishReason != "",
		StopReason: chunk.FinishReason,
	}, nil
}

func (a *chatStreamAdapter) Close() error {
	return a.stream.Close()
}

// Verify Provider implements inference.Provider
var _ inference.Provider = (*Provider)(nil)

// FallbackProvider wraps Herald with fallback to local providers.
type FallbackProvider struct {
	primary   *Provider
	fallbacks []inference.Provider
}

// NewFallbackProvider creates a provider that falls back to alternatives when Herald is unavailable.
func NewFallbackProvider(primary *Provider, fallbacks ...inference.Provider) *FallbackProvider {
	return &FallbackProvider{
		primary:   primary,
		fallbacks: fallbacks,
	}
}

// Name returns the provider name.
func (p *FallbackProvider) Name() string {
	return "herald-fallback"
}

// IsAvailable returns true if any provider is available.
func (p *FallbackProvider) IsAvailable(ctx context.Context) bool {
	if p.primary.IsAvailable(ctx) {
		return true
	}
	for _, fb := range p.fallbacks {
		if fb.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

// getAvailable returns the first available provider.
func (p *FallbackProvider) getAvailable(ctx context.Context) inference.Provider {
	if p.primary.IsAvailable(ctx) {
		return p.primary
	}
	for _, fb := range p.fallbacks {
		if fb.IsAvailable(ctx) {
			return fb
		}
	}
	return nil
}

// ListModels returns models from the available provider.
func (p *FallbackProvider) ListModels(ctx context.Context) ([]inference.Model, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return nil, inference.ErrProviderNotAvailable
	}
	return provider.ListModels(ctx)
}

// HasModel checks if a model is available.
func (p *FallbackProvider) HasModel(ctx context.Context, modelID string) (bool, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return false, inference.ErrProviderNotAvailable
	}
	return provider.HasModel(ctx, modelID)
}

// PullModel pulls a model from the available provider.
func (p *FallbackProvider) PullModel(ctx context.Context, modelID string, progress func(inference.PullProgress)) error {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return inference.ErrProviderNotAvailable
	}
	return provider.PullModel(ctx, modelID, progress)
}

// Complete generates a completion from the available provider.
func (p *FallbackProvider) Complete(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return nil, inference.ErrProviderNotAvailable
	}
	return provider.Complete(ctx, req)
}

// CompleteStream generates a streaming completion from the available provider.
func (p *FallbackProvider) CompleteStream(ctx context.Context, req inference.CompletionRequest) (inference.CompletionStream, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return nil, inference.ErrProviderNotAvailable
	}
	return provider.CompleteStream(ctx, req)
}

// Chat generates a chat completion from the available provider.
func (p *FallbackProvider) Chat(ctx context.Context, req inference.ChatRequest) (*inference.ChatResponse, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return nil, inference.ErrProviderNotAvailable
	}
	return provider.Chat(ctx, req)
}

// ChatStream generates a streaming chat completion from the available provider.
func (p *FallbackProvider) ChatStream(ctx context.Context, req inference.ChatRequest) (inference.ChatStream, error) {
	provider := p.getAvailable(ctx)
	if provider == nil {
		return nil, inference.ErrProviderNotAvailable
	}
	return provider.ChatStream(ctx, req)
}

// Verify FallbackProvider implements inference.Provider
var _ inference.Provider = (*FallbackProvider)(nil)
