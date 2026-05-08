package agentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Client provides access to agent definitions stored in PostgreSQL
type Client struct {
	db     *sql.DB
	schema string
	cache  *cache
}

// Config for database connection
type Config struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Database string `yaml:"database" json:"database"`
	Schema   string `yaml:"schema" json:"schema"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
	SSLMode  string `yaml:"ssl_mode" json:"ssl_mode"`
	CacheTTL time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5433,
		Database: "hive",
		Schema:   "agents",
		SSLMode:  "disable",
		CacheTTL: 5 * time.Minute,
	}
}

// simple in-memory cache
type cache struct {
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	value     *RichAgentDefinition
	expiresAt time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *cache) get(key string) (*RichAgentDefinition, bool) {
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.value, true
}

func (c *cache) set(key string, value *RichAgentDefinition) {
	c.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *cache) invalidate(key string) {
	delete(c.entries, key)
}

func (c *cache) clear() {
	c.entries = make(map[string]*cacheEntry)
}

// NewClient creates a new agentdb client
func NewClient(cfg Config) (*Client, error) {
	// Build connection string, handling empty password
	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
	)
	if cfg.User != "" {
		connStr += fmt.Sprintf(" user=%s", cfg.User)
	}
	if cfg.Password != "" {
		connStr += fmt.Sprintf(" password=%s", cfg.Password)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{
		db:     db,
		schema: cfg.Schema,
		cache:  newCache(cfg.CacheTTL),
	}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// GetDefinition retrieves an agent definition by agent_id
func (c *Client) GetDefinition(ctx context.Context, agentID string) (*RichAgentDefinition, error) {
	// Check cache first
	if cached, ok := c.cache.get(agentID); ok {
		return cached, nil
	}

	query := fmt.Sprintf(`
		SELECT id, agent_id, version, is_current, name, role, team,
		       system_prompt, personality, expertise, interaction_protocols,
		       decision_framework, behavioral_rules, capabilities, task_types,
		       model_config, created_at, updated_at
		FROM %s.definitions
		WHERE agent_id = $1 AND is_current = true
	`, c.schema)

	row := c.db.QueryRowContext(ctx, query, agentID)
	def, err := c.scanDefinition(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("agent not found: %s", agentID)
		}
		return nil, fmt.Errorf("failed to get definition: %w", err)
	}

	// Cache the result
	c.cache.set(agentID, def)
	return def, nil
}

// GetDefinitionVersion retrieves a specific version of an agent definition
func (c *Client) GetDefinitionVersion(ctx context.Context, agentID string, version int) (*RichAgentDefinition, error) {
	query := fmt.Sprintf(`
		SELECT id, agent_id, version, is_current, name, role, team,
		       system_prompt, personality, expertise, interaction_protocols,
		       decision_framework, behavioral_rules, capabilities, task_types,
		       model_config, created_at, updated_at
		FROM %s.definitions
		WHERE agent_id = $1 AND version = $2
	`, c.schema)

	row := c.db.QueryRowContext(ctx, query, agentID, version)
	return c.scanDefinition(row)
}

// ListDefinitions returns all current agent definitions
func (c *Client) ListDefinitions(ctx context.Context, opts QueryOptions) ([]*RichAgentDefinition, error) {
	query := fmt.Sprintf(`
		SELECT id, agent_id, version, is_current, name, role, team,
		       system_prompt, personality, expertise, interaction_protocols,
		       decision_framework, behavioral_rules, capabilities, task_types,
		       model_config, created_at, updated_at
		FROM %s.definitions
		WHERE is_current = true
	`, c.schema)

	var args []interface{}
	argNum := 1

	if opts.Team != "" {
		query += fmt.Sprintf(" AND team = $%d", argNum)
		args = append(args, opts.Team)
		argNum++
	}

	if opts.Capability != "" {
		query += fmt.Sprintf(" AND $%d = ANY(capabilities)", argNum)
		args = append(args, opts.Capability)
		argNum++
	}

	query += " ORDER BY name"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list definitions: %w", err)
	}
	defer rows.Close()

	var definitions []*RichAgentDefinition
	for rows.Next() {
		def, err := c.scanDefinitionFromRows(rows)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, def)
	}

	return definitions, rows.Err()
}

// ListSummaries returns lightweight agent summaries for routing
func (c *Client) ListSummaries(ctx context.Context, opts QueryOptions) ([]*AgentSummary, error) {
	query := fmt.Sprintf(`
		SELECT
			d.name,
			d.name,
			COALESCE(d.role, ''),
			COALESCE(t.name, ''),
			COALESCE(d.model_config->>'default_model', ''),
			COALESCE(d.task_types, '{}'),
			COALESCE(d.version, 1)
		FROM %s.definitions d
		LEFT JOIN %s.teams t ON d.team_id = t.id
		WHERE d.is_current = true AND d.status = 'active'
	`, c.schema, c.schema)

	var args []interface{}
	argNum := 1

	if opts.Team != "" {
		query += fmt.Sprintf(" AND t.name = $%d", argNum)
		args = append(args, opts.Team)
		argNum++
	}

	query += " ORDER BY d.name"

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*AgentSummary
	for rows.Next() {
		var s AgentSummary
		var taskTypes pq.StringArray

		err := rows.Scan(&s.AgentID, &s.Name, &s.Role, &s.Team, &s.Model, &taskTypes, &s.Version)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		s.TaskTypes = []string(taskTypes)

		summaries = append(summaries, &s)
	}

	return summaries, rows.Err()
}

// SaveDefinition saves an agent definition, creating a new version if needed
func (c *Client) SaveDefinition(ctx context.Context, def *RichAgentDefinition) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Mark current version as not current
	updateQuery := fmt.Sprintf(`
		UPDATE %s.definitions
		SET is_current = false, updated_at = NOW()
		WHERE agent_id = $1 AND is_current = true
	`, c.schema)

	if _, err := tx.ExecContext(ctx, updateQuery, def.AgentID); err != nil {
		return fmt.Errorf("failed to update current version: %w", err)
	}

	// Get next version number
	var maxVersion sql.NullInt32
	versionQuery := fmt.Sprintf(`
		SELECT MAX(version) FROM %s.definitions WHERE agent_id = $1
	`, c.schema)
	if err := tx.QueryRowContext(ctx, versionQuery, def.AgentID).Scan(&maxVersion); err != nil {
		return fmt.Errorf("failed to get max version: %w", err)
	}

	if maxVersion.Valid {
		def.Version = int(maxVersion.Int32) + 1
	} else {
		def.Version = 1
	}

	// Generate ID if not set
	if def.ID == "" {
		def.ID = uuid.New().String()
	}

	def.Current = true
	def.UpdatedAt = time.Now()

	// Marshal JSON fields
	personalityJSON, _ := json.Marshal(def.Personality)
	expertiseJSON, _ := json.Marshal(def.Expertise)
	interactionsJSON, _ := json.Marshal(def.InteractionProtocols)
	decisionJSON, _ := json.Marshal(def.DecisionFramework)
	behaviorJSON, _ := json.Marshal(def.BehavioralRules)
	capsJSON, _ := json.Marshal(def.Capabilities)
	taskTypesJSON, _ := json.Marshal(def.TaskTypes)
	modelJSON, _ := json.Marshal(def.ModelConfig)

	// Insert new version
	insertQuery := fmt.Sprintf(`
		INSERT INTO %s.definitions (
			id, agent_id, version, is_current, name, role, team,
			system_prompt, personality, expertise, interaction_protocols,
			decision_framework, behavioral_rules, capabilities, task_types,
			model_config, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`, c.schema)

	_, err = tx.ExecContext(ctx, insertQuery,
		def.ID, def.AgentID, def.Version, def.Current, def.Name, def.Role, def.Team,
		def.SystemPrompt, personalityJSON, expertiseJSON, interactionsJSON,
		decisionJSON, behaviorJSON, capsJSON, taskTypesJSON,
		modelJSON, def.CreatedAt, def.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert definition: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	c.cache.invalidate(def.AgentID)

	return nil
}

// DeleteDefinition deletes all versions of an agent definition
func (c *Client) DeleteDefinition(ctx context.Context, agentID string) error {
	query := fmt.Sprintf(`DELETE FROM %s.definitions WHERE agent_id = $1`, c.schema)
	_, err := c.db.ExecContext(ctx, query, agentID)
	if err != nil {
		return fmt.Errorf("failed to delete definition: %w", err)
	}

	c.cache.invalidate(agentID)
	return nil
}

// GetVersionHistory returns the version history for an agent
func (c *Client) GetVersionHistory(ctx context.Context, agentID string) ([]*DefinitionHistory, error) {
	query := fmt.Sprintf(`
		SELECT id, definition_id, version, changed_fields, changed_at, changed_by
		FROM %s.definition_history
		WHERE definition_id IN (
			SELECT id FROM %s.definitions WHERE agent_id = $1
		)
		ORDER BY changed_at DESC
	`, c.schema, c.schema)

	rows, err := c.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get version history: %w", err)
	}
	defer rows.Close()

	var history []*DefinitionHistory
	for rows.Next() {
		var h DefinitionHistory
		var changedFieldsJSON []byte

		err := rows.Scan(&h.ID, &h.DefinitionID, &h.Version, &changedFieldsJSON, &h.ChangedAt, &h.ChangedBy)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(changedFieldsJSON, &h.ChangedFields)
		history = append(history, &h)
	}

	return history, rows.Err()
}

// InvalidateCache clears the cache for an agent or all agents
func (c *Client) InvalidateCache(agentID string) {
	if agentID == "" {
		c.cache.clear()
	} else {
		c.cache.invalidate(agentID)
	}
}

// scanDefinition scans a single row into RichAgentDefinition
func (c *Client) scanDefinition(row *sql.Row) (*RichAgentDefinition, error) {
	var def RichAgentDefinition
	var personalityJSON, expertiseJSON, interactionsJSON, decisionJSON, behaviorJSON, capsJSON, taskTypesJSON, modelJSON []byte

	err := row.Scan(
		&def.ID, &def.AgentID, &def.Version, &def.Current, &def.Name, &def.Role, &def.Team,
		&def.SystemPrompt, &personalityJSON, &expertiseJSON, &interactionsJSON,
		&decisionJSON, &behaviorJSON, &capsJSON, &taskTypesJSON,
		&modelJSON, &def.CreatedAt, &def.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON fields
	if len(personalityJSON) > 0 {
		json.Unmarshal(personalityJSON, &def.Personality)
	}
	if len(expertiseJSON) > 0 {
		json.Unmarshal(expertiseJSON, &def.Expertise)
	}
	if len(interactionsJSON) > 0 {
		json.Unmarshal(interactionsJSON, &def.InteractionProtocols)
	}
	if len(decisionJSON) > 0 {
		json.Unmarshal(decisionJSON, &def.DecisionFramework)
	}
	if len(behaviorJSON) > 0 {
		json.Unmarshal(behaviorJSON, &def.BehavioralRules)
	}
	if len(capsJSON) > 0 {
		json.Unmarshal(capsJSON, &def.Capabilities)
	}
	if len(taskTypesJSON) > 0 {
		json.Unmarshal(taskTypesJSON, &def.TaskTypes)
	}
	if len(modelJSON) > 0 {
		json.Unmarshal(modelJSON, &def.ModelConfig)
	}

	return &def, nil
}

// scanDefinitionFromRows scans from sql.Rows
func (c *Client) scanDefinitionFromRows(rows *sql.Rows) (*RichAgentDefinition, error) {
	var def RichAgentDefinition
	var personalityJSON, expertiseJSON, interactionsJSON, decisionJSON, behaviorJSON, capsJSON, taskTypesJSON, modelJSON []byte

	err := rows.Scan(
		&def.ID, &def.AgentID, &def.Version, &def.Current, &def.Name, &def.Role, &def.Team,
		&def.SystemPrompt, &personalityJSON, &expertiseJSON, &interactionsJSON,
		&decisionJSON, &behaviorJSON, &capsJSON, &taskTypesJSON,
		&modelJSON, &def.CreatedAt, &def.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON fields
	if len(personalityJSON) > 0 {
		json.Unmarshal(personalityJSON, &def.Personality)
	}
	if len(expertiseJSON) > 0 {
		json.Unmarshal(expertiseJSON, &def.Expertise)
	}
	if len(interactionsJSON) > 0 {
		json.Unmarshal(interactionsJSON, &def.InteractionProtocols)
	}
	if len(decisionJSON) > 0 {
		json.Unmarshal(decisionJSON, &def.DecisionFramework)
	}
	if len(behaviorJSON) > 0 {
		json.Unmarshal(behaviorJSON, &def.BehavioralRules)
	}
	if len(capsJSON) > 0 {
		json.Unmarshal(capsJSON, &def.Capabilities)
	}
	if len(taskTypesJSON) > 0 {
		json.Unmarshal(taskTypesJSON, &def.TaskTypes)
	}
	if len(modelJSON) > 0 {
		json.Unmarshal(modelJSON, &def.ModelConfig)
	}

	return &def, nil
}
