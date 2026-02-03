package herald

import (
	"context"
	"fmt"
)

// TrustPolicy represents the trust configuration for a session or agent.
type TrustPolicy struct {
	Tier              TrustTier              `json:"tier"`
	AllowedOperations []OperationType        `json:"allowed_operations"`
	DeniedOperations  []OperationType        `json:"denied_operations,omitempty"`
	AllowedPaths      []string               `json:"allowed_paths,omitempty"`
	DeniedPaths       []string               `json:"denied_paths,omitempty"`
	AllowedCommands   []string               `json:"allowed_commands,omitempty"`
	DeniedCommands    []string               `json:"denied_commands,omitempty"`
	RateLimit         *RateLimitConfig       `json:"rate_limit,omitempty"`
	RequiresApproval  map[OperationType]bool `json:"requires_approval,omitempty"`
}

// RateLimitConfig specifies rate limiting parameters.
type RateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	TokensPerMinute   int `json:"tokens_per_minute"`
	BurstLimit        int `json:"burst_limit"`
}

// GetTrustPolicy retrieves the trust policy for a session.
func (c *Client) GetTrustPolicy(ctx context.Context, sessionID string) (*TrustPolicy, error) {
	var policy TrustPolicy
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/sessions/%s/trust", sessionID), nil, &policy); err != nil {
		return nil, fmt.Errorf("get trust policy: %w", err)
	}
	return &policy, nil
}

// UpdateTrustTier updates the trust tier for a session.
func (c *Client) UpdateTrustTier(ctx context.Context, sessionID string, tier TrustTier) error {
	body := struct {
		Tier TrustTier `json:"tier"`
	}{Tier: tier}

	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/sessions/%s/trust/tier", sessionID), body, nil); err != nil {
		return fmt.Errorf("update trust tier: %w", err)
	}
	return nil
}

// CheckPermission checks if an operation is permitted under the current trust policy.
func (c *Client) CheckPermission(ctx context.Context, sessionID string, op OperationType, target string) (*PermissionCheck, error) {
	body := struct {
		Operation OperationType `json:"operation"`
		Target    string        `json:"target"`
	}{
		Operation: op,
		Target:    target,
	}

	var result PermissionCheck
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/sessions/%s/trust/check", sessionID), body, &result); err != nil {
		return nil, fmt.Errorf("check permission: %w", err)
	}
	return &result, nil
}

// PermissionCheck is the result of a permission check.
type PermissionCheck struct {
	Allowed          bool          `json:"allowed"`
	RequiresApproval bool          `json:"requires_approval"`
	Reason           string        `json:"reason,omitempty"`
	ApprovalID       string        `json:"approval_id,omitempty"` // If approval was auto-created
}

// ElevateTrust requests a temporary trust elevation.
func (c *Client) ElevateTrust(ctx context.Context, sessionID string, targetTier TrustTier, reason string, duration int) (*TrustElevation, error) {
	body := struct {
		TargetTier TrustTier `json:"target_tier"`
		Reason     string    `json:"reason"`
		Duration   int       `json:"duration_seconds"`
	}{
		TargetTier: targetTier,
		Reason:     reason,
		Duration:   duration,
	}

	var elevation TrustElevation
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/sessions/%s/trust/elevate", sessionID), body, &elevation); err != nil {
		return nil, fmt.Errorf("elevate trust: %w", err)
	}
	return &elevation, nil
}

// TrustElevation represents a temporary trust elevation.
type TrustElevation struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	PreviousTier TrustTier `json:"previous_tier"`
	CurrentTier  TrustTier `json:"current_tier"`
	Reason       string    `json:"reason"`
	ExpiresAt    string    `json:"expires_at"`
	Status       string    `json:"status"` // pending, active, expired, revoked
}

// RevokeTrustElevation revokes a trust elevation.
func (c *Client) RevokeTrustElevation(ctx context.Context, elevationID string) error {
	if err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v1/trust/elevations/%s", elevationID), nil, nil); err != nil {
		return fmt.Errorf("revoke trust elevation: %w", err)
	}
	return nil
}

// TrustChecker provides a convenient interface for checking permissions locally
// when Herald is unavailable.
type TrustChecker struct {
	policy *TrustPolicy
}

// NewTrustChecker creates a local trust checker with the given policy.
func NewTrustChecker(policy *TrustPolicy) *TrustChecker {
	return &TrustChecker{policy: policy}
}

// DefaultTrustChecker creates a trust checker with default policy based on tier.
func DefaultTrustChecker(tier TrustTier) *TrustChecker {
	policy := &TrustPolicy{
		Tier:              tier,
		AllowedOperations: []OperationType{},
		RequiresApproval:  make(map[OperationType]bool),
	}

	switch tier {
	case T4:
		policy.AllowedOperations = []OperationType{OpRead, OpWrite, OpExecute, OpNetwork}
	case T3:
		policy.AllowedOperations = []OperationType{OpRead, OpWrite, OpExecute}
		policy.RequiresApproval[OpNetwork] = true
	case T2:
		policy.AllowedOperations = []OperationType{OpRead, OpWrite}
		policy.RequiresApproval[OpExecute] = true
		policy.RequiresApproval[OpNetwork] = true
	case T1:
		policy.AllowedOperations = []OperationType{OpRead}
		policy.RequiresApproval[OpWrite] = true
		policy.RequiresApproval[OpExecute] = true
		policy.RequiresApproval[OpNetwork] = true
	case T0:
		policy.AllowedOperations = []OperationType{}
		policy.RequiresApproval[OpRead] = true
		policy.RequiresApproval[OpWrite] = true
		policy.RequiresApproval[OpExecute] = true
		policy.RequiresApproval[OpNetwork] = true
	}

	return &TrustChecker{policy: policy}
}

// Check checks if an operation is permitted.
func (tc *TrustChecker) Check(op OperationType, target string) PermissionCheck {
	// Check denied operations first
	for _, denied := range tc.policy.DeniedOperations {
		if denied == op {
			return PermissionCheck{
				Allowed: false,
				Reason:  fmt.Sprintf("operation %s is explicitly denied", op),
			}
		}
	}

	// Check path restrictions for write/execute operations
	if op == OpWrite || op == OpExecute {
		if len(tc.policy.DeniedPaths) > 0 {
			for _, denied := range tc.policy.DeniedPaths {
				if matchPath(target, denied) {
					return PermissionCheck{
						Allowed: false,
						Reason:  fmt.Sprintf("path %s is denied", target),
					}
				}
			}
		}

		if len(tc.policy.AllowedPaths) > 0 {
			allowed := false
			for _, allowedPath := range tc.policy.AllowedPaths {
				if matchPath(target, allowedPath) {
					allowed = true
					break
				}
			}
			if !allowed {
				return PermissionCheck{
					Allowed: false,
					Reason:  fmt.Sprintf("path %s is not in allowed paths", target),
				}
			}
		}
	}

	// Check if operation is allowed
	for _, allowed := range tc.policy.AllowedOperations {
		if allowed == op {
			return PermissionCheck{
				Allowed:          true,
				RequiresApproval: tc.policy.RequiresApproval[op],
			}
		}
	}

	// Check if requires approval
	if requiresApproval, ok := tc.policy.RequiresApproval[op]; ok && requiresApproval {
		return PermissionCheck{
			Allowed:          true,
			RequiresApproval: true,
		}
	}

	// Default deny
	return PermissionCheck{
		Allowed: false,
		Reason:  fmt.Sprintf("operation %s not permitted at trust tier %s", op, tc.policy.Tier),
	}
}

// matchPath checks if a target path matches a pattern.
func matchPath(target, pattern string) bool {
	// Simple prefix matching for now
	// TODO: Implement glob pattern matching
	if len(pattern) == 0 {
		return false
	}
	if pattern[len(pattern)-1] == '*' {
		return len(target) >= len(pattern)-1 && target[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return target == pattern || (len(target) > len(pattern) && target[:len(pattern)+1] == pattern+"/")
}
