package executor

import "sync/atomic"

type nodeStat struct {
	err    atomic.Value
	cost   int32 // cost in milliseconds
	degree int32
	done   chan struct{}
}

func (n *nodeStat) SetErr(err error) {
	if err != nil {
		n.err.Store(err)
	}
}

func (n *nodeStat) GetErr() error {
	if v := n.err.Load(); v != nil {
		return v.(error)
	}
	return nil
}

func (n *nodeStat) Done() {
	close(n.done)
}
