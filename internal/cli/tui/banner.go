package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Banner colors - gradient effect using forest green theme
var (
	bannerColor1 = lipgloss.Color("#4ade80") // Brightest green
	bannerColor2 = lipgloss.Color("#3fcc73")
	bannerColor3 = lipgloss.Color("#35ba66")
	bannerColor4 = lipgloss.Color("#2ba859")
	bannerColor5 = lipgloss.Color("#22964c")
	bannerDim    = lipgloss.Color("#6b7b6b") // Muted gray-green
)

// GetStartupBanner returns the styled ASCII art banner
func GetStartupBanner(version string, width int) string {
	if width < 60 {
		width = 60
	}

	var sb strings.Builder

	// ASCII art lines (raw, before styling)
	artLines := []string{
		"░██████╗██╗░░░██╗███╗░░██╗████████╗░█████╗░██████╗░",
		"██╔════╝╚██╗░██╔╝████╗░██║╚══██╔══╝██╔══██╗██╔══██╗",
		"╚█████╗░░╚████╔╝░██╔██╗██║░░░██║░░░██║░░██║██████╔╝",
		"░╚═══██╗░░╚██╔╝░░██║╚████║░░░██║░░░██║░░██║██╔══██╗",
		"██████╔╝░░░██║░░░██║░╚███║░░░██║░░░╚█████╔╝██║░░██║",
		"╚═════╝░░░░╚═╝░░░╚═╝░░╚══╝░░░╚═╝░░░░╚════╝░╚═╝░░╚═╝",
	}

	colors := []lipgloss.Color{bannerColor1, bannerColor2, bannerColor3, bannerColor4, bannerColor5, bannerDim}

	// Calculate centering padding
	artWidth := 51 // Width of the ASCII art
	padding := (width - artWidth) / 2
	if padding < 0 {
		padding = 0
	}
	padStr := strings.Repeat(" ", padding)

	sb.WriteString("\n")
	for i, line := range artLines {
		style := lipgloss.NewStyle().Foreground(colors[i])
		if i < 5 {
			style = style.Bold(true)
		}
		sb.WriteString(padStr + style.Render(line) + "\n")
	}
	sb.WriteString("\n")

	// Tagline and version - centered
	tagline := lipgloss.NewStyle().Foreground(bannerDim).Render("Synthetic Orchestrator")
	versionStr := lipgloss.NewStyle().Foreground(bannerDim).Render(version)

	sb.WriteString(centerText(tagline, width) + "\n")
	sb.WriteString(centerText(versionStr, width) + "\n")
	sb.WriteString("\n")

	return sb.String()
}

// GetModernHeader returns a modern styled header for the TUI
func GetModernHeader(width int) string {
	if width < 40 {
		width = 40
	}

	// Create the header components
	logoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(bannerColor1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(bannerDim).
		Italic(true)

	// Mini logo
	logo := logoStyle.Render("◈ SYNTOR")
	subtitle := subtitleStyle.Render("Interactive Mode")

	// Decorative line
	lineStyle := lipgloss.NewStyle().Foreground(bannerDim)
	lineWidth := width - lipgloss.Width(logo) - lipgloss.Width(subtitle) - 6
	if lineWidth < 4 {
		lineWidth = 4
	}
	line := lineStyle.Render(strings.Repeat("─", lineWidth))

	// Compose header
	header := fmt.Sprintf("%s %s %s", logo, line, subtitle)

	return header
}

// GetCompactHeader returns a minimal header for smaller terminals
func GetCompactHeader() string {
	logoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(bannerColor1)

	return logoStyle.Render("◈ SYNTOR")
}

// centerText centers text within a given width
func centerText(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}
	padding := (width - textWidth) / 2
	return strings.Repeat(" ", padding) + text
}

// GetWelcomeMessage returns the welcome message shown after the banner
func GetWelcomeMessage() string {
	style := lipgloss.NewStyle().Foreground(bannerDim)

	lines := []string{
		"",
		style.Render("  Type a message to chat with the AI agent"),
		style.Render("  Use /help for available commands"),
		style.Render("  Press Ctrl+A to toggle Auto/Plan mode"),
		"",
	}

	return strings.Join(lines, "\n")
}
