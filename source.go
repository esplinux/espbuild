package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/ulikunitz/xz"
	"go.starlark.net/starlark"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func getHttpSource(url string, outputDir string) (starlark.Value, error) {
	source := ""

	urlSplit := strings.Split(url, "/")
	outputFile := urlSplit[len(urlSplit)-1]

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	var netClient = &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	if err != nil {
		return starlark.None, err
	}
	defer closer(resp.Body)

	var reader io.Reader
	if strings.HasSuffix(outputFile, ".gz") {
		gzipStream, err := gzip.NewReader(resp.Body)
		if err != nil {
			return starlark.None, err
		}
		defer closer(gzipStream)
		reader = gzipStream
	} else if strings.HasSuffix(outputFile, "bz") {
		reader = bzip2.NewReader(resp.Body)
	} else if strings.HasSuffix(outputFile, "xz") {
		reader, err = xz.NewReader(resp.Body)
		if err != nil {
			return starlark.None, err
		}
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
			return starlark.String(source), nil

		// return any other error
		case err != nil:
			return starlark.None, err

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
			// todo: eric@ this is evil and likely to eventually break
			// Assumption: The first directory present in the tarball is the source directory
			// this is an imperfect assumption but should almost always be correct.
			if source == "" {
				source = target
			}

			if _, err := os.Stat(target); err != nil {
				err = os.MkdirAll(target, 0755)
				if err != nil {
					return starlark.None, err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return starlark.None, err
			}

			// copy over contents
			_, err = io.Copy(f, tr)
			if err != nil {
				return starlark.None, err
			}

			// manually close here after each file operation; deferring would cause each file close
			// to wait until all operations have completed.
			err = f.Close()
			if err != nil {
				return starlark.None, err
			}
		}
	}
}

func getGit(url string, branch string, outputDir string) (starlark.Value, error) {
	urlSplit := strings.Split(url, "/")
	outputDir = outputDir + "/" + urlSplit[len(urlSplit)-1]
	if branch == "" {
		outputDir = outputDir + "-HEAD"
	} else {
		outputDir = outputDir + "-" + branch
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		cloneOptions := &git.CloneOptions{
			URL: url,
		}

		if branch != "" {
			cloneOptions.ReferenceName = plumbing.ReferenceName(branch)
		}

		_, err := git.PlainClone(outputDir, false, cloneOptions)
		if err != nil {
			return starlark.None, err
		}
	} else {
		repo, err := git.PlainOpen(outputDir)
		if err != nil {
			return starlark.None, err
		}

		workTree, err := repo.Worktree()
		if err != nil {
			return starlark.None, err
		}

		pullOptions := &git.PullOptions{RemoteName: "origin"}

		if branch != "" {
			pullOptions.ReferenceName = plumbing.ReferenceName(branch)
		}

		err = workTree.Pull(pullOptions)
		if err != git.NoErrAlreadyUpToDate {
			return starlark.None, err
		}
	}

	return starlark.String(outputDir), nil
}
