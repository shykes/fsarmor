package fsarmor

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	// "github.com/docker/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
	"archive/tar"
)

const (
	MetaTree = "_fs_meta"
	DataTree = "_fs_data"
)

func Log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
}

// Join generates a tar stream frmo the contents of t, and streams
// it to `dst`.
func Join(dir string, dst io.Writer) error {
	tw := tar.NewWriter(dst)
	defer tw.Close()
	// Walk the data tree
	dataDir := path.Join(dir, DataTree)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	return filepath.Walk(dataDir, func(name string, info os.FileInfo, e error) error {
		Log("Walk: %s\n", name)

		virtPath := name[len(dataDir):]
		Log("    --> virtPath = %v\n", virtPath)

		var (
			err error
			hdr *tar.Header
		)
		metaFile, err := os.Open(path.Join(dir, metaPath(virtPath)))
		if os.IsNotExist(err) {
			// There is no meta file: write a default header
			hdr, err = tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = virtPath
		} else if err != nil {
			return err
		} else {
			tr := tar.NewReader(metaFile)
			hdr, err = tr.Next()
			metaFile.Close() // close this no matter what
			if err != nil {
				return err
			}
		}
		// Write the reconstituted tar header+content
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "--> writing %d bytes for entry %s\n", hdr.Size, hdr.Name)
			if _, err := io.CopyN(tw, f, hdr.Size); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
		return nil
	})
}

// Split adds data to t from a tar strema decoded from `src`.
// Raw data is stored at the key `_fs_data/', and metadata in a
// separate key '_fs_metadata'.
func Split(src io.Reader, dir string) error {
	tr := tar.NewReader(src)
	if err := os.MkdirAll(path.Join(dir, DataTree), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(dir, MetaTree), 0700); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		Log("NEW TAR HEADER: %s\n", hdr.Name)
		Log("     -> metaPath(%v) = %v\n", hdr.Name, metaPath(hdr.Name))
		metaRealPath := path.Join(dir, metaPath(hdr.Name))
		{
			parentDir, _ := path.Split(metaRealPath)
			os.MkdirAll(parentDir, 0700)
		}
		Log("     -> storing tar header at %s\n", metaRealPath)
		metaFile, err := os.OpenFile(metaRealPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return err
		}
		err = writeHeaderTo(hdr, metaFile)
		metaFile.Close()
		if err != nil {
			return err
		}

		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			// FIXME: protect against unsafe tar headers, ../../ etc.
			dataFile, err := os.OpenFile(path.Join(dir, DataTree, hdr.Name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
			if err != nil {
				return err
			}
			_, err = io.Copy(dataFile, tr)
			dataFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}


func writeHeaderTo(hdr *tar.Header, f *os.File) error {
	w := tar.NewWriter(f)
	err := w.WriteHeader(hdr)
	w.Close()
	return err
}

// metaPath computes a path at which the metadata can be stored for a given path.
// For example if `name` is "/etc/resolv.conf", the corresponding metapath is
// "_fs_meta/194c1cbe5a8cfcb85c6a46b936da12ffdc32f90f"
// This path will be used to store and retrieve the tar header encoding the metadata
// for the corresponding file.
func metaPath(name string) string {
	return path.Join(MetaTree, MkAnnotation(name))
}
