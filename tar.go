/**
 * Tar helpers
 * Thanks Steve Domino
 * https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
 *
 */
package main

import (
	"github.com/dsnet/compress/bzip2"
	"github.com/ulikunitz/xz"
)

import "compress/gzip"
import "archive/tar"
import "bufio"
import "fmt"
import "io"
import "log"
import "os"
import "path"
import "path/filepath"
import "strings"

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
func untarReader(destination string, r io.Reader) error {


	/**
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	//defer gzr.Close()
	defer func() {
		if err := gzr.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	tr := tar.NewReader(gzr)
	**/

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(destination, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
				err = os.Chtimes(target, header.AccessTime, header.ModTime)
				if err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			err = f.Close()
			if err != nil {
				return err
			}

			err = os.Chtimes(target, header.AccessTime, header.ModTime)
			if err != nil {
				return err
			}
		}
	}
}

func extractTar(src string, file string) error {

	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}


	var r io.Reader
	fileExtension := path.Ext(file)

	fmt.Printf("fileExtension: %s\n", fileExtension)

	if fileExtension == ".gz" {
		fmt.Printf("Creating gunzip reader\n")
		r, err := gzip.NewReader(f)
		if err != nil {
			panic(err)
		}
		defer r.Close()

		/**
		tmpfile, err := ioutil.TempFile("", "temporary.*.tar")
		if err != nil {
			log.Fatal(err)
		}
		defer os.Remove(tmpfile.Name()) // clean up

		_, err = io.Copy(tmpfile, r)
		if err != nil {
			log.Fatal(err)
		}**/

		err = untarReader(src, r)
		if err != nil {
			panic(err)
		}

	} else if fileExtension == ".bz2" {
		fmt.Printf("Creating bunzip2 reader\n")
		r, err = bzip2.NewReader(f, &bzip2.ReaderConfig{})
		if err != nil {
			panic(err)
		}

		err = untarReader(src, r)
		if err != nil {
			panic(err)
		}
	} else if fileExtension == ".xz" {
		fmt.Printf("Creating xz reader\n")
		r, err =  xz.NewReader(f)
		if err != nil {
			panic(err)
		}

		err = untarReader(src, r)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Printf("Creating file reader\n")
		r = bufio.NewReader(f)

		err = untarReader(src, r)
		if err != nil {
			panic(err)
		}
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}

func tarWriter(source string, writers ...io.Writer) error {

	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("unable to tar files - %v", err.Error())
	}

	mw := io.MultiWriter(writers...)

	bzw, err := bzip2.NewWriter(mw, &bzip2.WriterConfig{Level: 9})
	if err != nil {
		return err
	}

	defer func() {
		if err := bzw.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	tw := tar.NewWriter(bzw)
	defer func() {
		if err := tw.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	// walk path
	return filepath.Walk(source, func(file string, fi os.FileInfo, err error) error {

		// Skip root of filepath walk
		if file == source {
			return nil
		}

		// return on any error
		if err != nil {
			return err
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, source, "", -1), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
		if !fi.Mode().IsRegular() {
			return nil
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		// manually close here after each file operation; defering would cause each file close
		// to wait until all operations have completed.
		err = f.Close()
		if err != nil {
			return err
		}

		return nil
	})
}

func createTar(src string, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)

	err = tarWriter(src, w)
	if err != nil {
		return err
	}

	err = w.Flush()
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}
