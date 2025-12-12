package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter formats tool results for LLM consumption
type Formatter struct {
	maxOutputLength int
}

// NewFormatter creates a new result formatter
func NewFormatter() *Formatter {
	return &Formatter{
		maxOutputLength: 30000, // Character limit for output
	}
}

// FormatResults formats tool results as XML-like structure for LLM
func (f *Formatter) FormatResults(results []ToolResult) string {
	var sb strings.Builder

	sb.WriteString("<tool_results>\n")

	for _, result := range results {
		sb.WriteString(f.formatSingleResult(result))
	}

	sb.WriteString("</tool_results>")

	return sb.String()
}

// FormatSingleResult formats a single tool result
func (f *Formatter) FormatSingleResult(result ToolResult) string {
	return f.formatSingleResult(result)
}

func (f *Formatter) formatSingleResult(result ToolResult) string {
	var sb strings.Builder

	successStr := "false"
	if result.Success {
		successStr = "true"
	}

	sb.WriteString(fmt.Sprintf(`<result call_id="%s" tool="%s" success="%s">`,
		result.CallID, result.ToolName, successStr))
	sb.WriteString("\n")

	if result.Error != nil {
		sb.WriteString(fmt.Sprintf(`<error code="%s">%s</error>`,
			result.Error.Code, escapeXML(result.Error.Message)))
		if result.Error.Details != "" {
			sb.WriteString(fmt.Sprintf("\n<details>%s</details>", escapeXML(result.Error.Details)))
		}
	} else if result.Output != nil {
		output := f.formatOutput(result.Output)
		if len(output) > f.maxOutputLength {
			output = output[:f.maxOutputLength]
			sb.WriteString(output)
			sb.WriteString("\n[OUTPUT TRUNCATED - exceeded ")
			sb.WriteString(fmt.Sprintf("%d", f.maxOutputLength))
			sb.WriteString(" characters]")
		} else {
			sb.WriteString(output)
		}
	}

	sb.WriteString("\n</result>\n")

	return sb.String()
}

func (f *Formatter) formatOutput(output interface{}) string {
	switch v := output.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case map[string]interface{}:
		// JSON encode maps with indentation
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", output)
		}
		return string(data)
	case []interface{}:
		// JSON encode arrays with indentation
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", output)
		}
		return string(data)
	default:
		// Try JSON encoding first
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", output)
		}
		return string(data)
	}
}

// FormatError creates a formatted error result
func (f *Formatter) FormatError(callID, toolName, code, message string) string {
	result := ToolResult{
		CallID:   callID,
		ToolName: toolName,
		Success:  false,
		Error: &ToolError{
			Code:    code,
			Message: message,
		},
	}
	return f.formatSingleResult(result)
}

// FormatApprovalRequest formats an approval request for display
func (f *Formatter) FormatApprovalRequest(req *ApprovalRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Tool: %s\n", req.ToolCall.Name))
	sb.WriteString(fmt.Sprintf("Risk: %s\n", req.Risk))

	if len(req.ToolCall.Parameters) > 0 {
		sb.WriteString("Parameters:\n")
		for key, value := range req.ToolCall.Parameters {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	sb.WriteString(fmt.Sprintf("\nReason: %s", req.Reason))

	return sb.String()
}

// SetMaxOutputLength updates the maximum output length
func (f *Formatter) SetMaxOutputLength(length int) {
	if length > 0 {
		f.maxOutputLength = length
	}
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
