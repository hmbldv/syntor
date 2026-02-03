package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/syntor/syntor/pkg/checkpoint"
)

// checkpointsCmd is the parent command for checkpoint management
var checkpointsCmd = &cobra.Command{
	Use:   "checkpoints",
	Short: "Manage session checkpoints",
	Long: `Manage checkpoints for session state recovery.

Checkpoints allow you to:
  - Save session state at specific points
  - Restore to previous states (rewind)
  - Compare changes between checkpoints

Examples:
  syntor checkpoints list           # List checkpoints
  syntor checkpoints create         # Create a checkpoint
  syntor checkpoints restore <id>   # Restore a checkpoint
  syntor checkpoints diff <id>      # Compare with current state`,
	Aliases: []string{"checkpoint", "cp"},
}

// checkpointsListCmd lists checkpoints
var checkpointsListCmd = &cobra.Command{
	Use:   "list [session-id]",
	Short: "List checkpoints for a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := "default"
		if len(args) > 0 {
			sessionID = args[0]
		}
		return listCheckpoints(sessionID)
	},
}

// checkpointsCreateCmd creates a checkpoint
var checkpointsCreateCmd = &cobra.Command{
	Use:   "create [files...]",
	Short: "Create a new checkpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		sessionID, _ := cmd.Flags().GetString("session")
		if sessionID == "" {
			sessionID = "default"
		}
		return createCheckpoint(sessionID, name, desc, args)
	},
}

// checkpointsRestoreCmd restores a checkpoint
var checkpointsRestoreCmd = &cobra.Command{
	Use:   "restore <checkpoint-id>",
	Short: "Restore session state from a checkpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backup, _ := cmd.Flags().GetBool("backup")
		return restoreCheckpoint(args[0], backup)
	},
}

// checkpointsDiffCmd compares checkpoints
var checkpointsDiffCmd = &cobra.Command{
	Use:   "diff <checkpoint-id> [other-checkpoint-id]",
	Short: "Compare checkpoint with current state or another checkpoint",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		otherID := ""
		if len(args) > 1 {
			otherID = args[1]
		}
		return diffCheckpoint(args[0], otherID)
	},
}

// checkpointsDeleteCmd deletes a checkpoint
var checkpointsDeleteCmd = &cobra.Command{
	Use:   "delete <checkpoint-id>",
	Short: "Delete a checkpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return deleteCheckpoint(args[0])
	},
}

func init() {
	// Add checkpoint subcommands
	checkpointsCmd.AddCommand(checkpointsListCmd)
	checkpointsCmd.AddCommand(checkpointsCreateCmd)
	checkpointsCmd.AddCommand(checkpointsRestoreCmd)
	checkpointsCmd.AddCommand(checkpointsDiffCmd)
	checkpointsCmd.AddCommand(checkpointsDeleteCmd)

	// Add flags
	checkpointsCreateCmd.Flags().StringP("name", "n", "", "checkpoint name")
	checkpointsCreateCmd.Flags().StringP("description", "d", "", "checkpoint description")
	checkpointsCreateCmd.Flags().StringP("session", "s", "", "session ID")
	checkpointsRestoreCmd.Flags().BoolP("backup", "b", true, "create backup before restore")

	// Add checkpoints command to root
	rootCmd.AddCommand(checkpointsCmd)
}

func getCheckpointManager() (*checkpoint.Manager, error) {
	config := checkpoint.DefaultStorageConfig()
	policy := checkpoint.DefaultPolicyConfig()
	return checkpoint.NewManager(config, policy)
}

func listCheckpoints(sessionID string) error {
	mgr, err := getCheckpointManager()
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summaries, err := mgr.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Printf("No checkpoints found for session %s\n", sessionID)
		return nil
	}

	fmt.Printf("Checkpoints for session %s:\n\n", sessionID)
	fmt.Printf("%-10s %-15s %-12s %-8s %-25s\n", "ID", "NAME", "TYPE", "FILES", "CREATED")
	fmt.Println(string(make([]byte, 75)))

	for _, s := range summaries {
		name := s.Name
		if name == "" {
			name = "-"
		}
		created := s.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Printf("%-10s %-15s %-12s %-8d %-25s\n",
			s.ID,
			truncateStr(name, 15),
			s.Type,
			s.FileCount,
			created,
		)
	}

	return nil
}

func createCheckpoint(sessionID, name, description string, files []string) error {
	mgr, err := getCheckpointManager()
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// If no files specified, use current directory
	if len(files) == 0 {
		cwd, _ := os.Getwd()
		files = []string{cwd}
	}

	req := checkpoint.CreateRequest{
		SessionID:       sessionID,
		Name:            name,
		Type:            checkpoint.TypeManual,
		Description:     description,
		IncludeMessages: true,
		IncludeFiles:    files,
		Compress:        true,
	}

	cp, err := mgr.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create checkpoint: %w", err)
	}

	fmt.Printf("Created checkpoint %s\n", cp.ID)
	if name != "" {
		fmt.Printf("  Name: %s\n", name)
	}
	fmt.Printf("  Files: %d\n", len(cp.Snapshots))
	fmt.Printf("  Size: %d bytes\n", cp.Size)

	return nil
}

func restoreCheckpoint(checkpointID string, createBackup bool) error {
	mgr, err := getCheckpointManager()
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := checkpoint.RestoreRequest{
		CheckpointID:     checkpointID,
		RestoreMessages:  true,
		RestoreFiles:     true,
		RestoreVariables: true,
		OverwriteExisting: true,
		CreateBackup:     createBackup,
	}

	result, err := mgr.Restore(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to restore checkpoint: %w", err)
	}

	if result.Success {
		fmt.Printf("Successfully restored checkpoint %s\n", checkpointID)
		fmt.Printf("  Files restored: %d\n", len(result.RestoredFiles))
		if len(result.FailedFiles) > 0 {
			fmt.Printf("  Files failed: %d\n", len(result.FailedFiles))
			for _, f := range result.FailedFiles {
				fmt.Printf("    - %s: %s\n", f.Path, f.Reason)
			}
		}
	} else {
		fmt.Printf("Restore partially failed\n")
		fmt.Printf("  Files restored: %d\n", len(result.RestoredFiles))
		fmt.Printf("  Files failed: %d\n", len(result.FailedFiles))
	}

	return nil
}

func diffCheckpoint(checkpointID, otherID string) error {
	mgr, err := getCheckpointManager()
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := checkpoint.DiffRequest{
		CheckpointID: checkpointID,
		CurrentState: otherID == "",
		OtherID:      otherID,
	}

	result, err := mgr.Diff(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to diff checkpoint: %w", err)
	}

	if otherID == "" {
		fmt.Printf("Diff between checkpoint %s and current state:\n\n", checkpointID)
	} else {
		fmt.Printf("Diff between checkpoint %s and %s:\n\n", checkpointID, otherID)
	}

	if len(result.FileDiffs) == 0 {
		fmt.Println("No file changes detected.")
		return nil
	}

	for _, diff := range result.FileDiffs {
		switch diff.ChangeType {
		case checkpoint.ChangeAdded:
			fmt.Printf("  + %s (added)\n", diff.Path)
		case checkpoint.ChangeModified:
			fmt.Printf("  M %s (modified)\n", diff.Path)
		case checkpoint.ChangeDeleted:
			fmt.Printf("  - %s (deleted)\n", diff.Path)
		case checkpoint.ChangeUnchanged:
			// Skip unchanged files
		}
	}

	if result.MessageDiff != nil {
		fmt.Printf("\nMessages: %d -> %d (", result.MessageDiff.OldCount, result.MessageDiff.NewCount)
		if result.MessageDiff.Added > 0 {
			fmt.Printf("+%d", result.MessageDiff.Added)
		}
		if result.MessageDiff.Removed > 0 {
			fmt.Printf("-%d", result.MessageDiff.Removed)
		}
		fmt.Println(")")
	}

	return nil
}

func deleteCheckpoint(checkpointID string) error {
	mgr, err := getCheckpointManager()
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Delete(ctx, checkpointID); err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	fmt.Printf("Deleted checkpoint %s\n", checkpointID)
	return nil
}
