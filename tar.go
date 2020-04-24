package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"go.starlark.net/starlark"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Tar up a set of files and return the name of the tarfile
func Tar(name string, baseDir string, files *starlark.List) (starlark.Value, error) {
	f, err := os.Create(name)
	if err != nil {
		return starlark.String(name), err
	}

	gZipWriter := gzip.NewWriter(f)
	tarWriter := tar.NewWriter(gZipWriter)

	iter := files.Iterate()
	defer iter.Done()

	var k starlark.Value
	for iter.Next(&k) {
		file, _ := starlark.AsString(k)

		// Skip baseDir
		if file == baseDir {
			continue
		}

		// ensure the file actually exists before trying to tar it
		fileInfo, err := os.Lstat(file)
		if err != nil {
			return starlark.String(name), fmt.Errorf("unable to tar files - %v", err.Error())
		}

		name := fileInfo.Name()
		if fileInfo.Mode()&os.ModeSymlink != 0 { // check if Symlink bit set
			name, err = os.Readlink(file) // Set name to link
			if err != nil {
				return starlark.String(name), err
			}
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fileInfo, name)
		if err != nil {
			return starlark.String(name), err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, baseDir, "", -1), string(filepath.Separator))

		// write the header
		if err := tarWriter.WriteHeader(header); err != nil {
			return starlark.String(name), err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fileInfo.Mode().IsRegular() {
			continue
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return starlark.String(name), err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tarWriter, f); err != nil {
			return starlark.String(name), err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		if err := f.Close(); err != nil {
			return starlark.String(name), err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, err
	}

	if err := gZipWriter.Close(); err != nil {
		return nil, err
	}

	return starlark.String(name), nil
}

func dir(target string, source string) string {
	// todo: eric@ this is evil and likely to eventually break
	// Assumption: The first directory present in the tarball is the source directory
	// this is an imperfect assumption but should almost always be correct.

	if fi, err := os.Lstat(target); !(err == nil && fi.IsDir()) {
		if err = os.MkdirAll(target, 0755); err != nil {
			fatal(err)
		}
	}

	if source == "" {
		return target
	}

	return source
}

func file(header *tar.Header, reader io.Reader, target string) {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
	if err != nil {
		fatal(err)
	}

	// copy over contents
	if _, err = io.Copy(f, reader); err != nil {
		fatal(err)
	}

	// manually close here after each file operation; deferring would cause each file close
	// to wait until all operations have completed.
	if err = f.Close(); err != nil {
		fatal(err)
	}
}

func link(header *tar.Header, target string) {
	if err := os.Link(header.Linkname, target); err != nil {
		fatal(err)
	}
}

func symlink(header *tar.Header, target string) {
	if err := os.Symlink(header.Linkname, target); err != nil {
		fatal(err)
	}
}

func char(header *tar.Header, target string) {
	mode := uint32(header.Mode & 07777)
	mode |= unix.S_IFCHR
	device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
	if err := unix.Mknod(target, mode, device); err != nil {
		fatal(err)
	}
}

func block(header *tar.Header, target string) {
	mode := uint32(header.Mode & 07777)
	mode |= unix.S_IFBLK
	device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
	if err := unix.Mknod(target, mode, device); err != nil {
		fatal(err)
	}
}

func fifo(header *tar.Header, target string) {
	mode := uint32(header.Mode & 07777)
	mode |= unix.S_IFIFO
	device := int(unix.Mkdev(uint32(header.Devmajor), uint32(header.Devminor)))
	if err := unix.Mknod(target, mode, device); err != nil {
		fatal(err)
	}
}

func processTarEntry(header *tar.Header, reader io.Reader, target string, source string) string {
	// the following switch could also be done using fi.Mode(), not sure if there a benefit of using one vs. the other.
	// fi := header.FileInfo()

	switch header.Typeflag {

	case tar.TypeDir:
		source = dir(target, source)

	case tar.TypeReg:
		file(header, reader, target)

	case tar.TypeLink:
		link(header, target)

	case tar.TypeSymlink:
		symlink(header, target)

	case tar.TypeChar:
		char(header, target)

	case tar.TypeBlock:
		block(header, target)

	case tar.TypeFifo:
		fifo(header, target)

	case tar.TypeXGlobalHeader:
		warn("ignoring unsupported PAX global header")

	default:
		fatal(fmt.Errorf("tar entry %s of %v is not supported", target, header.Typeflag))
	}

	return source
}

// UnTar a set of files and return the name of the first directory created
func UnTar(reader io.Reader, outputDir string) (starlark.Value, error) {
	source := ""

	// Derived from example by Steve Domino and extended by reading golang std library source
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
		source = processTarEntry(header, tr, target, source)
	}
}
