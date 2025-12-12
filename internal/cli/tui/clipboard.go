package tui

import (
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard copies text to the system clipboard
func CopyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			// Wayland support
			cmd = exec.Command("wl-copy")
		} else {
			// Fallback: try xclip anyway
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// HasClipboardSupport checks if clipboard commands are available
func HasClipboardSupport() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("pbcopy")
		return err == nil
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			return true
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return true
		}
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return true
		}
		return false
	case "windows":
		return true // clip.exe is always available
	default:
		return false
	}
}
