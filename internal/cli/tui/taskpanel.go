package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// maxVisibleTasks is the maximum number of tasks shown in the panel.
const maxVisibleTasks = 10

// TaskItem represents a tracked sub-agent task in the UI.
type TaskItem struct {
	ID        string
	Subject   string
	Status    string // "pending", "in_progress", "completed", "failed"
	Owner     string // agent name
	BlockedBy []string
	StartTime time.Time
	Duration  time.Duration
}

// TaskPanel is a Bubbletea component that displays active sub-agent tasks.
type TaskPanel struct {
	mu      sync.RWMutex
	visible bool
	tasks   []TaskItem
	width   int
	styles  taskPanelStyles
}

// taskPanelStyles holds styles specific to the task panel.
type taskPanelStyles struct {
	border    lipgloss.Style
	title     lipgloss.Style
	pending   lipgloss.Style
	active    lipgloss.Style
	completed lipgloss.Style
	failed    lipgloss.Style
	blocked   lipgloss.Style
	owner     lipgloss.Style
	duration  lipgloss.Style
	count     lipgloss.Style
}

func newTaskPanelStyles() taskPanelStyles {
	return taskPanelStyles{
		border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor).
			Padding(0, 1),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor),
		pending: lipgloss.NewStyle().
			Foreground(secondaryColor),
		active: lipgloss.NewStyle().
			Foreground(warningColor),
		completed: lipgloss.NewStyle().
			Foreground(successColor),
		failed: lipgloss.NewStyle().
			Foreground(errorColor),
		blocked: lipgloss.NewStyle().
			Foreground(errorColor).
			Italic(true),
		owner: lipgloss.NewStyle().
			Foreground(accentColor),
		duration: lipgloss.NewStyle().
			Foreground(secondaryColor),
		count: lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true),
	}
}

// NewTaskPanel creates a new TaskPanel with the given width.
func NewTaskPanel(width int) *TaskPanel {
	return &TaskPanel{
		width:  width,
		styles: newTaskPanelStyles(),
	}
}

// AddTask appends a task to the panel.
func (p *TaskPanel) AddTask(item TaskItem) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tasks = append(p.tasks, item)
}

// UpdateTask sets the status for a task by ID.
// If the status is "in_progress" and no start time was set, it records now.
func (p *TaskPanel) UpdateTask(id, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.tasks {
		if p.tasks[i].ID == id {
			p.tasks[i].Status = status
			if status == "in_progress" && p.tasks[i].StartTime.IsZero() {
				p.tasks[i].StartTime = time.Now()
			}
			if status == "completed" || status == "failed" {
				if !p.tasks[i].StartTime.IsZero() {
					p.tasks[i].Duration = time.Since(p.tasks[i].StartTime)
				}
			}
			return
		}
	}
}

// RemoveTask removes a task by ID.
func (p *TaskPanel) RemoveTask(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.tasks {
		if p.tasks[i].ID == id {
			p.tasks = append(p.tasks[:i], p.tasks[i+1:]...)
			return
		}
	}
}

// Toggle flips the panel visibility.
func (p *TaskPanel) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.visible = !p.visible
}

// Visible returns whether the panel is currently shown.
func (p *TaskPanel) Visible() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.visible
}

// SetWidth updates the panel render width.
func (p *TaskPanel) SetWidth(w int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.width = w
}

// View renders the task panel. Returns empty string when hidden or no tasks.
func (p *TaskPanel) View() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.visible || len(p.tasks) == 0 {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(p.styles.title.Render(fmt.Sprintf(" %s Tasks", IconAgent)))
	b.WriteString("\n")

	// Render up to maxVisibleTasks
	visible := p.tasks
	overflow := 0
	if len(visible) > maxVisibleTasks {
		overflow = len(visible) - maxVisibleTasks
		visible = visible[:maxVisibleTasks]
	}

	for _, t := range visible {
		b.WriteString(p.renderTask(t))
		b.WriteString("\n")
	}

	if overflow > 0 {
		b.WriteString(p.styles.count.Render(fmt.Sprintf("  +%d more tasks", overflow)))
		b.WriteString("\n")
	}

	// Apply border
	content := strings.TrimRight(b.String(), "\n")
	panelWidth := p.width
	if panelWidth <= 0 {
		panelWidth = 40
	}
	return p.styles.border.Width(panelWidth - 2).Render(content)
}

// renderTask formats a single task line.
func (p *TaskPanel) renderTask(t TaskItem) string {
	icon := p.statusIcon(t.Status)

	// Owner display
	owner := ""
	if t.Owner != "" {
		agentIcon := GetAgentIcon(t.Owner)
		owner = p.styles.owner.Render(fmt.Sprintf(" %s %s", agentIcon, t.Owner))
	}

	// Duration display
	dur := ""
	if t.Status == "in_progress" && !t.StartTime.IsZero() {
		elapsed := time.Since(t.StartTime).Truncate(time.Second)
		dur = p.styles.duration.Render(fmt.Sprintf(" [%s]", elapsed))
	} else if t.Duration > 0 {
		dur = p.styles.duration.Render(fmt.Sprintf(" [%s]", t.Duration.Truncate(time.Second)))
	}

	line := fmt.Sprintf("%s %s%s%s", icon, t.Subject, owner, dur)

	// Blocked indicator
	if len(t.BlockedBy) > 0 && t.Status == "pending" {
		blocked := p.styles.blocked.Render(fmt.Sprintf("  \u2298 blocked by: %s", strings.Join(t.BlockedBy, ", ")))
		line += "\n" + blocked
	}

	return line
}

// statusIcon returns the styled icon for a task status.
func (p *TaskPanel) statusIcon(status string) string {
	switch status {
	case "pending":
		return p.styles.pending.Render("\u25CB")  // ○
	case "in_progress":
		// Animated spinner based on elapsed time
		frame := NerdAnimatedSpinner.Frame(time.Since(time.Now()))
		return p.styles.active.Render(frame)
	case "completed":
		return p.styles.completed.Render("\u2713") // ✓
	case "failed":
		return p.styles.failed.Render("\u2717")    // ✗
	default:
		return p.styles.pending.Render("\u25CB")
	}
}
