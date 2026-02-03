package herald

import (
	"context"
	"fmt"
	"time"
)

// RequestApproval submits an approval request to Herald.
func (c *Client) RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalRequest, error) {
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = req.CreatedAt.Add(5 * time.Minute)
	}
	if req.Status == "" {
		req.Status = ApprovalStatusPending
	}

	var result ApprovalRequest
	if err := c.doRequest(ctx, "POST", "/api/v1/approvals", req, &result); err != nil {
		return nil, fmt.Errorf("request approval: %w", err)
	}
	return &result, nil
}

// GetApproval retrieves an approval request by ID.
func (c *Client) GetApproval(ctx context.Context, approvalID string) (*ApprovalRequest, error) {
	var approval ApprovalRequest
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/approvals/%s", approvalID), nil, &approval); err != nil {
		return nil, fmt.Errorf("get approval: %w", err)
	}
	return &approval, nil
}

// ListPendingApprovals retrieves all pending approval requests for a session.
func (c *Client) ListPendingApprovals(ctx context.Context, sessionID string) ([]ApprovalRequest, error) {
	var approvals []ApprovalRequest
	path := fmt.Sprintf("/api/v1/sessions/%s/approvals?status=pending", sessionID)
	if err := c.doRequest(ctx, "GET", path, nil, &approvals); err != nil {
		return nil, fmt.Errorf("list pending approvals: %w", err)
	}
	return approvals, nil
}

// RespondToApproval approves or denies an approval request.
func (c *Client) RespondToApproval(ctx context.Context, resp ApprovalResponse) error {
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/approvals/%s/respond", resp.RequestID), resp, nil); err != nil {
		return fmt.Errorf("respond to approval: %w", err)
	}
	return nil
}

// ApproveRequest is a convenience method to approve a request.
func (c *Client) ApproveRequest(ctx context.Context, approvalID string, reason string) error {
	return c.RespondToApproval(ctx, ApprovalResponse{
		RequestID: approvalID,
		Approved:  true,
		Reason:    reason,
	})
}

// DenyRequest is a convenience method to deny a request.
func (c *Client) DenyRequest(ctx context.Context, approvalID string, reason string) error {
	return c.RespondToApproval(ctx, ApprovalResponse{
		RequestID: approvalID,
		Approved:  false,
		Reason:    reason,
	})
}

// WaitForApproval polls for approval status until resolved or timeout.
func (c *Client) WaitForApproval(ctx context.Context, approvalID string, pollInterval time.Duration) (*ApprovalRequest, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			approval, err := c.GetApproval(ctx, approvalID)
			if err != nil {
				return nil, err
			}

			switch approval.Status {
			case ApprovalStatusApproved:
				return approval, nil
			case ApprovalStatusDenied:
				return approval, &Error{
					Code:    ErrCodeApprovalDenied,
					Message: approval.Reason,
				}
			case ApprovalStatusExpired:
				return approval, &Error{
					Code:    ErrCodeApprovalDenied,
					Message: "approval request expired",
				}
			}
			// Still pending, continue polling
		}
	}
}

// CancelApproval cancels a pending approval request.
func (c *Client) CancelApproval(ctx context.Context, approvalID string) error {
	if err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v1/approvals/%s", approvalID), nil, nil); err != nil {
		return fmt.Errorf("cancel approval: %w", err)
	}
	return nil
}

// ApprovalHandler provides a local approval workflow when Herald is unavailable.
type ApprovalHandler struct {
	pending    map[string]*ApprovalRequest
	callback   ApprovalCallback
	autoApprove bool
}

// ApprovalCallback is called when an approval decision is needed.
type ApprovalCallback func(req *ApprovalRequest) (approved bool, reason string)

// NewApprovalHandler creates a local approval handler.
func NewApprovalHandler(callback ApprovalCallback) *ApprovalHandler {
	return &ApprovalHandler{
		pending:  make(map[string]*ApprovalRequest),
		callback: callback,
	}
}

// NewAutoApprovalHandler creates a handler that auto-approves all requests.
func NewAutoApprovalHandler() *ApprovalHandler {
	return &ApprovalHandler{
		pending:     make(map[string]*ApprovalRequest),
		autoApprove: true,
	}
}

// Submit submits an approval request.
func (h *ApprovalHandler) Submit(req *ApprovalRequest) {
	req.CreatedAt = time.Now()
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = req.CreatedAt.Add(5 * time.Minute)
	}
	req.Status = ApprovalStatusPending
	h.pending[req.ID] = req
}

// Process processes a pending approval request.
func (h *ApprovalHandler) Process(approvalID string) (*ApprovalRequest, error) {
	req, ok := h.pending[approvalID]
	if !ok {
		return nil, &Error{Code: ErrCodeNotFound, Message: "approval not found"}
	}

	if time.Now().After(req.ExpiresAt) {
		req.Status = ApprovalStatusExpired
		delete(h.pending, approvalID)
		return req, nil
	}

	if h.autoApprove {
		req.Status = ApprovalStatusApproved
		now := time.Now()
		req.RespondedAt = &now
		delete(h.pending, approvalID)
		return req, nil
	}

	if h.callback != nil {
		approved, reason := h.callback(req)
		now := time.Now()
		req.RespondedAt = &now
		req.Reason = reason
		if approved {
			req.Status = ApprovalStatusApproved
		} else {
			req.Status = ApprovalStatusDenied
		}
		delete(h.pending, approvalID)
		return req, nil
	}

	return req, nil
}

// Get retrieves an approval request by ID.
func (h *ApprovalHandler) Get(approvalID string) (*ApprovalRequest, bool) {
	req, ok := h.pending[approvalID]
	return req, ok
}

// ListPending returns all pending approval requests.
func (h *ApprovalHandler) ListPending() []*ApprovalRequest {
	var result []*ApprovalRequest
	for _, req := range h.pending {
		if req.Status == ApprovalStatusPending {
			result = append(result, req)
		}
	}
	return result
}

// Approve approves a pending request.
func (h *ApprovalHandler) Approve(approvalID string, reason string) error {
	req, ok := h.pending[approvalID]
	if !ok {
		return &Error{Code: ErrCodeNotFound, Message: "approval not found"}
	}
	req.Status = ApprovalStatusApproved
	req.Reason = reason
	now := time.Now()
	req.RespondedAt = &now
	delete(h.pending, approvalID)
	return nil
}

// Deny denies a pending request.
func (h *ApprovalHandler) Deny(approvalID string, reason string) error {
	req, ok := h.pending[approvalID]
	if !ok {
		return &Error{Code: ErrCodeNotFound, Message: "approval not found"}
	}
	req.Status = ApprovalStatusDenied
	req.Reason = reason
	now := time.Now()
	req.RespondedAt = &now
	delete(h.pending, approvalID)
	return nil
}

// Clear removes all pending approvals.
func (h *ApprovalHandler) Clear() {
	h.pending = make(map[string]*ApprovalRequest)
}
