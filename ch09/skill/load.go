package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"babyagent/shared"
)

// skillIDPattern follows the agentskills.io name rules: lowercase alphanumeric
// segments separated by single hyphens. It also makes path traversal impossible
// because dots and path separators are rejected.
var skillIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// frontMatter represents the YAML front matter structure
type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// LoadSkill loads a skill by ID from .babyagent/skills/<id>/.
func LoadSkill(id string) (Skill, error) {
	if err := validateSkillID(id); err != nil {
		return Skill{}, err
	}

	workspaceDir := shared.GetWorkspaceDir()
	skillsRoot := filepath.Join(workspaceDir, ".babyagent", "skills")
	skillDir := filepath.Join(skillsRoot, id)
	instructionPath := filepath.Join(skillDir, "SKILL.md")

	// The id regex already blocks "../", but a symlinked skill directory could
	// still point outside the workspace. Compare the resolved paths before
	// reading anything.
	resolvedRoot, err := filepath.EvalSymlinks(skillsRoot)
	if err != nil {
		return Skill{}, fmt.Errorf("resolve skills directory: %w", err)
	}
	resolvedDir, err := filepath.EvalSymlinks(skillDir)
	if err != nil {
		return Skill{}, fmt.Errorf("resolve skill directory: %w", err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedDir)
	if err != nil {
		return Skill{}, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Skill{}, fmt.Errorf("skill %q resolves outside the skills directory", id)
	}

	// SKILL.md itself may be a symlink; verify it also stays inside the
	// resolved skill directory before reading it.
	resolvedInstruction, err := filepath.EvalSymlinks(instructionPath)
	if err != nil {
		return Skill{}, fmt.Errorf("resolve skill file: %w", err)
	}
	relFile, err := filepath.Rel(resolvedDir, resolvedInstruction)
	if err != nil {
		return Skill{}, err
	}
	if relFile == ".." || strings.HasPrefix(relFile, ".."+string(filepath.Separator)) {
		return Skill{}, fmt.Errorf("skill file %q resolves outside the skill directory", id)
	}

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
	if !skillIDPattern.MatchString(fm.Name) || len(fm.Name) > 64 {
		return Skill{}, fmt.Errorf("invalid skill name %q: use 1-64 lowercase letters, digits and single hyphens", fm.Name)
	}
	if fm.Name != id {
		return Skill{}, fmt.Errorf("skill name %q must match its directory name %q", fm.Name, id)
	}

	if fm.Description == "" {
		return Skill{}, errors.New("skill must have a 'description' field in front matter")
	}
	// Descriptions are injected into the system prompt verbatim; cap them so a
	// skill file cannot silently consume the whole context window.
	if len([]rune(fm.Description)) > 1024 {
		return Skill{}, errors.New("skill description must be at most 1024 characters")
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

func validateSkillID(id string) error {
	if id == "" {
		return errors.New("skill id is required")
	}
	if len(id) > 64 || !skillIDPattern.MatchString(id) {
		return fmt.Errorf("invalid skill id %q: use 1-64 lowercase letters, digits and single hyphens", id)
	}
	return nil
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
