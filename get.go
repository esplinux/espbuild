package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/ulikunitz/xz"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func closer(closer io.Closer) {
	err := closer.Close()
	fatal(err)
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
	fatal(err)
	defer closer(resp.Body)

	var reader io.Reader
	if strings.HasSuffix(outputFile, ".gz") {
		gzipStream, err := gzip.NewReader(resp.Body)
		fatal(err)
		defer closer(gzipStream)
		reader = gzipStream
	} else if strings.HasSuffix(outputFile, "bz") {
		reader = bzip2.NewReader(resp.Body)
	} else if strings.HasSuffix(outputFile, "xz") {
		reader, err = xz.NewReader(resp.Body)
		fatal(err)
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
			fatal(err)

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
				fatal(err)
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			fatal(err)

			// copy over contents
			_, err = io.Copy(f, tr)
			fatal(err)

			// manually close here after each file operation; deferring would cause each file close
			// to wait until all operations have completed.
			err = f.Close()
			fatal(err)
		}
	}
}

func getGit(url string, outputDir string) {
	urlSplit := strings.Split(url, "/")
	outputDir = outputDir + "/" + urlSplit[len(urlSplit)-1]

	_, err := git.PlainClone(outputDir, false, &git.CloneOptions{
		URL: url,
	})
	fatal(err)
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

func main() {
	test := getString()
	println("Hello " + test)

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
