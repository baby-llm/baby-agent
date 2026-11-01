package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	skillDir := filepath.Join(".babyagent", "skills", dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Instructions\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkill_Valid(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSkill(t, "code-review", "code-review", "Review code")

	got, err := LoadSkill("code-review")
	if err != nil {
		t.Fatalf("LoadSkill() error = %v", err)
	}
	if got.ID != "code-review" || got.Name != "code-review" {
		t.Fatalf("skill = %+v", got)
	}
}

func TestLoadSkill_RejectsPathTraversal(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, id := range []string{"../secret", "a/b", `a\b`, ".", "..", "/abs"} {
		if _, err := LoadSkill(id); err == nil {
			t.Errorf("LoadSkill(%q) should fail", id)
		}
	}
}

func TestLoadSkill_RejectsNameMismatch(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSkill(t, "wrong-dir", "code-review", "Review code")

	if _, err := LoadSkill("wrong-dir"); err == nil {
		t.Fatal("LoadSkill() should reject a name that differs from the directory")
	}
}

func TestLoadSkill_RejectsInvalidName(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSkill(t, "code-review", "Code Review", "Review code")

	if _, err := LoadSkill("code-review"); err == nil {
		t.Fatal("LoadSkill() should reject uppercase/spaces in name")
	}
}
