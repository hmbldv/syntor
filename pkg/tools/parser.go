package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
)

// Parser extracts tool calls from LLM responses
type Parser struct {
	jsonBlockRegex *regexp.Regexp
}

// NewParser creates a new tool parser
func NewParser() *Parser {
	return &Parser{
		jsonBlockRegex: regexp.MustCompile("(?s)```json\\s*\\n?(.+?)\\n?```"),
	}
}

// ParseResult contains parsed tool calls and remaining text
type ParseResult struct {
	ToolCalls   []ToolCall
	TextContent string
	HasTools    bool
	RawJSON     []string // The raw JSON blocks found
}

// Parse extracts tool calls from an LLM response
func (p *Parser) Parse(response string) (*ParseResult, error) {
	result := &ParseResult{
		ToolCalls: make([]ToolCall, 0),
		RawJSON:   make([]string, 0),
	}

	// Find all JSON blocks
	matches := p.jsonBlockRegex.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		jsonStr := strings.TrimSpace(match[1])
		result.RawJSON = append(result.RawJSON, jsonStr)

		// Try to parse as ToolCallBatch
		var batch ToolCallBatch
		if err := json.Unmarshal([]byte(jsonStr), &batch); err == nil {
			if len(batch.ToolCalls) > 0 {
				// Validate and add tool calls
				for _, call := range batch.ToolCalls {
					if call.Name != "" {
						// Generate ID if not provided
						if call.ID == "" {
							call.ID = generateCallID()
						}
						result.ToolCalls = append(result.ToolCalls, call)
					}
				}
				result.HasTools = len(result.ToolCalls) > 0
			}
		}
	}

	// Extract text content (remove JSON blocks)
	result.TextContent = p.jsonBlockRegex.ReplaceAllString(response, "")
	result.TextContent = strings.TrimSpace(result.TextContent)

	return result, nil
}

// ParseSingle attempts to parse a single JSON string as a tool call batch
func (p *Parser) ParseSingle(jsonStr string) ([]ToolCall, error) {
	var batch ToolCallBatch
	if err := json.Unmarshal([]byte(jsonStr), &batch); err != nil {
		return nil, err
	}

	calls := make([]ToolCall, 0, len(batch.ToolCalls))
	for _, call := range batch.ToolCalls {
		if call.Name != "" {
			if call.ID == "" {
				call.ID = generateCallID()
			}
			calls = append(calls, call)
		}
	}

	return calls, nil
}

// ContainsToolCalls quickly checks if a response contains potential tool calls
func (p *Parser) ContainsToolCalls(response string) bool {
	// Quick heuristic check before full parsing
	if !strings.Contains(response, "tool_calls") {
		return false
	}
	if !strings.Contains(response, "```json") {
		return false
	}
	return true
}

// call ID counter for generating unique IDs (atomic for thread safety)
var callIDCounter int64

func generateCallID() string {
	id := atomic.AddInt64(&callIDCounter, 1)
	return fmt.Sprintf("call_%03d", id%1000)
}

// ValidateToolCall checks if a tool call has required fields
func ValidateToolCall(call ToolCall) error {
	if call.Name == "" {
		return &ToolError{
			Code:    ErrCodeInvalidParams,
			Message: "tool call missing name",
		}
	}
	return nil
}
