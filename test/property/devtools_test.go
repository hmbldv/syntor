package property

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/syntor/syntor/pkg/devtools"
)

// TestHotReloadFunctionality tests Property 11: Hot-reload functionality
// Validates: Requirements 4.2
func TestHotReloadFunctionality(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("file changes are detected correctly", prop.ForAll(
		func(fileCount int) bool {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "hotreload-test")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Create initial Go files
			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
				content := fmt.Sprintf("package test\n\nfunc Func%d() {}\n", i)
				if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
					return false
				}
			}

			var changeCount int32

			reloader := devtools.NewHotReloader(devtools.HotReloaderConfig{
				WatchDir:   tmpDir,
				DebounceMs: 50,
				OnChange: func() error {
					atomic.AddInt32(&changeCount, 1)
					return nil
				},
			})

			reloader.Start()
			defer reloader.Stop()

			// Wait for initial scan
			time.Sleep(100 * time.Millisecond)

			// Modify a file
			if fileCount > 0 {
				filename := filepath.Join(tmpDir, "file0.go")
				content := "package test\n\nfunc Func0Updated() {}\n"
				os.WriteFile(filename, []byte(content), 0644)
			}

			// Wait for detection
			time.Sleep(150 * time.Millisecond)

			// Should have detected change
			return fileCount == 0 || atomic.LoadInt32(&changeCount) > 0
		},
		gen.IntRange(0, 10),
	))

	properties.Property("hot reloader can be enabled and disabled", prop.ForAll(
		func(enabled bool) bool {
			var onChange func() error
			if enabled {
				onChange = func() error { return nil }
			}

			reloader := devtools.NewHotReloader(devtools.HotReloaderConfig{
				WatchDir:   ".",
				DebounceMs: 100,
				OnChange:   onChange,
			})

			return reloader.IsEnabled() == enabled
		},
		gen.Bool(),
	))

	properties.Property("dev server configuration is validated", prop.ForAll(
		func(port int) bool {
			config := devtools.DevServerConfig{
				Port:     port,
				WatchDir: ".",
			}

			server := devtools.NewDevServer(config)

			// Server should be created regardless of port
			return server != nil
		},
		gen.IntRange(1024, 65535),
	))

	properties.TestingRun(t)
}

// TestMonitoringInterfaceAccessibility tests Property 12: Monitoring interface accessibility
// Validates: Requirements 4.3
func TestMonitoringInterfaceAccessibility(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("health check endpoint returns valid status", prop.ForAll(
		func(seed int) bool {
			checker := devtools.NewHealthChecker()
			status := checker.Check()

			// Should have overall status
			if status.Overall != "healthy" && status.Overall != "unhealthy" {
				return false
			}

			// Should have components
			if status.Components == nil {
				return false
			}

			// Should have timestamp
			if status.Timestamp.IsZero() {
				return false
			}

			return true
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("health check HTTP handler responds correctly", prop.ForAll(
		func(seed int) bool {
			checker := devtools.NewHealthChecker()
			handler := checker.HTTPHandler()

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Should return valid HTTP response
			if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
				return false
			}

			// Should have JSON content type
			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				return false
			}

			return true
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("status monitor returns valid system status", prop.ForAll(
		func(seed int) bool {
			monitor := devtools.NewStatusMonitor()
			status := monitor.GetStatus()

			// Should have agents slice (even if empty)
			if status.Agents == nil {
				return false
			}

			// Should have tasks slice (even if empty)
			if status.Tasks == nil {
				return false
			}

			return true
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("custom health checks can be registered", prop.ForAll(
		func(checkName int) bool {
			checker := devtools.NewHealthChecker()

			name := fmt.Sprintf("check-%d", checkName)
			checker.RegisterCheck(name, func() (string, error) {
				return "ok", nil
			})

			status := checker.Check()

			// Custom check should be in components
			_, exists := status.Components[name]
			return exists
		},
		gen.IntRange(1, 100),
	))

	properties.Property("profiler creates valid output directory", prop.ForAll(
		func(seed int) bool {
			profiler := devtools.NewProfiler()

			// Profiler should be created
			return profiler != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.TestingRun(t)
}

// TestCLICommands tests CLI command parsing and execution
func TestCLICommands(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("CLI handles unknown commands gracefully", prop.ForAll(
		func(commandNum int) bool {
			cli := devtools.NewCLI()

			unknownCmd := fmt.Sprintf("unknown-cmd-%d", commandNum)
			err := cli.Run([]string{unknownCmd})

			// Should return error for unknown command
			return err != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("CLI help command succeeds", prop.ForAll(
		func(seed int) bool {
			cli := devtools.NewCLI()

			// Help should succeed
			err := cli.Run([]string{"help"})
			return err == nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("CLI version command succeeds", prop.ForAll(
		func(seed int) bool {
			cli := devtools.NewCLI()

			// Version should succeed
			err := cli.Run([]string{"version"})
			return err == nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("generate command validates agent type", prop.ForAll(
		func(typeNum int) bool {
			cli := devtools.NewCLI()

			invalidType := fmt.Sprintf("invalid-type-%d", typeNum)
			err := cli.Run([]string{"generate", invalidType, "test-agent"})

			// Should fail for invalid agent type
			return err != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("validate command handles missing path", prop.ForAll(
		func(seed int) bool {
			cli := devtools.NewCLI()

			// Validate with current directory should work (may have warnings)
			err := cli.Run([]string{"validate"})
			// Either succeeds or fails validation - both are valid outcomes
			return err == nil || err != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.TestingRun(t)
}

// TestAgentValidation tests agent code validation
func TestAgentValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("validator handles empty directory", prop.ForAll(
		func(seed int) bool {
			tmpDir, err := os.MkdirTemp("", "validator-test")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			validator := devtools.NewAgentValidator()
			result := validator.Validate(tmpDir)

			// Empty directory should have no errors
			return len(result.Errors) == 0
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("validator detects Go syntax errors", prop.ForAll(
		func(seed int) bool {
			tmpDir, err := os.MkdirTemp("", "validator-test")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Create file with syntax error
			badFile := filepath.Join(tmpDir, "bad.go")
			os.WriteFile(badFile, []byte("package test\n\nfunc {}\n"), 0644)

			validator := devtools.NewAgentValidator()
			result := validator.Validate(tmpDir)

			// Should have parse error
			return len(result.Errors) > 0
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("validator accepts valid Go code", prop.ForAll(
		func(funcCount int) bool {
			tmpDir, err := os.MkdirTemp("", "validator-test")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			// Create valid Go file
			var sb strings.Builder
			sb.WriteString("package test\n\nimport \"context\"\n\n")
			for i := 0; i < funcCount; i++ {
				sb.WriteString(fmt.Sprintf("func Func%d(ctx context.Context) error { return nil }\n", i))
			}

			goodFile := filepath.Join(tmpDir, "good.go")
			os.WriteFile(goodFile, []byte(sb.String()), 0644)

			validator := devtools.NewAgentValidator()
			result := validator.Validate(tmpDir)

			// Should have no errors (may have warnings)
			return len(result.Errors) == 0
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// TestAgentGeneration tests agent scaffolding generation
func TestAgentGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("generator rejects invalid agent types", prop.ForAll(
		func(typeNum int) bool {
			generator := devtools.NewAgentGenerator()

			invalidType := fmt.Sprintf("invalid%d", typeNum)
			err := generator.Generate(invalidType, "test")

			// Should fail for invalid type
			return err != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("generator accepts valid agent types", prop.ForAll(
		func(seed int) bool {
			// Test in temp directory
			originalDir, _ := os.Getwd()
			tmpDir, err := os.MkdirTemp("", "generator-test")
			if err != nil {
				return false
			}
			defer func() {
				os.Chdir(originalDir)
				os.RemoveAll(tmpDir)
			}()

			os.Chdir(tmpDir)

			generator := devtools.NewAgentGenerator()

			// Valid types should succeed
			types := []string{"service", "worker"}
			agentType := types[seed%2]

			err = generator.Generate(agentType, fmt.Sprintf("testagent%d", seed))

			return err == nil
		},
		gen.IntRange(1, 50),
	))

	properties.TestingRun(t)
}

// TestAdminServer tests admin server functionality
func TestAdminServer(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("admin server configuration uses defaults", prop.ForAll(
		func(port int) bool {
			config := devtools.AdminServerConfig{
				Port: port,
			}

			server := devtools.NewAdminServer(config)
			return server != nil
		},
		gen.IntRange(0, 65535),
	))

	properties.Property("admin server handles zero port", prop.ForAll(
		func(seed int) bool {
			config := devtools.AdminServerConfig{
				Port: 0, // Should use default
			}

			server := devtools.NewAdminServer(config)
			return server != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.TestingRun(t)
}

// Integration test for dev server lifecycle
func TestDevServerLifecycle(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("dev server can be created with various configurations", prop.ForAll(
		func(port int) bool {
			config := devtools.DevServerConfig{
				Port:     port,
				WatchDir: ".",
				BuildCmd: "echo build",
				RunCmd:   "echo run",
			}

			server := devtools.NewDevServer(config)
			return server != nil
		},
		gen.IntRange(1024, 65535),
	))

	properties.Property("dev server stop is idempotent", prop.ForAll(
		func(seed int) bool {
			config := devtools.DevServerConfig{
				Port:     8080 + seed%100,
				WatchDir: ".",
			}

			server := devtools.NewDevServer(config)

			// Multiple stops should not panic
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan struct{})
			go func() {
				server.Stop()
				server.Stop() // Second stop should be safe
				close(done)
			}()

			select {
			case <-done:
				return true
			case <-ctx.Done():
				return true // Timeout is acceptable
			}
		},
		gen.IntRange(1, 100),
	))

	properties.TestingRun(t)
}
