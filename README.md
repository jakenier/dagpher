# Dagpher

Dagpher is a powerful and flexible Golang library for building and executing Directed Acyclic Graphs (DAGs) with a focus on type safety, modularity, and middleware support. It allows you to define complex workflows with clear dependencies, nested groupings, and reusable execution logic.

## Key Features

- **Type-Safe Execution:** Leverages Go generics to ensure type safety for the context shared across nodes.
- **Dependency Management:** Automatically resolves the execution order of nodes based on their declared dependencies.
- **Middleware Support:** Apply middleware at both the global and node-specific levels to handle cross-cutting concerns like logging, metrics, or error handling.
- **Nested Groups:** Organize nodes into logical, nested groups to create modular and reusable workflows.
- **Cycle Detection:** Automatically detects dependency cycles to prevent infinite loops.
- **Concurrent Execution:** Executes nodes with no dependencies on each other concurrently for maximum efficiency.
- **Context Cancellation:** Gracefully handles context cancellation to stop ongoing tasks.

## Installation

```bash
go get github.com/jakenier/dagpher
```

## Usage

Here's a simple example demonstrating how to create a DAG with nodes, groups, and middleware.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jakenier/dagpher"
)

// 1. Define a shared context for nodes
type MyContext struct {
	LogMessages []string
}

func (c *MyContext) Log(msg string) {
	c.LogMessages = append(c.LogMessages, msg)
	log.Println(msg)
}

// 2. Define a middleware
func LoggingMiddleware(next dagpher.Endpoint) dagpher.Endpoint {
	return func(ctx context.Context, req any) (any, error) {
		c := req.(*MyContext)
		nodeName := "unknown" // In a real app, you'd get the node name via context
		c.Log(fmt.Sprintf("starting node %s", nodeName))
		
		res, err := next(ctx, req)
		
		c.Log(fmt.Sprintf("finished node %s", nodeName))
		return res, err
	}
}

func main() {
	// 3. Create nodes
	nodeA := dagpher.NewQuickNode("A", func(ctx context.Context, c *MyContext) error {
		c.Log("executing A")
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	nodeB := dagpher.NewQuickNode("B", func(ctx context.Context, c *MyContext) error {
		c.Log("executing B")
		time.Sleep(20 * time.Millisecond)
		return nil
	}, "A") // B depends on A

	nodeC := dagpher.NewQuickNode("C", func(ctx context.Context, c *MyContext) error {
		c.Log("executing C")
		return nil
	}, "A") // C depends on A

	nodeD := dagpher.NewQuickNode("D", func(ctx context.Context, c *MyContext) error {
		c.Log("executing D")
		return nil
	}, "B", "C") // D depends on B and C

	// 4. Organize nodes into groups
	rootGroup := dagpher.NewGroup[*MyContext]()
	subGroup := dagpher.NewGroup[*MyContext]()

	rootGroup.AddNode(nodeA)
	subGroup.AddNode(nodeB)
	subGroup.AddNode(nodeC)
	subGroup.AddNode(nodeD, dagpher.WithMiddlewares(LoggingMiddleware)) // Apply middleware to a specific node

	rootGroup.AddSubGroup(subGroup)

	// 5. Create and run the executor
	execCtx := &MyContext{}
	executor := dagpher.NewGroupExecutor(execCtx, rootGroup)

	if err := executor.Build(); err != nil {
		log.Fatalf("Failed to build DAG: %v", err)
	}

	if err := executor.Execute(context.Background()); err != nil {
		log.Fatalf("Failed to execute DAG: %v", err)
	}

	fmt.Println("\nExecution Log:")
	for _, msg := range execCtx.LogMessages {
		fmt.Println(msg)
	}
}
```

## API Overview

### `Node[C any]`

The fundamental unit of execution. It has a name, a list of dependencies, and an `Exec` function.

### `Group[C any]`

A container for nodes and other groups. It helps organize your DAG into logical units.

### `Middleware`

A function that wraps an `Endpoint` to add pre- or post-processing logic to a node's execution.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
