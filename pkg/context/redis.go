package context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements Store using Redis
type RedisStore struct {
	client    *redis.Client
	keyPrefix string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string // Key prefix for namespacing
}

// DefaultRedisConfig returns sensible defaults
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Host:   "localhost",
		Port:   6379,
		DB:     0,
		Prefix: "syntor:",
	}
}

// NewRedisStore creates a new Redis-backed context store
func NewRedisStore(cfg RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "syntor:"
	}

	return &RedisStore{
		client:    client,
		keyPrefix: prefix,
	}, nil
}

// prefixKey adds the namespace prefix to a key
func (s *RedisStore) prefixKey(key string) string {
	return s.keyPrefix + key
}

// Get retrieves a context value by key
func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, s.prefixKey(key)).Result()
	if err == redis.Nil {
		return "", nil // Key doesn't exist
	}
	if err != nil {
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	return val, nil
}

// Set stores a context value with optional TTL
func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	err := s.client.Set(ctx, s.prefixKey(key), value, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	return nil
}

// Delete removes a context value
func (s *RedisStore) Delete(ctx context.Context, key string) error {
	err := s.client.Del(ctx, s.prefixKey(key)).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	return nil
}

// Query searches for context values matching a prefix
func (s *RedisStore) Query(ctx context.Context, prefix string, limit int) ([]Item, error) {
	pattern := s.prefixKey(prefix) + "*"
	if limit <= 0 {
		limit = 100
	}

	// Use SCAN to find matching keys
	var cursor uint64
	var keys []string

	for {
		var batch []string
		var err error
		batch, cursor, err = s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan keys: %w", err)
		}
		keys = append(keys, batch...)
		if cursor == 0 || len(keys) >= limit {
			break
		}
	}

	// Trim to limit
	if len(keys) > limit {
		keys = keys[:limit]
	}

	if len(keys) == 0 {
		return []Item{}, nil
	}

	// Get values for all keys
	items := make([]Item, 0, len(keys))
	for _, key := range keys {
		val, err := s.client.Get(ctx, key).Result()
		if err != nil && err != redis.Nil {
			continue
		}

		// Get TTL
		ttl, _ := s.client.TTL(ctx, key).Result()
		ttlSeconds := int64(0)
		if ttl > 0 {
			ttlSeconds = int64(ttl.Seconds())
		}

		// Remove prefix from key for display
		displayKey := key[len(s.keyPrefix):]

		items = append(items, Item{
			Key:       displayKey,
			Value:     val,
			Timestamp: time.Now(), // Redis doesn't store creation time
			TTL:       ttlSeconds,
		})
	}

	return items, nil
}

// GetSessionContext retrieves all context for a session
func (s *RedisStore) GetSessionContext(ctx context.Context, sessionID string) (*SessionContext, error) {
	key := s.prefixKey("session:" + sessionID)
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Session doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session %s: %w", sessionID, err)
	}

	var session SessionContext
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// SaveSessionContext stores session context
func (s *RedisStore) SaveSessionContext(ctx context.Context, session *SessionContext) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	session.LastUpdatedAt = time.Now()

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := s.prefixKey("session:" + session.SessionID)
	// Session TTL of 24 hours
	if err := s.client.Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// SaveTaskContext stores a task context
func (s *RedisStore) SaveTaskContext(ctx context.Context, task *TaskContext) error {
	if task == nil {
		return fmt.Errorf("task cannot be nil")
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	key := s.prefixKey("task:" + task.TaskID)
	// Task TTL of 1 hour
	if err := s.client.Set(ctx, key, data, time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// GetTaskContext retrieves a task context
func (s *RedisStore) GetTaskContext(ctx context.Context, taskID string) (*TaskContext, error) {
	key := s.prefixKey("task:" + taskID)
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task %s: %w", taskID, err)
	}

	var task TaskContext
	if err := json.Unmarshal([]byte(val), &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	return &task, nil
}

// AddAgentInteraction records an agent interaction for a session
func (s *RedisStore) AddAgentInteraction(ctx context.Context, sessionID string, interaction AgentInteraction) error {
	session, err := s.GetSessionContext(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		session = NewSessionContext(sessionID)
	}

	session.AddInteraction(interaction)
	return s.SaveSessionContext(ctx, session)
}

// SetSessionMemory stores a key-value in session memory
func (s *RedisStore) SetSessionMemory(ctx context.Context, sessionID, key, value string) error {
	session, err := s.GetSessionContext(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		session = NewSessionContext(sessionID)
	}

	session.SetMemory(key, value)
	return s.SaveSessionContext(ctx, session)
}
