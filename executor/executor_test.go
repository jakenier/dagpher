package executor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContext is a dummy context object for the tests.
type TestContext struct {
	mu           sync.Mutex
	executionLog []string
}

func (c *TestContext) Log(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.executionLog = append(c.executionLog, name)
}

func (c *TestContext) GetLog() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Return a copy
	logCopy := make([]string, len(c.executionLog))
	copy(logCopy, c.executionLog)
	return logCopy
}

// sleepAndLogNode is a helper function that creates a node that sleeps
// for a given duration and then logs its name to the TestContext.
func sleepAndLogNode(name string, duration time.Duration) func(context.Context, *TestContext) error {
	return func(ctx context.Context, c *TestContext) error {
		select {
		case <-time.After(duration):
			c.Log(name)
			return nil
		case <-ctx.Done():
			c.Log(fmt.Sprintf("%s-cancelled", name))
			return ctx.Err()
		}
	}
}

// errorNode returns a node that immediately fails.
func errorNode(err error) func(context.Context, *TestContext) error {
	return func(ctx context.Context, c *TestContext) error {
		return err
	}
}

func TestEngine_BuildAndExecute_HappyPath(t *testing.T) {
	t.Run("linear dependency", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		testCtx := &TestContext{}

		// A -> B -> C.
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 10*time.Millisecond)))
		require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 10*time.Millisecond), "A"))
		require.NoError(t, engine.AddNode("C", sleepAndLogNode("C", 10*time.Millisecond), "B"))

		require.NoError(t, engine.Build())

		err := engine.Execute(context.Background(), testCtx)
		require.NoError(t, err)

		log := testCtx.GetLog()
		expectedOrder := []string{"A", "B", "C"}
		assert.Equal(t, expectedOrder, log, "Nodes should execute in correct dependency order")
	})

	t.Run("complex DAG with concurrency", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		testCtx := &TestContext{}

		//    A --+--> B --+
		//          |      |--> D
		//          +--> C --+
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 10*time.Millisecond)))
		require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 20*time.Millisecond), "A"))
		require.NoError(t, engine.AddNode("C", sleepAndLogNode("C", 20*time.Millisecond), "A"))
		require.NoError(t, engine.AddNode("D", sleepAndLogNode("D", 10*time.Millisecond), "B", "C"))

		require.NoError(t, engine.Build())

		start := time.Now()
		err := engine.Execute(context.Background(), testCtx)
		duration := time.Since(start)
		require.NoError(t, err)

		// A (10ms) -> B,C (20ms parallel) -> D (10ms)
		// Total time should be roughly 10 + 20 + 10 = 40ms
		assert.GreaterOrEqual(t, duration, 40*time.Millisecond)
		assert.Less(t, duration, 80*time.Millisecond, "Execution should be shorter than sequential execution")

		log := testCtx.GetLog()
		require.Len(t, log, 4)
		assert.Equal(t, "A", log[0], "A should be the first node")
		// Sort the next two elements to handle concurrency non-determinism
		concurrentNodes := log[1:3]
		sort.Strings(concurrentNodes)
		assert.Equal(t, []string{"B", "C"}, concurrentNodes, "B and C should execute after A")
		assert.Equal(t, "D", log[3], "D should be the last node")
	})
}

func TestEngine_ErrorHandling(t *testing.T) {
	t.Run("detects cycle", func(t *testing.T) {
		engine := NewEngine[*TestContext]()

		// A -> B -> C -> A
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 1), "C"))
		require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 1), "A"))
		require.NoError(t, engine.AddNode("C", sleepAndLogNode("C", 1), "B"))

		err := engine.Build()
		require.ErrorIs(t, err, ErrCycleDetected)
	})

	t.Run("node execution fails", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		testCtx := &TestContext{}
		testErr := errors.New("something went wrong")

		// A -> B (fails)
		//   -> C (should be cancelled)
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 10*time.Millisecond)))
		require.NoError(t, engine.AddNode("B", errorNode(testErr), "A"))
		require.NoError(t, engine.AddNode("C", sleepAndLogNode("C", 50*time.Millisecond), "A"))

		require.NoError(t, engine.Build())

		err := engine.Execute(context.Background(), testCtx)
		require.ErrorIs(t, err, testErr)

		log := testCtx.GetLog()
		// A runs, B runs and fails, C starts but should be cancelled by the errgroup.
		assert.Contains(t, log, "A", "A should have run")
		assert.Contains(t, log, "C-cancelled", "C should have been cancelled")
		// B fails before logging, so it won't be in the log.
		assert.NotContains(t, log, "B", "B should not complete")
	})

	t.Run("add existing node", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 1)))
		err := engine.AddNode("A", sleepAndLogNode("A", 1))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node already exists: A")
	})

	t.Run("build fails with missing dependency", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		// B depends on A, but A is never added.
		require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 1), "A"))
		err := engine.Build()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dependency node not found: A")
	})

	t.Run("build fails with missing dependent node", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		// A is a dependency for B, but B is never added to the engine.
		engine.nodeExec["A"] = sleepAndLogNode("A", 1)
		engine.nodeStat["A"] = &nodeStat{done: make(chan struct{})}
		engine.nodeToNext["A"] = []string{"B"} // Manually create the inconsistent state

		err := engine.Build()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node 'B' (dependent on 'A') was not added")
	})

	t.Run("no root nodes", func(t *testing.T) {
		engine := NewEngine[*TestContext]()
		require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 1), "A")) // Cycle
		err := engine.Build()
		require.ErrorIs(t, err, ErrCycleDetected)
	})
}

func TestEngine_ContextCancellation(t *testing.T) {
	engine := NewEngine[*TestContext]()
	testCtx := &TestContext{}

	// A (long task) -> B
	require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 100*time.Millisecond)))
	require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 10*time.Millisecond), "A"))

	require.NoError(t, engine.Build())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := engine.Execute(ctx, testCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	log := testCtx.GetLog()
	assert.Contains(t, log, "A-cancelled", "Long-running node should be cancelled")
	assert.NotContains(t, log, "B", "Dependent node should not have run")
}

func TestEngine_Build_CorrectRootNodeDetection(t *testing.T) {
	t.Log("Verifying that Build() correctly identifies root nodes.")

	engine := NewEngine[*TestContext]()

	// A -> B, C -> D
	require.NoError(t, engine.AddNode("A", sleepAndLogNode("A", 1)))
	require.NoError(t, engine.AddNode("B", sleepAndLogNode("B", 1), "A"))
	require.NoError(t, engine.AddNode("C", sleepAndLogNode("C", 1)))
	require.NoError(t, engine.AddNode("D", sleepAndLogNode("D", 1), "C"))

	err := engine.Build()
	require.NoError(t, err)

	// Roots should be A and C
	assert.ElementsMatch(t, []string{"A", "C"}, engine.rootNode, "Should correctly identify nodes with no dependencies as roots")
}

func TestEngine_NoNodes(t *testing.T) {
	engine := NewEngine[*TestContext]()
	testCtx := &TestContext{}

	require.NoError(t, engine.Build())
	err := engine.Execute(context.Background(), testCtx)
	require.NoError(t, err)
	assert.Empty(t, testCtx.GetLog())
}

