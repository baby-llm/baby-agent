package tool

import (
	"regexp"
	"testing"
)

func TestGenerateContainerName_UniqueAndValid(t *testing.T) {
	a := generateContainerName("/workspace/a/project")
	b := generateContainerName("/workspace/b/project")
	if a == b {
		t.Fatalf("same container name for different workspaces: %s", a)
	}

	valid := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	for _, name := range []string{a, b, generateContainerName("/workspace/项目 project")} {
		if !valid.MatchString(name) {
			t.Errorf("invalid Docker container name: %q", name)
		}
	}
}
