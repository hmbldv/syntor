// Package checkpoint provides session checkpointing and rewind capabilities.
// Checkpoints capture session state and file snapshots to enable safe
// recovery from mistakes or unwanted changes.
package checkpoint

import (
	"time"
)

// Checkpoint represents a saved point in session history.
type Checkpoint struct {
	// Identity
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Name      string    `json:"name,omitempty"` // Optional user-provided name

	// Timing
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// State
	Type        CheckpointType `json:"type"`
	Description string         `json:"description,omitempty"`

	// Content
	Messages    []Message    `json:"messages,omitempty"`
	Snapshots   []FileSnapshot `json:"snapshots,omitempty"`
	Variables   map[string]any `json:"variables,omitempty"`
	ToolHistory []ToolRecord   `json:"tool_history,omitempty"`

	// Metadata
	Metadata    map[string]string `json:"metadata,omitempty"`
	Size        int64             `json:"size"` // Total size in bytes
	Restorable  bool              `json:"restorable"`
}

// CheckpointType categorizes checkpoints.
type CheckpointType string

const (
	// TypeAuto is an automatically created checkpoint before risky operations.
	TypeAuto CheckpointType = "auto"

	// TypeManual is a user-requested checkpoint.
	TypeManual CheckpointType = "manual"

	// TypePeriodic is created at regular intervals.
	TypePeriodic CheckpointType = "periodic"

	// TypePreTool is created before tool execution.
	TypePreTool CheckpointType = "pre_tool"
)

// Message represents a conversation message in the checkpoint.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// FileSnapshot represents the state of a file at checkpoint time.
type FileSnapshot struct {
	Path         string    `json:"path"`
	Content      []byte    `json:"content,omitempty"`
	Hash         string    `json:"hash"` // SHA-256 hash
	ModTime      time.Time `json:"mod_time"`
	Size         int64     `json:"size"`
	IsDirectory  bool      `json:"is_directory"`
	Permissions  string    `json:"permissions"`

	// For large files, store reference instead of content
	ContentRef   string    `json:"content_ref,omitempty"`
	Compressed   bool      `json:"compressed"`
}

// ToolRecord captures a tool execution.
type ToolRecord struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
	Output     string         `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	Duration   time.Duration  `json:"duration"`
	Success    bool           `json:"success"`
}

// RestoreResult contains the outcome of a restore operation.
type RestoreResult struct {
	CheckpointID   string          `json:"checkpoint_id"`
	Success        bool            `json:"success"`
	RestoredFiles  []string        `json:"restored_files"`
	FailedFiles    []RestoreError  `json:"failed_files,omitempty"`
	MessagesRestored int           `json:"messages_restored"`
	Duration       time.Duration   `json:"duration"`
}

// RestoreError describes a file restoration failure.
type RestoreError struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// CheckpointSummary is a lightweight representation for listing.
type CheckpointSummary struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	Name        string         `json:"name,omitempty"`
	Type        CheckpointType `json:"type"`
	Description string         `json:"description,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	Size        int64          `json:"size"`
	FileCount   int            `json:"file_count"`
	Restorable  bool           `json:"restorable"`
}

// CreateRequest specifies checkpoint creation parameters.
type CreateRequest struct {
	SessionID   string            `json:"session_id"`
	Name        string            `json:"name,omitempty"`
	Type        CheckpointType    `json:"type"`
	Description string            `json:"description,omitempty"`

	// What to include
	IncludeMessages bool          `json:"include_messages"`
	IncludeFiles    []string      `json:"include_files,omitempty"` // Specific files
	IncludeDir      string        `json:"include_dir,omitempty"`   // Directory to snapshot
	IncludeToolHistory bool       `json:"include_tool_history"`

	// Options
	ExpiresIn   time.Duration     `json:"expires_in,omitempty"`
	Compress    bool              `json:"compress"`
	MaxFileSize int64             `json:"max_file_size,omitempty"` // Skip files larger than this

	// Context to capture
	Messages    []Message         `json:"messages,omitempty"`
	Variables   map[string]any    `json:"variables,omitempty"`
	ToolHistory []ToolRecord      `json:"tool_history,omitempty"`
}

// RestoreRequest specifies restore parameters.
type RestoreRequest struct {
	CheckpointID string   `json:"checkpoint_id"`

	// What to restore
	RestoreMessages bool     `json:"restore_messages"`
	RestoreFiles    bool     `json:"restore_files"`
	RestoreVariables bool    `json:"restore_variables"`

	// Options
	FilesToRestore []string `json:"files_to_restore,omitempty"` // Specific files (empty = all)
	OverwriteExisting bool  `json:"overwrite_existing"`
	CreateBackup    bool    `json:"create_backup"` // Backup current state before restore
}

// DiffRequest specifies checkpoint comparison parameters.
type DiffRequest struct {
	CheckpointID string `json:"checkpoint_id"`
	CurrentState bool   `json:"current_state"` // Compare against current state
	OtherID      string `json:"other_id,omitempty"` // Compare against another checkpoint
}

// DiffResult contains the differences between states.
type DiffResult struct {
	FromID      string      `json:"from_id"`
	ToID        string      `json:"to_id,omitempty"`
	FileDiffs   []FileDiff  `json:"file_diffs,omitempty"`
	MessageDiff *MessageDiff `json:"message_diff,omitempty"`
}

// FileDiff describes changes to a file.
type FileDiff struct {
	Path       string     `json:"path"`
	ChangeType ChangeType `json:"change_type"`
	OldHash    string     `json:"old_hash,omitempty"`
	NewHash    string     `json:"new_hash,omitempty"`
	OldSize    int64      `json:"old_size,omitempty"`
	NewSize    int64      `json:"new_size,omitempty"`
	LineDiff   string     `json:"line_diff,omitempty"` // Unified diff format
}

// ChangeType describes the type of file change.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeModified ChangeType = "modified"
	ChangeDeleted  ChangeType = "deleted"
	ChangeUnchanged ChangeType = "unchanged"
)

// MessageDiff describes changes to conversation history.
type MessageDiff struct {
	OldCount int `json:"old_count"`
	NewCount int `json:"new_count"`
	Added    int `json:"added"`
	Removed  int `json:"removed"`
}

// StorageConfig configures checkpoint storage.
type StorageConfig struct {
	// BaseDir is the directory for storing checkpoints
	BaseDir string `yaml:"base_dir" json:"base_dir"`

	// MaxCheckpoints limits the total number of checkpoints per session
	MaxCheckpoints int `yaml:"max_checkpoints" json:"max_checkpoints"`

	// MaxAge is the maximum age of checkpoints before cleanup
	MaxAge time.Duration `yaml:"max_age" json:"max_age"`

	// MaxTotalSize limits total storage size
	MaxTotalSize int64 `yaml:"max_total_size" json:"max_total_size"`

	// CompressThreshold compresses files larger than this
	CompressThreshold int64 `yaml:"compress_threshold" json:"compress_threshold"`

	// LargeFileThreshold stores files larger than this as references
	LargeFileThreshold int64 `yaml:"large_file_threshold" json:"large_file_threshold"`
}

// DefaultStorageConfig returns sensible defaults.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		BaseDir:            "~/.syntor/checkpoints",
		MaxCheckpoints:     50,
		MaxAge:             7 * 24 * time.Hour, // 7 days
		MaxTotalSize:       500 * 1024 * 1024,  // 500 MB
		CompressThreshold:  10 * 1024,          // 10 KB
		LargeFileThreshold: 1024 * 1024,        // 1 MB
	}
}

// PolicyConfig configures automatic checkpoint creation.
type PolicyConfig struct {
	// AutoBeforeEdit creates checkpoints before file edits
	AutoBeforeEdit bool `yaml:"auto_before_edit" json:"auto_before_edit"`

	// AutoBeforeBash creates checkpoints before bash commands
	AutoBeforeBash bool `yaml:"auto_before_bash" json:"auto_before_bash"`

	// AutoInterval creates periodic checkpoints
	AutoInterval time.Duration `yaml:"auto_interval" json:"auto_interval"`

	// MinIntervalBetween enforces minimum time between checkpoints
	MinIntervalBetween time.Duration `yaml:"min_interval_between" json:"min_interval_between"`

	// ExcludePatterns skips files matching these patterns
	ExcludePatterns []string `yaml:"exclude_patterns" json:"exclude_patterns"`
}

// DefaultPolicyConfig returns sensible defaults.
func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		AutoBeforeEdit:     true,
		AutoBeforeBash:     true,
		AutoInterval:       5 * time.Minute,
		MinIntervalBetween: 30 * time.Second,
		ExcludePatterns: []string{
			"*.log",
			"*.tmp",
			".git/*",
			"node_modules/*",
			"__pycache__/*",
			"*.pyc",
			".DS_Store",
		},
	}
}
