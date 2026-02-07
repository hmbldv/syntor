package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appendResult writes a test result to /tmp/centaur-test-results.jsonl.
func appendResult(t *testing.T, batch int, testID, name, result string, duration time.Duration, notes string) {
	t.Helper()
	entry := map[string]interface{}{
		"batch":      batch,
		"test_id":    testID,
		"name":       name,
		"result":     result,
		"duration_s": duration.Seconds(),
		"notes":      notes,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile("/tmp/centaur-test-results.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Logf("warning: could not write result: %v", err)
		return
	}
	defer f.Close()
	f.Write(data)
	f.WriteString("\n")
}

// T031: Default directories are set when empty strings are passed.
func TestNewManager_Defaults(t *testing.T) {
	start := time.Now()
	m := NewManager("", "")

	home, _ := os.UserHomeDir()
	expectedGlobal := filepath.Join(home, ".syntor")

	assert.Equal(t, expectedGlobal, m.globalDir, "globalDir should default to ~/.syntor")
	assert.Equal(t, ".syntor", m.projectDir, "projectDir should default to .syntor")

	dur := time.Since(start)
	appendResult(t, 4, "T031", "TestNewManager_Defaults", "pass", dur, "default dirs verified")
}

// T032: LoadGlobal returns empty when MEMORY.md does not exist.
func TestLoadGlobal_Missing(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(tmp, t.TempDir())

	content, err := m.LoadGlobal()

	assert.NoError(t, err)
	assert.Empty(t, content, "should return empty string when MEMORY.md is missing")

	dur := time.Since(start)
	appendResult(t, 4, "T032", "TestLoadGlobal_Missing", "pass", dur, "empty on missing file")
}

// T033: LoadGlobal reads MEMORY.md content when the file exists.
func TestLoadGlobal_Exists(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	expected := "# Global Memory\n- insight one\n- insight two\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, GlobalMemoryFile), []byte(expected), 0644))

	m := NewManager(tmp, t.TempDir())
	content, err := m.LoadGlobal()

	assert.NoError(t, err)
	assert.Equal(t, expected, content)

	dur := time.Since(start)
	appendResult(t, 4, "T033", "TestLoadGlobal_Exists", "pass", dur, "reads existing MEMORY.md")
}

// T034: LoadProject returns empty when no project MEMORY.md exists.
func TestLoadProject_Missing(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(t.TempDir(), tmp)

	content, err := m.LoadProject()

	assert.NoError(t, err)
	assert.Empty(t, content, "should return empty string when project MEMORY.md is missing")

	dur := time.Since(start)
	appendResult(t, 4, "T034", "TestLoadProject_Missing", "pass", dur, "empty on missing project file")
}

// T035: Write appends content to global MEMORY.md.
func TestWrite_Global(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(tmp, t.TempDir())

	err := m.Write("global", "- first insight")
	require.NoError(t, err)
	err = m.Write("global", "- second insight")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmp, GlobalMemoryFile))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "- first insight")
	assert.Contains(t, content, "- second insight")

	dur := time.Since(start)
	appendResult(t, 4, "T035", "TestWrite_Global", "pass", dur, "global write appends correctly")
}

// T036: Write appends content to project MEMORY.md.
func TestWrite_Project(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(t.TempDir(), tmp)

	err := m.Write("project", "- project insight")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmp, GlobalMemoryFile))
	require.NoError(t, err)

	assert.Contains(t, string(data), "- project insight")

	dur := time.Since(start)
	appendResult(t, 4, "T036", "TestWrite_Project", "pass", dur, "project write appends correctly")
}

// T037: CreateTopicFile creates a topic file with header containing name and timestamp.
func TestCreateTopicFile(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(tmp, t.TempDir())

	err := m.CreateTopicFile("global", "debugging", "Common pitfalls and fixes.")
	require.NoError(t, err)

	path := filepath.Join(tmp, MemoryDir, "debugging.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "# debugging")
	assert.Contains(t, content, "Created:")
	assert.Contains(t, content, "Common pitfalls and fixes.")

	dur := time.Since(start)
	appendResult(t, 4, "T037", "TestCreateTopicFile", "pass", dur, "topic file created with header")
}

// T038: UpdateTopicFile appends to an existing topic file.
func TestUpdateTopicFile(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(tmp, t.TempDir())

	// Create the topic file first
	require.NoError(t, m.CreateTopicFile("global", "patterns", "Initial content."))

	// Update it
	err := m.UpdateTopicFile("global", "patterns", "- new pattern discovered")
	require.NoError(t, err)

	path := filepath.Join(tmp, MemoryDir, "patterns.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "Initial content.")
	assert.Contains(t, content, "- new pattern discovered")

	dur := time.Since(start)
	appendResult(t, 4, "T038", "TestUpdateTopicFile", "pass", dur, "appended to existing topic file")
}

// T039: TruncateMemory truncates content to MaxMemoryLines (200).
func TestTruncateMemory(t *testing.T) {
	start := time.Now()
	tmp := t.TempDir()
	m := NewManager(tmp, t.TempDir())

	// Write a MEMORY.md with 300 lines
	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(tmp, GlobalMemoryFile),
		[]byte(strings.Join(lines, "\n")),
		0644,
	))

	err := m.TruncateMemory("global")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmp, GlobalMemoryFile))
	require.NoError(t, err)

	resultLines := strings.Split(string(data), "\n")
	assert.Equal(t, MaxMemoryLines, len(resultLines), "should truncate to exactly MaxMemoryLines")
	assert.Equal(t, "line 0", resultLines[0])
	assert.Equal(t, fmt.Sprintf("line %d", MaxMemoryLines-1), resultLines[MaxMemoryLines-1])

	dur := time.Since(start)
	appendResult(t, 4, "T039", "TestTruncateMemory", "pass", dur, "truncated to 200 lines")
}

// T040: FormatForPrompt formats memory with XML tags and applies truncation.
func TestFormatForPrompt(t *testing.T) {
	start := time.Now()
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	m := NewManager(globalDir, projectDir)

	// Write global memory
	require.NoError(t, os.WriteFile(
		filepath.Join(globalDir, GlobalMemoryFile),
		[]byte("# Global\n- insight A"),
		0644,
	))
	// Write project memory
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, GlobalMemoryFile),
		[]byte("# Project\n- insight B"),
		0644,
	))

	result := m.FormatForPrompt()

	assert.Contains(t, result, `<memory scope="global">`)
	assert.Contains(t, result, `</memory>`)
	assert.Contains(t, result, `<memory scope="project">`)
	assert.Contains(t, result, "- insight A")
	assert.Contains(t, result, "- insight B")

	// Verify truncation applies for oversized content
	var longLines []string
	for i := 0; i < 250; i++ {
		longLines = append(longLines, fmt.Sprintf("line %d", i))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(globalDir, GlobalMemoryFile),
		[]byte(strings.Join(longLines, "\n")),
		0644,
	))

	result2 := m.FormatForPrompt()
	assert.Contains(t, result2, "... (truncated)")
	assert.NotContains(t, result2, "line 249", "line 249 should be truncated away")

	dur := time.Since(start)
	appendResult(t, 4, "T040", "TestFormatForPrompt", "pass", dur, "formatted with memory tags and truncation")
}
