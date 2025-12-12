package devtools

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnknownCommand = errors.New("unknown command")
	ErrMissingArgs    = errors.New("missing required arguments")
)

// CLI represents the SYNTOR development CLI
type CLI struct {
	commands map[string]Command
	version  string
}

// Command represents a CLI command
type Command interface {
	Name() string
	Description() string
	Usage() string
	Execute(args []string) error
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	cli := &CLI{
		commands: make(map[string]Command),
		version:  "0.1.0",
	}

	// Register commands
	cli.registerCommand(&GenerateCommand{})
	cli.registerCommand(&DevServerCommand{})
	cli.registerCommand(&ValidateCommand{})
	cli.registerCommand(&HealthCommand{})
	cli.registerCommand(&StatusCommand{})
	cli.registerCommand(&ProfileCommand{})

	return cli
}

func (c *CLI) registerCommand(cmd Command) {
	c.commands[cmd.Name()] = cmd
}

// Run executes the CLI with the given arguments
func (c *CLI) Run(args []string) error {
	if len(args) == 0 {
		c.printHelp()
		return nil
	}

	cmdName := args[0]
	cmdArgs := args[1:]

	switch cmdName {
	case "help", "-h", "--help":
		if len(cmdArgs) > 0 {
			return c.printCommandHelp(cmdArgs[0])
		}
		c.printHelp()
		return nil
	case "version", "-v", "--version":
		fmt.Printf("syntor-cli version %s\n", c.version)
		return nil
	}

	cmd, exists := c.commands[cmdName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownCommand, cmdName)
	}

	return cmd.Execute(cmdArgs)
}

func (c *CLI) printHelp() {
	fmt.Println("SYNTOR Development CLI")
	fmt.Println()
	fmt.Println("Usage: syntor-cli <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")

	for _, cmd := range c.commands {
		fmt.Printf("  %-12s %s\n", cmd.Name(), cmd.Description())
	}

	fmt.Println()
	fmt.Println("Use 'syntor-cli help <command>' for more information about a command.")
}

func (c *CLI) printCommandHelp(cmdName string) error {
	cmd, exists := c.commands[cmdName]
	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownCommand, cmdName)
	}

	fmt.Printf("Usage: syntor-cli %s\n", cmd.Usage())
	fmt.Println()
	fmt.Println(cmd.Description())

	return nil
}

// GenerateCommand handles agent scaffolding
type GenerateCommand struct{}

func (c *GenerateCommand) Name() string        { return "generate" }
func (c *GenerateCommand) Description() string { return "Generate agent scaffolding code" }
func (c *GenerateCommand) Usage() string       { return "generate <agent-type> <name> [options]" }

func (c *GenerateCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("%w: agent type and name required", ErrMissingArgs)
	}

	agentType := args[0]
	name := args[1]

	generator := NewAgentGenerator()
	return generator.Generate(agentType, name)
}

// DevServerCommand handles the development server
type DevServerCommand struct{}

func (c *DevServerCommand) Name() string        { return "dev" }
func (c *DevServerCommand) Description() string { return "Start development server with hot-reload" }
func (c *DevServerCommand) Usage() string       { return "dev [--port <port>] [--watch <dir>]" }

func (c *DevServerCommand) Execute(args []string) error {
	config := DevServerConfig{
		Port:     8080,
		WatchDir: ".",
	}

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &config.Port)
				i++
			}
		case "--watch", "-w":
			if i+1 < len(args) {
				config.WatchDir = args[i+1]
				i++
			}
		}
	}

	server := NewDevServer(config)
	return server.Start()
}

// ValidateCommand handles agent validation
type ValidateCommand struct{}

func (c *ValidateCommand) Name() string        { return "validate" }
func (c *ValidateCommand) Description() string { return "Validate agent configuration and code" }
func (c *ValidateCommand) Usage() string       { return "validate <path>" }

func (c *ValidateCommand) Execute(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	validator := NewAgentValidator()
	results := validator.Validate(path)

	if len(results.Errors) > 0 {
		fmt.Println("Validation errors:")
		for _, err := range results.Errors {
			fmt.Printf("  - %s\n", err)
		}
		return errors.New("validation failed")
	}

	if len(results.Warnings) > 0 {
		fmt.Println("Validation warnings:")
		for _, warn := range results.Warnings {
			fmt.Printf("  - %s\n", warn)
		}
	}

	fmt.Println("Validation passed!")
	return nil
}

// HealthCommand checks system health
type HealthCommand struct{}

func (c *HealthCommand) Name() string        { return "health" }
func (c *HealthCommand) Description() string { return "Check system health status" }
func (c *HealthCommand) Usage() string       { return "health [--verbose]" }

func (c *HealthCommand) Execute(args []string) error {
	verbose := false
	for _, arg := range args {
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		}
	}

	checker := NewHealthChecker()
	status := checker.Check()

	fmt.Printf("System Health: %s\n", status.Overall)

	if verbose || status.Overall != "healthy" {
		fmt.Println("\nComponent Status:")
		for name, componentStatus := range status.Components {
			fmt.Printf("  %-20s %s\n", name+":", componentStatus)
		}
	}

	if status.Overall != "healthy" {
		return errors.New("system is not healthy")
	}

	return nil
}

// StatusCommand shows system status
type StatusCommand struct{}

func (c *StatusCommand) Name() string        { return "status" }
func (c *StatusCommand) Description() string { return "Show agent and system status" }
func (c *StatusCommand) Usage() string       { return "status [--agents] [--tasks]" }

func (c *StatusCommand) Execute(args []string) error {
	showAgents := false
	showTasks := false

	for _, arg := range args {
		switch arg {
		case "--agents", "-a":
			showAgents = true
		case "--tasks", "-t":
			showTasks = true
		}
	}

	// If no specific flag, show both
	if !showAgents && !showTasks {
		showAgents = true
		showTasks = true
	}

	monitor := NewStatusMonitor()
	status := monitor.GetStatus()

	if showAgents {
		fmt.Println("Agents:")
		if len(status.Agents) == 0 {
			fmt.Println("  No agents registered")
		} else {
			for _, agent := range status.Agents {
				fmt.Printf("  %-20s %-10s %s\n", agent.ID, agent.Type, agent.Status)
			}
		}
		fmt.Println()
	}

	if showTasks {
		fmt.Println("Tasks:")
		if len(status.Tasks) == 0 {
			fmt.Println("  No active tasks")
		} else {
			for _, task := range status.Tasks {
				fmt.Printf("  %-20s %-10s %s\n", task.ID, task.Type, task.Status)
			}
		}
	}

	return nil
}

// ProfileCommand handles performance profiling
type ProfileCommand struct{}

func (c *ProfileCommand) Name() string        { return "profile" }
func (c *ProfileCommand) Description() string { return "Performance profiling tools" }
func (c *ProfileCommand) Usage() string       { return "profile <cpu|mem|trace> [--duration <seconds>]" }

func (c *ProfileCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%w: profile type required (cpu, mem, trace)", ErrMissingArgs)
	}

	profileType := args[0]
	duration := 30 // default 30 seconds

	for i := 1; i < len(args); i++ {
		if args[i] == "--duration" || args[i] == "-d" {
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &duration)
				i++
			}
		}
	}

	profiler := NewProfiler()

	switch strings.ToLower(profileType) {
	case "cpu":
		return profiler.CPUProfile(duration)
	case "mem":
		return profiler.MemProfile()
	case "trace":
		return profiler.Trace(duration)
	default:
		return fmt.Errorf("unknown profile type: %s", profileType)
	}
}
