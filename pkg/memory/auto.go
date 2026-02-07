package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/syntor/syntor/pkg/inference"
)

// AutoExtractor analyzes completed sessions and extracts reusable insights.
type AutoExtractor struct {
	provider inference.Provider
	manager  *Manager
}

// NewAutoExtractor creates an auto-memory extractor.
func NewAutoExtractor(provider inference.Provider, manager *Manager) *AutoExtractor {
	return &AutoExtractor{
		provider: provider,
		manager:  manager,
	}
}

// ExtractFromConversation analyzes a conversation and writes insights to memory.
func (e *AutoExtractor) ExtractFromConversation(ctx context.Context, messages []inference.Message) error {
	if len(messages) < 4 {
		return nil // Too short to extract meaningful patterns
	}

	// Build conversation transcript (limit to last 50 messages for efficiency)
	startIdx := 0
	if len(messages) > 50 {
		startIdx = len(messages) - 50
	}

	var transcript strings.Builder
	for _, m := range messages[startIdx:] {
		// Truncate very long messages
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		transcript.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, content))
	}

	// Ask LLM to extract insights
	prompt := fmt.Sprintf(`Analyze this conversation and extract reusable insights for future sessions.

Focus on:
1. User preferences and working patterns discovered
2. Mistakes made and lessons learned
3. Effective problem-solving strategies used
4. Project constraints or requirements uncovered
5. Tool usage patterns that worked well

Format each insight as a bullet point starting with "- ".
Only include genuinely reusable insights (not task-specific details).
Maximum 10 insights. Be concise.

Conversation:
%s`, transcript.String())

	req := inference.ChatRequest{
		Messages: []inference.Message{
			{Role: "user", Content: prompt},
		},
		System:    "You extract reusable insights from conversations. Be concise and practical.",
		MaxTokens: 1024,
	}

	resp, err := e.provider.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("extract insights: %w", err)
	}

	insights := resp.Message.Content
	if strings.TrimSpace(insights) == "" {
		return nil
	}

	// Write insights to global MEMORY.md
	entry := fmt.Sprintf("\n## Auto-Insights (Session)\n%s", insights)
	if err := e.manager.Write("global", entry); err != nil {
		return fmt.Errorf("write insights: %w", err)
	}

	// Truncate if needed
	return e.manager.TruncateMemory("global")
}

// ExtractAndCategorize extracts insights and sorts them into topic files.
func (e *AutoExtractor) ExtractAndCategorize(ctx context.Context, messages []inference.Message) error {
	if len(messages) < 4 {
		return nil
	}

	// Start with basic extraction
	return e.ExtractFromConversation(ctx, messages)
}
