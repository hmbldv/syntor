package tui

// Nerd Font icons for TUI activity indicators
// Requires JetBrains Mono Nerd Font or compatible Nerd Font installed
// Reference: https://www.nerdfonts.com/cheat-sheet

// Activity icons - displayed during different operation types
const (
	// Core activity icons
	IconThinking  = "\uF09D1" // 󰧑 nf-md-head_cog
	IconStreaming = "\uF09A8" // 󰦨 nf-md-text_box_outline
	IconPlanning  = "\uF0756" // 󰝖 nf-md-clipboard_list
	IconSearching = "\uF0349" // 󰍉 nf-md-magnify
	IconTools     = "\uF0493" // 󰒓 nf-md-wrench
	IconAgent     = "\uF544"  //  nf-fa-robot
	IconLoading   = "\uF253"  //  nf-fa-hourglass_half

	// File operation icons
	IconFileRead   = "\uF0219" // 󰈙 nf-md-file_document
	IconFileCreate = "\uF0752" // 󰝒 nf-md-file_plus
	IconFileEdit   = "\uF0DC8" // 󰷈 nf-md-file_edit

	// Git icons
	IconGit       = "\uE702"  //  nf-dev-git
	IconGitCommit = "\uF0718" // 󰜘 nf-md-source_commit

	// Terminal/Bash icon
	IconBash = "\uEBAF" //  nf-cod-terminal_bash

	// Search/Grep icon
	IconGrep = "\uF0451" // 󰑑 nf-md-text_search

	// Status icons
	IconSuccess = "\uF00C" //  nf-fa-check
	IconError   = "\uF00D" //  nf-fa-times
	IconWarning = "\uF071" //  nf-fa-warning
	IconInfo    = "\uF129" //  nf-fa-info

	// Handoff/delegation icons
	IconHandoff = "\uF0EC"  //  nf-fa-exchange
	IconRoute   = "\uF0A73" // 󰩳 nf-md-directions

	// Plan mode icons
	IconPlanMode = "\uF0774" // 󰝴 nf-md-clipboard_check
	IconAutoMode = "\uF04BB" // 󰒻 nf-md-lightning_bolt
)

// Agent-specific icons
const (
	IconSNTR    = "\uF0AD1" // 󰫑 nf-md-crown (orchestrator)
	IconWorker  = "\uF01A7" // 󰆧 nf-md-account_hard_hat
	IconCode    = "\uF121"  //  nf-fa-code
	IconDocs    = "\uF0219" // 󰈙 nf-md-file_document
	IconGitAgent = "\uE702" //  nf-dev-git
	IconPaladin = "\uF0513" // 󰔓 nf-md-shield_check (security)
)

// ActivityIcons maps activity types to their icons
var ActivityIcons = map[string]string{
	"thinking":  IconThinking,
	"streaming": IconStreaming,
	"planning":  IconPlanning,
	"searching": IconSearching,
	"tools":     IconTools,
	"agent":     IconAgent,
	"loading":   IconLoading,
	"handoff":   IconHandoff,
	"routing":   IconRoute,
}

// ToolIcons maps tool names to their icons
var ToolIcons = map[string]string{
	"read_file":      IconFileRead,
	"write_file":     IconFileCreate,
	"edit_file":      IconFileEdit,
	"list_directory": IconSearching,
	"bash":           IconBash,
	"grep":           IconGrep,
	"glob":           IconSearching,
}

// AgentIcons maps agent names to their icons
var AgentIcons = map[string]string{
	"sntr":          IconSNTR,
	"coordination":  IconSNTR,
	"worker":        IconWorker,
	"code":          IconCode,
	"docs":          IconDocs,
	"documentation": IconDocs,
	"git":           IconGitAgent,
	"paladin":       IconPaladin,
	"security":      IconPaladin,
}

// GetActivityIcon returns the icon for an activity type
func GetActivityIcon(activityType string) string {
	if icon, ok := ActivityIcons[activityType]; ok {
		return icon
	}
	return IconLoading
}

// GetToolIcon returns the icon for a tool name
func GetToolIcon(toolName string) string {
	if icon, ok := ToolIcons[toolName]; ok {
		return icon
	}
	return IconTools
}

// GetAgentIcon returns the icon for an agent name
func GetAgentIcon(agentName string) string {
	if icon, ok := AgentIcons[agentName]; ok {
		return icon
	}
	return IconAgent
}

// StatusIcon returns the appropriate status icon
func StatusIcon(success bool) string {
	if success {
		return IconSuccess
	}
	return IconError
}
