package context

import (
	"context"
)

type ContextStrategy interface {
	Name() string
	ShouldApply(ctx context.Context, engine *ContextEngine) bool
	Apply(ctx context.Context, engine *ContextEngine) error
}
