package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSystemStorage_StoreAndOverwrite(t *testing.T) {
	baseDir := t.TempDir()
	store := NewFileSystemStorage(baseDir)
	ctx := context.Background()

	if err := store.Store(ctx, "memory/MEMORY.md", "v1"); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	got, err := store.Load(ctx, "memory/MEMORY.md")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "v1" {
		t.Fatalf("Load() = %q, want v1", got)
	}

	if err := store.Store(ctx, "memory/MEMORY.md", "v2"); err != nil {
		t.Fatalf("Store() overwrite error = %v", err)
	}
	got, err = store.Load(ctx, "memory/MEMORY.md")
	if err != nil {
		t.Fatalf("Load() after overwrite error = %v", err)
	}
	if got != "v2" {
		t.Fatalf("Load() = %q, want v2", got)
	}

	entries, err := os.ReadDir(filepath.Join(baseDir, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}
