package help

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// If name begins with "dot.", replace with ".". Otherwise leave it alone.
func DotRenamer(name string) string {
	return strings.Replace(name, "dot.", ".", 1)
}

func IdentityRenamer(name string) string {
	return name
}

// Recursive copy src directory below dst directory, renaming any directory with renamer.
// It expects dst directory to exist.
//
// For example, if src directory is `foo`:
// foo
// └── dot.git
//     └── config
//
// and dst directory is `bar`, src will be copied as:
// bar
// └── foo
//     └── .git        <= dot renamed
//         └── config
func CopyDir(dst string, src string, renamer func(string) string) error {
	for _, dir := range []string{dst, src} {
		fi, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%v is not a directory", dst)
		}
	}

	renamedDir := renamer(filepath.Base(src))
	tgtDir := filepath.Join(dst, renamedDir)
	if err := os.MkdirAll(tgtDir, 0770); err != nil {
		return fmt.Errorf("making src dir: %w", err)
	}

	srcEntries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range srcEntries {
		if e.IsDir() {
			if err := CopyDir(tgtDir, filepath.Join(src, e.Name()), renamer); err != nil {
				return err
			}
		} else {
			if err := copyFile(filepath.Join(tgtDir, e.Name()), filepath.Join(src, e.Name())); err != nil {
				return err
			}
		}

	}
	return nil
}

func copyFile(dstPath, srcPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening src file: %w", err)
	}
	defer srcFile.Close()

	// We want an error if the file already exists
	dstFile, err := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0660)
	if err != nil {
		return fmt.Errorf("creating dst file: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
