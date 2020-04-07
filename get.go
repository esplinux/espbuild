package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// ExampleThread_Load_parallel demonstrates a parallel implementation
// of 'load' with caching, duplicate suppression, and cycle detection.
func ExampleThread_Load_parallel() {
	cache := &cache{
		cache: make(map[string]*entry),
		fakeFilesystem: map[string]string{
			"c.star": `load("a.star", "a"); c = a * 2`,
			"b.star": `load("a.star", "a"); b = a * 3`,
			"a.star": `a = 1; print("loaded a"); print(repeat("mur", 2))`,
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
