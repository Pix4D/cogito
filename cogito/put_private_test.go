package cogito

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pix4D/cogito/testhelp"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

func TestCollectInputDirs(t *testing.T) {
	type testCase = struct {
		name    string
		dir     string
		wantErr error
		wantN   int
	}

	test := func(t *testing.T, tc testCase) {
		dirs, err := collectInputDirs(tc.dir)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("sut(%v): error: have %v; want %v", tc.dir, err, tc.wantErr)
		}
		gotN := len(dirs)
		if gotN != tc.wantN {
			t.Errorf("sut(%v): len(dirs): have %v; want %v", tc.dir, gotN, tc.wantN)
		}
	}

	var testCases = []testCase{
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
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestCheckGitRepoDirSuccess(t *testing.T) {
	type testCase struct {
		name    string
		dir     string // repoURL to put in file <dir>/.git/config
		repoURL string
	}

	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	test := func(t *testing.T, tc testCase) {
		inputDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL,
			"dummySHA", "dummyHead")

		err := checkGitRepoDir(filepath.Join(inputDir, filepath.Base(tc.dir)),
			wantOwner, wantRepo)

		assert.NilError(t, err)
	}

	testCases := []testCase{
		{
			name:    "repo with good SSH remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.SshRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTPS remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.HttpsRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTP remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.HttpRemote(wantOwner, wantRepo),
		},
		{
			name:    "PR resource but with basic auth in URL (see PR #46)",
			dir:     "testdata/one-repo/a-repo",
			repoURL: "https://x-oauth-basic:ghp_XXX@github.com/smiling/butterfly.git",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestCheckGitRepoDirFailure(t *testing.T) {
	type testCase struct {
		name        string
		dir         string
		repoURL     string // repoURL to put in file <dir>/.git/config
		wantErrWild string // wildcard matching
	}

	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	test := func(t *testing.T, tc testCase) {
		inDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL,
			"dummySHA", "dummyHead")

		err := checkGitRepoDir(filepath.Join(inDir, filepath.Base(tc.dir)),
			wantOwner, wantRepo)

		assert.ErrorContains(t, err, tc.wantErrWild)
	}

	testCases := []testCase{
		{
			name:        "dir is not a repo",
			dir:         "testdata/not-a-repo",
			repoURL:     "dummyurl",
			wantErrWild: "parsing .git/config: open ",
		},
		{
			name:        "bad file .git/config",
			dir:         "testdata/repo-bad-git-config",
			repoURL:     "dummyurl",
			wantErrWild: `.git/config: key [remote "origin"]/url: not found`,
		},
		{
			name:    "repo with unrelated HTTPS remote",
			dir:     "testdata/one-repo/a-repo",
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
			dir:     "testdata/one-repo/a-repo",
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
			dir:         "testdata/one-repo/a-repo",
			repoURL:     "foo://bar",
			wantErrWild: `.git/config: remote: invalid git URL foo://bar: invalid scheme: foo`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
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
		{
			name:  "valid HTTP URL with username:password",
			inURL: "http://username:password@github.com/Pix4D/cogito.git",
			wantGU: gitURL{
				URL: &url.URL{
					Scheme: "http",
					User:   url.UserPassword("username", "password"),
					Host:   "github.com",
					Path:   "/Pix4D/cogito.git",
				},
				Owner: "Pix4D",
				Repo:  "cogito",
			},
		},
		{
			name:  "valid HTTP URL with username:password",
			inURL: "https://username:password@github.com/Pix4D/cogito.git",
			wantGU: gitURL{
				URL: &url.URL{
					Scheme: "https",
					User:   url.UserPassword("username", "password"),
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

func TestGitGetCommitSuccess(t *testing.T) {
	type testCase struct {
		name    string
		dir     string
		repoURL string
		head    string
	}

	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"
	const defHead = "ref: refs/heads/a-branch-FIXME"

	test := func(t *testing.T, tc testCase) {
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL, wantSHA, tc.head)

		sha, err := getGitCommit(filepath.Join(tmpDir, filepath.Base(tc.dir)))

		assert.NilError(t, err)
		assert.Equal(t, sha, wantSHA)
	}

	testCases := []testCase{
		{
			name:    "happy path for branch checkout",
			dir:     "testdata/one-repo/a-repo",
			repoURL: "dummy",
			head:    defHead,
		},
		{
			name:    "happy path for detached HEAD checkout",
			dir:     "testdata/one-repo/a-repo",
			repoURL: "dummy",
			head:    wantSHA,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestGitGetCommitFailure(t *testing.T) {
	type testCase struct {
		name    string
		dir     string
		repoURL string
		head    string
		wantErr string
	}

	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"

	test := func(t *testing.T, tc testCase) {
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL, wantSHA, tc.head)

		_, err := getGitCommit(filepath.Join(tmpDir, filepath.Base(tc.dir)))

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "missing HEAD",
			dir:     "testdata/not-a-repo",
			repoURL: "dummy",
			head:    "dummy",
			wantErr: "git commit: read HEAD: open ",
		},
		{
			name:    "invalid format for HEAD",
			dir:     "testdata/one-repo/a-repo",
			repoURL: "dummyURL",
			head:    "this is a bad head",
			wantErr: `git commit: invalid HEAD format: "this is a bad head"`,
		},
		{
			name:    "HEAD points to non-existent file",
			dir:     "testdata/one-repo/a-repo",
			repoURL: "dummyURL",
			head:    "banana mango",
			wantErr: "git commit: branch checkout: read SHA file: open ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestMultiErrString(t *testing.T) {
	type testCase struct {
		name    string
		errs    []error
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Equal(t, multiErrString(tc.errs), tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "one error",
			errs:    []error{errors.New("error 1")},
			wantErr: "error 1",
		},
		{
			name: "multiple errors",
			errs: []error{errors.New("error 1"), errors.New("error 2")},
			wantErr: `multiple errors:
	error 1
	error 2`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestConcourseBuildURL(t *testing.T) {
	type testCase struct {
		name string
		env  Environment
		want string
	}

	test := func(t *testing.T, tc testCase) {
		if tc.want == "" {
			t.Fatal("tc.want: empty")
		}

		have := concourseBuildURL(tc.env)

		if have != tc.want {
			t.Fatalf("\nhave: %s\nwant: %s", have, tc.want)
		}
	}

	baseEnv := Environment{
		BuildId:                   "",
		BuildName:                 "42",
		BuildJobName:              "paint",
		BuildPipelineName:         "magritte",
		BuildPipelineInstanceVars: "",
		BuildTeamName:             "devs",
		BuildCreatedBy:            "",
		AtcExternalUrl:            "https://ci.example.com",
	}

	testCases := []testCase{
		{
			name: "all defaults",
			env:  baseEnv,
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42",
		},
		{
			name: "instanced vars 1",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"stable"}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%7D",
		},
		{
			name: "instanced vars 2",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"stable","foo":"bar"}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%2C%22foo%22%3A%22bar%22%7D",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}
