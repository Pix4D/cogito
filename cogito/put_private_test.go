package cogito

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pix4D/cogito/testhelp"
	"github.com/gertd/wild"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

func TestProcessInputDirFailure(t *testing.T) {
	type testCase struct {
		name     string
		inputDir string
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.inputDir,
			"https://github.com/foo", "dummySHA", "dummyHead")

		err := processInputDir(filepath.Join(tmpDir, filepath.Base(tc.inputDir)),
			"dummy-owner", "dummy-repo")

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:     "two input dirs",
			inputDir: "testdata/two-dirs",
			wantErr:  "found 2 input dirs: [dir-1 dir-2]. Want exactly 1, corresponding to the GitHub repo dummy-owner/dummy-repo",
		},
		{
			name:     "one input dir but not a repo",
			inputDir: "testdata/not-a-repo",
			wantErr:  "parsing .git/config: open ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}

}

func TestCollectInputDirs(t *testing.T) {
	var testCases = []struct {
		name    string
		dir     string
		wantErr error
		wantN   int
	}{
		{
			name:    "non existing base directory",
			dir:     "non-existing",
			wantErr: os.ErrNotExist,
			wantN:   0,
		},
		{
			name:    "empty directory",
			dir:     "testdata/empty-dir",
			wantErr: nil,
			wantN:   0,
		},
		{
			name:    "two directories and one file",
			dir:     "testdata/two-dirs",
			wantErr: nil,
			wantN:   2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dirs, err := collectInputDirs(tc.dir)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("sut(%v): error: have %v; want %v", tc.dir, err, tc.wantErr)
			}
			gotN := len(dirs)
			if gotN != tc.wantN {
				t.Errorf("sut(%v): len(dirs): have %v; want %v", tc.dir, gotN, tc.wantN)
			}
		})
	}
}

func TestCheckGitRepoDirSuccess(t *testing.T) {
	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	testCases := []struct {
		name    string
		dir     string // repoURL to put in file <dir>/.git/config
		repoURL string
	}{
		{
			name:    "repo with good SSH remote",
			dir:     "a-repo",
			repoURL: testhelp.SshRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTPS remote",
			dir:     "a-repo",
			repoURL: testhelp.HttpsRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTP remote",
			dir:     "a-repo",
			repoURL: testhelp.HttpRemote(wantOwner, wantRepo),
		},
		{
			name:    "PR resource but with basic auth in URL (see PR #46)",
			dir:     "a-repo",
			repoURL: "https://x-oauth-basic:ghp_XXX@github.com/smiling/butterfly.git",
		},
	}

	for _, tc := range testCases {
		inputDir := testhelp.MakeGitRepoFromTestdata(t, filepath.Join("testdata", tc.dir),
			tc.repoURL, "dummySHA", "dummyHead")

		t.Run(tc.name, func(t *testing.T) {
			err := checkGitRepoDir(filepath.Join(inputDir, tc.dir), wantOwner, wantRepo)

			assert.NilError(t, err)
		})
	}
}

func TestCheckGitRepoDirFailure(t *testing.T) {
	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	testCases := []struct {
		name        string
		dir         string
		repoURL     string // repoURL to put in file <dir>/.git/config
		wantErrWild string // wildcard matching
	}{
		{
			name:        "dir is not a repo",
			dir:         "not-a-repo",
			repoURL:     "dummyurl",
			wantErrWild: `parsing .git/config: open */not-a-repo/.git/config: no such file or directory`,
		},
		{
			name:        "bad file .git/config",
			dir:         "repo-bad-git-config",
			repoURL:     "dummyurl",
			wantErrWild: `.git/config: key [remote "origin"]/url: not found`,
		},
		{
			name:    "repo with unrelated HTTPS remote",
			dir:     "a-repo",
			repoURL: testhelp.HttpsRemote("owner-a", "repo-a"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: https://github.com/owner-a/repo-a.git
    owner: owner-a
     repo: repo-a

Cogito SOURCE configuration:
    owner: smiling
     repo: butterfly`,
		},
		{
			name:    "repo with unrelated SSH remote or wrong source config",
			dir:     "a-repo",
			repoURL: testhelp.SshRemote("owner-a", "repo-a"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: git@github.com:owner-a/repo-a.git
    owner: owner-a
     repo: repo-a

Cogito SOURCE configuration:
    owner: smiling
     repo: butterfly`,
		},
		{
			name:        "invalid git pseudo URL in .git/config",
			dir:         "a-repo",
			repoURL:     "foo://bar",
			wantErrWild: `.git/config: remote: invalid git URL foo://bar: invalid scheme: foo`,
		},
	}

	for _, tc := range testCases {
		inDir := testhelp.MakeGitRepoFromTestdata(t, filepath.Join("testdata", tc.dir),
			tc.repoURL, "dummySHA", "dummyHead")

		t.Run(tc.name, func(t *testing.T) {
			err := checkGitRepoDir(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErrWild)
			}

			have := err.Error()
			if !wild.Match(tc.wantErrWild, have, false) {
				diff := cmp.Diff(tc.wantErrWild, have)
				t.Fatalf("error msg wildcard mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestParseGitPseudoURLSuccess(t *testing.T) {
	testCases := []struct {
		name   string
		inURL  string
		wantGU gitURL
	}{
		{
			name:  "valid SSH URL",
			inURL: "git@github.com:Pix4D/cogito.git",
			wantGU: gitURL{
				URL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "/Pix4D/cogito.git",
				},
				Owner: "Pix4D",
				Repo:  "cogito",
			},
		},
		{
			name:  "valid HTTPS URL",
			inURL: "https://github.com/Pix4D/cogito.git",
			wantGU: gitURL{
				URL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "/Pix4D/cogito.git",
				},
				Owner: "Pix4D",
				Repo:  "cogito",
			},
		},
		{
			name:  "valid HTTP URL",
			inURL: "http://github.com/Pix4D/cogito.git",
			wantGU: gitURL{
				URL: &url.URL{
					Scheme: "http",
					Host:   "github.com",
					Path:   "/Pix4D/cogito.git",
				},
				Owner: "Pix4D",
				Repo:  "cogito",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gitUrl, err := parseGitPseudoURL(tc.inURL)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
			if diff := cmp.Diff(tc.wantGU, gitUrl, cmp.Comparer(
				func(x, y *url.Userinfo) bool {
					return x.String() == y.String()
				})); diff != "" {
				t.Errorf("gitURL: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestParseGitPseudoURLFailure(t *testing.T) {
	testCases := []struct {
		name    string
		inURL   string
		wantErr string
	}{
		{
			name:    "totally invalid URL",
			inURL:   "hello",
			wantErr: "invalid git URL hello: missing scheme",
		},
		{
			name:    "invalid SSH URL",
			inURL:   "git@github.com/Pix4D/cogito.git",
			wantErr: "invalid git SSH URL git@github.com/Pix4D/cogito.git: want exactly one ':'",
		},
		{
			name:    "invalid HTTPS URL",
			inURL:   "https://github.com:Pix4D/cogito.git",
			wantErr: `parse "https://github.com:Pix4D/cogito.git": invalid port ":Pix4D" after host`,
		},
		{
			name:    "invalid HTTP URL",
			inURL:   "http://github.com:Pix4D/cogito.git",
			wantErr: `parse "http://github.com:Pix4D/cogito.git": invalid port ":Pix4D" after host`,
		},
		{
			name:    "too few path components",
			inURL:   "http://github.com/cogito.git",
			wantErr: "invalid git URL: path: want: 3 components; have: 2 [ cogito.git]",
		},
		{
			name:    "too many path components",
			inURL:   "http://github.com/1/2/cogito.git",
			wantErr: "invalid git URL: path: want: 3 components; have: 4 [ 1 2 cogito.git]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseGitPseudoURL(tc.inURL)

			assert.Error(t, err, tc.wantErr)
		})
	}
}
