package coordination

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Parser extracts structured intents from LLM responses
type Parser struct {
	jsonBlockRegex *regexp.Regexp
	jsonInlineRegex *regexp.Regexp
}

// NewParser creates a new intent parser
func NewParser() *Parser {
	return &Parser{
		// Match ```json ... ``` blocks
		jsonBlockRegex: regexp.MustCompile("(?s)```json\\s*\\n?(.+?)\\n?```"),
		// Match inline JSON objects (for fallback)
		jsonInlineRegex: regexp.MustCompile(`\{[^{}]*"action"\s*:\s*"[^"]+"`),
	}
}

// ParseResponse parses an LLM response for handoff intents or plans
func (p *Parser) ParseResponse(response string) (*ParseResult, error) {
	result := &ParseResult{
		RawResponse: response,
	}

	// Try to find JSON blocks
	matches := p.jsonBlockRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		jsonStr := strings.TrimSpace(match[1])

		// Try to parse as HandoffIntent
		var intent HandoffIntent
		if err := json.Unmarshal([]byte(jsonStr), &intent); err == nil {
			if intent.Action != "" {
				if intent.Action == ActionPlan {
					// Parse as plan
					var plan ExecutionPlan
					if err := json.Unmarshal([]byte(jsonStr), &plan); err == nil {
						result.Plan = &plan
						result.HasPlan = true
					}
				} else {
					result.Intents = append(result.Intents, &intent)
					result.HasIntent = true
				}
			}
		}

		// Also try to parse as ExecutionPlan directly
		var plan ExecutionPlan
		if err := json.Unmarshal([]byte(jsonStr), &plan); err == nil {
			if len(plan.Steps) > 0 {
				result.Plan = &plan
				result.HasPlan = true
			}
		}
	}

	// Extract text content (everything outside JSON blocks)
	result.TextContent = p.extractTextContent(response)

	return result, nil
}

// ParseHandoffIntent parses a single handoff intent from JSON
func (p *Parser) ParseHandoffIntent(jsonStr string) (*HandoffIntent, error) {
	var intent HandoffIntent
	if err := json.Unmarshal([]byte(jsonStr), &intent); err != nil {
		return nil, fmt.Errorf("failed to parse handoff intent: %w", err)
	}

	// Validate required fields
	if intent.Action == "" {
		return nil, fmt.Errorf("handoff intent missing action field")
	}
	if intent.Target == "" && intent.Action != ActionPlan {
		return nil, fmt.Errorf("handoff intent missing target field")
	}

	return &intent, nil
}

// ParseExecutionPlan parses an execution plan from JSON
func (p *Parser) ParseExecutionPlan(jsonStr string) (*ExecutionPlan, error) {
	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse execution plan: %w", err)
	}

	// Validate required fields
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("execution plan has no steps")
	}

	// Auto-populate TotalAgents if not set
	if plan.TotalAgents == 0 {
		plan.TotalAgents = len(plan.GetAgentNames())
	}

	return &plan, nil
}

// extractTextContent removes JSON blocks and returns the text content
func (p *Parser) extractTextContent(response string) string {
	// Remove JSON blocks
	text := p.jsonBlockRegex.ReplaceAllString(response, "")

	// Clean up extra whitespace
	text = strings.TrimSpace(text)

	// Collapse multiple newlines into two
	multiNewline := regexp.MustCompile(`\n{3,}`)
	text = multiNewline.ReplaceAllString(text, "\n\n")

	return text
}

// ContainsIntent checks if a response contains any handoff intent
func (p *Parser) ContainsIntent(response string) bool {
	result, _ := p.ParseResponse(response)
	return result != nil && (result.HasIntent || result.HasPlan)
}

// ParseResult contains the parsed response
type ParseResult struct {
	RawResponse string           // Original response
	TextContent string           // Text content without JSON blocks
	Intents     []*HandoffIntent // Extracted handoff intents
	Plan        *ExecutionPlan   // Extracted execution plan (if any)
	HasIntent   bool             // Whether any intent was found
	HasPlan     bool             // Whether a plan was found
}

// GetFirstIntent returns the first parsed intent, if any
func (r *ParseResult) GetFirstIntent() *HandoffIntent {
	if len(r.Intents) > 0 {
		return r.Intents[0]
	}
	return nil
}

// FormatIntentForDisplay formats a handoff intent for user display
func FormatIntentForDisplay(intent *HandoffIntent) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Action: %s\n", intent.Action))
	sb.WriteString(fmt.Sprintf("Target: %s\n", intent.Target))
	sb.WriteString(fmt.Sprintf("Task: %s\n", intent.Task))

	if intent.Priority != "" {
		sb.WriteString(fmt.Sprintf("Priority: %s\n", intent.Priority))
	}

	if len(intent.Context) > 0 {
		sb.WriteString("Context:\n")
		for k, v := range intent.Context {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	return sb.String()
}

// FormatPlanForDisplay formats an execution plan for user display
func FormatPlanForDisplay(plan *ExecutionPlan, detailed bool) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Plan: %s\n", plan.Summary))
	sb.WriteString(fmt.Sprintf("Agents: %d | Steps: %d", plan.TotalAgents, len(plan.Steps)))

	if plan.Estimated != "" {
		sb.WriteString(fmt.Sprintf(" | Est: %s", plan.Estimated))
	}
	sb.WriteString("\n")

	if detailed {
		sb.WriteString("\nSteps:\n")
		for _, step := range plan.Steps {
			deps := ""
			if len(step.DependsOn) > 0 {
				deps = fmt.Sprintf(" (after %v)", step.DependsOn)
			}
			sb.WriteString(fmt.Sprintf("  %d. [%s] %s%s\n", step.StepNumber, step.Agent, step.Action, deps))
		}
	}

	return sb.String()
}
