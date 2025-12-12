package documentation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// Agent implements the DocumentationAgent interface
// It handles documentation generation, search, and management
type Agent struct {
	*agent.BaseAgent

	// Dependencies
	registry   registry.Registry
	messageBus *kafka.Client

	// Documentation storage
	docsPath   string
	docIndex   map[string]*DocEntry
	indexMu    sync.RWMutex

	// Configuration
	config     Config

	// Background workers
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// DocEntry represents an indexed documentation entry
type DocEntry struct {
	Path        string    `json:"path"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Format      string    `json:"format"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

// Config holds configuration for the Documentation Agent
type Config struct {
	AgentConfig       agent.Config
	DocsPath          string
	HeartbeatInterval time.Duration
	IndexInterval     time.Duration
	SupportedFormats  []string
}

// DefaultConfig returns default documentation agent configuration
func DefaultConfig() Config {
	return Config{
		DocsPath:          "/app/docs",
		HeartbeatInterval: 30 * time.Second,
		IndexInterval:     5 * time.Minute,
		SupportedFormats:  []string{".md", ".txt", ".html", ".rst"},
	}
}

// New creates a new Documentation Agent
func New(config Config, reg registry.Registry, bus *kafka.Client) *Agent {
	baseConfig := config.AgentConfig
	baseConfig.Type = models.ServiceAgentType
	baseConfig.Capabilities = []models.Capability{
		{Name: "documentation", Version: "1.0", Description: "Documentation management"},
		{Name: "doc-generation", Version: "1.0", Description: "Generate documentation from code"},
		{Name: "doc-search", Version: "1.0", Description: "Search documentation"},
		{Name: "doc-versioning", Version: "1.0", Description: "Documentation versioning"},
	}

	a := &Agent{
		BaseAgent:  agent.NewBaseAgent(baseConfig),
		registry:   reg,
		messageBus: bus,
		docsPath:   config.DocsPath,
		docIndex:   make(map[string]*DocEntry),
		config:     config,
	}

	// Register message handlers
	a.RegisterHandler(models.MsgServiceRequest, a.handleServiceRequest)
	a.RegisterHandler(models.MsgTaskAssignment, a.handleTaskAssignment)

	return a
}

// Start begins the documentation agent operation
func (a *Agent) Start(ctx context.Context) error {
	if err := a.BaseAgent.Start(ctx); err != nil {
		return err
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// Ensure docs directory exists
	if err := os.MkdirAll(a.docsPath, 0755); err != nil {
		return fmt.Errorf("failed to create docs directory: %w", err)
	}

	// Initial index build
	if err := a.buildIndex(); err != nil {
		// Log but don't fail
	}

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
	a.wg.Add(2)
	go a.runHeartbeat()
	go a.runIndexer()

	return nil
}

// Stop gracefully shuts down the documentation agent
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

// GenerateDocumentation generates documentation from source
func (a *Agent) GenerateDocumentation(ctx context.Context, req agent.DocRequest) (*agent.DocResponse, error) {
	response := &agent.DocResponse{
		Path:      req.OutputPath,
		Generated: 0,
		Errors:    0,
	}

	// Validate source path exists
	if _, err := os.Stat(req.SourcePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("source path does not exist: %s", req.SourcePath)
	}

	// Determine output path
	outputPath := req.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(a.docsPath, "generated")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Walk source directory and generate docs
	err := filepath.Walk(req.SourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			response.Errors++
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Generate doc for supported file types
		ext := filepath.Ext(path)
		if a.isSupportedSourceFormat(ext) {
			docPath, err := a.generateDocForFile(path, outputPath, req.Format)
			if err != nil {
				response.Errors++
				return nil
			}

			// Index the new doc
			a.indexDocument(docPath)
			response.Generated++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk source directory: %w", err)
	}

	response.Path = outputPath
	return response, nil
}

// UpdateDocumentation updates existing documentation
func (a *Agent) UpdateDocumentation(ctx context.Context, req agent.UpdateDocRequest) error {
	a.indexMu.Lock()
	defer a.indexMu.Unlock()

	entry, ok := a.docIndex[req.Path]
	if !ok {
		return fmt.Errorf("document not found: %s", req.Path)
	}

	// Update content
	if content, ok := req.Content["content"].(string); ok {
		entry.Content = content
	}
	if title, ok := req.Content["title"].(string); ok {
		entry.Title = title
	}
	if tags, ok := req.Content["tags"].([]string); ok {
		entry.Tags = tags
	}

	entry.UpdatedAt = time.Now()
	entry.Version++

	// Write to file
	return a.writeDocEntry(entry)
}

// SearchDocumentation searches the documentation index
func (a *Agent) SearchDocumentation(ctx context.Context, query string) (*agent.SearchResponse, error) {
	a.indexMu.RLock()
	defer a.indexMu.RUnlock()

	response := &agent.SearchResponse{
		Results: make([]agent.SearchResult, 0),
		Total:   0,
	}

	queryLower := strings.ToLower(query)
	keywords := strings.Fields(queryLower)

	for path, entry := range a.docIndex {
		score := a.calculateSearchScore(entry, keywords)
		if score > 0 {
			snippet := a.extractSnippet(entry.Content, keywords)
			response.Results = append(response.Results, agent.SearchResult{
				Path:    path,
				Title:   entry.Title,
				Snippet: snippet,
				Score:   score,
			})
			response.Total++
		}
	}

	// Sort by score (simple bubble sort for now)
	for i := 0; i < len(response.Results)-1; i++ {
		for j := 0; j < len(response.Results)-i-1; j++ {
			if response.Results[j].Score < response.Results[j+1].Score {
				response.Results[j], response.Results[j+1] = response.Results[j+1], response.Results[j]
			}
		}
	}

	return response, nil
}

// GetDocument retrieves a specific document
func (a *Agent) GetDocument(ctx context.Context, path string) (*DocEntry, error) {
	a.indexMu.RLock()
	defer a.indexMu.RUnlock()

	entry, ok := a.docIndex[path]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", path)
	}

	return entry, nil
}

// ListDocuments returns all indexed documents
func (a *Agent) ListDocuments(ctx context.Context) ([]*DocEntry, error) {
	a.indexMu.RLock()
	defer a.indexMu.RUnlock()

	docs := make([]*DocEntry, 0, len(a.docIndex))
	for _, entry := range a.docIndex {
		docs = append(docs, entry)
	}

	return docs, nil
}

// Message handlers

func (a *Agent) handleMessage(ctx context.Context, msg models.Message) error {
	return a.HandleMessage(ctx, msg)
}

func (a *Agent) handleServiceRequest(ctx context.Context, msg models.Message) error {
	// Only handle requests targeted to this agent or doc service
	if msg.Target != "" && msg.Target != a.ID() && msg.Target != "documentation" {
		return nil
	}

	serviceType, ok := msg.Payload["service_type"].(string)
	if !ok {
		return nil
	}

	var response map[string]interface{}
	var err error

	switch serviceType {
	case "search":
		query, _ := msg.Payload["query"].(string)
		results, e := a.SearchDocumentation(ctx, query)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"results": results}
		}

	case "get_document":
		path, _ := msg.Payload["path"].(string)
		doc, e := a.GetDocument(ctx, path)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"document": doc}
		}

	case "list_documents":
		docs, e := a.ListDocuments(ctx)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"documents": docs}
		}

	case "generate":
		req := agent.DocRequest{
			SourcePath: getString(msg.Payload, "source_path"),
			OutputPath: getString(msg.Payload, "output_path"),
			Format:     getString(msg.Payload, "format"),
		}
		result, e := a.GenerateDocumentation(ctx, req)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"result": result}
		}

	default:
		return nil // Not for us
	}

	// Send response
	var taskErr *models.TaskError
	if err != nil {
		taskErr = &models.TaskError{Code: "DOC_ERROR", Message: err.Error()}
	}

	respMsg := models.NewServiceResponseMessage(a.ID(), msg.Source, msg.CorrelationID, response, taskErr)
	return a.messageBus.Publish(ctx, kafka.TopicServiceResponse, respMsg)
}

func (a *Agent) handleTaskAssignment(ctx context.Context, msg models.Message) error {
	task, err := models.TaskFromPayload(msg.Payload)
	if err != nil {
		return err
	}

	// Check if task is for us
	if task.AssignedAgent != a.ID() {
		return nil
	}

	// Send status update
	statusMsg := models.NewTaskStatusMessage(a.ID(), task.ID, models.TaskRunning)
	a.messageBus.PublishAsync(kafka.TopicTaskStatus, statusMsg)

	// Execute task based on type
	var result *models.TaskResult
	var taskErr *models.TaskError

	switch task.Type {
	case "generate_docs":
		req := agent.DocRequest{
			SourcePath: getString(task.Payload, "source_path"),
			OutputPath: getString(task.Payload, "output_path"),
			Format:     getString(task.Payload, "format"),
		}
		docResult, err := a.GenerateDocumentation(ctx, req)
		if err != nil {
			taskErr = &models.TaskError{Code: "GENERATION_FAILED", Message: err.Error()}
		} else {
			data, _ := json.Marshal(docResult)
			var resultData map[string]interface{}
			json.Unmarshal(data, &resultData)
			result = &models.TaskResult{Data: resultData}
		}

	case "search_docs":
		query := getString(task.Payload, "query")
		searchResult, err := a.SearchDocumentation(ctx, query)
		if err != nil {
			taskErr = &models.TaskError{Code: "SEARCH_FAILED", Message: err.Error()}
		} else {
			data, _ := json.Marshal(searchResult)
			var resultData map[string]interface{}
			json.Unmarshal(data, &resultData)
			result = &models.TaskResult{Data: resultData}
		}

	default:
		taskErr = &models.TaskError{Code: "UNKNOWN_TASK", Message: fmt.Sprintf("Unknown task type: %s", task.Type)}
	}

	// Send completion
	completeMsg := models.NewTaskCompleteMessage(a.ID(), task.ID, result, taskErr)
	return a.messageBus.Publish(ctx, kafka.TopicTaskComplete, completeMsg)
}

// Internal methods

func (a *Agent) buildIndex() error {
	return filepath.Walk(a.docsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if a.isSupportedFormat(filepath.Ext(path)) {
			a.indexDocument(path)
		}

		return nil
	})
}

func (a *Agent) indexDocument(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	entry := &DocEntry{
		Path:      path,
		Title:     a.extractTitle(string(content), path),
		Content:   string(content),
		Format:    filepath.Ext(path),
		CreatedAt: info.ModTime(),
		UpdatedAt: info.ModTime(),
		Version:   1,
	}

	a.indexMu.Lock()
	a.docIndex[path] = entry
	a.indexMu.Unlock()

	return nil
}

func (a *Agent) writeDocEntry(entry *DocEntry) error {
	return os.WriteFile(entry.Path, []byte(entry.Content), 0644)
}

func (a *Agent) isSupportedFormat(ext string) bool {
	for _, supported := range a.config.SupportedFormats {
		if ext == supported {
			return true
		}
	}
	return false
}

func (a *Agent) isSupportedSourceFormat(ext string) bool {
	sourceFormats := []string{".go", ".py", ".js", ".ts", ".java", ".rs", ".c", ".cpp", ".h"}
	for _, supported := range sourceFormats {
		if ext == supported {
			return true
		}
	}
	return false
}

func (a *Agent) generateDocForFile(sourcePath, outputPath, format string) (string, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}

	// Simple doc generation - extract comments and structure
	doc := a.extractDocFromSource(string(content), filepath.Ext(sourcePath))

	// Determine output format
	if format == "" {
		format = "md"
	}

	// Create output file
	baseName := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	outputFile := filepath.Join(outputPath, baseName+"."+format)

	if err := os.WriteFile(outputFile, []byte(doc), 0644); err != nil {
		return "", err
	}

	return outputFile, nil
}

func (a *Agent) extractDocFromSource(content, ext string) string {
	var doc strings.Builder

	lines := strings.Split(content, "\n")
	inComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect comment blocks
		if strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "/**") {
			inComment = true
			doc.WriteString(strings.TrimPrefix(trimmed, "/*"))
			doc.WriteString(strings.TrimPrefix(doc.String(), "*"))
			doc.WriteString("\n")
		} else if strings.HasSuffix(trimmed, "*/") {
			inComment = false
			doc.WriteString(strings.TrimSuffix(trimmed, "*/"))
			doc.WriteString("\n\n")
		} else if inComment {
			doc.WriteString(strings.TrimPrefix(trimmed, "* "))
			doc.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "//") {
			doc.WriteString(strings.TrimPrefix(trimmed, "// "))
			doc.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "#") && (ext == ".py" || ext == ".sh") {
			doc.WriteString(strings.TrimPrefix(trimmed, "# "))
			doc.WriteString("\n")
		}
	}

	return doc.String()
}

func (a *Agent) extractTitle(content, path string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Markdown heading
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	// Use filename as fallback
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func (a *Agent) calculateSearchScore(entry *DocEntry, keywords []string) float64 {
	score := 0.0
	contentLower := strings.ToLower(entry.Content)
	titleLower := strings.ToLower(entry.Title)

	for _, keyword := range keywords {
		// Title match (higher weight)
		if strings.Contains(titleLower, keyword) {
			score += 2.0
		}
		// Content match
		if strings.Contains(contentLower, keyword) {
			score += 1.0
		}
		// Tag match
		for _, tag := range entry.Tags {
			if strings.ToLower(tag) == keyword {
				score += 1.5
			}
		}
	}

	return score
}

func (a *Agent) extractSnippet(content string, keywords []string) string {
	contentLower := strings.ToLower(content)

	// Find first keyword occurrence
	minPos := len(content)
	for _, keyword := range keywords {
		pos := strings.Index(contentLower, keyword)
		if pos >= 0 && pos < minPos {
			minPos = pos
		}
	}

	// Extract snippet around match
	start := minPos - 50
	if start < 0 {
		start = 0
	}
	end := minPos + 150
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}

	return strings.ReplaceAll(snippet, "\n", " ")
}

// Background workers

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

func (a *Agent) runIndexer() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.IndexInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.buildIndex()
		}
	}
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
