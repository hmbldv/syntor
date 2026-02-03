package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// InferenceClient is the interface for LLM inference.
type InferenceClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// PromptExecutor runs prompt-based hooks using an LLM.
type PromptExecutor struct {
	client InferenceClient
}

// NewPromptExecutor creates a prompt executor.
func NewPromptExecutor(client InferenceClient) *PromptExecutor {
	return &PromptExecutor{
		client: client,
	}
}

// Execute runs a prompt hook.
func (e *PromptExecutor) Execute(ctx context.Context, hook *Hook, hookCtx *HookContext) (*HookResult, error) {
	if hook.PromptTemplate == "" {
		hook.PromptTemplate = defaultPromptTemplate
	}

	// Build prompt from template
	prompt := e.buildPrompt(hook.PromptTemplate, hookCtx)

	// Get LLM response
	response, err := e.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	return e.parseResponse(response)
}

// buildPrompt substitutes context values into the template.
func (e *PromptExecutor) buildPrompt(template string, ctx *HookContext) string {
	paramsJSON, _ := json.MarshalIndent(ctx.ToolParams, "", "  ")

	replacements := map[string]string{
		"{{tool_name}}":   ctx.ToolName,
		"{{tool_params}}": string(paramsJSON),
		"{{file_path}}":   getPathFromParams(ctx.ToolParams),
		"{{session_id}}":  ctx.SessionID,
		"{{working_dir}}": ctx.WorkingDir,
	}

	result := template
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

// parseResponse converts LLM response to HookResult.
func (e *PromptExecutor) parseResponse(response string) (*HookResult, error) {
	result := &HookResult{}

	// Try to extract JSON from response
	response = strings.TrimSpace(response)

	// Look for JSON block
	if start := strings.Index(response, "{"); start >= 0 {
		if end := strings.LastIndex(response, "}"); end > start {
			jsonStr := response[start : end+1]

			var jsonResult struct {
				Decision string         `json:"decision"`
				Reason   string         `json:"reason"`
				Params   map[string]any `json:"params"`
			}

			if json.Unmarshal([]byte(jsonStr), &jsonResult) == nil {
				switch strings.ToLower(jsonResult.Decision) {
				case "approve", "allow", "yes", "ok", "proceed":
					result.Action = ActionApprove
				case "block", "deny", "no", "reject", "stop":
					result.Action = ActionBlock
				case "modify", "change", "update":
					result.Action = ActionModify
					result.ModifiedParams = jsonResult.Params
				default:
					result.Action = ActionContinue
				}
				result.Reason = jsonResult.Reason
				return result, nil
			}
		}
	}

	// Fallback: look for keywords
	lower := strings.ToLower(response)
	if strings.Contains(lower, "approve") || strings.Contains(lower, "allow") || strings.Contains(lower, "proceed") {
		result.Action = ActionApprove
	} else if strings.Contains(lower, "block") || strings.Contains(lower, "deny") || strings.Contains(lower, "reject") {
		result.Action = ActionBlock
	} else {
		result.Action = ActionContinue
	}

	// Extract reason if present
	if idx := strings.Index(lower, "reason:"); idx >= 0 {
		result.Reason = strings.TrimSpace(response[idx+7:])
	} else if idx := strings.Index(lower, "because"); idx >= 0 {
		result.Reason = strings.TrimSpace(response[idx+7:])
	} else {
		result.Reason = response
	}

	return result, nil
}

const defaultPromptTemplate = `You are a security hook evaluating whether to allow a tool execution.

Tool: {{tool_name}}
Parameters:
{{tool_params}}

Analyze this tool call and decide whether to:
1. APPROVE - Allow the tool to execute
2. BLOCK - Prevent execution with a reason
3. MODIFY - Suggest modified parameters

Consider:
- Security implications
- Potential for unintended consequences
- Whether the operation is safe

Respond with a JSON object:
{
  "decision": "approve|block|modify",
  "reason": "explanation of your decision",
  "params": {} // only if modifying
}
`
