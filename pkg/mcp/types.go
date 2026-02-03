// Package mcp provides a client for the Model Context Protocol (MCP).
// MCP enables tool discovery and invocation from external servers,
// supporting stdio, SSE, and HTTP transports.
package mcp

import (
	"context"
	"encoding/json"
	"time"
)

// Protocol version
const ProtocolVersion = "2024-11-05"

// Transport types
const (
	TransportStdio = "stdio"
	TransportSSE   = "sse"
	TransportHTTP  = "http"
)

// MessageType identifies JSON-RPC message types.
type MessageType string

const (
	MessageRequest      MessageType = "request"
	MessageResponse     MessageType = "response"
	MessageNotification MessageType = "notification"
	MessageError        MessageType = "error"
)

// JSONRPCMessage is the base structure for all MCP messages.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return e.Message
}

// Standard JSON-RPC error codes
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// ServerInfo describes an MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ClientInfo describes this MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeParams are sent during initialization.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    ClientCapabilities     `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// InitializeResult is returned from initialization.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ClientCapabilities describes client features.
type ClientCapabilities struct {
	Roots     *RootsCapability     `json:"roots,omitempty"`
	Sampling  *SamplingCapability  `json:"sampling,omitempty"`
}

// ServerCapabilities describes server features.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
}

// RootsCapability indicates root listing support.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indicates sampling support.
type SamplingCapability struct{}

// ToolsCapability indicates tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates resource support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates prompt support.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability indicates logging support.
type LoggingCapability struct{}

// Tool represents an available tool from an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolInputSchema describes tool parameters using JSON Schema.
type ToolInputSchema struct {
	Type        string                       `json:"type"`
	Properties  map[string]PropertySchema    `json:"properties,omitempty"`
	Required    []string                     `json:"required,omitempty"`
}

// PropertySchema describes a single property.
type PropertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// ListToolsResult contains the list of available tools.
type ListToolsResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor string  `json:"nextCursor,omitempty"`
}

// CallToolParams are sent when calling a tool.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallToolResult contains the tool execution result.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents content returned by a tool.
type Content struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	MimeType string      `json:"mimeType,omitempty"`
	Data     string      `json:"data,omitempty"` // Base64 for blobs/images
	Resource *Resource   `json:"resource,omitempty"`
}

// ContentType identifies the type of content.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeResource ContentType = "resource"
)

// Resource represents an MCP resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ListResourcesResult contains available resources.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceParams are sent when reading a resource.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// ReadResourceResult contains resource content.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent is the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // Base64
}

// Prompt represents a prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes a prompt parameter.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsResult contains available prompts.
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// GetPromptParams are sent when getting a prompt.
type GetPromptParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// GetPromptResult contains the expanded prompt.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage is a message in a prompt.
type PromptMessage struct {
	Role    string    `json:"role"`
	Content Content   `json:"content"`
}

// ServerConfig describes how to connect to an MCP server.
type ServerConfig struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"` // stdio, sse, http
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Timeout   time.Duration     `json:"timeout,omitempty"`
}

// Transport defines the interface for MCP communication.
type Transport interface {
	// Start initializes the transport.
	Start(ctx context.Context) error

	// Send sends a message and waits for a response.
	Send(ctx context.Context, msg *JSONRPCMessage) (*JSONRPCMessage, error)

	// SendNotification sends a notification (no response expected).
	SendNotification(ctx context.Context, msg *JSONRPCMessage) error

	// OnNotification sets a handler for incoming notifications.
	OnNotification(handler func(*JSONRPCMessage))

	// Close shuts down the transport.
	Close() error

	// IsConnected returns whether the transport is connected.
	IsConnected() bool
}

// DiscoveredTool combines tool info with its source server.
type DiscoveredTool struct {
	Tool       Tool   `json:"tool"`
	ServerName string `json:"server_name"`
	FullName   string `json:"full_name"` // server__tool format
}

// ToolCallRequest represents a request to call an MCP tool.
type ToolCallRequest struct {
	FullName  string         `json:"full_name"` // server__tool format
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolCallResponse contains the result of a tool call.
type ToolCallResponse struct {
	Success  bool      `json:"success"`
	Content  []Content `json:"content,omitempty"`
	Error    string    `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}
