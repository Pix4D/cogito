package cogito

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/Pix4D/cogito/testhelp"
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

	const wantHostname = "github.com"
	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	test := func(t *testing.T, tc testCase) {
		inputDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL,
			"dummySHA", "dummyHead")

		err := checkGitRepoDir(filepath.Join(inputDir, filepath.Base(tc.dir)),
			wantHostname, wantOwner, wantRepo)

		assert.NilError(t, err)
	}

	testCases := []testCase{
		{
			name:    "repo with good SSH remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.SshRemote(wantHostname, wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTPS remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.HttpsRemote(wantHostname, wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTP remote",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.HttpRemote(wantHostname, wantOwner, wantRepo),
		},
		{
			name:    "PR resource but with basic auth in URL (see PR #46)",
			dir:     "testdata/one-repo/a-repo",
			repoURL: fmt.Sprintf("https://x-oauth-basic:ghp_XXX@%s/%s/%s.git", wantHostname, wantOwner, wantRepo),
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

	const wantHostname = "github.com"
	const wantOwner = "smiling"
	const wantRepo = "butterfly"

	test := func(t *testing.T, tc testCase) {
		inDir := testhelp.MakeGitRepoFromTestdata(t, tc.dir, tc.repoURL,
			"dummySHA", "dummyHead")

		err := checkGitRepoDir(filepath.Join(inDir, filepath.Base(tc.dir)),
			wantHostname, wantOwner, wantRepo)

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
			repoURL: testhelp.HttpsRemote("github.com", "owner-a", "repo-a"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
    url: https://github.com/owner-a/repo-a.git
    owner: owner-a
    repo: repo-a

Cogito SOURCE configuration:
    hostname: github.com
    owner: smiling
    repo: butterfly`,
		},
		{
			name:    "repo with unrelated SSH remote or wrong source config",
			dir:     "testdata/one-repo/a-repo",
			repoURL: testhelp.SshRemote("github.com", "owner-a", "repo-a"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
    url: git@github.com:owner-a/repo-a.git
    owner: owner-a
    repo: repo-a

Cogito SOURCE configuration:
    hostname: github.com
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
			name: "single instance variable",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"stable"}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%7D",
		},
		{
			name: "single instance variable with spaces",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"foo bar"}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22foo%20bar%22%7D",
		},
		{
			name: "multiple instance variables",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"stable","foo":"bar"}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%2C%22foo%22%3A%22bar%22%7D",
		},
		{
			name: "multiple instance variables: nested json with spaces",
			env: testhelp.MergeStructs(baseEnv,
				Environment{BuildPipelineInstanceVars: `{"branch":"foo bar","version":{"from":1.0,"to":2.0}}`}),
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22foo%20bar%22%2C%22version%22%3A%7B%22from%22%3A1.0%2C%22to%22%3A2.0%7D%7D",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}
