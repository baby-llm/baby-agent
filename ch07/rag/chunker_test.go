package rag

import "testing"

func TestLineChunker_RespectsMaxLines(t *testing.T) {
	chunker := NewLineChunker(2, 0)
	chunks := chunker.Chunk("doc.go", "a\nb\nc\nd\ne")

	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(chunks))
	}

	wantRanges := [][2]int{{1, 2}, {3, 4}, {5, 5}}
	for i, want := range wantRanges {
		got := chunks[i].Meta
		if got.StartPos != want[0] || got.EndPos != want[1] {
			t.Errorf("chunk %d range = %d-%d, want %d-%d", i, got.StartPos, got.EndPos, want[0], want[1])
		}
	}
	if chunks[0].Content != "a\nb" {
		t.Errorf("chunk 0 content = %q", chunks[0].Content)
	}
}

func TestParagraphChunker_GroupsParagraphs(t *testing.T) {
	content := "p1 line1\np1 line2\n\np2 line1\n\np3 line1"
	chunker := NewParagraphChunker(2)
	chunks := chunker.Chunk("doc.md", content)

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2: %#v", len(chunks), chunks)
	}

	first := chunks[0]
	if first.Meta.StartPos != 1 || first.Meta.EndPos != 5 {
		t.Errorf("first chunk range = %d-%d, want 1-5", first.Meta.StartPos, first.Meta.EndPos)
	}
	if first.Content != "p1 line1\np1 line2\n\np2 line1" {
		t.Errorf("first chunk content = %q", first.Content)
	}

	second := chunks[1]
	if second.Meta.StartPos != 6 || second.Meta.EndPos != 6 {
		t.Errorf("second chunk range = %d-%d, want 6-6", second.Meta.StartPos, second.Meta.EndPos)
	}
	if second.Content != "p3 line1" {
		t.Errorf("second chunk content = %q", second.Content)
	}
}

func TestParagraphChunker_OneParagraphPerChunk(t *testing.T) {
	content := "a\n\nb\n\nc"
	chunks := NewParagraphChunker(1).Chunk("doc.md", content)
	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(chunks))
	}
	for i, want := range []string{"a", "b", "c"} {
		if chunks[i].Content != want {
			t.Errorf("chunk %d content = %q, want %q", i, chunks[i].Content, want)
		}
	}
}

func TestParagraphChunker_ConsecutiveBlankLines(t *testing.T) {
	chunks := NewParagraphChunker(2).Chunk("doc.md", "a\n\n\nb")
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Content != "a\n\n\nb" {
		t.Errorf("content = %q", chunks[0].Content)
	}
}
