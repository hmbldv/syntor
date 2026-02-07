package herald

import (
	"context"
	"fmt"
)

// Machine represents a system in the fleet.
type Machine struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Type     string `json:"type"` // mac, linux, kali
	SSHAlias string `json:"ssh_alias"`
	Status   string `json:"status"` // online, offline
}

// DispatchRequest is the request body for dispatching a task to a machine.
type DispatchRequest struct {
	Machine   string `json:"machine"`
	Task      string `json:"task"`
	Agent     string `json:"agent,omitempty"`
	TrustTier int    `json:"trust_tier"`
}

// DispatchResult is the response from a dispatch operation.
type DispatchResult struct {
	JobID   string `json:"job_id"`
	Machine string `json:"machine"`
	Status  string `json:"status"`
}

// Dispatch sends a task to a specific machine via Herald.
func (c *Client) Dispatch(ctx context.Context, machine, task string) (*DispatchResult, error) {
	if !c.config.Enabled {
		return nil, &Error{Code: ErrCodeServiceUnavailable, Message: "Herald is disabled"}
	}

	req := DispatchRequest{
		Machine:   machine,
		Task:      task,
		TrustTier: int(c.config.DefaultTrustTier),
	}

	var result DispatchResult
	if err := c.doRequest(ctx, "POST", "/api/v1/dispatch", req, &result); err != nil {
		return nil, fmt.Errorf("dispatch to %s: %w", machine, err)
	}
	return &result, nil
}

// DispatchWithAgent sends a task to a specific machine targeting a specific agent.
func (c *Client) DispatchWithAgent(ctx context.Context, machine, task, agent string) (*DispatchResult, error) {
	if !c.config.Enabled {
		return nil, &Error{Code: ErrCodeServiceUnavailable, Message: "Herald is disabled"}
	}

	req := DispatchRequest{
		Machine:   machine,
		Task:      task,
		Agent:     agent,
		TrustTier: int(c.config.DefaultTrustTier),
	}

	var result DispatchResult
	if err := c.doRequest(ctx, "POST", "/api/v1/dispatch", req, &result); err != nil {
		return nil, fmt.Errorf("dispatch to %s (agent %s): %w", machine, agent, err)
	}
	return &result, nil
}

// ListMachines returns all known machines from Herald.
func (c *Client) ListMachines(ctx context.Context) ([]Machine, error) {
	if !c.config.Enabled {
		return nil, &Error{Code: ErrCodeServiceUnavailable, Message: "Herald is disabled"}
	}

	var resp struct {
		Machines []Machine `json:"machines"`
		Total    int       `json:"total"`
	}
	if err := c.doRequest(ctx, "GET", "/api/v1/machines", nil, &resp); err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	return resp.Machines, nil
}
