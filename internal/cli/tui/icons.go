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
	// Orchestration
	IconSNTR     = "\uF0AD1" // 󰫑 nf-md-crown (orchestrator)
	IconWorker   = "\uF01A7" // 󰆧 nf-md-account_hard_hat
	IconCode     = "\uF121"  //  nf-fa-code
	IconDocs     = "\uF0219" // 󰈙 nf-md-file_document
	IconGitAgent = "\uE702"  //  nf-dev-git

	// Security (CRBRS team)
	IconPaladin = "\uF0513" // 󰔓 nf-md-shield_check (CISO)
	IconNEXUS   = "\uF0212" // 󰈒 nf-md-file_link (coordination)
	IconHRDN    = "\uF0510" // 󰔐 nf-md-shield_lock (hardening)
	IconDART    = "\uF0E6E" // 󰹮 nf-md-radar (detection)
	IconGHST    = "\uF20F"  //  nf-fa-user_secret (offensive)
	IconPROBE   = "\uF0341" // 󰍁 nf-md-magnify_scan (forensics)

	// Research (Axiom team)
	IconAxiom  = "\uF0341" // 󰍁 nf-md-magnify_scan (research)
	IconThesis = "\uF0B00" // 󰬀 nf-md-head_lightbulb (research lead)
	IconProof  = "\uF0667" // 󰙧 nf-md-check_decagram (verification)
	IconCite   = "\uF018D" // 󰆍 nf-md-bookmark (citations)
	IconQuery  = "\uF0349" // 󰍉 nf-md-magnify (queries)

	// Communications (SIGNAL team)
	IconSignal = "\uF0A80" // 󰪀 nf-md-broadcast (communications)
	IconChorus = "\uF0AD2" // 󰫒 nf-md-crown_circle (comms lead)
	IconPulse  = "\uF05F5" // 󰗵 nf-md-heart_pulse (PM)
	IconBeacon = "\uF0335" // 󰌵 nf-md-lightbulb (outreach)
	IconPolish = "\uF0DD8" // 󰷘 nf-md-format_paint (editor)
	IconBrief  = "\uF0219" // 󰈙 nf-md-file_document (summaries)

	// Personal Brand (BRND team)
	IconBRND   = "\uF0651" // 󰙑 nf-md-star_circle (brand)
	IconMarq   = "\uF0E32" // 󰸲 nf-md-diamond_stone (brand lead)
	IconHerald = "\uF04DE" // 󰓞 nf-md-send (brand PM)
	IconLinked = "\uF0D18" // 󱄘 nf-md-linkedin (linkedin)
	IconResume = "\uF0219" // 󰈙 nf-md-file_document (resume)

	// Development (FOUNDRY team)
	IconFoundry = "\uF0864" // 󰡤 nf-md-anvil (development)
	IconAnvil   = "\uF0864" // 󰡤 nf-md-anvil (dev lead)
	IconSpark   = "\uF0E4F" // 󰹏 nf-md-lightning_bolt (PM)
	IconCraft   = "\uF0493" // 󰒓 nf-md-wrench (implementation)
	IconAssay   = "\uF0668" // 󰙨 nf-md-test_tube (testing)
	IconFrame   = "\uF0E25" // 󰸥 nf-md-application (frontend)
	IconGlyph   = "\uF0312" // 󰌒 nf-md-image (assets)

	// Finance (BARAKA team)
	IconBaraka = "\uF0306" // 󰌆 nf-md-currency_usd (finance)
	IconAMIL   = "\uF0306" // 󰌆 nf-md-currency_usd (finance lead)
	IconWAKIL  = "\uF04DE" // 󰓞 nf-md-send (finance PM)
	IconDAYN   = "\uF0E8E" // 󰺎 nf-md-cash_multiple (debt)
	IconHALAL  = "\uF05E0" // 󰗠 nf-md-check_circle (compliance)
	IconKANZ   = "\uF0870" // 󰡰 nf-md-treasure_chest (savings)

	// Infrastructure
	IconHive   = "\uF01BC" // 󰆼 nf-md-database (database)
	IconKuber  = "\uF10FE" // 󱃾 nf-md-kubernetes (kubernetes)
	IconNetty  = "\uF0318" // 󰌘 nf-md-lan (network)
	IconTriage = "\uF0E1F" // 󰸟 nf-md-stethoscope (diagnostics)

	// Agent Architecture (AGNT team)
	IconAGNT     = "\uF1246" // 󱉆 nf-md-robot_outline (agent design)
	IconAPEX     = "\uF1246" // 󱉆 nf-md-robot_outline (agent lead)
	IconDispatch = "\uF04DE" // 󰓞 nf-md-send (task dispatch)
	IconMatrix   = "\uF0626" // 󰘦 nf-md-grid (agent grid)
	IconForge    = "\uF0864" // 󰡤 nf-md-anvil (agent forge)
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
	// Orchestration
	"sntr":          IconSNTR,
	"SNTR":          IconSNTR,
	"coordination":  IconSNTR,
	"worker":        IconWorker,
	"code":          IconCode,
	"Coder":         IconCode,
	"docs":          IconDocs,
	"documentation": IconDocs,
	"git":           IconGitAgent,

	// Security (CRBRS)
	"PALADIN": IconPaladin,
	"paladin": IconPaladin,
	"security": IconPaladin,
	"NEXUS":   IconNEXUS,
	"nexus":   IconNEXUS,
	"HRDN":    IconHRDN,
	"hrdn":    IconHRDN,
	"DART":    IconDART,
	"dart":    IconDART,
	"GHST":    IconGHST,
	"ghst":    IconGHST,
	"PROBE":   IconPROBE,
	"probe":   IconPROBE,

	// Research (Axiom)
	"Axiom":  IconAxiom,
	"axiom":  IconAxiom,
	"Thesis": IconThesis,
	"thesis": IconThesis,
	"Proof":  IconProof,
	"proof":  IconProof,
	"Cite":   IconCite,
	"cite":   IconCite,
	"Query":  IconQuery,
	"query":  IconQuery,

	// Communications (SIGNAL)
	"SIGNAL": IconSignal,
	"signal": IconSignal,
	"Chorus": IconChorus,
	"chorus": IconChorus,
	"Pulse":  IconPulse,
	"pulse":  IconPulse,
	"Beacon": IconBeacon,
	"beacon": IconBeacon,
	"Polish": IconPolish,
	"polish": IconPolish,
	"Brief":  IconBrief,
	"brief":  IconBrief,
	"Editor": IconPolish,
	"editor": IconPolish,

	// Personal Brand (BRND)
	"BRND":   IconBRND,
	"brnd":   IconBRND,
	"Marq":   IconMarq,
	"marq":   IconMarq,
	"Herald": IconHerald,
	"herald": IconHerald,

	// Development (FOUNDRY)
	"FOUNDRY": IconFoundry,
	"foundry": IconFoundry,
	"ANVIL":   IconAnvil,
	"anvil":   IconAnvil,
	"Spark":   IconSpark,
	"spark":   IconSpark,
	"Craft":   IconCraft,
	"craft":   IconCraft,
	"Assay":   IconAssay,
	"assay":   IconAssay,
	"Frame":   IconFrame,
	"frame":   IconFrame,
	"Glyph":   IconGlyph,
	"glyph":   IconGlyph,

	// Finance (BARAKA)
	"BARAKA": IconBaraka,
	"baraka": IconBaraka,
	"AMIL":   IconAMIL,
	"amil":   IconAMIL,
	"WAKIL":  IconWAKIL,
	"wakil":  IconWAKIL,
	"DAYN":   IconDAYN,
	"dayn":   IconDAYN,
	"HALAL":  IconHALAL,
	"halal":  IconHALAL,
	"KANZ":   IconKANZ,
	"kanz":   IconKANZ,

	// Infrastructure
	"Hive":   IconHive,
	"hive":   IconHive,
	"Kuber":  IconKuber,
	"kuber":  IconKuber,
	"Netty":  IconNetty,
	"netty":  IconNetty,
	"TRIAGE": IconTriage,
	"triage": IconTriage,

	// Agent Architecture (AGNT)
	"AGNT":     IconAGNT,
	"agnt":     IconAGNT,
	"APEX":     IconAPEX,
	"apex":     IconAPEX,
	"Dispatch": IconDispatch,
	"dispatch": IconDispatch,
	"Matrix":   IconMatrix,
	"matrix":   IconMatrix,
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
