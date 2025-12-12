package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PathValidator validates file system paths
type PathValidator struct {
	workingDir   string
	deniedPaths  []string
	deniedNames  []string
	allowedPaths []string
}

// NewPathValidator creates a new path validator
func NewPathValidator(workingDir string) *PathValidator {
	return &PathValidator{
		workingDir: workingDir,
		deniedPaths: []string{
			"/etc/shadow",
			"/etc/passwd",
			"/etc/sudoers",
			"~/.ssh",
			"~/.gnupg",
			"~/.aws/credentials",
			"~/.config/gcloud",
		},
		deniedNames: []string{
			".env",
			".env.local",
			".env.production",
			"*.pem",
			"*.key",
			"*.p12",
			"*.pfx",
			"id_rsa",
			"id_ed25519",
			"credentials.json",
			"secrets.yaml",
			"secrets.yml",
		},
	}
}

// ValidatePath checks if a path is safe to access
func (v *PathValidator) ValidatePath(path string, write bool) error {
	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot expand home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Make path absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		// Clean the path and verify it doesn't escape
		cleanPath := filepath.Clean(absPath)
		if cleanPath != absPath {
			return fmt.Errorf("path traversal detected")
		}
	}

	// Check denied paths
	for _, denied := range v.deniedPaths {
		expandedDenied := denied
		if strings.HasPrefix(denied, "~") {
			home, _ := os.UserHomeDir()
			expandedDenied = filepath.Join(home, denied[1:])
		}
		if strings.HasPrefix(absPath, expandedDenied) {
			return fmt.Errorf("access to path denied: %s", path)
		}
	}

	// Check denied file names
	baseName := filepath.Base(absPath)
	for _, pattern := range v.deniedNames {
		if strings.Contains(pattern, "*") {
			// Glob pattern matching
			matched, _ := filepath.Match(pattern, baseName)
			if matched {
				return fmt.Errorf("access to sensitive file denied: %s", baseName)
			}
		} else if baseName == pattern {
			return fmt.Errorf("access to sensitive file denied: %s", baseName)
		}
	}

	// For write operations, ensure we're within working directory
	if write && v.workingDir != "" {
		absWorkDir, _ := filepath.Abs(v.workingDir)
		if !strings.HasPrefix(absPath, absWorkDir) {
			return fmt.Errorf("write operations restricted to working directory: %s", v.workingDir)
		}
	}

	return nil
}

// SetWorkingDir updates the working directory
func (v *PathValidator) SetWorkingDir(dir string) {
	v.workingDir = dir
}

// AddDeniedPath adds a path to the deny list
func (v *PathValidator) AddDeniedPath(path string) {
	v.deniedPaths = append(v.deniedPaths, path)
}

// AddAllowedPath adds an exception to allow a specific path
func (v *PathValidator) AddAllowedPath(path string) {
	v.allowedPaths = append(v.allowedPaths, path)
}

// CommandValidator validates shell commands
type CommandValidator struct {
	allowedCommands []string
	deniedPatterns  []*regexp.Regexp
	deniedCommands  []string
}

// NewCommandValidator creates a new command validator
func NewCommandValidator() *CommandValidator {
	cv := &CommandValidator{
		allowedCommands: []string{
			// File operations (read-only)
			"ls", "cat", "head", "tail", "less", "more", "file", "stat", "wc",
			// Search
			"find", "grep", "rg", "fd", "ag", "awk", "sed",
			// Navigation
			"pwd", "cd", "tree",
			// Development tools
			"git", "go", "npm", "npx", "yarn", "pnpm", "node", "python", "python3",
			"pip", "pip3", "cargo", "rustc", "make", "cmake", "gcc", "g++",
			// Build and test
			"go build", "go test", "go run", "go mod", "go get",
			"npm run", "npm test", "npm install", "npm ci",
			"yarn build", "yarn test", "yarn install",
			"cargo build", "cargo test", "cargo run",
			"make", "cmake",
			// Docker (read operations)
			"docker ps", "docker images", "docker logs",
			// Utilities
			"echo", "printf", "date", "whoami", "env", "which", "type",
			"curl", "wget", "jq", "yq",
		},
		deniedCommands: []string{
			"sudo", "su", "doas",
			"rm -rf /", "rm -rf /*", "rm -rf ~",
			"chmod 777", "chmod -R 777",
			"mkfs", "dd if=",
			":(){ :|:& };:", // Fork bomb
			"> /dev/sda",
			"mv / ", "cp / ",
		},
	}

	// Compile denied patterns
	cv.deniedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)sudo\s+`),
		regexp.MustCompile(`(?i)rm\s+-[rf]*\s+/`),
		regexp.MustCompile(`(?i)rm\s+-[rf]*\s+\*`),
		regexp.MustCompile(`>\s*/dev/`),
		regexp.MustCompile(`(?i)chmod\s+777`),
		regexp.MustCompile(`(?i)chmod\s+-R\s+777`),
		regexp.MustCompile(`(?i)mkfs\.`),
		regexp.MustCompile(`(?i)dd\s+if=/dev/`),
		regexp.MustCompile(`;\s*:`),                  // Fork bomb patterns
		regexp.MustCompile(`\|\s*sh\s*$`),            // Pipe to shell
		regexp.MustCompile(`\|\s*bash\s*$`),          // Pipe to bash
		regexp.MustCompile(`eval\s+.*\$`),            // Eval with variable
		regexp.MustCompile(`(?i)curl.*\|\s*sh`),      // Curl pipe to shell
		regexp.MustCompile(`(?i)wget.*\|\s*sh`),      // Wget pipe to shell
		regexp.MustCompile(`(?i)curl.*\|\s*bash`),    // Curl pipe to bash
		regexp.MustCompile(`(?i)wget.*\|\s*bash`),    // Wget pipe to bash
		regexp.MustCompile(`>\s*/etc/`),              // Overwrite system files
		regexp.MustCompile(`(?i)passwd`),             // Password operations
		regexp.MustCompile(`(?i)useradd|userdel`),    // User management
		regexp.MustCompile(`(?i)groupadd|groupdel`),  // Group management
		regexp.MustCompile(`(?i)chown\s+-R\s+/`),     // Recursive chown on root
		regexp.MustCompile(`(?i)shutdown|reboot`),    // System control
		regexp.MustCompile(`(?i)systemctl\s+stop`),   // Service control
		regexp.MustCompile(`(?i)service\s+.*\s+stop`),
	}

	return cv
}

// ValidateCommand checks if a command is safe to execute
func (cv *CommandValidator) ValidateCommand(command string) error {
	command = strings.TrimSpace(command)

	if command == "" {
		return fmt.Errorf("empty command")
	}

	// Check denied commands (exact match)
	for _, denied := range cv.deniedCommands {
		if strings.Contains(command, denied) {
			return fmt.Errorf("command denied: contains '%s'", denied)
		}
	}

	// Check denied patterns
	for _, pattern := range cv.deniedPatterns {
		if pattern.MatchString(command) {
			return fmt.Errorf("command denied: matches dangerous pattern")
		}
	}

	return nil
}

// IsReadOnlyCommand checks if a command is read-only
func (cv *CommandValidator) IsReadOnlyCommand(command string) bool {
	readOnlyCommands := []string{
		"ls", "cat", "head", "tail", "less", "more", "file", "stat", "wc",
		"find", "grep", "rg", "fd", "ag",
		"pwd", "tree",
		"git status", "git log", "git diff", "git show", "git branch",
		"docker ps", "docker images", "docker logs",
		"echo", "printf", "date", "whoami", "env", "which", "type",
	}

	cmd := strings.Fields(command)
	if len(cmd) == 0 {
		return false
	}

	baseCmd := cmd[0]
	for _, ro := range readOnlyCommands {
		if baseCmd == ro || strings.HasPrefix(command, ro+" ") {
			return true
		}
	}

	return false
}

// AddAllowedCommand adds a command to the allow list
func (cv *CommandValidator) AddAllowedCommand(command string) {
	cv.allowedCommands = append(cv.allowedCommands, command)
}

// AddDeniedPattern adds a regex pattern to the deny list
func (cv *CommandValidator) AddDeniedPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	cv.deniedPatterns = append(cv.deniedPatterns, re)
	return nil
}
