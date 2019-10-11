package help

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// Passed to template.Execute()
type TemplateData map[string]string

type Renamer func(string) string

// If name begins with "dot.", replace with ".". Otherwise leave it alone.
func DotRenamer(name string) string {
	return strings.Replace(name, "dot.", ".", 1)
}

func IdentityRenamer(name string) string {
	return name
}

// Recursive copy src directory below dst directory, with optional transformations.
// It performs the following transformations:
// - Renames any directory with renamer.
// - If templatedata is not empty, will consider each file ending with ".template" as a Go
//   template.
// - If a file name contains basic Go template formatting (eg: `foo-{{.bar}}.template`), the
//   file will be renamed accordingly.
//
// It will fail if the dst directory doesn't exist.
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
func CopyDir(dst string, src string, dirRenamer Renamer, templatedata TemplateData) error {
	for _, dir := range []string{dst, src} {
		fi, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%v is not a directory", dst)
		}
	}

	renamedDir := dirRenamer(filepath.Base(src))
	tgtDir := filepath.Join(dst, renamedDir)
	if err := os.MkdirAll(tgtDir, 0770); err != nil {
		return fmt.Errorf("making src dir: %w", err)
	}

	srcEntries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range srcEntries {
		src := filepath.Join(src, e.Name())
		if e.IsDir() {
			if err := CopyDir(tgtDir, src, dirRenamer, templatedata); err != nil {
				return err
			}
		} else {
			name := e.Name()
			if len(templatedata) != 0 {
				// FIXME longstanding bug: we apply template processing always, also if the file
				// doesn't have the .template suffix!
				name = strings.TrimSuffix(name, ".template")
				// Subject the file name itself to template expansion
				tmpl, err := template.New("file-name").Parse(name)
				if err != nil {
					return fmt.Errorf("parsing file name as template %v: %w", src, err)
				}
				tmpl.Option("missingkey=error")
				buf := &bytes.Buffer{}
				if err := tmpl.Execute(buf, templatedata); err != nil {
					return fmt.Errorf("executing template file name %v with data %v: %w",
						src, templatedata, err)
				}
				name = buf.String()
			}
			if err := copyFile(filepath.Join(tgtDir, name), src, templatedata); err != nil {
				return err
			}
		}

	}
	return nil
}

func copyFile(dstPath string, srcPath string, templatedata TemplateData) error {
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

	if len(templatedata) == 0 {
		_, err = io.Copy(dstFile, srcFile)
		return err
	}
	buf, err := ioutil.ReadAll(srcFile)
	if err != nil {
		return err
	}
	tmpl, err := template.New(path.Base(srcPath)).Parse(string(buf))
	if err != nil {
		return fmt.Errorf("parsing template %v: %w", srcPath, err)
	}
	tmpl.Option("missingkey=error")
	if err := tmpl.Execute(dstFile, templatedata); err != nil {
		return fmt.Errorf("executing template %v with data %v: %w", srcPath, templatedata, err)
	}
	return nil
}
