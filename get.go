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
	"strings"
	"time"
)

func handle(err error) {
	if err != nil {
		log.Fatal(err)
	}
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

func main() {
	//getHttp(os.Args[1], os.Args[2])

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
