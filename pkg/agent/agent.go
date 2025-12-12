package agent

import (
	"context"

	"github.com/syntor/syntor/pkg/models"
)

// Agent represents the base interface for all SYNTOR agents
type Agent interface {
	// Lifecycle management
	Initialize(ctx context.Context, config Config) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() models.HealthStatus

	// Message handling
	HandleMessage(ctx context.Context, msg models.Message) error
	SendMessage(ctx context.Context, target string, msg models.Message) error

	// Capability management
	GetCapabilities() []models.Capability
	CanHandle(taskType string) bool

	// Metrics and monitoring
	GetMetrics() models.AgentMetrics
	GetTraceContext() models.TraceContext

	// Identity
	ID() string
	Name() string
	Type() models.AgentType
}

// Config holds configuration for agent initialization
type Config struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         models.AgentType       `json:"type"`
	Capabilities []models.Capability    `json:"capabilities"`
	Kafka        KafkaConfig            `json:"kafka"`
	Redis        RedisConfig            `json:"redis"`
	Postgres     PostgresConfig         `json:"postgres,omitempty"`
	Metadata     map[string]string      `json:"metadata"`
}

// KafkaConfig holds Kafka connection configuration
type KafkaConfig struct {
	Brokers         []string `json:"brokers"`
	ConsumerGroup   string   `json:"consumer_group"`
	Topics          []string `json:"topics"`
	SecurityEnabled bool     `json:"security_enabled"`
	Username        string   `json:"username,omitempty"`
	Password        string   `json:"password,omitempty"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Address  string `json:"address"`
	Password string `json:"password,omitempty"`
	DB       int    `json:"db"`
}

// PostgresConfig holds PostgreSQL connection configuration
type PostgresConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
}

// MessageHandler is a function type for handling incoming messages
type MessageHandler func(ctx context.Context, msg models.Message) error

// DocumentationAgent handles documentation generation and management
type DocumentationAgent interface {
	Agent
	GenerateDocumentation(ctx context.Context, req DocRequest) (*DocResponse, error)
	UpdateDocumentation(ctx context.Context, req UpdateDocRequest) error
	SearchDocumentation(ctx context.Context, query string) (*SearchResponse, error)
}

// DocRequest represents a documentation generation request
type DocRequest struct {
	SourcePath string            `json:"source_path"`
	OutputPath string            `json:"output_path"`
	Format     string            `json:"format"`
	Options    map[string]string `json:"options,omitempty"`
}

// DocResponse represents a documentation generation response
type DocResponse struct {
	Path      string `json:"path"`
	Generated int    `json:"generated"`
	Errors    int    `json:"errors"`
}

// UpdateDocRequest represents a documentation update request
type UpdateDocRequest struct {
	Path    string                 `json:"path"`
	Content map[string]interface{} `json:"content"`
}

// SearchResponse represents a documentation search response
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}

// SearchResult represents a single documentation search result
type SearchResult struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// GitOperationsAgent handles version control operations
type GitOperationsAgent interface {
	Agent
	CloneRepository(ctx context.Context, req CloneRequest) error
	CommitChanges(ctx context.Context, req CommitRequest) error
	CreateBranch(ctx context.Context, req BranchRequest) error
	MergeChanges(ctx context.Context, req MergeRequest) error
}

// CloneRequest represents a repository clone request
type CloneRequest struct {
	URL    string `json:"url"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
}

// CommitRequest represents a commit request
type CommitRequest struct {
	Path    string   `json:"path"`
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Author  string   `json:"author,omitempty"`
}

// BranchRequest represents a branch creation request
type BranchRequest struct {
	Path       string `json:"path"`
	BranchName string `json:"branch_name"`
	BaseBranch string `json:"base_branch,omitempty"`
}

// MergeRequest represents a merge request
type MergeRequest struct {
	Path         string `json:"path"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Message      string `json:"message,omitempty"`
}

// CoordinationAgent manages task routing and agent coordination
type CoordinationAgent interface {
	Agent
	RouteTask(ctx context.Context, task models.Task) error
	MonitorAgents(ctx context.Context) error
	HandleFailover(ctx context.Context, failedAgent string) error
	BalanceLoad(ctx context.Context) error
}

// WorkerAgent executes project-specific tasks
type WorkerAgent interface {
	Agent
	ExecuteTask(ctx context.Context, task models.Task) (*models.TaskResult, error)
	GetTaskStatus(ctx context.Context, taskID string) (models.TaskStatus, error)
	CancelTask(ctx context.Context, taskID string) error
}
