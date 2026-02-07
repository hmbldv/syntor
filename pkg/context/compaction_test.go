// Tests for context compaction (Phases 0-10 UX parity).
// Batch 1: T001-T010
package context

import (
	gocontext "context"
	"strings"
	"testing"
	"time"

	"github.com/syntor/syntor/pkg/inference"
)

// mockProvider implements inference.Provider with canned responses
// for compaction summary tests.
type mockProvider struct {
	summaryResponse string
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) IsAvailable(_ gocontext.Context) bool {
	return true
}
func (m *mockProvider) ListModels(_ gocontext.Context) ([]inference.Model, error) {
	return nil, nil
}
func (m *mockProvider) HasModel(_ gocontext.Context, _ string) (bool, error) {
	return true, nil
}
func (m *mockProvider) PullModel(_ gocontext.Context, _ string, _ func(inference.PullProgress)) error {
	return nil
}
func (m *mockProvider) Complete(_ gocontext.Context, _ inference.CompletionRequest) (*inference.CompletionResponse, error) {
	return &inference.CompletionResponse{Content: m.summaryResponse}, nil
}
func (m *mockProvider) CompleteStream(_ gocontext.Context, _ inference.CompletionRequest) (inference.CompletionStream, error) {
	return nil, nil
}
func (m *mockProvider) Chat(_ gocontext.Context, _ inference.ChatRequest) (*inference.ChatResponse, error) {
	return &inference.ChatResponse{
		Message: inference.Message{
			Role:    "assistant",
			Content: m.summaryResponse,
		},
		CreatedAt: time.Now(),
	}, nil
}
func (m *mockProvider) ChatStream(_ gocontext.Context, _ inference.ChatRequest) (inference.ChatStream, error) {
	return nil, nil
}

// --- T001: Token estimation uses 4 chars = 1 token ---
func TestEstimateTokens(t *testing.T) {
	messages := []inference.Message{
		{Role: "user", Content: "abcd"},     // 4 chars = 1 token
		{Role: "assistant", Content: "abcd"}, // 4 chars = 1 token
	}

	got := EstimateTokens(messages)
	if got != 2 {
		t.Errorf("EstimateTokens: got %d, want 2", got)
	}
}

// --- T002: Empty messages return 0 tokens ---
func TestEstimateTokensEmpty(t *testing.T) {
	got := EstimateTokens(nil)
	if got != 0 {
		t.Errorf("EstimateTokens(nil): got %d, want 0", got)
	}

	got = EstimateTokens([]inference.Message{})
	if got != 0 {
		t.Errorf("EstimateTokens([]): got %d, want 0", got)
	}
}

// --- T003: Below threshold returns false ---
func TestShouldCompact_BelowThreshold(t *testing.T) {
	provider := &mockProvider{}
	c := NewCompactor(provider, CompactorConfig{
		MaxTokens:      1000,
		CompactAt:      0.75,
		PreserveRecent: 5,
	})

	// 20 chars = 5 tokens, threshold = 750 tokens
	messages := []inference.Message{
		{Role: "user", Content: "12345678901234567890"},
	}

	if c.ShouldCompact(messages) {
		t.Error("ShouldCompact: expected false when below threshold")
	}
}

// --- T004: Above threshold returns true ---
func TestShouldCompact_AboveThreshold(t *testing.T) {
	provider := &mockProvider{}
	c := NewCompactor(provider, CompactorConfig{
		MaxTokens:      100,
		CompactAt:      0.75,
		PreserveRecent: 5,
	})

	// 400 chars = 100 tokens, threshold = 75 tokens
	messages := []inference.Message{
		{Role: "user", Content: strings.Repeat("x", 400)},
	}

	if !c.ShouldCompact(messages) {
		t.Error("ShouldCompact: expected true when above threshold")
	}
}

// --- T005: Default config has sensible values ---
func TestDefaultCompactorConfig(t *testing.T) {
	cfg := DefaultCompactorConfig()

	if cfg.MaxTokens != 120000 {
		t.Errorf("MaxTokens: got %d, want 120000", cfg.MaxTokens)
	}
	if cfg.CompactAt != 0.75 {
		t.Errorf("CompactAt: got %f, want 0.75", cfg.CompactAt)
	}
	if cfg.PreserveRecent != 10 {
		t.Errorf("PreserveRecent: got %d, want 10", cfg.PreserveRecent)
	}
	if len(cfg.PreserveKeys) == 0 {
		t.Error("PreserveKeys: expected non-empty slice")
	}
}

// --- T006: Short history stays unchanged ---
func TestCompact_ShortHistory(t *testing.T) {
	provider := &mockProvider{summaryResponse: "summary"}
	c := NewCompactor(provider, CompactorConfig{
		MaxTokens:      1000,
		CompactAt:      0.75,
		PreserveRecent: 10,
	})

	// Only 3 messages, PreserveRecent=10 so nothing to compact
	messages := []inference.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "bye"},
	}

	result, err := c.Compact(gocontext.Background(), messages)
	if err != nil {
		t.Fatalf("Compact: unexpected error: %v", err)
	}

	if len(result) != len(messages) {
		t.Errorf("Compact: got %d messages, want %d (unchanged)", len(result), len(messages))
	}

	// Verify content is identical
	for i := range messages {
		if result[i].Content != messages[i].Content {
			t.Errorf("message[%d]: got %q, want %q", i, result[i].Content, messages[i].Content)
		}
	}
}

// --- T007: Compaction preserves the last N messages ---
func TestCompact_PreservesRecent(t *testing.T) {
	provider := &mockProvider{summaryResponse: "## Conversation Summary\nSummarized older messages."}
	preserveRecent := 3
	c := NewCompactor(provider, CompactorConfig{
		MaxTokens:      1000,
		CompactAt:      0.75,
		PreserveRecent: preserveRecent,
	})

	// 6 messages: first 3 will be summarized, last 3 preserved
	messages := []inference.Message{
		{Role: "user", Content: "old message 1"},
		{Role: "assistant", Content: "old reply 1"},
		{Role: "user", Content: "old message 2"},
		{Role: "user", Content: "recent 1"},
		{Role: "assistant", Content: "recent 2"},
		{Role: "user", Content: "recent 3"},
	}

	result, err := c.Compact(gocontext.Background(), messages)
	if err != nil {
		t.Fatalf("Compact: unexpected error: %v", err)
	}

	// Should be 1 summary + 3 recent = 4
	wantLen := preserveRecent + 1
	if len(result) != wantLen {
		t.Fatalf("Compact: got %d messages, want %d", len(result), wantLen)
	}

	// First message should be the summary (system role)
	if result[0].Role != "system" {
		t.Errorf("result[0].Role: got %q, want %q", result[0].Role, "system")
	}
	if !strings.Contains(result[0].Content, "summarized") {
		t.Errorf("result[0].Content: expected summary text, got %q", result[0].Content)
	}

	// Last 3 messages should be the recent ones, verbatim
	for i := 1; i < len(result); i++ {
		origIdx := len(messages) - preserveRecent + (i - 1)
		if result[i].Content != messages[origIdx].Content {
			t.Errorf("result[%d].Content: got %q, want %q", i, result[i].Content, messages[origIdx].Content)
		}
	}
}

// --- T008: CompactWithResult returns proper CompactResult ---
func TestCompactWithResult_ReturnsStats(t *testing.T) {
	provider := &mockProvider{summaryResponse: "summary of older messages"}
	c := NewCompactor(provider, CompactorConfig{
		MaxTokens:      1000,
		CompactAt:      0.75,
		PreserveRecent: 2,
	})

	messages := []inference.Message{
		{Role: "user", Content: strings.Repeat("a", 100)},
		{Role: "assistant", Content: strings.Repeat("b", 100)},
		{Role: "user", Content: strings.Repeat("c", 100)},
		{Role: "assistant", Content: strings.Repeat("d", 100)},
	}

	compacted, stats, err := c.CompactWithResult(gocontext.Background(), messages)
	if err != nil {
		t.Fatalf("CompactWithResult: unexpected error: %v", err)
	}

	if stats == nil {
		t.Fatal("CompactWithResult: stats is nil")
	}

	if stats.OriginalMessages != 4 {
		t.Errorf("OriginalMessages: got %d, want 4", stats.OriginalMessages)
	}

	// 1 summary + 2 preserved = 3
	if stats.CompactedMessages != 3 {
		t.Errorf("CompactedMessages: got %d, want 3", stats.CompactedMessages)
	}

	// Original tokens: 400 chars / 4 = 100
	if stats.OriginalTokens != 100 {
		t.Errorf("OriginalTokens: got %d, want 100", stats.OriginalTokens)
	}

	// Compacted tokens should be less than original
	if stats.CompactedTokens >= stats.OriginalTokens {
		t.Errorf("CompactedTokens (%d) should be less than OriginalTokens (%d)",
			stats.CompactedTokens, stats.OriginalTokens)
	}

	if len(compacted) != 3 {
		t.Errorf("compacted length: got %d, want 3", len(compacted))
	}
}

// --- T009: Passing zero MaxTokens uses defaults ---
func TestNewCompactor_DefaultConfig(t *testing.T) {
	provider := &mockProvider{}
	c := NewCompactor(provider, CompactorConfig{}) // All zeros

	defaults := DefaultCompactorConfig()

	if c.config.MaxTokens != defaults.MaxTokens {
		t.Errorf("MaxTokens: got %d, want %d", c.config.MaxTokens, defaults.MaxTokens)
	}
	if c.config.CompactAt != defaults.CompactAt {
		t.Errorf("CompactAt: got %f, want %f", c.config.CompactAt, defaults.CompactAt)
	}
	if c.config.PreserveRecent != defaults.PreserveRecent {
		t.Errorf("PreserveRecent: got %d, want %d", c.config.PreserveRecent, defaults.PreserveRecent)
	}
}

// --- T010: Handles large message content correctly ---
func TestEstimateTokens_LargeMessages(t *testing.T) {
	// 1MB message
	largeContent := strings.Repeat("x", 1_000_000)
	messages := []inference.Message{
		{Role: "user", Content: largeContent},
	}

	got := EstimateTokens(messages)
	want := 1_000_000 / 4
	if got != want {
		t.Errorf("EstimateTokens large: got %d, want %d", got, want)
	}

	// Multiple large messages
	messages = append(messages, inference.Message{Role: "assistant", Content: largeContent})
	got = EstimateTokens(messages)
	want = 2 * (1_000_000 / 4)
	if got != want {
		t.Errorf("EstimateTokens 2x large: got %d, want %d", got, want)
	}
}
