package main

import (
	"fmt"
	"go.starlark.net/starlark"
	"sync"
	"sync/atomic"
	"unsafe"
)

// cache is a concurrency-safe, duplicate-suppressing,
// non-blocking cache of the doLoad function.
// See Section 9.7 of gopl.io for an explanation of this structure.
// It also features online deadlock (load cycle) detection.
type cache struct {
	cacheMu sync.Mutex
	cache   map[string]*entry

	predeclared starlark.StringDict
}

type entry struct {
	owner   unsafe.Pointer // a *cycleChecker; see cycleCheck
	globals starlark.StringDict
	err     error
	ready   chan struct{}
}

// A cycleChecker is used for concurrent deadlock detection.
// Each top-level call to Load creates its own cycleChecker,
// which is passed to all recursive calls it makes.
// It corresponds to a logical thread in the deadlock detection literature.
type cycleChecker struct {
	waitsFor unsafe.Pointer // an *entry; see cycleCheck
}

func (c *cache) Load(buildfile string) (starlark.StringDict, error) {
	return c.get(new(cycleChecker), buildfile)
}

// get loads and returns an entry (if not already loaded).
func (c *cache) get(cc *cycleChecker, buildfile string) (starlark.StringDict, error) {
	c.cacheMu.Lock()
	e := c.cache[buildfile]
	if e != nil {
		c.cacheMu.Unlock()
		// Some other goroutine is getting this module.
		// Wait for it to become ready.

		// Detect load cycles to avoid deadlocks.
		if err := cycleCheck(e, cc); err != nil {
			return nil, err
		}

		cc.setWaitsFor(e)
		<-e.ready
		cc.setWaitsFor(nil)
	} else {
		// First request for this module.
		e = &entry{ready: make(chan struct{})}
		c.cache[buildfile] = e
		c.cacheMu.Unlock()

		e.setOwner(cc)
		e.globals, e.err = c.doLoad(cc, buildfile)
		e.setOwner(nil)

		// Broadcast that the entry is now ready.
		close(e.ready)
	}
	return e.globals, e.err
}

func (c *cache) doLoad(cc *cycleChecker, buildfile string) (starlark.StringDict, error) {
	thread := &starlark.Thread{
		Name: buildfile,
		Load: func(_ *starlark.Thread, buildfile string) (starlark.StringDict, error) {
			// Tunnel the cycle-checker state for this "thread of loading".
			return c.get(cc, buildfile)
		},
	}

	return starlark.ExecFile(thread, buildfile, nil, c.predeclared)
}

// -- concurrent cycle checking --
func (cc *cycleChecker) setWaitsFor(e *entry) {
	atomic.StorePointer(&cc.waitsFor, unsafe.Pointer(e))
}

func (e *entry) setOwner(cc *cycleChecker) {
	atomic.StorePointer(&e.owner, unsafe.Pointer(cc))
}

// cycleCheck reports whether there is a path in the waits-for graph
// from resource 'e' to thread 'me'.
//
// The waits-for graph (WFG) is a bipartite graph whose nodes are
// alternately of type entry and cycleChecker.  Each node has at most
// one outgoing edge.  An entry has an "owner" edge to a cycleChecker
// while it is being readied by that cycleChecker, and a cycleChecker
// has a "waits-for" edge to an entry while it is waiting for that entry
// to become ready.
//
// Before adding a waits-for edge, the cache checks whether the new edge
// would form a cycle.  If so, this indicates that the load graph is
// cyclic and that the following wait operation would deadlock.
func cycleCheck(e *entry, me *cycleChecker) error {
	for e != nil {
		cc := (*cycleChecker)(atomic.LoadPointer(&e.owner))
		if cc == nil {
			break
		}
		if cc == me {
			return fmt.Errorf("cycle in load graph")
		}
		e = (*entry)(atomic.LoadPointer(&cc.waitsFor))
	}
	return nil
}
