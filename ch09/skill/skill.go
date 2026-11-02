package skill

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"babyagent/shared"
)

// Skill contains the complete skill payload used by load_skill tool.
type Skill struct {
	ID              string
	Name            string
	Description     string
	MainInstruction string
	Scripts         []string
	References      []string
}

// Manager handles skill discovery and metadata loading
type Manager struct {
	skillsDir string
	skills    []Skill
}

// NewManager creates a new skill manager
func NewManager() *Manager {
	skillsDir := filepath.Join(shared.GetWorkspaceDir(), ".babyagent", "skills")
	return &Manager{
		skillsDir: skillsDir,
		skills:    make([]Skill, 0),
	}
}

// LoadAll discovers and loads all skill metadata from .babyagent/skills/
func (m *Manager) LoadAll() error {
	if _, err := os.Stat(m.skillsDir); os.IsNotExist(err) {
		// Skills directory doesn't exist, that's ok
		return nil
	}

	entries, err := os.ReadDir(m.skillsDir)
	if err != nil {
		return fmt.Errorf("failed to read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillID := entry.Name()

		skillData, err := LoadSkill(skillID)
		if err != nil {
			// One broken skill should not prevent the others from loading, but
			// the warning must be visible instead of disappearing into stdout.
			log.Printf("failed to load skill %s: %v", skillID, err)
			continue
		}

		m.skills = append(m.skills, skillData)
	}

	return nil
}

// FormatForPrompt formats skill metadata for system prompt injection
func (m *Manager) FormatForPrompt() string {
	if len(m.skills) == 0 {
		return "No skills available."
	}

	var sb strings.Builder
	sb.WriteString("You have access to the following skills. ")
	sb.WriteString("When a user request matches a skill's purpose, use the `load_skill` tool to load the full skill instructions.\n\n")

	for _, loadedSkill := range m.skills {
		// The model must call load_skill with the ID (directory name), so print
		// that rather than the display name.
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", loadedSkill.ID, loadedSkill.Description))
	}

	return sb.String()
}
