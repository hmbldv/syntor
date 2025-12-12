package tui

import (
	"github.com/charmbracelet/glamour"
)

// MarkdownRenderer handles markdown rendering for the TUI
type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// NewMarkdownRenderer creates a new markdown renderer with custom styling
func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	// Use dark style explicitly to avoid terminal color detection escape sequences
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	return &MarkdownRenderer{
		renderer: r,
		width:    width,
	}, nil
}

// Render renders markdown content to styled terminal output
func (m *MarkdownRenderer) Render(content string) (string, error) {
	return m.renderer.Render(content)
}

// UpdateWidth updates the word wrap width
func (m *MarkdownRenderer) UpdateWidth(width int) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}
	m.renderer = r
	m.width = width
	return nil
}
