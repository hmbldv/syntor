package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Colors - Forest green dark theme (matching thehumble.dev)
var (
	// Primary accent - forest green
	primaryColor = lipgloss.Color("#4ade80") // Bright forest green
	// Secondary - muted gray-green
	secondaryColor = lipgloss.Color("#6b7b6b") // Gray-green
	// Success - lighter green
	successColor = lipgloss.Color("#22c55e") // Green
	// Error - soft red
	errorColor = lipgloss.Color("#ef4444") // Red
	// Warning - amber/yellow
	warningColor = lipgloss.Color("#eab308") // Amber
	// Accent - complementary teal
	accentColor = lipgloss.Color("#2dd4bf") // Teal

	// Background colors (for reference, terminals handle bg)
	bgPrimary   = lipgloss.Color("#0f1410") // Very dark green-black
	bgSecondary = lipgloss.Color("#1a211a") // Dark green-gray
)

// Styles defines all the visual styles for the TUI
type Styles struct {
	// Header styles
	Header      lipgloss.Style
	HeaderAgent lipgloss.Style
	HeaderModel lipgloss.Style

	// Chat styles
	UserPrompt    lipgloss.Style
	UserMessage   lipgloss.Style
	AgentResponse lipgloss.Style
	SystemMessage lipgloss.Style

	// Input styles
	InputPrompt      lipgloss.Style
	InputText        lipgloss.Style
	InputPlaceholder lipgloss.Style

	// Autocomplete styles
	SuggestionBox      lipgloss.Style
	SuggestionItem     lipgloss.Style
	SuggestionSelected lipgloss.Style
	SuggestionDesc     lipgloss.Style

	// Status bar styles
	StatusBar      lipgloss.Style
	StatusAgent    lipgloss.Style
	StatusModel    lipgloss.Style
	StatusStreaming lipgloss.Style

	// Help bar styles
	HelpBar     lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style

	// Activity status styles
	ActivityBar      lipgloss.Style
	ActivityIcon     lipgloss.Style
	ActivityText     lipgloss.Style
	ActivityDuration lipgloss.Style

	// Code block styles
	CodeBlock       lipgloss.Style
	CodeBlockHeader lipgloss.Style
	CodeBlockLang   lipgloss.Style
	CodeBlockCopy   lipgloss.Style
	CodeContent     lipgloss.Style
	InlineCode      lipgloss.Style

	// Separators
	InputSeparator lipgloss.Style

	// General
	Error   lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
}

// DefaultStyles returns the default style configuration
func DefaultStyles() Styles {
	return Styles{
		// Header
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(secondaryColor).
			Padding(0, 1),

		HeaderAgent: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor),

		HeaderModel: lipgloss.NewStyle().
			Foreground(secondaryColor),

		// Chat
		UserPrompt: lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor),

		UserMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e5e7eb")), // Light gray text

		AgentResponse: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d1d5db")), // Slightly muted gray

		SystemMessage: lipgloss.NewStyle().
			Italic(true).
			Foreground(secondaryColor),

		// Input
		InputPrompt: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor),

		InputText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")),

		InputPlaceholder: lipgloss.NewStyle().
			Foreground(secondaryColor),

		// Autocomplete
		SuggestionBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Background(bgSecondary).
			Padding(0, 1),

		SuggestionItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d1d5db")).
			Padding(0, 1),

		SuggestionSelected: lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(bgPrimary).
			Bold(true).
			Padding(0, 1),

		SuggestionDesc: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Background(bgSecondary).
			Padding(0, 1),

		StatusAgent: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor),

		StatusModel: lipgloss.NewStyle().
			Foreground(secondaryColor),

		StatusStreaming: lipgloss.NewStyle().
			Foreground(warningColor).
			Italic(true),

		// Help bar
		HelpBar: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Padding(0, 1),

		HelpKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor),

		HelpDesc: lipgloss.NewStyle().
			Foreground(secondaryColor),

		// Activity status
		ActivityBar: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Background(bgSecondary).
			Padding(0, 1),

		ActivityIcon: lipgloss.NewStyle().
			Foreground(primaryColor),

		ActivityText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d1d5db")).
			Italic(true),

		ActivityDuration: lipgloss.NewStyle().
			Foreground(secondaryColor),

		// Code blocks - compact style
		CodeBlock: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor).
			Padding(0, 1),

		CodeBlockHeader: lipgloss.NewStyle().
			Foreground(secondaryColor),

		CodeBlockLang: lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true),

		CodeBlockCopy: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true),

		CodeContent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e5e7eb")),

		InlineCode: lipgloss.NewStyle().
			Foreground(accentColor),

		// Separators
		InputSeparator: lipgloss.NewStyle().
			Foreground(secondaryColor),

		// General
		Error: lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor),

		Success: lipgloss.NewStyle().
			Bold(true).
			Foreground(successColor),

		Warning: lipgloss.NewStyle().
			Bold(true).
			Foreground(warningColor),
	}
}
