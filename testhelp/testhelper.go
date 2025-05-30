package testhelp

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"dario.cat/mergo"
	"github.com/golang-jwt/jwt/v5"
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

// GhTestCfg contains the secrets needed to run integration tests against the
// GitHub Commit Status API.
type GhTestCfg struct {
	Token               string
	GhAppClientID       string
	GhAppInstallationID string
	GhAppPrivateKey     string
	Owner               string
	Repo                string
	SHA                 string
}

// FakeTestCfg is a fake test configuration that can be used in some tests that need
// configuration but don't really use any external service.
var FakeTestCfg = GhTestCfg{
	Token: "fakeToken",
	Owner: "fakeOwner",
	Repo:  "fakeRepo",
	SHA:   "0123456789012345678901234567890123456789",
}

// GitHubSecretsOrFail returns the secrets needed to run integration tests against the
// GitHub Commit Status API. If the secrets are missing, GitHubSecretsOrFail fails the test.
func GitHubSecretsOrFail(t *testing.T) GhTestCfg {
	t.Helper()

	return GhTestCfg{
		Token:               getEnvOrFail(t, "COGITO_TEST_OAUTH_TOKEN"),
		GhAppClientID:       getEnvOrFail(t, "COGITO_TEST_GH_APP_CLIENT_ID"),
		GhAppInstallationID: getEnvOrFail(t, "COGITO_TEST_GH_APP_INSTALLATION_ID"),
		GhAppPrivateKey:     getEnvOrFail(t, "COGITO_TEST_GH_APP_PRIVATE_KEY"),
		Owner:               getEnvOrFail(t, "COGITO_TEST_REPO_OWNER"),
		Repo:                getEnvOrFail(t, "COGITO_TEST_REPO_NAME"),
		SHA:                 getEnvOrFail(t, "COGITO_TEST_COMMIT_SHA"),
	}
}

// GChatTestCfg contains the secrets needed to run integration tests against the
// Google Chat API.
type GChatTestCfg struct {
	Hook string
}

// GoogleChatSecretsOrFail returns the secrets needed to run integration tests against the
// Google Chat API. If the secrets are missing, GoogleChatSecretsOrFail fails the test.
func GoogleChatSecretsOrFail(t *testing.T) GChatTestCfg {
	t.Helper()

	return GChatTestCfg{
		Hook: getEnvOrFail(t, "COGITO_TEST_GCHAT_HOOK"),
	}
}

// getEnvOrFail returns the value of environment variable key. If key is missing,
// getEnvOrFail fails the test.
func getEnvOrFail(t *testing.T, key string) string {
	t.Helper()

	value := os.Getenv(key)
	if len(value) == 0 {
		t.Fatalf("Missing environment variable (see CONTRIBUTING): %s", key)
	}
	return value
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
func SshRemote(hostname, owner, repo string) string {
	return fmt.Sprintf("git@%s:%s/%s.git", hostname, owner, repo)
}

// HttpsRemote returns a GitHub HTTPS URL
func HttpsRemote(hostname, owner, repo string) string {
	return fmt.Sprintf("https://%s/%s/%s.git", hostname, owner, repo)
}

// HttpRemote returns a GitHub HTTP URL
func HttpRemote(hostname, owner, repo string) string {
	return fmt.Sprintf("http://%s/%s/%s.git", hostname, owner, repo)
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

// GeneratePrivateKey creates a RSA Private Key of specified byte size
func GeneratePrivateKey(t *testing.T, bitSize int) (*rsa.PrivateKey, error) {
	// Private Key generation
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	assert.NilError(t, err)

	// Validate Private Key
	err = privateKey.Validate()
	assert.NilError(t, err)

	return privateKey, nil
}

// EncodePrivateKeyToPEM encodes Private Key from RSA to PEM format
func EncodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	// Get ASN.1 DER format
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	return pem.EncodeToMemory(&privBlock)
}

// DecodeJWT decodes the HTTP request authorization header with the given RSA key
// and returns the registered claims of the decoded token.
func DecodeJWT(t *testing.T, r *http.Request, key *rsa.PrivateKey) *jwt.RegisteredClaims {
	token := strings.Fields(r.Header.Get("Authorization"))[1]
	tok, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Header["alg"] != "RS256" {
			return nil, fmt.Errorf("unexpected signing method: %v, expected: %v", t.Header["alg"], "RS256")
		}
		return &key.PublicKey, nil
	})
	assert.NilError(t, err)

	return tok.Claims.(*jwt.RegisteredClaims)
}
