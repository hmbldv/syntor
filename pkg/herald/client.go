package herald

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Config holds Herald client configuration.
type Config struct {
	// BaseURL is the Herald session manager URL (e.g., http://localhost:8090)
	BaseURL string `yaml:"base_url" json:"base_url"`

	// Timeout for HTTP requests
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// RetryAttempts is the number of retry attempts for failed requests
	RetryAttempts int `yaml:"retry_attempts" json:"retry_attempts"`

	// RetryDelay is the initial delay between retries
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`

	// DefaultTrustTier is the default trust tier for new sessions
	DefaultTrustTier TrustTier `yaml:"default_trust_tier" json:"default_trust_tier"`

	// Enabled controls whether Herald integration is active
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:          "http://localhost:8090",
		Timeout:          30 * time.Second,
		RetryAttempts:    3,
		RetryDelay:       500 * time.Millisecond,
		DefaultTrustTier: T1,
		Enabled:          true,
	}
}

// Client provides access to Herald services.
type Client struct {
	config     Config
	httpClient *http.Client
	baseURL    *url.URL

	// Current session context
	sessionID string
	sessionMu sync.RWMutex

	// Health cache
	healthCache *HealthStatus
	healthMu    sync.RWMutex
	healthTTL   time.Duration
}

// New creates a new Herald client.
func New(config Config) (*Client, error) {
	if !config.Enabled {
		return &Client{config: config}, nil
	}

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		baseURL:   baseURL,
		healthTTL: 30 * time.Second,
	}, nil
}

// IsEnabled returns true if Herald integration is active.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// SetSession sets the current session ID for the client.
func (c *Client) SetSession(sessionID string) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.sessionID = sessionID
}

// GetSession returns the current session ID.
func (c *Client) GetSession() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.sessionID
}

// Health checks the Herald service health.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	if !c.config.Enabled {
		return &HealthStatus{
			Status:      "disabled",
			LastChecked: time.Now(),
		}, nil
	}

	// Check cache
	c.healthMu.RLock()
	if c.healthCache != nil && time.Since(c.healthCache.LastChecked) < c.healthTTL {
		cached := *c.healthCache
		c.healthMu.RUnlock()
		return &cached, nil
	}
	c.healthMu.RUnlock()

	// Fetch fresh health status
	var health HealthStatus
	err := c.doRequest(ctx, "GET", "/health", nil, &health)
	if err != nil {
		return &HealthStatus{
			Status:      "unhealthy",
			LastChecked: time.Now(),
		}, err
	}

	health.LastChecked = time.Now()

	// Update cache
	c.healthMu.Lock()
	c.healthCache = &health
	c.healthMu.Unlock()

	return &health, nil
}

// IsAvailable checks if Herald is reachable and healthy.
func (c *Client) IsAvailable(ctx context.Context) bool {
	if !c.config.Enabled {
		return false
	}

	health, err := c.Health(ctx)
	if err != nil {
		return false
	}

	return health.Status == "healthy" || health.Status == "degraded"
}

// doRequest performs an HTTP request with retry logic.
func (c *Client) doRequest(ctx context.Context, method, path string, body, result any) error {
	if !c.config.Enabled {
		return &Error{Code: ErrCodeServiceUnavailable, Message: "Herald is disabled"}
	}

	reqURL := c.baseURL.JoinPath(path)

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.RetryDelay * time.Duration(attempt)):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		// Add session ID if available
		if sessionID := c.GetSession(); sessionID != "" {
			req.Header.Set("X-Session-ID", sessionID)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode >= 400 {
			var heraldErr Error
			if json.Unmarshal(respBody, &heraldErr) == nil && heraldErr.Code != "" {
				lastErr = &heraldErr
			} else {
				lastErr = &Error{
					Code:    fmt.Sprintf("http_%d", resp.StatusCode),
					Message: string(respBody),
				}
			}

			// Don't retry client errors
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return lastErr
			}
			continue
		}

		if result != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("unmarshal response: %w", err)
			}
		}

		return nil
	}

	return lastErr
}

// doStreamRequest performs a streaming HTTP request.
func (c *Client) doStreamRequest(ctx context.Context, path string, body any) (io.ReadCloser, error) {
	if !c.config.Enabled {
		return nil, &Error{Code: ErrCodeServiceUnavailable, Message: "Herald is disabled"}
	}

	reqURL := c.baseURL.JoinPath(path)

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if sessionID := c.GetSession(); sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var heraldErr Error
		if json.Unmarshal(respBody, &heraldErr) == nil && heraldErr.Code != "" {
			return nil, &heraldErr
		}
		return nil, &Error{
			Code:    fmt.Sprintf("http_%d", resp.StatusCode),
			Message: string(respBody),
		}
	}

	return resp.Body, nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
