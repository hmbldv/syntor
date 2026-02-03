package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded skill definition
type Skill struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AlwaysActive bool     `yaml:"always_active"`
	Triggers     []string `yaml:"triggers"` // edit, write, bash, etc.
	Path         string   `yaml:"-"`        // File path (not from YAML)
	Content      string   `yaml:"-"`        // Full content (not from YAML)
}

// SkillManager handles loading and managing skills
type SkillManager struct {
	skills    map[string]*Skill
	skillsDir string
}

// NewSkillManager creates a new skill manager
func NewSkillManager() *SkillManager {
	home, _ := os.UserHomeDir()
	return &SkillManager{
		skills:    make(map[string]*Skill),
		skillsDir: filepath.Join(home, ".syntor", "skills"),
	}
}

// NewSkillManagerWithDir creates a skill manager with a custom directory
func NewSkillManagerWithDir(dir string) *SkillManager {
	return &SkillManager{
		skills:    make(map[string]*Skill),
		skillsDir: dir,
	}
}

// LoadAll loads all skills from the skills directory
func (m *SkillManager) LoadAll() error {
	if _, err := os.Stat(m.skillsDir); os.IsNotExist(err) {
		return nil // No skills directory, not an error
	}

	entries, err := os.ReadDir(m.skillsDir)
	if err != nil {
		return fmt.Errorf("reading skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(m.skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue // No SKILL.md in this directory
		}

		skill, err := LoadSkill(skillPath)
		if err != nil {
			// Log but don't fail on individual skill errors
			continue
		}

		m.skills[skill.Name] = skill
	}

	return nil
}

// LoadSkill loads a single skill from a SKILL.md file
func LoadSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading skill file: %w", err)
	}

	content := string(data)
	skill := &Skill{
		Path:    path,
		Content: content,
	}

	// Parse YAML frontmatter
	if err := parseFrontmatter(content, skill); err != nil {
		// If no frontmatter, use directory name as skill name
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return skill, nil
}

// parseFrontmatter extracts and parses YAML frontmatter from markdown content
func parseFrontmatter(content string, skill *Skill) error {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// Check for opening ---
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return fmt.Errorf("no frontmatter found")
	}

	var frontmatterLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		frontmatterLines = append(frontmatterLines, line)
	}

	if len(frontmatterLines) == 0 {
		return fmt.Errorf("empty frontmatter")
	}

	frontmatter := strings.Join(frontmatterLines, "\n")
	return yaml.Unmarshal([]byte(frontmatter), skill)
}

// Get returns a skill by name
func (m *SkillManager) Get(name string) (*Skill, bool) {
	skill, ok := m.skills[name]
	return skill, ok
}

// GetAll returns all loaded skills
func (m *SkillManager) GetAll() []*Skill {
	skills := make([]*Skill, 0, len(m.skills))
	for _, skill := range m.skills {
		skills = append(skills, skill)
	}
	return skills
}

// GetAlwaysActive returns skills that should always be active
func (m *SkillManager) GetAlwaysActive() []*Skill {
	var active []*Skill
	for _, skill := range m.skills {
		if skill.AlwaysActive {
			active = append(active, skill)
		}
	}
	return active
}

// GetByTrigger returns skills that should activate for a given trigger
func (m *SkillManager) GetByTrigger(trigger string) []*Skill {
	var triggered []*Skill
	for _, skill := range m.skills {
		for _, t := range skill.Triggers {
			if t == trigger {
				triggered = append(triggered, skill)
				break
			}
		}
	}
	return triggered
}

// InjectIntoPrompt formats a skill for injection into a prompt
func (s *Skill) InjectIntoPrompt() string {
	// Extract content after frontmatter
	content := s.Content
	if strings.HasPrefix(content, "---") {
		// Skip frontmatter
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			content = strings.TrimSpace(parts[2])
		}
	}

	return fmt.Sprintf("<skill name=\"%s\">\n%s\n</skill>", s.Name, content)
}

// InjectAll formats all provided skills for prompt injection
func InjectAll(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "<skills>")
	for _, skill := range skills {
		parts = append(parts, skill.InjectIntoPrompt())
	}
	parts = append(parts, "</skills>")

	return strings.Join(parts, "\n\n")
}

// Count returns the number of loaded skills
func (m *SkillManager) Count() int {
	return len(m.skills)
}

// Names returns the names of all loaded skills
func (m *SkillManager) Names() []string {
	names := make([]string, 0, len(m.skills))
	for name := range m.skills {
		names = append(names, name)
	}
	return names
}
