package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/syntor/syntor/pkg/models"
)

// Built-in task handlers for common operations

// DataProcessingHandler handles data transformation tasks
func DataProcessingHandler(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	inputData, ok := task.Payload["data"]
	if !ok {
		return nil, fmt.Errorf("missing 'data' in payload")
	}

	operation, _ := task.Payload["operation"].(string)
	if operation == "" {
		operation = "passthrough"
	}

	var result interface{}

	switch operation {
	case "uppercase":
		if str, ok := inputData.(string); ok {
			result = strings.ToUpper(str)
		} else {
			return nil, fmt.Errorf("uppercase requires string input")
		}

	case "lowercase":
		if str, ok := inputData.(string); ok {
			result = strings.ToLower(str)
		} else {
			return nil, fmt.Errorf("lowercase requires string input")
		}

	case "json_parse":
		if str, ok := inputData.(string); ok {
			var parsed interface{}
			if err := json.Unmarshal([]byte(str), &parsed); err != nil {
				return nil, fmt.Errorf("json parse failed: %w", err)
			}
			result = parsed
		} else {
			return nil, fmt.Errorf("json_parse requires string input")
		}

	case "json_stringify":
		data, err := json.Marshal(inputData)
		if err != nil {
			return nil, fmt.Errorf("json stringify failed: %w", err)
		}
		result = string(data)

	case "passthrough":
		result = inputData

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	return &models.TaskResult{
		Data: map[string]interface{}{
			"output":    result,
			"operation": operation,
		},
	}, nil
}

// FileProcessingHandler handles file operations
func FileProcessingHandler(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	operation, _ := task.Payload["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("missing 'operation' in payload")
	}

	switch operation {
	case "read":
		path, _ := task.Payload["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("missing 'path' for read operation")
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"content": string(content),
				"size":    len(content),
				"path":    path,
			},
		}, nil

	case "write":
		path, _ := task.Payload["path"].(string)
		content, _ := task.Payload["content"].(string)
		if path == "" || content == "" {
			return nil, fmt.Errorf("missing 'path' or 'content' for write operation")
		}

		// Ensure directory exists
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"path":    path,
				"written": len(content),
			},
		}, nil

	case "delete":
		path, _ := task.Payload["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("missing 'path' for delete operation")
		}

		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("failed to delete file: %w", err)
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"deleted": path,
			},
		}, nil

	case "list":
		path, _ := task.Payload["path"].(string)
		if path == "" {
			path = "."
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to list directory: %w", err)
		}

		files := make([]map[string]interface{}, 0, len(entries))
		for _, entry := range entries {
			info, _ := entry.Info()
			files = append(files, map[string]interface{}{
				"name":  entry.Name(),
				"isDir": entry.IsDir(),
				"size":  info.Size(),
			})
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"path":  path,
				"files": files,
				"count": len(files),
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown file operation: %s", operation)
	}
}

// HTTPRequestHandler handles HTTP request tasks
func HTTPRequestHandler(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	url, _ := task.Payload["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("missing 'url' in payload")
	}

	method, _ := task.Payload["method"].(string)
	if method == "" {
		method = "GET"
	}

	// Create request
	var body io.Reader
	if bodyStr, ok := task.Payload["body"].(string); ok {
		body = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	if headers, ok := task.Payload["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &models.TaskResult{
		Data: map[string]interface{}{
			"status_code": resp.StatusCode,
			"status":      resp.Status,
			"body":        string(respBody),
			"headers":     resp.Header,
		},
	}, nil
}

// ShellCommandHandler handles shell command execution
func ShellCommandHandler(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	command, _ := task.Payload["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("missing 'command' in payload")
	}

	// Get working directory
	workDir, _ := task.Payload["work_dir"].(string)

	// Create command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Execute
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("command execution failed: %w", err)
		}
	}

	return &models.TaskResult{
		Data: map[string]interface{}{
			"output":    string(output),
			"exit_code": exitCode,
			"command":   command,
		},
	}, nil
}

// ComputeHandler handles compute-intensive tasks
func ComputeHandler(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	operation, _ := task.Payload["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("missing 'operation' in payload")
	}

	switch operation {
	case "sum":
		numbers, ok := task.Payload["numbers"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("missing 'numbers' array for sum operation")
		}

		var sum float64
		for _, n := range numbers {
			switch v := n.(type) {
			case float64:
				sum += v
			case int:
				sum += float64(v)
			}
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"sum":   sum,
				"count": len(numbers),
			},
		}, nil

	case "average":
		numbers, ok := task.Payload["numbers"].([]interface{})
		if !ok || len(numbers) == 0 {
			return nil, fmt.Errorf("missing or empty 'numbers' array for average operation")
		}

		var sum float64
		for _, n := range numbers {
			switch v := n.(type) {
			case float64:
				sum += v
			case int:
				sum += float64(v)
			}
		}

		return &models.TaskResult{
			Data: map[string]interface{}{
				"average": sum / float64(len(numbers)),
				"sum":     sum,
				"count":   len(numbers),
			},
		}, nil

	case "fibonacci":
		n, ok := task.Payload["n"].(float64)
		if !ok {
			return nil, fmt.Errorf("missing 'n' for fibonacci operation")
		}

		result := fibonacci(int(n))
		return &models.TaskResult{
			Data: map[string]interface{}{
				"n":      int(n),
				"result": result,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown compute operation: %s", operation)
	}
}

func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}

// RegisterDefaultHandlers registers all built-in handlers to an agent
func RegisterDefaultHandlers(a *Agent) {
	a.RegisterTaskHandler("data-processing", DataProcessingHandler)
	a.RegisterTaskHandler("file-processing", FileProcessingHandler)
	a.RegisterTaskHandler("http-request", HTTPRequestHandler)
	a.RegisterTaskHandler("shell-command", ShellCommandHandler)
	a.RegisterTaskHandler("compute", ComputeHandler)
}
