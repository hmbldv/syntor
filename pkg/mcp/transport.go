package mcp

import (
	"syntor/pkg/mcp/transport"
)

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(config ServerConfig) (Transport, error) {
	return transport.NewStdioTransport(transport.StdioConfig{
		Command: config.Command,
		Args:    config.Args,
		Env:     config.Env,
		Timeout: config.Timeout,
	}), nil
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport(config ServerConfig) (Transport, error) {
	headers := make(map[string]string)
	// Add any authentication headers from config

	return transport.NewSSETransport(transport.SSEConfig{
		URL:     config.URL,
		Headers: headers,
		Timeout: config.Timeout,
	}), nil
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(config ServerConfig) (Transport, error) {
	headers := make(map[string]string)
	// Add any authentication headers from config

	return transport.NewHTTPTransport(transport.HTTPConfig{
		URL:     config.URL,
		Headers: headers,
		Timeout: config.Timeout,
	}), nil
}
