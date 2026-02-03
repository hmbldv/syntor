package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client manages connections to MCP servers.
type Client struct {
	config Config

	// Connected servers
	servers   map[string]*ServerConnection
	serversMu sync.RWMutex

	// Message ID counter
	nextID int64

	// Tool cache
	toolCache   map[string]*DiscoveredTool
	toolCacheMu sync.RWMutex
}

// Config holds client configuration.
type Config struct {
	// Servers is the list of MCP servers to connect to
	Servers []ServerConfig `yaml:"servers" json:"servers"`

	// DefaultTimeout for server operations
	DefaultTimeout time.Duration `yaml:"default_timeout" json:"default_timeout"`

	// AutoConnect connects to all servers on Start
	AutoConnect bool `yaml:"auto_connect" json:"auto_connect"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultTimeout: 30 * time.Second,
		AutoConnect:    true,
	}
}

// ServerConnection represents a connection to an MCP server.
type ServerConnection struct {
	config       ServerConfig
	transport    Transport
	capabilities ServerCapabilities
	serverInfo   ServerInfo
	connected    bool
	mu           sync.Mutex
}

// NewClient creates a new MCP client.
func NewClient(config Config) *Client {
	return &Client{
		config:    config,
		servers:   make(map[string]*ServerConnection),
		toolCache: make(map[string]*DiscoveredTool),
	}
}

// Start initializes the client and optionally connects to servers.
func (c *Client) Start(ctx context.Context) error {
	if c.config.AutoConnect {
		for _, serverConfig := range c.config.Servers {
			if err := c.Connect(ctx, serverConfig); err != nil {
				// Log but continue
				fmt.Printf("Failed to connect to MCP server %s: %v\n", serverConfig.Name, err)
			}
		}
	}
	return nil
}

// Connect establishes a connection to an MCP server.
func (c *Client) Connect(ctx context.Context, config ServerConfig) error {
	c.serversMu.Lock()
	defer c.serversMu.Unlock()

	// Check if already connected
	if _, exists := c.servers[config.Name]; exists {
		return fmt.Errorf("server already connected: %s", config.Name)
	}

	// Create transport
	var transport Transport
	var err error

	switch config.Type {
	case TransportStdio:
		transport, err = NewStdioTransport(config)
	case TransportSSE:
		transport, err = NewSSETransport(config)
	case TransportHTTP:
		transport, err = NewHTTPTransport(config)
	default:
		return fmt.Errorf("unknown transport type: %s", config.Type)
	}

	if err != nil {
		return fmt.Errorf("create transport: %w", err)
	}

	// Start transport
	if err := transport.Start(ctx); err != nil {
		return fmt.Errorf("start transport: %w", err)
	}

	conn := &ServerConnection{
		config:    config,
		transport: transport,
	}

	// Initialize connection
	if err := c.initialize(ctx, conn); err != nil {
		transport.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	conn.connected = true
	c.servers[config.Name] = conn

	// Cache tools from this server
	go c.cacheServerTools(ctx, config.Name)

	return nil
}

// initialize performs the MCP initialization handshake.
func (c *Client) initialize(ctx context.Context, conn *ServerConnection) error {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ClientCapabilities{
			Roots:    &RootsCapability{},
			Sampling: &SamplingCapability{},
		},
		ClientInfo: ClientInfo{
			Name:    "syntor",
			Version: "1.0.0",
		},
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "initialize",
		Params:  paramsJSON,
	}

	resp, err := conn.transport.Send(ctx, msg)
	if err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}

	if resp.Error != nil {
		return resp.Error
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	conn.capabilities = result.Capabilities
	conn.serverInfo = result.ServerInfo

	// Send initialized notification
	initNotif := &JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return conn.transport.SendNotification(ctx, initNotif)
}

// Disconnect closes a server connection.
func (c *Client) Disconnect(serverName string) error {
	c.serversMu.Lock()
	defer c.serversMu.Unlock()

	conn, exists := c.servers[serverName]
	if !exists {
		return fmt.Errorf("server not connected: %s", serverName)
	}

	if err := conn.transport.Close(); err != nil {
		return err
	}

	delete(c.servers, serverName)

	// Clear cached tools from this server
	c.clearServerTools(serverName)

	return nil
}

// Close shuts down all connections.
func (c *Client) Close() error {
	c.serversMu.Lock()
	defer c.serversMu.Unlock()

	var lastErr error
	for name, conn := range c.servers {
		if err := conn.transport.Close(); err != nil {
			lastErr = err
		}
		delete(c.servers, name)
	}

	return lastErr
}

// ListServers returns connected server names.
func (c *Client) ListServers() []string {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()

	var names []string
	for name := range c.servers {
		names = append(names, name)
	}
	return names
}

// GetServerInfo returns information about a connected server.
func (c *Client) GetServerInfo(serverName string) (*ServerInfo, error) {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()

	conn, exists := c.servers[serverName]
	if !exists {
		return nil, fmt.Errorf("server not connected: %s", serverName)
	}

	return &conn.serverInfo, nil
}

// Tool Operations

// ListTools returns all tools from a specific server.
func (c *Client) ListTools(ctx context.Context, serverName string) ([]Tool, error) {
	conn, err := c.getConnection(serverName)
	if err != nil {
		return nil, err
	}

	if conn.capabilities.Tools == nil {
		return nil, fmt.Errorf("server does not support tools")
	}

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/list",
	}

	resp, err := conn.transport.Send(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("send tools/list: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return result.Tools, nil
}

// ListAllTools returns tools from all connected servers.
func (c *Client) ListAllTools(ctx context.Context) ([]DiscoveredTool, error) {
	c.serversMu.RLock()
	serverNames := make([]string, 0, len(c.servers))
	for name := range c.servers {
		serverNames = append(serverNames, name)
	}
	c.serversMu.RUnlock()

	var allTools []DiscoveredTool
	for _, serverName := range serverNames {
		tools, err := c.ListTools(ctx, serverName)
		if err != nil {
			continue
		}

		for _, tool := range tools {
			allTools = append(allTools, DiscoveredTool{
				Tool:       tool,
				ServerName: serverName,
				FullName:   fmt.Sprintf("mcp__%s__%s", serverName, tool.Name),
			})
		}
	}

	return allTools, nil
}

// CallTool invokes a tool on a server.
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, arguments map[string]any) (*CallToolResult, error) {
	conn, err := c.getConnection(serverName)
	if err != nil {
		return nil, err
	}

	params := CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	resp, err := conn.transport.Send(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("send tools/call: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return &result, nil
}

// CallToolByFullName calls a tool using its full name (mcp__server__tool).
func (c *Client) CallToolByFullName(ctx context.Context, fullName string, arguments map[string]any) (*ToolCallResponse, error) {
	start := time.Now()

	serverName, toolName, err := parseFullToolName(fullName)
	if err != nil {
		return nil, err
	}

	result, err := c.CallTool(ctx, serverName, toolName, arguments)
	if err != nil {
		return &ToolCallResponse{
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, nil
	}

	return &ToolCallResponse{
		Success:  !result.IsError,
		Content:  result.Content,
		Duration: time.Since(start),
	}, nil
}

// Resource Operations

// ListResources returns resources from a server.
func (c *Client) ListResources(ctx context.Context, serverName string) ([]Resource, error) {
	conn, err := c.getConnection(serverName)
	if err != nil {
		return nil, err
	}

	if conn.capabilities.Resources == nil {
		return nil, fmt.Errorf("server does not support resources")
	}

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "resources/list",
	}

	resp, err := conn.transport.Send(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("send resources/list: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ListResourcesResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return result.Resources, nil
}

// ReadResource reads a resource from a server.
func (c *Client) ReadResource(ctx context.Context, serverName, uri string) (*ReadResourceResult, error) {
	conn, err := c.getConnection(serverName)
	if err != nil {
		return nil, err
	}

	params := ReadResourceParams{URI: uri}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      c.getNextID(),
		Method:  "resources/read",
		Params:  paramsJSON,
	}

	resp, err := conn.transport.Send(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("send resources/read: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ReadResourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	return &result, nil
}

// Helper methods

func (c *Client) getConnection(serverName string) (*ServerConnection, error) {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()

	conn, exists := c.servers[serverName]
	if !exists {
		return nil, fmt.Errorf("server not connected: %s", serverName)
	}

	if !conn.connected {
		return nil, fmt.Errorf("server not initialized: %s", serverName)
	}

	return conn, nil
}

func (c *Client) getNextID() *int64 {
	id := atomic.AddInt64(&c.nextID, 1)
	return &id
}

func (c *Client) cacheServerTools(ctx context.Context, serverName string) {
	tools, err := c.ListTools(ctx, serverName)
	if err != nil {
		return
	}

	c.toolCacheMu.Lock()
	defer c.toolCacheMu.Unlock()

	for _, tool := range tools {
		fullName := fmt.Sprintf("mcp__%s__%s", serverName, tool.Name)
		c.toolCache[fullName] = &DiscoveredTool{
			Tool:       tool,
			ServerName: serverName,
			FullName:   fullName,
		}
	}
}

func (c *Client) clearServerTools(serverName string) {
	c.toolCacheMu.Lock()
	defer c.toolCacheMu.Unlock()

	prefix := fmt.Sprintf("mcp__%s__", serverName)
	for fullName := range c.toolCache {
		if strings.HasPrefix(fullName, prefix) {
			delete(c.toolCache, fullName)
		}
	}
}

// GetCachedTools returns all cached tools.
func (c *Client) GetCachedTools() []DiscoveredTool {
	c.toolCacheMu.RLock()
	defer c.toolCacheMu.RUnlock()

	tools := make([]DiscoveredTool, 0, len(c.toolCache))
	for _, tool := range c.toolCache {
		tools = append(tools, *tool)
	}
	return tools
}

// FindTool looks up a tool by full name.
func (c *Client) FindTool(fullName string) (*DiscoveredTool, bool) {
	c.toolCacheMu.RLock()
	defer c.toolCacheMu.RUnlock()

	tool, exists := c.toolCache[fullName]
	return tool, exists
}

func parseFullToolName(fullName string) (serverName, toolName string, err error) {
	// Format: mcp__server__tool
	if !strings.HasPrefix(fullName, "mcp__") {
		return "", "", fmt.Errorf("invalid tool name format: %s", fullName)
	}

	parts := strings.SplitN(fullName[5:], "__", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tool name format: %s", fullName)
	}

	return parts[0], parts[1], nil
}
