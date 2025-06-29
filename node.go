package dagpher

import "context"

type Node[C any] interface {
	Name() string           // name, global unique
	Dependencies() []string // dependencies name
	Exec(context.Context, C) error
}

type quickNode[C any] struct {
	name string
	deps []string
	exec func(ctx context.Context, c C) error
}

func (q *quickNode[C]) Dependencies() []string {
	return q.deps
}

// Exec implements Node.
func (q *quickNode[C]) Exec(ctx context.Context, c C) error {
	return q.exec(ctx, c)
}

// Name implements Node.
func (q *quickNode[C]) Name() string {
	return q.name
}

func NewQuickNode[C any](name string, fn func(ctx context.Context, c C) error, deps ...string) Node[C] {
	return &quickNode[C]{
		name: name,
		deps: deps,
		exec: fn,
	}
}
