package falkordb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds FalkorDB client configuration.
type Config struct {
	// Address is the Redis/FalkorDB server address (e.g., localhost:6379)
	Address string `yaml:"address" json:"address"`

	// Password for authentication (if required)
	Password string `yaml:"password" json:"password"`

	// Database number
	Database int `yaml:"database" json:"database"`

	// GraphName is the name of the graph to query
	GraphName string `yaml:"graph_name" json:"graph_name"`

	// Timeout for queries
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// CacheTTL is how long to cache routing results
	CacheTTL time.Duration `yaml:"cache_ttl" json:"cache_ttl"`

	// Enabled controls whether FalkorDB integration is active
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Address:   "localhost:6379",
		Database:  0,
		GraphName: "agents",
		Timeout:   10 * time.Second,
		CacheTTL:  5 * time.Minute,
		Enabled:   true,
	}
}

// Client provides access to FalkorDB graph operations.
type Client struct {
	config Config
	redis  *redis.Client

	// Route cache
	cache    map[string]*CacheEntry
	cacheMu  sync.RWMutex

	// Connection state
	connected bool
	connMu    sync.RWMutex
}

// New creates a new FalkorDB client.
func New(config Config) (*Client, error) {
	if !config.Enabled {
		return &Client{
			config: config,
			cache:  make(map[string]*CacheEntry),
		}, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:         config.Address,
		Password:     config.Password,
		DB:           config.Database,
		DialTimeout:  config.Timeout,
		ReadTimeout:  config.Timeout,
		WriteTimeout: config.Timeout,
	})

	return &Client{
		config: config,
		redis:  client,
		cache:  make(map[string]*CacheEntry),
	}, nil
}

// IsEnabled returns true if FalkorDB integration is active.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled
}

// Connect establishes connection to FalkorDB.
func (c *Client) Connect(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	// Test connection
	if err := c.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to FalkorDB: %w", err)
	}

	c.connected = true
	return nil
}

// IsConnected returns true if connected to FalkorDB.
func (c *Client) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connected
}

// Close closes the FalkorDB connection.
func (c *Client) Close() error {
	if c.redis == nil {
		return nil
	}

	c.connMu.Lock()
	c.connected = false
	c.connMu.Unlock()

	return c.redis.Close()
}

// Query executes a Cypher query against the graph.
func (c *Client) Query(ctx context.Context, query string, params map[string]any) (*QueryResult, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	// Build the GRAPH.QUERY command
	// FalkorDB uses the GRAPH.QUERY command with optional parameters
	args := []any{"GRAPH.QUERY", c.config.GraphName, query}

	// Add parameters if provided
	if len(params) > 0 {
		// FalkorDB expects parameters in CYPHER format
		paramStr := buildParamString(params)
		args[2] = paramStr + query
	}

	result, err := c.redis.Do(ctx, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return parseQueryResult(result)
}

// buildParamString converts params map to Cypher parameter format.
func buildParamString(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}

	var parts []string
	for k, v := range params {
		switch val := v.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("CYPHER %s='%s'", k, val))
		case int, int64, float64:
			parts = append(parts, fmt.Sprintf("CYPHER %s=%v", k, val))
		default:
			parts = append(parts, fmt.Sprintf("CYPHER %s='%v'", k, val))
		}
	}

	return strings.Join(parts, " ") + " "
}

// parseQueryResult converts the raw Redis response to QueryResult.
func parseQueryResult(result any) (*QueryResult, error) {
	// FalkorDB returns results as a nested array:
	// [0]: Column names
	// [1]: Result rows
	// [2]: Statistics

	arr, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	qr := &QueryResult{
		Stats: make(map[string]any),
	}

	if len(arr) > 0 {
		// Parse column names
		if cols, ok := arr[0].([]any); ok {
			for _, col := range cols {
				if colStr, ok := col.(string); ok {
					qr.Columns = append(qr.Columns, colStr)
				}
			}
		}
	}

	if len(arr) > 1 {
		// Parse rows
		if rows, ok := arr[1].([]any); ok {
			for _, row := range rows {
				if rowArr, ok := row.([]any); ok {
					qr.Rows = append(qr.Rows, rowArr)
				}
			}
		}
	}

	if len(arr) > 2 {
		// Parse statistics
		if stats, ok := arr[2].([]any); ok {
			for _, stat := range stats {
				if statStr, ok := stat.(string); ok {
					parts := strings.SplitN(statStr, ":", 2)
					if len(parts) == 2 {
						qr.Stats[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}

	return qr, nil
}

// GetStats returns statistics about the agent graph.
func (c *Client) GetStats(ctx context.Context) (*GraphStats, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return &GraphStats{
			AgentCount: len(FallbackRoutes),
			TeamCount:  len(FallbackTeams),
		}, nil
	}

	// Query for counts
	query := `
		MATCH (a:Agent) WITH count(a) as agents
		MATCH (t:Team) WITH agents, count(t) as teams
		MATCH ()-[r]->() WITH agents, teams, count(r) as rels
		RETURN agents, teams, rels
	`

	result, err := c.Query(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	stats := &GraphStats{
		LastUpdated: time.Now(),
	}

	if len(result.Rows) > 0 && len(result.Rows[0]) >= 3 {
		if agents, ok := result.Rows[0][0].(int64); ok {
			stats.AgentCount = int(agents)
		}
		if teams, ok := result.Rows[0][1].(int64); ok {
			stats.TeamCount = int(teams)
		}
		if rels, ok := result.Rows[0][2].(int64); ok {
			stats.RelationshipCount = int(rels)
		}
	}

	return stats, nil
}

// getCacheKey generates a cache key for a route query.
func getCacheKey(q RouteQuery) string {
	parts := []string{q.TaskType}
	if q.FromAgent != "" {
		parts = append(parts, "from:"+q.FromAgent)
	}
	if q.TeamFilter != "" {
		parts = append(parts, "team:"+q.TeamFilter)
	}
	return strings.Join(parts, "|")
}

// getFromCache retrieves a cached route result.
func (c *Client) getFromCache(key string) (*RouteResult, bool) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	entry.HitCount++
	return entry.Route, true
}

// setCache stores a route result in the cache.
func (c *Client) setCache(key string, result *RouteResult) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.cache[key] = &CacheEntry{
		Route:     result,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(c.config.CacheTTL),
		HitCount:  0,
	}
}

// ClearCache clears the routing cache.
func (c *Client) ClearCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache = make(map[string]*CacheEntry)
}

// CacheStats returns cache statistics.
func (c *Client) CacheStats() (size int, totalHits int) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	size = len(c.cache)
	for _, entry := range c.cache {
		totalHits += entry.HitCount
	}
	return
}
