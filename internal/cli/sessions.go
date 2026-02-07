package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/syntor/syntor/pkg/herald"
)

var (
	// Session flags
	resumeSession string
	forkSession   string
	sessionName   string
	trustTier     int
)

// sessionsCmd is the parent command for session management
var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage SYNTOR sessions",
	Long: `Manage SYNTOR sessions including create, list, resume, and fork operations.

Sessions allow you to:
  - Resume previous conversations
  - Fork sessions to explore different approaches
  - Manage session state and history

Examples:
  syntor sessions list              # List all sessions
  syntor sessions resume <id>       # Resume a session
  syntor sessions fork <id> <name>  # Fork a session`,
	Aliases: []string{"session", "sess"},
}

// sessionsListCmd lists all sessions
var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listSessions()
	},
}

// sessionsResumeCmd resumes a session
var sessionsResumeCmd = &cobra.Command{
	Use:   "resume <id-or-name>",
	Short: "Resume a previous session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resumeSession = args[0]
		return runInteractiveWithSession()
	},
}

// sessionsForkCmd forks a session
var sessionsForkCmd = &cobra.Command{
	Use:   "fork <id-or-name> [new-name]",
	Short: "Fork an existing session",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceSession := args[0]
		newName := ""
		if len(args) > 1 {
			newName = args[1]
		}
		return forkSessionCmd(sourceSession, newName)
	},
}

// sessionsDeleteCmd deletes a session
var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteSession(args[0])
	},
}

func init() {
	// Add session subcommands
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsResumeCmd)
	sessionsCmd.AddCommand(sessionsForkCmd)
	sessionsCmd.AddCommand(sessionsDeleteCmd)

	// Add sessions command to root
	rootCmd.AddCommand(sessionsCmd)

	// Add session flags to root command
	rootCmd.PersistentFlags().StringVar(&resumeSession, "resume", "", "resume session by ID or name")
	rootCmd.PersistentFlags().StringVar(&forkSession, "fork", "", "fork session by ID or name")
	rootCmd.PersistentFlags().StringVar(&sessionName, "session-name", "", "name for new session")
	rootCmd.PersistentFlags().IntVar(&trustTier, "trust-tier", 1, "trust tier for session (0-4)")
}

func listSessions() error {
	// Try Herald first
	heraldClient, err := getHeraldClient()
	if err == nil && heraldClient.IsEnabled() {
		if err := listHeraldSessions(heraldClient); err == nil {
			return nil
		}
		// Herald failed, fall through to local
		if verbose {
			fmt.Fprintf(os.Stderr, "Herald sessions unavailable, checking local...\n")
		}
	}

	// Fall back to local sessions
	return listLocalSessions()
}

func listHeraldSessions(client *herald.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessions, err := client.ListSessions(ctx, herald.ListSessionsFilter{
		Limit: 50,
	})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	fmt.Printf("%-12s %-20s %-10s %-15s %-20s\n", "ID", "NAME", "STATUS", "TRUST", "LAST ACTIVE")
	fmt.Println(string(make([]byte, 80)))

	for _, s := range sessions {
		lastActive := s.LastActive.Format("2006-01-02 15:04")
		if time.Since(s.LastActive) < 24*time.Hour {
			lastActive = time.Since(s.LastActive).Truncate(time.Minute).String() + " ago"
		}
		fmt.Printf("%-12s %-20s %-10s %-15s %-20s\n",
			s.ID,
			truncateStr(s.Name, 20),
			s.Status,
			s.TrustTier.String(),
			lastActive,
		)
	}

	return nil
}

func listLocalSessions() error {
	sessionsDir := getSessionsDir()
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No local sessions found.")
			return nil
		}
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No local sessions found.")
		return nil
	}

	fmt.Printf("%-12s %-30s %-20s\n", "ID", "NAME", "MODIFIED")
	fmt.Println(string(make([]byte, 65)))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		modTime := "unknown"
		if info != nil {
			modTime = info.ModTime().Format("2006-01-02 15:04")
		}
		fmt.Printf("%-12s %-30s %-20s\n", entry.Name(), entry.Name(), modTime)
	}

	return nil
}

func runInteractiveWithSession() error {
	// Set the resume session flag and run interactive
	if resumeSession != "" {
		os.Setenv("SYNTOR_RESUME_SESSION", resumeSession)
	}
	return runInteractive()
}

func forkSessionCmd(sourceID, newName string) error {
	heraldClient, err := getHeraldClient()
	if err != nil || !heraldClient.IsEnabled() {
		return forkLocalSession(sourceID, newName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := heraldClient.ForkSession(ctx, sourceID, newName)
	if err != nil {
		return fmt.Errorf("failed to fork session: %w", err)
	}

	fmt.Printf("Forked session %s as %s (ID: %s)\n", sourceID, session.Name, session.ID)
	return nil
}

func forkLocalSession(sourceID, newName string) error {
	sessionsDir := getSessionsDir()
	sourceDir := filepath.Join(sessionsDir, sourceID)

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", sourceID)
	}

	if newName == "" {
		newName = fmt.Sprintf("%s-fork-%d", sourceID, time.Now().Unix())
	}

	destDir := filepath.Join(sessionsDir, newName)
	if err := copyDir(sourceDir, destDir); err != nil {
		return fmt.Errorf("failed to fork session: %w", err)
	}

	fmt.Printf("Forked session %s as %s\n", sourceID, newName)
	return nil
}

func deleteSession(sessionID string) error {
	heraldClient, err := getHeraldClient()
	if err == nil && heraldClient.IsEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := heraldClient.TerminateSession(ctx, sessionID); err != nil {
			return fmt.Errorf("failed to delete session: %w", err)
		}
		fmt.Printf("Deleted session %s\n", sessionID)
		return nil
	}

	// Delete local session
	sessionsDir := getSessionsDir()
	sessionDir := filepath.Join(sessionsDir, sessionID)

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	fmt.Printf("Deleted session %s\n", sessionID)
	return nil
}

func getHeraldClient() (*herald.Client, error) {
	if syntorConfig == nil || !syntorConfig.Integrations.Herald.Enabled {
		return nil, fmt.Errorf("herald not configured")
	}
	heraldCfg := syntorConfig.Integrations.Herald.ToHeraldConfig()
	return herald.New(heraldCfg)
}

func getSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".syntor", "sessions")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func copyDir(src, dst string) error {
	// Simple directory copy implementation
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}
