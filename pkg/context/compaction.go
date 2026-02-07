package context

import (
	gocontext "context"
	"fmt"
	"strings"

	"github.com/syntor/syntor/pkg/inference"
)

// CompactorConfig holds configuration for context compaction.
type CompactorConfig struct {
	// MaxTokens is the estimated context window size.
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`

	// CompactAt is the threshold ratio (0-1) at which compaction triggers.
	CompactAt float64 `yaml:"compact_at" json:"compact_at"`

	// PreserveRecent is the number of recent turns to keep verbatim.
	PreserveRecent int `yaml:"preserve_recent" json:"preserve_recent"`

	// PreserveKeys are critical pieces of info that must survive compaction.
	PreserveKeys []string `yaml:"preserve_keys" json:"preserve_keys"`
}

// DefaultCompactorConfig returns sensible defaults for compaction.
func DefaultCompactorConfig() CompactorConfig {
	return CompactorConfig{
		MaxTokens:      120000,
		CompactAt:      0.75,
		PreserveRecent: 10,
		PreserveKeys:   []string{"working_directory", "active_agent"},
	}
}

// Compactor handles automatic compression of conversation history
// when approaching context window limits.
type Compactor struct {
	provider inference.Provider
	config   CompactorConfig
}

// NewCompactor creates a new context compactor.
func NewCompactor(provider inference.Provider, config CompactorConfig) *Compactor {
	if config.MaxTokens == 0 {
		config = DefaultCompactorConfig()
	}
	return &Compactor{
		provider: provider,
		config:   config,
	}
}

// EstimateTokens estimates the token count for a list of messages.
// Uses a simple heuristic: ~4 characters per token.
func EstimateTokens(messages []inference.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content) / 4
	}
	return total
}

// ShouldCompact returns true if the conversation history should be compacted.
func (c *Compactor) ShouldCompact(history []inference.Message) bool {
	estimated := EstimateTokens(history)
	threshold := int(float64(c.config.MaxTokens) * c.config.CompactAt)
	return estimated >= threshold
}

// Compact compresses older messages in the conversation history while
// preserving recent turns and critical information.
// Returns the compacted history: [summary_message, ...recent_messages]
func (c *Compactor) Compact(ctx gocontext.Context, history []inference.Message) ([]inference.Message, error) {
	if len(history) <= c.config.PreserveRecent {
		// Nothing to compact
		return history, nil
	}

	// Split into old (to summarize) and recent (to preserve)
	splitIdx := len(history) - c.config.PreserveRecent
	oldMessages := history[:splitIdx]
	recentMessages := history[splitIdx:]

	// Build compaction prompt
	summary, err := c.summarize(ctx, oldMessages)
	if err != nil {
		return history, fmt.Errorf("compaction failed: %w", err)
	}

	// Build new history: summary + recent messages
	compacted := make([]inference.Message, 0, c.config.PreserveRecent+1)
	compacted = append(compacted, inference.Message{
		Role:    "system",
		Content: summary,
	})
	compacted = append(compacted, recentMessages...)

	return compacted, nil
}

// summarize uses the LLM to create a concise summary of old messages.
func (c *Compactor) summarize(ctx gocontext.Context, messages []inference.Message) (string, error) {
	// Build the conversation transcript for summarization
	var transcript strings.Builder
	for _, m := range messages {
		transcript.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	preserveKeysStr := ""
	if len(c.config.PreserveKeys) > 0 {
		preserveKeysStr = fmt.Sprintf("\n\nCritical information to preserve:\n- %s",
			strings.Join(c.config.PreserveKeys, "\n- "))
	}

	prompt := fmt.Sprintf(`Summarize the following conversation history concisely. Preserve:
- All tool execution results and their outcomes
- File paths mentioned or modified
- Decisions made and their rationale
- User preferences and corrections
- Current working state and context%s

Conversation:
%s

Provide a structured summary that a new AI assistant could use to continue this conversation seamlessly.
Format as: "## Conversation Summary\n[summary content]"`, preserveKeysStr, transcript.String())

	req := inference.ChatRequest{
		Messages: []inference.Message{
			{Role: "user", Content: prompt},
		},
		System:    "You are a conversation summarizer. Create concise but complete summaries that preserve all actionable context.",
		MaxTokens: 4096,
	}

	resp, err := c.provider.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("[Context compacted — %d messages summarized]\n\n%s",
		len(messages), resp.Message.Content), nil
}

// CompactResult contains information about a compaction operation.
type CompactResult struct {
	// OriginalMessages is the count before compaction.
	OriginalMessages int
	// CompactedMessages is the count after compaction.
	CompactedMessages int
	// OriginalTokens is the estimated tokens before.
	OriginalTokens int
	// CompactedTokens is the estimated tokens after.
	CompactedTokens int
}

// CompactWithResult performs compaction and returns detailed results.
func (c *Compactor) CompactWithResult(ctx gocontext.Context, history []inference.Message) ([]inference.Message, *CompactResult, error) {
	origTokens := EstimateTokens(history)
	origCount := len(history)

	compacted, err := c.Compact(ctx, history)
	if err != nil {
		return history, nil, err
	}

	return compacted, &CompactResult{
		OriginalMessages:  origCount,
		CompactedMessages: len(compacted),
		OriginalTokens:    origTokens,
		CompactedTokens:   EstimateTokens(compacted),
	}, nil
}
