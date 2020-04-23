package main

import (
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

// Files have a timeout of 30 seconds
func getHttpFile(url string, outputDir string, file string) (starlark.Value, error) {
	target := filepath.Join(outputDir, file)

	println("\u001b[37;1mDownloading: " + url + " to " + target + "\u001b[0m")

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	var netClient = &http.Client{
		Timeout:   time.Second * 30,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	if err != nil {
		return starlark.None, err
	}

	outputDir = filepath.Dir(target)
	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return starlark.None, err
	}

	out, err := os.Create(target)
	if err != nil {
		return starlark.None, err
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		return starlark.None, err
	}

	if err := out.Close(); err != nil {
		return nil, err
	}

	if err := resp.Body.Close(); err != nil {
		return nil, err
	}

	return starlark.String(target), nil
}

// Sources have a timeout of 300 seconds aka 5 minutes
func getHttpSource(url string, outputDir string) (starlark.Value, error) {
	urlSplit := strings.Split(url, "/")
	outputFile := urlSplit[len(urlSplit)-1]

	println("\u001b[37;1mDownloading: " + url + "\u001b[0m")

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	// Disable automagic decompression for sources
	netTransport.DisableCompression = true

	var netClient = &http.Client{
		Timeout:   time.Second * 300,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	if err != nil {
		return starlark.None, err
	}

	defer func() {
		err := resp.Body.Close()
		if err == nil {
			fatal(err)
		}
	}()

	var reader io.Reader
	if strings.HasSuffix(outputFile, "gz") {
		gzipStream, err := gzip.NewReader(resp.Body)
		if err != nil {
			return starlark.None, err
		}

		defer func() {
			err := gzipStream.Close()
			if err == nil {
				fatal(err)
			}
		}()

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

	return UnTar(reader, outputDir)
}

func getGit(url string, branch string, outputDir string) (starlark.Value, error) {
	urlSplit := strings.Split(url, "/")
	outputDir = outputDir + "/" + urlSplit[len(urlSplit)-1]
	if branch == "" {
		println("\u001b[37;1mCloning: " + url + "\u001b[0m")
		outputDir = outputDir + "-HEAD"
	} else {
		println("\u001b[37;1mCloning[" + branch + "]: " + url + "\u001b[0m")
		outputDir = outputDir + "-" + branch
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		cloneOptions := &git.CloneOptions{
			URL: url,
		}

		if branch != "" {
			cloneOptions.ReferenceName = plumbing.ReferenceName(branch)
		}

		if _, err := git.PlainClone(outputDir, false, cloneOptions); err != nil {
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

		if err := workTree.Pull(pullOptions); err != git.NoErrAlreadyUpToDate {
			return starlark.None, err
		}
	}

	return starlark.String(outputDir), nil
}
