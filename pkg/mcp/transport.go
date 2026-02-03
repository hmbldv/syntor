package mcp

import (
	"context"

	"github.com/syntor/syntor/pkg/mcp/transport"
)

// transportAdapter wraps a transport package implementation to satisfy mcp.Transport interface.
// This is needed because transport package has its own JSONRPCMessage type to avoid import cycles.
type transportAdapter struct {
	inner interface {
		Start(ctx context.Context) error
		Send(ctx context.Context, msg *transport.JSONRPCMessage) (*transport.JSONRPCMessage, error)
		SendNotification(ctx context.Context, msg *transport.JSONRPCMessage) error
		OnNotification(handler func(*transport.JSONRPCMessage))
		Close() error
		IsConnected() bool
	}
}

func (a *transportAdapter) Start(ctx context.Context) error {
	return a.inner.Start(ctx)
}

func (a *transportAdapter) Send(ctx context.Context, msg *JSONRPCMessage) (*JSONRPCMessage, error) {
	// Convert to transport type
	tMsg := toTransportMessage(msg)
	resp, err := a.inner.Send(ctx, tMsg)
	if err != nil {
		return nil, err
	}
	return fromTransportMessage(resp), nil
}

func (a *transportAdapter) SendNotification(ctx context.Context, msg *JSONRPCMessage) error {
	return a.inner.SendNotification(ctx, toTransportMessage(msg))
}

func (a *transportAdapter) OnNotification(handler func(*JSONRPCMessage)) {
	a.inner.OnNotification(func(msg *transport.JSONRPCMessage) {
		handler(fromTransportMessage(msg))
	})
}

func (a *transportAdapter) Close() error {
	return a.inner.Close()
}

func (a *transportAdapter) IsConnected() bool {
	return a.inner.IsConnected()
}

// toTransportMessage converts mcp.JSONRPCMessage to transport.JSONRPCMessage
func toTransportMessage(msg *JSONRPCMessage) *transport.JSONRPCMessage {
	if msg == nil {
		return nil
	}
	t := &transport.JSONRPCMessage{
		JSONRPC: msg.JSONRPC,
		ID:      msg.ID,
		Method:  msg.Method,
		Params:  msg.Params,
		Result:  msg.Result,
	}
	if msg.Error != nil {
		t.Error = &transport.JSONRPCError{
			Code:    msg.Error.Code,
			Message: msg.Error.Message,
			Data:    msg.Error.Data,
		}
	}
	return t
}

// fromTransportMessage converts transport.JSONRPCMessage to mcp.JSONRPCMessage
func fromTransportMessage(msg *transport.JSONRPCMessage) *JSONRPCMessage {
	if msg == nil {
		return nil
	}
	m := &JSONRPCMessage{
		JSONRPC: msg.JSONRPC,
		ID:      msg.ID,
		Method:  msg.Method,
		Params:  msg.Params,
		Result:  msg.Result,
	}
	if msg.Error != nil {
		m.Error = &JSONRPCError{
			Code:    msg.Error.Code,
			Message: msg.Error.Message,
			Data:    msg.Error.Data,
		}
	}
	return m
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(config ServerConfig) (Transport, error) {
	inner := transport.NewStdioTransport(transport.StdioConfig{
		Command: config.Command,
		Args:    config.Args,
		Env:     config.Env,
		Timeout: config.Timeout,
	})
	return &transportAdapter{inner: inner}, nil
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport(config ServerConfig) (Transport, error) {
	headers := make(map[string]string)
	// Add any authentication headers from config

	inner := transport.NewSSETransport(transport.SSEConfig{
		URL:     config.URL,
		Headers: headers,
		Timeout: config.Timeout,
	})
	return &transportAdapter{inner: inner}, nil
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(config ServerConfig) (Transport, error) {
	headers := make(map[string]string)
	// Add any authentication headers from config

	inner := transport.NewHTTPTransport(transport.HTTPConfig{
		URL:     config.URL,
		Headers: headers,
		Timeout: config.Timeout,
	})
	return &transportAdapter{inner: inner}, nil
}
