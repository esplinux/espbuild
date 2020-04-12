package main

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/ulikunitz/xz"
	"go.starlark.net/starlark"
	"golang.org/x/sys/unix"
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

	println("Downloading: " + url)

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	var netClient = &http.Client{
		Timeout:   time.Second * 60,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	if err != nil {
		return starlark.None, err
	}
	defer closer(resp.Body)

	var reader io.Reader
	if strings.HasSuffix(outputFile, "gz") {
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

	// Derived from example by Steve Domino and extended by reading golang std library source
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

			if fi, err := os.Lstat(target); !(err == nil && fi.IsDir()) {
				if err = os.MkdirAll(target, 0755); err != nil {
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
			if _, err = io.Copy(f, tr); err != nil {
				return starlark.None, err
			}

			// manually close here after each file operation; deferring would cause each file close
			// to wait until all operations have completed.
			if err = f.Close(); err != nil {
				return starlark.None, err
			}
			
		case tar.TypeLink:
			if err := os.Link(header.Linkname, target); err != nil {
				return starlark.None, err
			}

		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil {
				return starlark.None, err
			}
			
		case tar.TypeChar:
			mode := uint32(header.Mode & 07777)
			mode |= unix.S_IFCHR
			device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
			if err := unix.Mknod(target, mode, device); err != nil {
				return starlark.None, err
			}

		case tar.TypeBlock:
			mode := uint32(header.Mode & 07777)
			mode |= unix.S_IFBLK
			device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
			if err := unix.Mknod(target, mode, device); err != nil {
				return starlark.None, err
			}

		case tar.TypeFifo:
			mode := uint32(header.Mode & 07777)
			mode |= unix.S_IFIFO
			device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
			if err := unix.Mknod(target, mode, device); err != nil {
				return starlark.None, err
			}

		case tar.TypeXGlobalHeader:
			warn(url + " contains a PAX Global Header which is unsupported, ignoring\n")

		case tar.TypeGNUSparse:
			return starlark.None, fmt.Errorf("tar entry %s of TypeGNUSparse is not suported", target)

		default:
			return starlark.None, fmt.Errorf("tar entry %s of %v is not supported", target, header.Typeflag)
		}
	}
}

func getGit(url string, branch string, outputDir string) (starlark.Value, error) {
	urlSplit := strings.Split(url, "/")
	outputDir = outputDir + "/" + urlSplit[len(urlSplit)-1]
	if branch == "" {
		println("Cloning: " + url)
		outputDir = outputDir + "-HEAD"
	} else {
		println("Cloning[" + branch + "]: " + url)
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
