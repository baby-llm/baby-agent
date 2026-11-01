package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"babyagent/ch07/rag"
)

type fakeStore struct {
	indexedTime time.Time
	replaced    []string
	points      []int
	insertCalls int
	deleteCalls int
}

func (f *fakeStore) InsertBatch(context.Context, []rag.VectorPoint) error {
	f.insertCalls++
	return nil
}

func (f *fakeStore) ReplaceDocument(_ context.Context, documentID string, vps []rag.VectorPoint) error {
	f.replaced = append(f.replaced, documentID)
	f.points = append(f.points, len(vps))
	return nil
}

func (f *fakeStore) Search(context.Context, rag.Vector, int) ([]rag.VectorPointResult, error) {
	return nil, nil
}

func (f *fakeStore) DeleteByDocument(context.Context, string) error {
	f.deleteCalls++
	return nil
}

func (f *fakeStore) GetDocumentIndexedTime(context.Context, string) (time.Time, error) {
	return f.indexedTime, nil
}

func (f *fakeStore) Clear(context.Context) error { return nil }
func (f *fakeStore) Close() error                { return nil }

type fakeEmbedder struct {
	err   error
	calls int
}

func (f *fakeEmbedder) Embed(context.Context, string) (rag.Vector, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return rag.Vector{1, 2, 3}, nil
}

func newTestIndexer(t *testing.T, store *fakeStore, embedder *fakeEmbedder) (*Indexer, string) {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "sample.go")
	if err := os.WriteFile(file, []byte("package sample\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer(IndexerConfig{
		RootPath:    root,
		ChunkerType: rag.ChunkerTypeLine,
		MaxLines:    100,
		MaxChars:    2000,
	}, store, embedder)
	return idx, file
}

func TestIndexer_ReplacesDocumentWithSlashPath(t *testing.T) {
	store := &fakeStore{}
	embedder := &fakeEmbedder{}
	idx, _ := newTestIndexer(t, store, embedder)

	result, err := idx.Index(context.Background())
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if result.FailedFiles != 0 || result.SuccessFiles != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.replaced) != 1 || store.replaced[0] != "sample.go" {
		t.Fatalf("replaced = %v, want [sample.go]", store.replaced)
	}
	if store.insertCalls != 0 {
		t.Fatalf("InsertBatch called %d times; indexing should use ReplaceDocument", store.insertCalls)
	}
	if len(store.points) != 1 || store.points[0] == 0 {
		t.Fatalf("points = %v, want a non-empty batch", store.points)
	}
}

func TestIndexer_EmbeddingFailureKeepsOldIndex(t *testing.T) {
	store := &fakeStore{}
	embedder := &fakeEmbedder{err: errors.New("embedding down")}
	idx, _ := newTestIndexer(t, store, embedder)

	result, err := idx.Index(context.Background())
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if result.FailedFiles != 1 {
		t.Fatalf("FailedFiles = %d, want 1", result.FailedFiles)
	}
	if len(store.replaced) != 0 {
		t.Fatalf("ReplaceDocument called on embedding failure: %v", store.replaced)
	}
}

func TestIndexer_SkipsUnchangedFile(t *testing.T) {
	store := &fakeStore{indexedTime: time.Now().Add(time.Hour)}
	embedder := &fakeEmbedder{}
	idx, _ := newTestIndexer(t, store, embedder)

	result, err := idx.Index(context.Background())
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if result.SkippedFiles != 1 {
		t.Fatalf("SkippedFiles = %d, want 1", result.SkippedFiles)
	}
	if embedder.calls != 0 || len(store.replaced) != 0 {
		t.Fatalf("unchanged file was re-indexed: embed=%d replace=%v", embedder.calls, store.replaced)
	}
}

func TestIndexer_ReindexesChangedFile(t *testing.T) {
	store := &fakeStore{indexedTime: time.Now().Add(-time.Hour)}
	embedder := &fakeEmbedder{}
	idx, _ := newTestIndexer(t, store, embedder)

	result, err := idx.Index(context.Background())
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if result.ReindexedFiles != 1 {
		t.Fatalf("ReindexedFiles = %d, want 1", result.ReindexedFiles)
	}
	if len(store.replaced) != 1 {
		t.Fatalf("replaced = %v, want one call", store.replaced)
	}
	if store.deleteCalls != 0 {
		t.Fatalf("DeleteByDocument called %d times; ReplaceDocument should own the transaction", store.deleteCalls)
	}
}
