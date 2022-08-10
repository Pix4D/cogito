package testhelp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/imdario/mergo"
	"gotest.tools/v3/assert"
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

// CopyDir recursively copies src directory below dst directory, with optional
// transformations.
// It performs the following transformations:
//   - Renames any directory with renamer.
//   - If templatedata is not empty, will consider each file ending with ".template" as a Go
//     template.
//   - If a file name contains basic Go template formatting (eg: `foo-{{.bar}}.template`), the
//     file will be renamed accordingly.
//
// It will fail if the dst directory doesn't exist.
//
// For example, if src directory is `foo`:
//
//	foo
//	└── dot.git
//	   └── config
//
// and dst directory is `bar`, src will be copied as:
//
//	bar
//	└── foo
//	   └── .git        <= dot renamed
//	     └── config
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

	srcEntries, err := os.ReadDir(src)
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
	buf, err := io.ReadAll(srcFile)
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

// TestCfg is a test configuration.
type TestCfg struct {
	Token string
	Owner string
	Repo  string
	SHA   string
}

// FakeTestCfg is a fake test configuration that can be used in some tests that need
// configuration but don't really use any external service.
var FakeTestCfg = TestCfg{
	Token: "fakeToken",
	Owner: "fakeOwner",
	Repo:  "fakeRepo",
	SHA:   "0123456789012345678901234567890123456789",
}

// SkipTestIfNoEnvVars is used to decide whether to run an end-to-end test or not.
// The decision is based on the presence or absence of environment variables detailed
// in the CONTRIBUTING file.
func SkipTestIfNoEnvVars(t *testing.T) TestCfg {
	t.Helper()

	token := os.Getenv("COGITO_TEST_OAUTH_TOKEN")
	owner := os.Getenv("COGITO_TEST_REPO_OWNER")
	repo := os.Getenv("COGITO_TEST_REPO_NAME")
	SHA := os.Getenv("COGITO_TEST_COMMIT_SHA")

	// If none of the environment variables are set, we skip the test.
	if len(token) == 0 && len(owner) == 0 && len(repo) == 0 && len(SHA) == 0 {
		t.Skip("Skipping integration test. See CONTRIBUTING for how to enable.")
	}
	// If some of the environment variables are set and some not, we fail the test.
	if len(token) == 0 || len(owner) == 0 || len(repo) == 0 || len(SHA) == 0 {
		t.Fatal("Some integration env vars are set and some not. See CONTRIBUTING for how to fix.")
	}

	return TestCfg{token, owner, repo, SHA}
}

// MergeMap returns a new map, the merge of b over a, meaning that duplicated keys in
// a will be overwritten by the corresponding keys in b.
func MergeMap(a, b map[string]any) map[string]any {
	c := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		c[k] = v
	}
	for k, v := range b {
		c[k] = v
	}
	return c
}

// MakeGitRepoFromTestdata creates a temporary directory by rendering the templated
// contents of testdataDir with values from (repoURL, commitSHA, head) and returns the
// path to the directory.
//
// MakeGitRepoFromTestdata also renames directories of the form 'dot.git' to '.git',
// thus making said directory a git repository. This allows to supply the 'dot.git'
// directory as test input, avoiding the problem of having this testdata .git directory
// a nested repository in the project repository.
//
// The temporary directory is registered for removal via t.Cleanup.
// If any operation fails, makeGitRepoFromTestdata terminates the test by calling t.Fatal.
func MakeGitRepoFromTestdata(
	t *testing.T,
	testdataDir string,
	repoURL string,
	commitSHA string,
	head string,
) string {
	t.Helper()
	dstDir, err := os.MkdirTemp("", "cogito-test-")
	if err != nil {
		t.Fatal("makeGitRepoFromTestdata: MkdirTemp", err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dstDir); err != nil {
			t.Fatal("makeGitRepoFromTestdata: cleanup: RemoveAll:", err)
		}
	})

	// Prepare the template data.
	tdata := make(TemplateData)
	tdata["repo_url"] = repoURL
	tdata["commit_sha"] = commitSHA
	tdata["head"] = head
	tdata["branch_name"] = "a-branch-FIXME"

	err = CopyDir(dstDir, testdataDir, DotRenamer, tdata)
	if err != nil {
		t.Fatal("CopyDir:", err)
	}

	return dstDir
}

// SshRemote returns a GitHub SSH URL
func SshRemote(owner, repo string) string {
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
}

// HttpsRemote returns a GitHub HTTPS URL
func HttpsRemote(owner, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

// HttpRemote returns a GitHub HTTP URL
func HttpRemote(owner, repo string) string {
	return fmt.Sprintf("http://github.com/%s/%s.git", owner, repo)
}

// ToJSON returns the JSON encoding of thing.
func ToJSON(t *testing.T, thing any) []byte {
	t.Helper()
	buf, err := json.Marshal(thing)
	assert.NilError(t, err)
	return buf
}

// FromJSON unmarshals the JSON-encoded data into thing.
func FromJSON(t *testing.T, data []byte, thing any) {
	t.Helper()
	err := json.Unmarshal(data, thing)
	assert.NilError(t, err)
}

// MergeStructs merges b into a and returns the merged copy.
// Said in another way, a is the default and b is the override.
// Used to express succinctly the delta in the test cases.
// Since it is a test helper, it will panic in case of error.
func MergeStructs[T any](a, b T) T {
	if err := mergo.Merge(&a, b, mergo.WithOverride); err != nil {
		panic(err)
	}
	return a
}

// FailingWriter is an io.Writer that always returns an error.
type FailingWriter struct{}

func (t *FailingWriter) Write([]byte) (n int, err error) {
	return 0, errors.New("test write error")
}