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
		metaRealPath := path.Join(dir, metaPath(virtPath))
		metaFile, err := os.Open(metaRealPath)
		if os.IsNotExist(err) {
			// If we want to create a default header when no tar header is
			// present, we should do it here.
			// At the moment we consider it an error.
			return fmt.Errorf("missing tar header for %s: %s: no such file or directory", virtPath, metaRealPath)
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

		// ensure tree exists
		fPath := filepath.Join(dir, DataTree, hdr.Name)
		baseDir := filepath.Dir(fPath)
		if err := os.MkdirAll(baseDir, 0700); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			// FIXME: protect against unsafe tar headers, ../../ etc.
			dataFile, err := os.OpenFile(fPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
			if err != nil {
				return err
			}
			_, err = io.Copy(dataFile, tr)
			dataFile.Close()
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			fmt.Printf("[SYMLINK] %s %d bytes\n", hdr.Name, hdr.Size)
			link, err := os.Readlink(hdr.Name)
			if err != nil {
				return err
			}

			Log("     -> link %s\n", link)

			// link target
			linkPath := path.Join(".", link)

			// check if odd symlinks (i.e. mkdir -> /bin/busybox inside of /bin)
			linkBaseParent := path.Dir(path.Dir(path.Join(baseDir, link)))

			Log("     -> file path %s\n", path.Dir(hdr.Name))
			Log("     -> link parent %s\n", linkBaseParent)
			Log("     -> base parent %s\n", path.Dir(fPath))

			// if file is not in root and it's a symlink to a full path
			if path.Dir(hdr.Name) != "." && linkBaseParent == path.Dir(fPath) {
				linkPath = path.Join(".", path.Base(link))
				Log("     -> updating link %s\n", linkPath)
			}

			linkTarget := path.Base(hdr.Name)
			Log("     -> create link %s -> %s\n", linkPath, linkTarget)

			// get current dir for breadcrumb
			workDir, err := os.Getwd()
			if err != nil {
				return err
			}

			// chdir to get proper symlink
			if err := os.Chdir(baseDir); err != nil {
				return err
			}

			linkDir := filepath.Dir(linkPath)
			if err := os.MkdirAll(linkDir, 0700); err != nil {
				return err
			}

			if err := os.Symlink(linkPath, linkTarget); err != nil {
				return err
			}

			// change back to work dir
			if err := os.Chdir(workDir); err != nil {
				return err
			}
		default:
			Log("     -> unable to handle %s. unknown type %d\n", hdr.Name, hdr.Typeflag)
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
