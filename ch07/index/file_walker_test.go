package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileWalker_SkipsSecretsAndBuildDirs(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("main.go", "package main")
	writeFile("README.md", "# demo")
	writeFile(".env", "OPENAI_API_KEY=secret")
	writeFile("config.json", `{"api_key":"secret"}`)
	writeFile("config.example.json", `{"api_key":"placeholder"}`)
	writeFile("credentials.json", `{"token":"secret"}`)
	writeFile("key.pem", "secret")
	writeFile(".babyagent/MEMORY.md", "secret")
	writeFile("node_modules/pkg/index.js", "x")

	files, err := NewFileWalker().Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	got := make(map[string]bool, len(files))
	for _, file := range files {
		got[filepath.Base(file)] = true
	}

	for _, want := range []string{"main.go", "README.md"} {
		if !got[want] {
			t.Errorf("expected %s to be indexed, got %v", want, got)
		}
	}
	for _, unexpected := range []string{".env", "config.json", "config.example.json", "credentials.json", "key.pem", "MEMORY.md", "index.js"} {
		if got[unexpected] {
			t.Errorf("did not expect %s to be indexed", unexpected)
		}
	}
}
