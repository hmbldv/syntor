package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// Nerd Font spinner using circle slice animation
// These are the nf-md-circle_slice icons that animate smoothly
var NerdSpinnerFrames = []string{
	"\uF0A9E", // 󰪞 nf-md-circle_slice_1
	"\uF0A9F", // 󰪟 nf-md-circle_slice_2
	"\uF0AA0", // 󰪠 nf-md-circle_slice_3
	"\uF0AA1", // 󰪡 nf-md-circle_slice_4
	"\uF0AA2", // 󰪢 nf-md-circle_slice_5
	"\uF0AA3", // 󰪣 nf-md-circle_slice_6
	"\uF0AA4", // 󰪤 nf-md-circle_slice_7
	"\uF0AA5", // 󰪥 nf-md-circle_slice_8
}

// NerdSpinner is an animated spinner using Nerd Font circle slice icons
var NerdSpinner = spinner.Spinner{
	Frames: NerdSpinnerFrames,
	FPS:    time.Second / 12,
}

// Alternative spinners for different contexts

// DotsSpinner uses Braille dots for a subtle animation
var DotsSpinner = spinner.Spinner{
	Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	FPS:    time.Second / 10,
}

// PulseSpinner uses filled/empty circles
var PulseSpinner = spinner.Spinner{
	Frames: []string{"\uF111", "\uF10C"}, //  nf-fa-circle,  nf-fa-circle_o
	FPS:    time.Second / 2,
}

// GearSpinner for tool operations
var GearSpinner = spinner.Spinner{
	Frames: []string{
		"\uF013", // nf-fa-gear
		"\uF085", // nf-fa-gears (rotated appearance)
		"\uF013",
		"\uF085",
	},
	FPS: time.Second / 4,
}

// SpinnerType defines which spinner style to use
type SpinnerType int

const (
	SpinnerNerd SpinnerType = iota
	SpinnerDots
	SpinnerPulse
	SpinnerGear
)

// GetSpinner returns a spinner model configured for the given type
func GetSpinner(t SpinnerType) spinner.Model {
	s := spinner.New()
	switch t {
	case SpinnerNerd:
		s.Spinner = NerdSpinner
	case SpinnerDots:
		s.Spinner = DotsSpinner
	case SpinnerPulse:
		s.Spinner = PulseSpinner
	case SpinnerGear:
		s.Spinner = GearSpinner
	default:
		s.Spinner = NerdSpinner
	}
	return s
}

// SpinnerForActivity returns the appropriate spinner for an activity type
func SpinnerForActivity(activityType string) spinner.Model {
	switch activityType {
	case "tools":
		return GetSpinner(SpinnerGear)
	case "thinking", "streaming":
		return GetSpinner(SpinnerNerd)
	default:
		return GetSpinner(SpinnerNerd)
	}
}

// AnimatedFrame returns the current frame for a manual animation
// based on elapsed time and frame duration
type AnimatedFrame struct {
	frames   []string
	duration time.Duration
}

// NewAnimatedFrame creates a new animated frame helper
func NewAnimatedFrame(frames []string, fps int) *AnimatedFrame {
	return &AnimatedFrame{
		frames:   frames,
		duration: time.Second / time.Duration(fps),
	}
}

// Frame returns the current frame based on elapsed time
func (a *AnimatedFrame) Frame(elapsed time.Duration) string {
	if len(a.frames) == 0 {
		return ""
	}
	frameIndex := int(elapsed/a.duration) % len(a.frames)
	return a.frames[frameIndex]
}

// NerdAnimatedSpinner is a helper for manual spinner animation
var NerdAnimatedSpinner = NewAnimatedFrame(NerdSpinnerFrames, 12)
