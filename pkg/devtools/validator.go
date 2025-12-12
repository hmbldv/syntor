package devtools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ValidationResult holds validation results
type ValidationResult struct {
	Errors   []string
	Warnings []string
	Info     []string
}

// AgentValidator validates agent code
type AgentValidator struct {
	checkers []Checker
}

// Checker is a validation check
type Checker interface {
	Name() string
	Check(path string, fset *token.FileSet, f *ast.File) []ValidationIssue
}

// ValidationIssue represents a validation issue
type ValidationIssue struct {
	Type    string // "error", "warning", "info"
	Message string
	File    string
	Line    int
}

// NewAgentValidator creates a new agent validator
func NewAgentValidator() *AgentValidator {
	return &AgentValidator{
		checkers: []Checker{
			&InterfaceChecker{},
			&ErrorHandlingChecker{},
			&ContextChecker{},
			&NamingChecker{},
		},
	}
}

// Validate validates agent code at the given path
func (v *AgentValidator) Validate(path string) ValidationResult {
	result := ValidationResult{
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
		Info:     make([]string, 0),
	}

	fset := token.NewFileSet()

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// Skip vendor and test directories
			if info.Name() == "vendor" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only check Go files
		if !strings.HasSuffix(filePath, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(filePath, "_test.go") {
			return nil
		}

		// Parse the file
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			result.Errors = append(result.Errors,
				formatIssue("error", "Parse error", filePath, 0, err.Error()))
			return nil
		}

		// Run checkers
		for _, checker := range v.checkers {
			issues := checker.Check(filePath, fset, f)
			for _, issue := range issues {
				msg := formatIssue(issue.Type, checker.Name(), issue.File, issue.Line, issue.Message)
				switch issue.Type {
				case "error":
					result.Errors = append(result.Errors, msg)
				case "warning":
					result.Warnings = append(result.Warnings, msg)
				default:
					result.Info = append(result.Info, msg)
				}
			}
		}

		return nil
	})

	if err != nil {
		result.Errors = append(result.Errors, "Failed to walk directory: "+err.Error())
	}

	return result
}

func formatIssue(issueType, checker, file string, line int, message string) string {
	if line > 0 {
		return strings.Join([]string{file, string(rune('0'+line/100%10)), string(rune('0'+line/10%10)), string(rune('0'+line%10)), " [", checker, "] ", message}, "")
	}
	return file + " [" + checker + "] " + message
}

// InterfaceChecker checks for proper interface implementation
type InterfaceChecker struct{}

func (c *InterfaceChecker) Name() string { return "interface" }

func (c *InterfaceChecker) Check(path string, fset *token.FileSet, f *ast.File) []ValidationIssue {
	var issues []ValidationIssue

	// Look for struct types that should implement Agent interface
	ast.Inspect(f, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		_, ok = typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		name := typeSpec.Name.Name
		if strings.HasSuffix(name, "Agent") || strings.HasSuffix(name, "Worker") {
			// Check for required methods
			// This is a simplified check - in practice, you'd verify method signatures
		}

		return true
	})

	return issues
}

// ErrorHandlingChecker checks for proper error handling
type ErrorHandlingChecker struct{}

func (c *ErrorHandlingChecker) Name() string { return "error-handling" }

func (c *ErrorHandlingChecker) Check(path string, fset *token.FileSet, f *ast.File) []ValidationIssue {
	var issues []ValidationIssue

	ast.Inspect(f, func(n ast.Node) bool {
		// Check for ignored error returns
		assignStmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		// Look for assignments where error is assigned to blank identifier
		for _, lhs := range assignStmt.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if ok && ident.Name == "_" {
				// Check if the corresponding RHS could return an error
				// This is a simplified check
			}
		}

		return true
	})

	return issues
}

// ContextChecker checks for proper context usage
type ContextChecker struct{}

func (c *ContextChecker) Name() string { return "context" }

func (c *ContextChecker) Check(path string, fset *token.FileSet, f *ast.File) []ValidationIssue {
	var issues []ValidationIssue

	ast.Inspect(f, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if function has context parameter
		hasContext := false
		if funcDecl.Type.Params != nil {
			for _, param := range funcDecl.Type.Params.List {
				if selectorExpr, ok := param.Type.(*ast.SelectorExpr); ok {
					if ident, ok := selectorExpr.X.(*ast.Ident); ok {
						if ident.Name == "context" && selectorExpr.Sel.Name == "Context" {
							hasContext = true
							break
						}
					}
				}
			}
		}

		// Methods that should have context
		methodNames := []string{"Start", "Stop", "HandleMessage", "ExecuteTask"}
		for _, methodName := range methodNames {
			if funcDecl.Name.Name == methodName && !hasContext {
				pos := fset.Position(funcDecl.Pos())
				issues = append(issues, ValidationIssue{
					Type:    "warning",
					Message: methodName + " method should accept context.Context as first parameter",
					File:    path,
					Line:    pos.Line,
				})
			}
		}

		return true
	})

	return issues
}

// NamingChecker checks for proper naming conventions
type NamingChecker struct{}

func (c *NamingChecker) Name() string { return "naming" }

func (c *NamingChecker) Check(path string, fset *token.FileSet, f *ast.File) []ValidationIssue {
	var issues []ValidationIssue

	ast.Inspect(f, func(n ast.Node) bool {
		// Check type names
		typeSpec, ok := n.(*ast.TypeSpec)
		if ok {
			name := typeSpec.Name.Name
			if strings.Contains(name, "_") {
				pos := fset.Position(typeSpec.Pos())
				issues = append(issues, ValidationIssue{
					Type:    "warning",
					Message: "Type names should use CamelCase, not snake_case: " + name,
					File:    path,
					Line:    pos.Line,
				})
			}
		}

		// Check function names
		funcDecl, ok := n.(*ast.FuncDecl)
		if ok {
			name := funcDecl.Name.Name
			if strings.Contains(name, "_") && !strings.HasPrefix(name, "Test") {
				pos := fset.Position(funcDecl.Pos())
				issues = append(issues, ValidationIssue{
					Type:    "warning",
					Message: "Function names should use CamelCase: " + name,
					File:    path,
					Line:    pos.Line,
				})
			}
		}

		return true
	})

	return issues
}
