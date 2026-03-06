package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"babyagent/shared"
)

// frontMatter represents the YAML front matter structure
type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// LoadSkill loads a skill by ID from .babyagent/skills/<id>/.
func LoadSkill(id string) (Skill, error) {
	workspaceDir := shared.GetWorkspaceDir()
	skillDir := filepath.Join(workspaceDir, ".babyagent", "skills", id)
	instructionPath := filepath.Join(skillDir, "SKILL.md")

	instructionBytes, err := os.ReadFile(instructionPath)
	if err != nil {
		return Skill{}, fmt.Errorf("failed to read skill file: %w", err)
	}

	text := string(instructionBytes)

	// Split by front matter delimiter
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return Skill{}, errors.New("skill file must have YAML front matter enclosed in `---`")
	}

	// parts[0] is empty (before first ---)
	// parts[1] is the YAML front matter
	// parts[2] is the body content

	// Parse front matter using yaml.Unmarshal
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return Skill{}, fmt.Errorf("failed to parse front matter: %w", err)
	}

	if fm.Name == "" {
		return Skill{}, errors.New("skill must have a 'name' field in front matter")
	}

	if fm.Description == "" {
		return Skill{}, errors.New("skill must have a 'description' field in front matter")
	}

	scripts, err := listFiles(filepath.Join(skillDir, "scripts"), workspaceDir)
	if err != nil {
		return Skill{}, fmt.Errorf("failed to discover skill scripts: %w", err)
	}

	references, err := listFiles(filepath.Join(skillDir, "references"), workspaceDir)
	if err != nil {
		return Skill{}, fmt.Errorf("failed to discover skill references: %w", err)
	}

	return Skill{
		ID:              id,
		Name:            fm.Name,
		Description:     fm.Description,
		MainInstruction: strings.TrimSpace(parts[2]),
		Scripts:         scripts,
		References:      references,
	}, nil
}

func listFiles(baseDir, workspaceDir string) ([]string, error) {
	if _, err := os.Stat(baseDir); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	files := make([]string, 0)
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return err
		}

		files = append(files, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}
