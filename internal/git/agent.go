package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// Agent implements the GitOperationsAgent interface
// It handles Git repository operations like clone, commit, branch, and merge
type Agent struct {
	*agent.BaseAgent

	// Dependencies
	registry   registry.Registry
	messageBus *kafka.Client

	// Repository management
	reposPath  string
	repos      map[string]*RepoInfo
	reposMu    sync.RWMutex

	// Configuration
	config     Config

	// Background workers
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// RepoInfo holds information about a managed repository
type RepoInfo struct {
	Path          string    `json:"path"`
	URL           string    `json:"url"`
	CurrentBranch string    `json:"current_branch"`
	LastSync      time.Time `json:"last_sync"`
	Status        string    `json:"status"`
}

// Config holds configuration for the Git Agent
type Config struct {
	AgentConfig       agent.Config
	ReposPath         string
	HeartbeatInterval time.Duration
	SyncInterval      time.Duration
	GitAuthor         string
	GitEmail          string
}

// DefaultConfig returns default git agent configuration
func DefaultConfig() Config {
	return Config{
		ReposPath:         "/app/repos",
		HeartbeatInterval: 30 * time.Second,
		SyncInterval:      5 * time.Minute,
		GitAuthor:         "SYNTOR Git Agent",
		GitEmail:          "syntor@local",
	}
}

// New creates a new Git Operations Agent
func New(config Config, reg registry.Registry, bus *kafka.Client) *Agent {
	baseConfig := config.AgentConfig
	baseConfig.Type = models.ServiceAgentType
	baseConfig.Capabilities = []models.Capability{
		{Name: "git-operations", Version: "1.0", Description: "Git repository operations"},
		{Name: "git-clone", Version: "1.0", Description: "Clone repositories"},
		{Name: "git-commit", Version: "1.0", Description: "Create commits"},
		{Name: "git-branch", Version: "1.0", Description: "Branch management"},
		{Name: "git-merge", Version: "1.0", Description: "Merge branches"},
	}

	a := &Agent{
		BaseAgent:  agent.NewBaseAgent(baseConfig),
		registry:   reg,
		messageBus: bus,
		reposPath:  config.ReposPath,
		repos:      make(map[string]*RepoInfo),
		config:     config,
	}

	// Register message handlers
	a.RegisterHandler(models.MsgServiceRequest, a.handleServiceRequest)
	a.RegisterHandler(models.MsgTaskAssignment, a.handleTaskAssignment)

	return a
}

// Start begins the git agent operation
func (a *Agent) Start(ctx context.Context) error {
	if err := a.BaseAgent.Start(ctx); err != nil {
		return err
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// Ensure repos directory exists
	if err := os.MkdirAll(a.reposPath, 0755); err != nil {
		return fmt.Errorf("failed to create repos directory: %w", err)
	}

	// Scan existing repositories
	a.scanRepositories()

	// Subscribe to relevant topics
	topics := []string{
		kafka.TopicServiceRequest,
		kafka.TopicTaskAssignment,
	}

	for _, topic := range topics {
		if err := a.messageBus.Subscribe(ctx, topic, a.handleMessage); err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", topic, err)
		}
	}

	// Register self with registry
	agentInfo := registry.AgentInfo{
		ID:           a.ID(),
		Name:         a.Name(),
		Type:         a.Type(),
		Capabilities: a.GetCapabilities(),
		Status: registry.AgentStatus{
			State:  registry.AgentStateRunning,
			Health: models.HealthHealthy,
		},
	}
	if err := a.registry.RegisterAgent(ctx, agentInfo); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Start background workers
	a.wg.Add(1)
	go a.runHeartbeat()

	return nil
}

// Stop gracefully shuts down the git agent
func (a *Agent) Stop(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := a.registry.DeregisterAgent(ctx, a.ID()); err != nil {
		// Log but don't fail
	}

	return a.BaseAgent.Stop(ctx)
}

// CloneRepository clones a Git repository
func (a *Agent) CloneRepository(ctx context.Context, req agent.CloneRequest) error {
	// Validate URL
	if req.URL == "" {
		return fmt.Errorf("repository URL is required")
	}

	// Determine local path
	localPath := req.Path
	if localPath == "" {
		// Extract repo name from URL
		repoName := extractRepoName(req.URL)
		localPath = filepath.Join(a.reposPath, repoName)
	}

	// Check if already exists
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		return fmt.Errorf("repository already exists at %s", localPath)
	}

	// Build clone command
	args := []string{"clone"}
	if req.Branch != "" {
		args = append(args, "-b", req.Branch)
	}
	args = append(args, req.URL, localPath)

	// Execute clone
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s - %w", string(output), err)
	}

	// Get current branch
	branch := req.Branch
	if branch == "" {
		branch = a.getCurrentBranch(localPath)
	}

	// Register repository
	a.reposMu.Lock()
	a.repos[localPath] = &RepoInfo{
		Path:          localPath,
		URL:           req.URL,
		CurrentBranch: branch,
		LastSync:      time.Now(),
		Status:        "cloned",
	}
	a.reposMu.Unlock()

	return nil
}

// CommitChanges creates a commit in a repository
func (a *Agent) CommitChanges(ctx context.Context, req agent.CommitRequest) error {
	if req.Path == "" {
		return fmt.Errorf("repository path is required")
	}

	if req.Message == "" {
		return fmt.Errorf("commit message is required")
	}

	// Check repository exists
	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", req.Path)
	}

	// Stage files
	if len(req.Files) > 0 {
		args := append([]string{"add"}, req.Files...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = req.Path
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add failed: %s - %w", string(output), err)
		}
	} else {
		// Add all changes
		cmd := exec.CommandContext(ctx, "git", "add", "-A")
		cmd.Dir = req.Path
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add failed: %s - %w", string(output), err)
		}
	}

	// Check if there are changes to commit
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	cmd.Dir = req.Path
	if err := cmd.Run(); err == nil {
		return fmt.Errorf("no changes to commit")
	}

	// Set author if provided
	author := req.Author
	if author == "" {
		author = fmt.Sprintf("%s <%s>", a.config.GitAuthor, a.config.GitEmail)
	}

	// Commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", req.Message, "--author", author)
	commitCmd.Dir = req.Path
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s - %w", string(output), err)
	}

	// Update repo info
	a.reposMu.Lock()
	if repo, ok := a.repos[req.Path]; ok {
		repo.LastSync = time.Now()
		repo.Status = "committed"
	}
	a.reposMu.Unlock()

	return nil
}

// CreateBranch creates a new branch
func (a *Agent) CreateBranch(ctx context.Context, req agent.BranchRequest) error {
	if req.Path == "" {
		return fmt.Errorf("repository path is required")
	}

	if req.BranchName == "" {
		return fmt.Errorf("branch name is required")
	}

	// Check repository exists
	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", req.Path)
	}

	// Checkout base branch if specified
	if req.BaseBranch != "" {
		cmd := exec.CommandContext(ctx, "git", "checkout", req.BaseBranch)
		cmd.Dir = req.Path
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout base branch failed: %s - %w", string(output), err)
		}
	}

	// Create and checkout new branch
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", req.BranchName)
	cmd.Dir = req.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch creation failed: %s - %w", string(output), err)
	}

	// Update repo info
	a.reposMu.Lock()
	if repo, ok := a.repos[req.Path]; ok {
		repo.CurrentBranch = req.BranchName
		repo.LastSync = time.Now()
	}
	a.reposMu.Unlock()

	return nil
}

// MergeChanges merges branches
func (a *Agent) MergeChanges(ctx context.Context, req agent.MergeRequest) error {
	if req.Path == "" {
		return fmt.Errorf("repository path is required")
	}

	if req.SourceBranch == "" {
		return fmt.Errorf("source branch is required")
	}

	if req.TargetBranch == "" {
		return fmt.Errorf("target branch is required")
	}

	// Check repository exists
	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", req.Path)
	}

	// Checkout target branch
	cmd := exec.CommandContext(ctx, "git", "checkout", req.TargetBranch)
	cmd.Dir = req.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout target branch failed: %s - %w", string(output), err)
	}

	// Merge source branch
	mergeArgs := []string{"merge", req.SourceBranch}
	if req.Message != "" {
		mergeArgs = append(mergeArgs, "-m", req.Message)
	}

	mergeCmd := exec.CommandContext(ctx, "git", mergeArgs...)
	mergeCmd.Dir = req.Path
	if output, err := mergeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge failed: %s - %w", string(output), err)
	}

	// Update repo info
	a.reposMu.Lock()
	if repo, ok := a.repos[req.Path]; ok {
		repo.CurrentBranch = req.TargetBranch
		repo.LastSync = time.Now()
		repo.Status = "merged"
	}
	a.reposMu.Unlock()

	return nil
}

// Pull fetches and merges changes from remote
func (a *Agent) Pull(ctx context.Context, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", path)
	}

	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull failed: %s - %w", string(output), err)
	}

	return nil
}

// Push pushes commits to remote
func (a *Agent) Push(ctx context.Context, path string, branch string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", path)
	}

	args := []string{"push"}
	if branch != "" {
		args = append(args, "origin", branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %s - %w", string(output), err)
	}

	return nil
}

// GetStatus returns the status of a repository
func (a *Agent) GetStatus(ctx context.Context, path string) (map[string]interface{}, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("repository not found: %s", path)
	}

	status := make(map[string]interface{})

	// Get current branch
	status["branch"] = a.getCurrentBranch(path)

	// Get status
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	modified := 0
	added := 0
	deleted := 0

	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'M', ' ':
			if line[1] == 'M' {
				modified++
			}
		case 'A', '?':
			added++
		case 'D':
			deleted++
		}
	}

	status["modified"] = modified
	status["added"] = added
	status["deleted"] = deleted
	status["clean"] = len(output) == 0

	// Get last commit
	logCmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H|%s|%an|%ai")
	logCmd.Dir = path
	logOutput, err := logCmd.Output()
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(logOutput)), "|", 4)
		if len(parts) >= 4 {
			status["last_commit"] = map[string]string{
				"hash":    parts[0],
				"message": parts[1],
				"author":  parts[2],
				"date":    parts[3],
			}
		}
	}

	return status, nil
}

// ListRepositories returns all managed repositories
func (a *Agent) ListRepositories(ctx context.Context) ([]*RepoInfo, error) {
	a.reposMu.RLock()
	defer a.reposMu.RUnlock()

	repos := make([]*RepoInfo, 0, len(a.repos))
	for _, repo := range a.repos {
		repos = append(repos, repo)
	}

	return repos, nil
}

// Message handlers

func (a *Agent) handleMessage(ctx context.Context, msg models.Message) error {
	return a.HandleMessage(ctx, msg)
}

func (a *Agent) handleServiceRequest(ctx context.Context, msg models.Message) error {
	if msg.Target != "" && msg.Target != a.ID() && msg.Target != "git" {
		return nil
	}

	serviceType, ok := msg.Payload["service_type"].(string)
	if !ok {
		return nil
	}

	var response map[string]interface{}
	var err error

	switch serviceType {
	case "clone":
		req := agent.CloneRequest{
			URL:    getString(msg.Payload, "url"),
			Path:   getString(msg.Payload, "path"),
			Branch: getString(msg.Payload, "branch"),
		}
		err = a.CloneRepository(ctx, req)
		if err == nil {
			response = map[string]interface{}{"status": "cloned"}
		}

	case "commit":
		req := agent.CommitRequest{
			Path:    getString(msg.Payload, "path"),
			Message: getString(msg.Payload, "message"),
			Author:  getString(msg.Payload, "author"),
		}
		err = a.CommitChanges(ctx, req)
		if err == nil {
			response = map[string]interface{}{"status": "committed"}
		}

	case "branch":
		req := agent.BranchRequest{
			Path:       getString(msg.Payload, "path"),
			BranchName: getString(msg.Payload, "branch_name"),
			BaseBranch: getString(msg.Payload, "base_branch"),
		}
		err = a.CreateBranch(ctx, req)
		if err == nil {
			response = map[string]interface{}{"status": "branch_created"}
		}

	case "merge":
		req := agent.MergeRequest{
			Path:         getString(msg.Payload, "path"),
			SourceBranch: getString(msg.Payload, "source_branch"),
			TargetBranch: getString(msg.Payload, "target_branch"),
			Message:      getString(msg.Payload, "message"),
		}
		err = a.MergeChanges(ctx, req)
		if err == nil {
			response = map[string]interface{}{"status": "merged"}
		}

	case "status":
		path := getString(msg.Payload, "path")
		response, err = a.GetStatus(ctx, path)

	case "list_repos":
		repos, e := a.ListRepositories(ctx)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"repositories": repos}
		}

	case "pull":
		path := getString(msg.Payload, "path")
		err = a.Pull(ctx, path)
		if err == nil {
			response = map[string]interface{}{"status": "pulled"}
		}

	case "push":
		path := getString(msg.Payload, "path")
		branch := getString(msg.Payload, "branch")
		err = a.Push(ctx, path, branch)
		if err == nil {
			response = map[string]interface{}{"status": "pushed"}
		}

	default:
		return nil
	}

	// Send response
	var taskErr *models.TaskError
	if err != nil {
		taskErr = &models.TaskError{Code: "GIT_ERROR", Message: err.Error()}
	}

	respMsg := models.NewServiceResponseMessage(a.ID(), msg.Source, msg.CorrelationID, response, taskErr)
	return a.messageBus.Publish(ctx, kafka.TopicServiceResponse, respMsg)
}

func (a *Agent) handleTaskAssignment(ctx context.Context, msg models.Message) error {
	task, err := models.TaskFromPayload(msg.Payload)
	if err != nil {
		return err
	}

	if task.AssignedAgent != a.ID() {
		return nil
	}

	// Send status update
	statusMsg := models.NewTaskStatusMessage(a.ID(), task.ID, models.TaskRunning)
	a.messageBus.PublishAsync(kafka.TopicTaskStatus, statusMsg)

	var result *models.TaskResult
	var taskErr *models.TaskError

	switch task.Type {
	case "git_clone":
		req := agent.CloneRequest{
			URL:    getString(task.Payload, "url"),
			Path:   getString(task.Payload, "path"),
			Branch: getString(task.Payload, "branch"),
		}
		err := a.CloneRepository(ctx, req)
		if err != nil {
			taskErr = &models.TaskError{Code: "CLONE_FAILED", Message: err.Error()}
		} else {
			result = &models.TaskResult{Data: map[string]interface{}{"path": req.Path}}
		}

	case "git_commit":
		req := agent.CommitRequest{
			Path:    getString(task.Payload, "path"),
			Message: getString(task.Payload, "message"),
		}
		err := a.CommitChanges(ctx, req)
		if err != nil {
			taskErr = &models.TaskError{Code: "COMMIT_FAILED", Message: err.Error()}
		} else {
			result = &models.TaskResult{Data: map[string]interface{}{"status": "committed"}}
		}

	case "git_branch":
		req := agent.BranchRequest{
			Path:       getString(task.Payload, "path"),
			BranchName: getString(task.Payload, "branch_name"),
			BaseBranch: getString(task.Payload, "base_branch"),
		}
		err := a.CreateBranch(ctx, req)
		if err != nil {
			taskErr = &models.TaskError{Code: "BRANCH_FAILED", Message: err.Error()}
		} else {
			result = &models.TaskResult{Data: map[string]interface{}{"branch": req.BranchName}}
		}

	case "git_merge":
		req := agent.MergeRequest{
			Path:         getString(task.Payload, "path"),
			SourceBranch: getString(task.Payload, "source_branch"),
			TargetBranch: getString(task.Payload, "target_branch"),
		}
		err := a.MergeChanges(ctx, req)
		if err != nil {
			taskErr = &models.TaskError{Code: "MERGE_FAILED", Message: err.Error()}
		} else {
			result = &models.TaskResult{Data: map[string]interface{}{"status": "merged"}}
		}

	default:
		taskErr = &models.TaskError{Code: "UNKNOWN_TASK", Message: fmt.Sprintf("Unknown task type: %s", task.Type)}
	}

	// Send completion
	completeMsg := models.NewTaskCompleteMessage(a.ID(), task.ID, result, taskErr)
	return a.messageBus.Publish(ctx, kafka.TopicTaskComplete, completeMsg)
}

// Internal methods

func (a *Agent) scanRepositories() {
	entries, err := os.ReadDir(a.reposPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(a.reposPath, entry.Name())
		gitPath := filepath.Join(repoPath, ".git")

		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue // Not a git repo
		}

		// Get remote URL
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = repoPath
		output, err := cmd.Output()
		url := ""
		if err == nil {
			url = strings.TrimSpace(string(output))
		}

		a.reposMu.Lock()
		a.repos[repoPath] = &RepoInfo{
			Path:          repoPath,
			URL:           url,
			CurrentBranch: a.getCurrentBranch(repoPath),
			LastSync:      time.Now(),
			Status:        "scanned",
		}
		a.reposMu.Unlock()
	}
}

func (a *Agent) getCurrentBranch(path string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func (a *Agent) runHeartbeat() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			msg := models.NewAgentHeartbeatMessage(a.ID(), a.GetMetrics())
			a.messageBus.PublishAsync(kafka.TopicAgentHeartbeat, msg)
			a.registry.Heartbeat(a.ctx, a.ID())
		}
	}
}

// Helper functions

func extractRepoName(url string) string {
	// Handle both HTTPS and SSH URLs
	url = strings.TrimSuffix(url, ".git")

	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "repo"
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Ensure Agent implements required interfaces
var _ agent.GitOperationsAgent = (*Agent)(nil)
