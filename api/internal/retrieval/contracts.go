// Package retrieval defines class-scoped, traceable knowledge retrieval ports.
// It intentionally contains no MySQL or Qdrant implementation in the skeleton.
package retrieval

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

type Scope struct {
	ClassID     string
	MaterialIDs []string
}

type Query struct {
	Text  string
	Limit int
}

// ChunkRef binds a hit to an immutable source version and exact text range.
type ChunkRef struct {
	MaterialID      string
	MaterialVersion string
	ChunkID         string
	ChunkIndex      int
	CharacterStart  int
	CharacterEnd    int
	ContentHash     string
	Text            string
	KeywordScore    float64
	VectorScore     float64
	FusionScore     float64
}

type SearchResult struct {
	RetrievalRunID string
	Hits           []ChunkRef
}

// Retriever never accepts user-supplied class scope directly; callers build
// Scope from the authenticated server-side principal and resource bindings.
type Retriever interface {
	Search(ctx context.Context, scope Scope, query Query) (SearchResult, error)
}

type StubRetriever struct{}

func (StubRetriever) Search(_ context.Context, _ Scope, _ Query) (SearchResult, error) {
	return SearchResult{}, apperrors.ErrNotImplemented
}
