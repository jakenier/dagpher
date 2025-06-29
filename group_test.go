package dagpher

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestContext struct {
	mu  sync.Mutex
	log []string
}

func (c *TestContext) Log(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = append(c.log, msg)
}

func (c *TestContext) GetLog() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	logCopy := make([]string, len(c.log))
	copy(logCopy, c.log)
	return logCopy
}

func TestGroupExecution(t *testing.T) {
	t.Run("simple group with one node", func(t *testing.T) {
		testCtx := &TestContext{}
		group := NewGroup[*TestContext]()
		nodeA := NewQuickNode("A", func(ctx context.Context, c *TestContext) error {
			c.Log("A executed")
			return nil
		})
		group.AddNode(nodeA)

		executor := NewGroupExecutor(testCtx, group)
		require.NoError(t, executor.Build())
		require.NoError(t, executor.Execute(context.Background()))

		assert.Equal(t, []string{"A executed"}, testCtx.GetLog())
	})

	t.Run("group with dependencies", func(t *testing.T) {
		testCtx := &TestContext{}
		group := NewGroup[*TestContext]()
		nodeA := NewQuickNode("A", func(ctx context.Context, c *TestContext) error {
			c.Log("A executed")
			return nil
		})
		nodeB := NewQuickNode("B", func(ctx context.Context, c *TestContext) error {
			c.Log("B executed")
			return nil
		}, "A")
		group.AddNode(nodeA)
		group.AddNode(nodeB)

		executor := NewGroupExecutor(testCtx, group)
		require.NoError(t, executor.Build())
		require.NoError(t, executor.Execute(context.Background()))

		assert.Equal(t, []string{"A executed", "B executed"}, testCtx.GetLog())
	})

	t.Run("nested subgroups", func(t *testing.T) {
		testCtx := &TestContext{}
		rootGroup := NewGroup[*TestContext]()
		subGroup1 := NewGroup[*TestContext]()
		subGroup2 := NewGroup[*TestContext]()

		nodeA := NewQuickNode("A", func(ctx context.Context, c *TestContext) error {
			c.Log("A executed")
			return nil
		})
		nodeB := NewQuickNode("B", func(ctx context.Context, c *TestContext) error {
			c.Log("B executed")
			return nil
		}, "A")
		nodeC := NewQuickNode("C", func(ctx context.Context, c *TestContext) error {
			c.Log("C executed")
			return nil
		}, "B")

		rootGroup.AddNode(nodeA)
		subGroup1.AddNode(nodeB)
		subGroup2.AddNode(nodeC)
		subGroup1.AddSubGroup(subGroup2)
		rootGroup.AddSubGroup(subGroup1)

		executor := NewGroupExecutor(testCtx, rootGroup)
		require.NoError(t, executor.Build())
		require.NoError(t, executor.Execute(context.Background()))

		assert.Equal(t, []string{"A executed", "B executed", "C executed"}, testCtx.GetLog())
	})

	t.Run("duplicate node in different groups panics", func(t *testing.T) {
		rootGroup := NewGroup[*TestContext]()
		subGroup := NewGroup[*TestContext]()

		nodeA1 := NewQuickNode("A", func(ctx context.Context, c *TestContext) error { return nil })
		nodeA2 := NewQuickNode("A", func(ctx context.Context, c *TestContext) error { return nil })

		rootGroup.AddNode(nodeA1)
		subGroup.AddNode(nodeA2)
		rootGroup.AddSubGroup(subGroup)

		executor := NewGroupExecutor(&TestContext{}, rootGroup)
		assert.PanicsWithValue(t, "node with the same name already exists: A", func() {
			executor.Build()
		})
	})

	t.Run("middleware execution order", func(t *testing.T) {
		testCtx := &TestContext{}
		group := NewGroup[*TestContext]()

		mw1 := func(next Endpoint) Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				testCtx.Log("global mw1 start")
				res, err := next(ctx, req)
				testCtx.Log("global mw1 end")
				return res, err
			}
		}
		mw2 := func(next Endpoint) Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				testCtx.Log("node mw2 start")
				res, err := next(ctx, req)
				testCtx.Log("node mw2 end")
				return res, err
			}
		}

		nodeA := NewQuickNode("A", func(ctx context.Context, c *TestContext) error {
			c.Log("A executed")
			return nil
		})
		group.AddNode(nodeA, WithMiddlewares(mw2))

		executor := NewGroupExecutor(testCtx, group, mw1)
		require.NoError(t, executor.Build())
		require.NoError(t, executor.Execute(context.Background()))

		expectedLog := []string{
			"global mw1 start",
			"node mw2 start",
			"A executed",
			"node mw2 end",
			"global mw1 end",
		}
		assert.Equal(t, expectedLog, testCtx.GetLog())
	})

	t.Run("loop variable capture", func(t *testing.T) {
		testCtx := &TestContext{}
		group := NewGroup[*TestContext]()

		// Add multiple nodes to test if the loop variable was captured correctly.
		nodes := []Node[*TestContext]{
			NewQuickNode("A", func(ctx context.Context, c *TestContext) error { c.Log("A"); return nil }),
			NewQuickNode("B", func(ctx context.Context, c *TestContext) error { c.Log("B"); return nil }),
			NewQuickNode("C", func(ctx context.Context, c *TestContext) error { c.Log("C"); return nil }),
		}
		for _, n := range nodes {
			group.AddNode(n)
		}

		executor := NewGroupExecutor(testCtx, group)
		require.NoError(t, executor.Build())
		require.NoError(t, executor.Execute(context.Background()))

		// The order is not guaranteed, so we check for the presence of all logs.
		logs := testCtx.GetLog()
		assert.ElementsMatch(t, []string{"A", "B", "C"}, logs)
	})
}
