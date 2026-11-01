package rag

import (
	"context"
	"testing"
)

func TestEmbeddingRequiresBaseURL(t *testing.T) {
	service := NewHTTPEmbeddingService(HTTPEmbeddingConfig{})
	if _, err := service.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("Embed() should fail when BaseURL is empty")
	}
}

func TestRerankRequiresBaseURL(t *testing.T) {
	service := NewHTTPRerankService(HTTPRerankConfig{})
	_, err := service.Rerank(context.Background(), "hello", []Chunk{{Content: "doc"}})
	if err == nil {
		t.Fatal("Rerank() should fail when BaseURL is empty")
	}
}
