package devtools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// AgentGenerator generates agent scaffolding
type AgentGenerator struct {
	templates map[string]*template.Template
}

// AgentTemplateData holds data for agent templates
type AgentTemplateData struct {
	Name           string
	PackageName    string
	CapitalName    string
	Type           string
	Capabilities   []string
	HasHTTP        bool
	HasGRPC        bool
}

// NewAgentGenerator creates a new agent generator
func NewAgentGenerator() *AgentGenerator {
	g := &AgentGenerator{
		templates: make(map[string]*template.Template),
	}
	g.loadTemplates()
	return g
}

func (g *AgentGenerator) loadTemplates() {
	// Service agent template
	g.templates["service"] = template.Must(template.New("service").Parse(serviceAgentTemplate))

	// Worker agent template
	g.templates["worker"] = template.Must(template.New("worker").Parse(workerAgentTemplate))

	// Main file template
	g.templates["main"] = template.Must(template.New("main").Parse(mainTemplate))

	// Dockerfile template
	g.templates["dockerfile"] = template.Must(template.New("dockerfile").Parse(dockerfileTemplate))

	// Config template
	g.templates["config"] = template.Must(template.New("config").Parse(configTemplate))
}

// Generate generates agent scaffolding
func (g *AgentGenerator) Generate(agentType, name string) error {
	// Validate agent type
	if agentType != "service" && agentType != "worker" {
		return fmt.Errorf("unknown agent type: %s (use 'service' or 'worker')", agentType)
	}

	// Prepare template data
	data := AgentTemplateData{
		Name:         name,
		PackageName:  strings.ToLower(name),
		CapitalName:  strings.Title(name),
		Type:         agentType,
		Capabilities: []string{"default"},
	}

	// Create directories
	dirs := []string{
		filepath.Join("internal", data.PackageName),
		filepath.Join("cmd", data.PackageName),
		filepath.Join("deployments", "docker"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate agent file
	agentPath := filepath.Join("internal", data.PackageName, "agent.go")
	if err := g.generateFile(agentType, agentPath, data); err != nil {
		return err
	}

	// Generate main file
	mainPath := filepath.Join("cmd", data.PackageName, "main.go")
	if err := g.generateFile("main", mainPath, data); err != nil {
		return err
	}

	// Generate Dockerfile
	dockerfilePath := filepath.Join("deployments", "docker", fmt.Sprintf("Dockerfile.%s", data.PackageName))
	if err := g.generateFile("dockerfile", dockerfilePath, data); err != nil {
		return err
	}

	// Generate config file
	configPath := filepath.Join("internal", data.PackageName, "config.go")
	if err := g.generateFile("config", configPath, data); err != nil {
		return err
	}

	fmt.Printf("Generated %s agent '%s' scaffolding:\n", agentType, name)
	fmt.Printf("  - %s\n", agentPath)
	fmt.Printf("  - %s\n", mainPath)
	fmt.Printf("  - %s\n", dockerfilePath)
	fmt.Printf("  - %s\n", configPath)

	return nil
}

func (g *AgentGenerator) generateFile(templateName, path string, data AgentTemplateData) error {
	tmpl, exists := g.templates[templateName]
	if !exists {
		return fmt.Errorf("template not found: %s", templateName)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template for %s: %w", path, err)
	}

	return nil
}

// Templates
const serviceAgentTemplate = `package {{.PackageName}}

import (
	"context"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/models"
)

// {{.CapitalName}}Agent is a service agent for {{.Name}}
type {{.CapitalName}}Agent struct {
	*agent.BaseAgent
	config Config
	mu     sync.RWMutex
}

// New{{.CapitalName}}Agent creates a new {{.Name}} agent
func New{{.CapitalName}}Agent(config Config) (*{{.CapitalName}}Agent, error) {
	baseConfig := agent.Config{
		ID:           config.ID,
		Type:         agent.ServiceAgent,
		Capabilities: config.Capabilities,
	}

	base, err := agent.NewBaseAgent(baseConfig)
	if err != nil {
		return nil, err
	}

	return &{{.CapitalName}}Agent{
		BaseAgent: base,
		config:    config,
	}, nil
}

// Start starts the agent
func (a *{{.CapitalName}}Agent) Start(ctx context.Context) error {
	// Initialize resources
	if err := a.initialize(); err != nil {
		return err
	}

	// Start base agent
	return a.BaseAgent.Start(ctx)
}

// Stop stops the agent
func (a *{{.CapitalName}}Agent) Stop(ctx context.Context) error {
	// Cleanup resources
	a.cleanup()

	// Stop base agent
	return a.BaseAgent.Stop(ctx)
}

// HandleMessage handles incoming messages
func (a *{{.CapitalName}}Agent) HandleMessage(ctx context.Context, msg *models.Message) error {
	switch msg.Type {
	case "request":
		return a.handleRequest(ctx, msg)
	case "command":
		return a.handleCommand(ctx, msg)
	default:
		return a.BaseAgent.HandleMessage(ctx, msg)
	}
}

func (a *{{.CapitalName}}Agent) initialize() error {
	// TODO: Add initialization logic
	return nil
}

func (a *{{.CapitalName}}Agent) cleanup() {
	// TODO: Add cleanup logic
}

func (a *{{.CapitalName}}Agent) handleRequest(ctx context.Context, msg *models.Message) error {
	// TODO: Handle request messages
	return nil
}

func (a *{{.CapitalName}}Agent) handleCommand(ctx context.Context, msg *models.Message) error {
	// TODO: Handle command messages
	return nil
}
`

const workerAgentTemplate = `package {{.PackageName}}

import (
	"context"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/models"
)

// {{.CapitalName}}Worker is a worker agent for {{.Name}}
type {{.CapitalName}}Worker struct {
	*agent.BaseAgent
	config      Config
	taskQueue   chan *models.Task
	concurrency int
	mu          sync.RWMutex
	wg          sync.WaitGroup
}

// New{{.CapitalName}}Worker creates a new {{.Name}} worker
func New{{.CapitalName}}Worker(config Config) (*{{.CapitalName}}Worker, error) {
	baseConfig := agent.Config{
		ID:           config.ID,
		Type:         agent.WorkerAgent,
		Capabilities: config.Capabilities,
	}

	base, err := agent.NewBaseAgent(baseConfig)
	if err != nil {
		return nil, err
	}

	return &{{.CapitalName}}Worker{
		BaseAgent:   base,
		config:      config,
		taskQueue:   make(chan *models.Task, config.QueueSize),
		concurrency: config.Concurrency,
	}, nil
}

// Start starts the worker
func (w *{{.CapitalName}}Worker) Start(ctx context.Context) error {
	// Start task processors
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.processTaskLoop(ctx)
	}

	// Start base agent
	return w.BaseAgent.Start(ctx)
}

// Stop stops the worker
func (w *{{.CapitalName}}Worker) Stop(ctx context.Context) error {
	close(w.taskQueue)
	w.wg.Wait()
	return w.BaseAgent.Stop(ctx)
}

// ExecuteTask executes a task
func (w *{{.CapitalName}}Worker) ExecuteTask(ctx context.Context, task *models.Task) (*models.TaskResult, error) {
	// TODO: Implement task execution
	result := &models.TaskResult{
		TaskID:      task.ID,
		Status:      "completed",
		CompletedAt: time.Now(),
	}
	return result, nil
}

func (w *{{.CapitalName}}Worker) processTaskLoop(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-w.taskQueue:
			if !ok {
				return
			}
			w.processTask(ctx, task)
		}
	}
}

func (w *{{.CapitalName}}Worker) processTask(ctx context.Context, task *models.Task) {
	// TODO: Add task processing logic
	_, err := w.ExecuteTask(ctx, task)
	if err != nil {
		// Handle error
	}
}
`

const mainTemplate = `package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/syntor/syntor/internal/{{.PackageName}}"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	config := {{.PackageName}}.LoadConfig()

	// Create agent
	{{if eq .Type "service"}}
	agent, err := {{.PackageName}}.New{{.CapitalName}}Agent(config)
	{{else}}
	agent, err := {{.PackageName}}.New{{.CapitalName}}Worker(config)
	{{end}}
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Start agent
	if err := agent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	cancel()

	if err := agent.Stop(context.Background()); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
`

const dockerfileTemplate = `# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /{{.PackageName}} ./cmd/{{.PackageName}}

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /{{.PackageName}} .

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080

ENTRYPOINT ["./{{.PackageName}}"]
`

const configTemplate = `package {{.PackageName}}

import (
	"os"
	"strconv"
)

// Config holds the {{.Name}} configuration
type Config struct {
	ID           string
	Capabilities []string
	{{if eq .Type "worker"}}
	QueueSize    int
	Concurrency  int
	{{end}}
	KafkaBrokers []string
	RedisAddr    string
}

// LoadConfig loads configuration from environment
func LoadConfig() Config {
	config := Config{
		ID:           getEnv("AGENT_ID", "{{.PackageName}}-1"),
		Capabilities: []string{"{{.PackageName}}"},
		KafkaBrokers: []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
	}

	{{if eq .Type "worker"}}
	config.QueueSize = getEnvInt("QUEUE_SIZE", 100)
	config.Concurrency = getEnvInt("CONCURRENCY", 4)
	{{end}}

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
`
