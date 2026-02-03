package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Stats represents usage statistics
type Stats struct {
	Version  int            `json:"version"`
	Sessions SessionStats   `json:"sessions"`
	Tokens   TokenStats     `json:"tokens"`
	Tools    ToolStats      `json:"tools"`
	Daily    map[string]Day `json:"daily"`

	mu   sync.Mutex `json:"-"`
	path string     `json:"-"`
}

// SessionStats tracks session information
type SessionStats struct {
	Total int        `json:"total"`
	Last  *time.Time `json:"last"`
}

// TokenStats tracks token usage
type TokenStats struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// ToolStats tracks tool usage
type ToolStats struct {
	Calls  int64          `json:"calls"`
	ByName map[string]int `json:"byName"`
}

// Day tracks daily statistics
type Day struct {
	Sessions int   `json:"sessions"`
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Tools    int64 `json:"tools"`
}

// DefaultStatsPath returns the default path for stats file
func DefaultStatsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".syntor", "stats.json")
}

// Load loads stats from the default location
func Load() (*Stats, error) {
	return LoadFrom(DefaultStatsPath())
}

// LoadFrom loads stats from a specific path
func LoadFrom(path string) (*Stats, error) {
	stats := &Stats{
		Version: 1,
		Daily:   make(map[string]Day),
		Tools:   ToolStats{ByName: make(map[string]int)},
		path:    path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil // Return defaults
		}
		return nil, fmt.Errorf("reading stats file: %w", err)
	}

	if err := json.Unmarshal(data, stats); err != nil {
		return nil, fmt.Errorf("parsing stats file: %w", err)
	}

	stats.path = path

	// Ensure maps are initialized
	if stats.Daily == nil {
		stats.Daily = make(map[string]Day)
	}
	if stats.Tools.ByName == nil {
		stats.Tools.ByName = make(map[string]int)
	}

	return stats, nil
}

// Save saves stats to disk
func (s *Stats) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating stats directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling stats: %w", err)
	}

	return os.WriteFile(s.path, data, 0644)
}

// RecordSession records a new session
func (s *Stats) RecordSession() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Sessions.Total++
	now := time.Now()
	s.Sessions.Last = &now

	today := now.Format("2006-01-02")
	day := s.Daily[today]
	day.Sessions++
	s.Daily[today] = day
}

// RecordTokens records token usage
func (s *Stats) RecordTokens(input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Tokens.Input += input
	s.Tokens.Output += output

	today := time.Now().Format("2006-01-02")
	day := s.Daily[today]
	day.Input += input
	day.Output += output
	s.Daily[today] = day
}

// RecordToolCall records a tool invocation
func (s *Stats) RecordToolCall(toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Tools.Calls++
	s.Tools.ByName[toolName]++

	today := time.Now().Format("2006-01-02")
	day := s.Daily[today]
	day.Tools++
	s.Daily[today] = day
}

// GetTodayStats returns statistics for today
func (s *Stats) GetTodayStats() Day {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	return s.Daily[today]
}

// TotalTokens returns total tokens used
func (s *Stats) TotalTokens() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Tokens.Input + s.Tokens.Output
}

// CleanupOldDays removes daily stats older than the specified number of days
func (s *Stats) CleanupOldDays(keepDays int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -keepDays)
	cutoffStr := cutoff.Format("2006-01-02")

	for date := range s.Daily {
		if date < cutoffStr {
			delete(s.Daily, date)
		}
	}
}

// Summary returns a formatted summary of stats
func (s *Stats) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := s.Daily[time.Now().Format("2006-01-02")]

	var lastSession string
	if s.Sessions.Last != nil {
		lastSession = s.Sessions.Last.Format("2006-01-02 15:04")
	} else {
		lastSession = "never"
	}

	return fmt.Sprintf(`Stats Summary:
  Sessions: %d total (last: %s)
  Tokens: %d input, %d output
  Tools: %d calls
  Today: %d sessions, %d tokens, %d tool calls`,
		s.Sessions.Total, lastSession,
		s.Tokens.Input, s.Tokens.Output,
		s.Tools.Calls,
		today.Sessions, today.Input+today.Output, today.Tools)
}
