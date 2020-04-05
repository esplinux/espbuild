package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/ulikunitz/xz"
	"go.starlark.net/starlark"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// cache is a concurrency-safe, duplicate-suppressing,
// non-blocking cache of the doLoad function.
// See Section 9.7 of gopl.io for an explanation of this structure.
// It also features online deadlock (load cycle) detection.
type cache struct {
	cacheMu sync.Mutex
	cache   map[string]*entry

	fakeFilesystem map[string]string
}

type entry struct {
	owner   unsafe.Pointer // a *cycleChecker; see cycleCheck
	globals starlark.StringDict
	err     error
	ready   chan struct{}
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
	thread := &starlark.Thread{
		Name:  "exec " + module,
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
		Load: func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			// Tunnel the cycle-checker state for this "thread of loading".
			return c.get(cc, module)
		},
	}
	data := c.fakeFilesystem[module]
	return starlark.ExecFile(thread, module, data, nil)
}

func handle(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// -- concurrent cycle checking --

// A cycleChecker is used for concurrent deadlock detection.
// Each top-level call to Load creates its own cycleChecker,
// which is passed to all recursive calls it makes.
// It corresponds to a logical thread in the deadlock detection literature.
type cycleChecker struct {
	waitsFor unsafe.Pointer // an *entry; see cycleCheck
}

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

func closer(closer io.Closer) {
	err := closer.Close()
	handle(err)
}

func getHttpSource(url string, outputDir string) {
	urlSplit := strings.Split(url, "/")
	outputFile := urlSplit[len(urlSplit)-1]

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	var netClient = &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	handle(err)
	defer closer(resp.Body)

	var reader io.Reader
	if strings.HasSuffix(outputFile, ".gz") {
		gzipStream, err := gzip.NewReader(resp.Body)
		handle(err)
		defer closer(gzipStream)
		reader = gzipStream
	} else if strings.HasSuffix(outputFile, "bz") {
		reader = bzip2.NewReader(resp.Body)
	} else if strings.HasSuffix(outputFile, "xz") {
		reader, err = xz.NewReader(resp.Body)
		handle(err)
	} else {
		reader = resp.Body
	}

	// Derived from example by Steve Domino
	// gist.githubusercontent.com/sdomino/635a5ed4f32c93aad131/raw/1f1a2609f9bf04f3a681a96c26350b0d694549bf/untargz.go
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return

		// return any other error
		case err != nil:
			handle(err)

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(outputDir, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				err = os.MkdirAll(target, 0755)
				handle(err)
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			handle(err)

			// copy over contents
			_, err = io.Copy(f, tr)
			handle(err)

			// manually close here after each file operation; deferring would cause each file close
			// to wait until all operations have completed.
			err = f.Close()
			handle(err)
		}
	}
}

func getGit(url string, outputDir string) {
	urlSplit := strings.Split(url, "/")
	outputDir = outputDir + "/" + urlSplit[len(urlSplit)-1]

	_, err := git.PlainClone(outputDir, false, &git.CloneOptions{
		URL: url,
	})
	handle(err)
}

// ExampleThread_Load_parallel demonstrates a parallel implementation
// of 'load' with caching, duplicate suppression, and cycle detection.
func ExampleThread_Load_parallel() {
	cache := &cache{
		cache: make(map[string]*entry),
		fakeFilesystem: map[string]string{
			"c.star": `load("a.star", "a"); c = a * 2`,
			"b.star": `load("a.star", "a"); b = a * 3`,
			"a.star": `a = 1; print("loaded a")`,
		},
	}

	// We load modules b and c in parallel by concurrent calls to
	// cache.Load.  Both of them load module a, but a is executed
	// only once, as witnessed by the sole output of its print
	// statement.

	ch := make(chan string)
	for _, name := range []string{"b", "c"} {
		go func(name string) {
			globals, err := cache.Load(name + ".star")
			if err != nil {
				log.Fatal(err)
			}
			ch <- fmt.Sprintf("%s = %s", name, globals[name])
		}(name)
	}
	got := []string{<-ch, <-ch}
	sort.Strings(got)
	fmt.Println(strings.Join(got, "\n"))

	// Output:
	// loaded a
	// b = 3
	// c = 2
}

// TestThread_Load_parallelCycle demonstrates detection
// of cycles during parallel loading.
func TestThreadLoad_ParallelCycle(t *testing.T) {
	cache := &cache{
		cache: make(map[string]*entry),
		fakeFilesystem: map[string]string{
			"c.star": `load("b.star", "b"); c = b * 2`,
			"b.star": `load("a.star", "a"); b = a * 3`,
			"a.star": `load("c.star", "c"); a = c * 5; print("loaded a")`,
		},
	}

	ch := make(chan string)
	for _, name := range "bc" {
		name := string(name)
		go func() {
			_, err := cache.Load(name + ".star")
			if err == nil {
				log.Fatalf("Load of %s.star succeeded unexpectedly", name)
			}
			ch <- err.Error()
		}()
	}
	got := []string{<-ch, <-ch}
	sort.Strings(got)

	// Typically, the c goroutine quickly blocks behind b;
	// b loads a, and a then fails to load c because it forms a cycle.
	// The errors observed by the two goroutines are:
	want1 := []string{
		"cannot load a.star: cannot load c.star: cycle in load graph",                     // from b
		"cannot load b.star: cannot load a.star: cannot load c.star: cycle in load graph", // from c
	}
	// But if the c goroutine is slow to start, b loads a,
	// and a loads c; then c fails to load b because it forms a cycle.
	// The errors this time are:
	want2 := []string{
		"cannot load a.star: cannot load c.star: cannot load b.star: cycle in load graph", // from b
		"cannot load b.star: cycle in load graph",                                         // from c
	}
	if !reflect.DeepEqual(got, want1) && !reflect.DeepEqual(got, want2) {
		t.Error(got)
	}
}

func main() {
	ExampleThread_Load_parallel()

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tget https://example.com/filename.tar.gz")
		fmt.Println("\tget http https://example.com/filename.tar.gz")
		fmt.Println("\tget git https://exmample.com/git/url")
	} else {
		switch os.Args[1] {
		case "git":
			getGit(os.Args[2], ".")
		default:
			if os.Args[1] == "http" {
				getHttpSource(os.Args[2], ".")
			} else {
				getHttpSource(os.Args[1], ".")
			}
		}
	}
}
