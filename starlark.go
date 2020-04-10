package main

import (
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"log"
	"path/filepath"
	"runtime"
	"strconv"
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

func (c *cache) Load(module string) (starlark.StringDict, error) {
	return c.get(new(cycleChecker), module)
}

// get loads and returns an entry (if not already loaded).
func (c *cache) get(cc *cycleChecker, module string) (starlark.StringDict, error) {
	c.cacheMu.Lock()
	e := c.cache[module]
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
		c.cache[module] = e
		c.cacheMu.Unlock()

		e.setOwner(cc)
		e.globals, e.err = c.doLoad(cc, module)
		e.setOwner(nil)

		// Broadcast that the entry is now ready.
		close(e.ready)
	}
	return e.globals, e.err
}

func (c *cache) doLoad(cc *cycleChecker, module string) (starlark.StringDict, error) {
	BUILDFILE, err := filepath.Abs(module)
	fatal(err)
	CURDIR := filepath.Dir(BUILDFILE)

	packageBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var name string
		var version string
		var rev string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "version", &version, "rev", &rev); err != nil {
			return nil, err
		}

		stringDictionary := starlark.StringDict{
			"name":    starlark.String(name),
			"version": starlark.String(version),
			"rev":     starlark.String(rev),
		}

		return starlarkstruct.FromStringDict(starlark.String("struct"), stringDictionary), nil
	}

	pathBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var path string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		return starlark.String(CURDIR + "/" + path), nil
	}

	shellBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		env := starlark.NewDict(3)
		var command string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command, "env?", &env); err != nil {
			return nil, err
		}
		println(module + ": " + command)
		shell(command, env)
		return starlark.None, nil
	}

	sourceBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		http := ""
		git := ""
		branch := ""
		source := ""

		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "http?", &http, "git?", &git, "branch?", &branch); err != nil {
			return nil, err
		}

		if http != "" {
			source = getHttpSource(http, CURDIR)
		} else if git != "" {
			source = getGit(git, branch, CURDIR)
		} else {
			log.Fatal("Error: Source only supports git and http")
		}

		if source == "" {
			log.Fatal("Error: Source directory has not been set, aborting")
		}

		return starlark.String(source), nil
	}

	// This dictionary defines the pre-declared environment.
	predeclared := starlark.StringDict{
		"ESP_BUILD_VERSION": starlark.String(ESP_BUILD_VERSION),
		"NPROC":             starlark.String(strconv.Itoa(runtime.NumCPU())),
		"package":           starlark.NewBuiltin("package", packageBuiltIn),
		"path":              starlark.NewBuiltin("path", pathBuiltIn),
		"shell":             starlark.NewBuiltin("shell", shellBuiltIn),
		"source":            starlark.NewBuiltin("source", sourceBuiltIn),
	}

	thread := &starlark.Thread{
		Name: "exec " + module,
		Load: func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			// Tunnel the cycle-checker state for this "thread of loading".
			return c.get(cc, module)
		},
	}
	return starlark.ExecFile(thread, module, nil, predeclared)
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
