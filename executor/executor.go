package executor

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	ErrCycleDetected = errors.New("dependency cycle detected")
	ErrNoRootNodes   = errors.New("no root nodes to execute")
)

// Engine is the main structure for managing nodes and their execution for DAG execution.
type Engine[C any] struct {
	rootNode   []string
	nodeToNext map[string][]string
	nodeStat   map[string]*nodeStat
	nodeExec   map[string]func(context.Context, C) error

	opt *option
}

// NewEngine creates a new Engine instance.
func NewEngine[C any](opts ...Option) *Engine[C] {
	return &Engine[C]{
		rootNode:   make([]string, 0),
		nodeToNext: make(map[string][]string),
		nodeStat:   make(map[string]*nodeStat),
		nodeExec:   make(map[string]func(context.Context, C) error),
		opt:        getOption(opts...),
	}
}

func (e *Engine[C]) AddNode(name string, exec func(context.Context, C) error, deps ...string) error {
	if _, exists := e.nodeExec[name]; exists {
		return errors.New("node already exists: " + name)
	}

	e.nodeExec[name] = exec
	e.nodeStat[name] = &nodeStat{
		done: make(chan struct{}),
	}

	for _, dep := range deps {
		if len(dep) == 0 {
			continue // skip empty dependencies
		}

		e.nodeToNext[dep] = append(e.nodeToNext[dep], name)
	}
	return nil
}

func (e *Engine[C]) detectCycle() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var visit func(string) bool
	visit = func(node string) bool {
		if recStack[node] {
			return true // cycle detected
		}
		if visited[node] {
			return false // already visited from another path, no cycle here
		}

		visited[node] = true
		recStack[node] = true

		for _, next := range e.nodeToNext[node] {
			if visit(next) {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	// Iterate over all nodes to detect cycles in any part of the graph.
	for node := range e.nodeExec {
		if !visited[node] {
			if visit(node) {
				return ErrCycleDetected
			}
		}
	}

	return nil
}

func (e *Engine[C]) Build() error {
	// Check that all mentioned dependencies exist as nodes.
	for dep, dependents := range e.nodeToNext {
		if _, exists := e.nodeExec[dep]; !exists {
			return errors.New("dependency node not found: " + dep)
		}
		for _, dependent := range dependents {
			if _, exists := e.nodeExec[dependent]; !exists {
				return errors.New("node '" + dependent + "' (dependent on '" + dep + "') was not added")
			}
		}
	}

	// Calculate in-degrees for all nodes.
	inDegrees := make(map[string]int32)
	for name := range e.nodeExec {
		inDegrees[name] = 0 // Initialize all nodes with an in-degree of 0.
	}
	for _, dependents := range e.nodeToNext {
		for _, dependent := range dependents {
			inDegrees[dependent]++ // Increment in-degree for each dependency.
		}
	}

	// Set the final initial degree on the node stats and identify root nodes.
	// e.rootNode = e.rootNode[:0] // Clear slice before recalculating
	for name, degree := range inDegrees {
		e.nodeStat[name].degree = degree
		if degree == 0 {
			e.rootNode = append(e.rootNode, name)
		}
	}

	// Check for cycles, which is the most common reason for having no root nodes in a non-empty graph.
	if err := e.detectCycle(); err != nil {
		return err
	}

	// If there are nodes but no roots and no cycle was detected, it's an unexecutable graph.
	if len(e.nodeExec) > 0 && len(e.rootNode) == 0 {
		return ErrNoRootNodes
	}

	return nil
}

func (e *Engine[C]) Execute(ctx context.Context, c C) error {
	eg, gCtx := errgroup.WithContext(ctx)
	for _, name := range e.rootNode {
		nodeToRun := name // Capture loop variable to prevent race condition.
		eg.Go(func() error {
			return e.executeNode(gCtx, nodeToRun, c, eg)
		})
	}
	return eg.Wait()
}

func (e *Engine[C]) executeNode(ctx context.Context, name string, c C, eg *errgroup.Group) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	start := time.Now()
	stat := e.nodeStat[name]

	defer func() {
		stat.Done()
		stat.cost = int32(time.Since(start).Milliseconds())
	}()

	if err := e.nodeExec[name](ctx, c); err != nil {
		stat.err.Store(err)
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	for _, next := range e.nodeToNext[name] {
		nextStat := e.nodeStat[next]
		if nextStat == nil {
			continue // Should not happen with a successful Build()
		}

		if atomic.AddInt32(&nextStat.degree, -1) == 0 {
			nodeToRun := next // Capture loop variable to prevent race condition.
			eg.Go(func() error {
				return e.executeNode(ctx, nodeToRun, c, eg)
			})
		}
	}

	return nil
}
