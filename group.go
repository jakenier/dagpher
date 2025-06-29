package dagpher

import (
	"context"
	"fmt"

	"github.com/jakenier/dagpher/executor"
)

type Group[C any] struct {
	nodeMap     map[string]Node[C]
	nodeOptions map[string]*option
	subGroups   []*Group[C]
}

func NewGroup[C any]() *Group[C] {
	return &Group[C]{
		nodeMap:     make(map[string]Node[C]),
		nodeOptions: make(map[string]*option),
		subGroups:   make([]*Group[C], 0),
	}
}

func (g *Group[C]) AddNode(node Node[C], opts ...Option) {
	if _, exists := g.nodeMap[node.Name()]; exists {
		panic("node with the same name already exists: " + node.Name())
	}

	g.nodeMap[node.Name()] = node
	g.nodeOptions[node.Name()] = getOption(opts...)
}

func (g *Group[C]) AddSubGroup(subGroup *Group[C]) {
	if subGroup == nil {
		panic("subGroup cannot be nil")
	}
	g.subGroups = append(g.subGroups, subGroup)
}

type groupExecutor[C any] struct {
	execCtx   C
	group     *Group[C]
	globalMws []Middleware
	exec      *executor.Engine[C]
}

func NewGroupExecutor[C any](execCtx C, group *Group[C], globalMws ...Middleware) *groupExecutor[C] {
	return &groupExecutor[C]{
		execCtx:   execCtx,
		group:     group,
		globalMws: globalMws,
		exec:      executor.NewEngine[C](),
	}
}

func (g *groupExecutor[C]) Build() error {
	allNodes := make(map[string]Node[C])
	allOptions := make(map[string]*option)
	visited := make(map[*Group[C]]bool)

	var collect func(group *Group[C])
	collect = func(group *Group[C]) {
		if visited[group] {
			return
		}
		visited[group] = true

		for name, node := range group.nodeMap {
			if _, exists := allNodes[name]; exists {
				panic("node with the same name already exists: " + name)
			}
			allNodes[name] = node
			allOptions[name] = group.nodeOptions[name]
		}

		for _, sub := range group.subGroups {
			collect(sub)
		}
	}

	collect(g.group)

	for name, node := range allNodes {
		capturedNode := node
		capturedName := name

		opt := allOptions[capturedName]
		mws := opt.mergeMws(g.globalMws)

		execNode := func(ctx context.Context, c C) error {
			_, err := Chain(mws...)(func(ctx context.Context, in any) (out any, err error) {
				realIn, ok := in.(C)
				if !ok {
					return nil, fmt.Errorf("expected input type %T, got %T", c, in)
				}

				err = capturedNode.Exec(ctx, realIn)
				if err != nil {
					return nil, err
				}
				return realIn, nil
			})(ctx, c)
			if err != nil {
				return err
			}
			return nil
		}

		g.exec.AddNode(capturedName, execNode, capturedNode.Dependencies()...)
	}

	return g.exec.Build()
}

func (g *groupExecutor[C]) Execute(ctx context.Context) error {
	if g.exec == nil {
		return fmt.Errorf("executor is not built, call Build() first")
	}

	return g.exec.Execute(ctx, g.execCtx)
}
