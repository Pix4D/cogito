package resource

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	oc "github.com/cloudboss/ofcourse/ofcourse"
	"github.com/gertd/wild"
	"github.com/google/go-cmp/cmp"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/help"
)

var (
	silentLog = oc.NewLogger(oc.SilentLevel)

	defVersion  = oc.Version{"ref": "dummy"}
	defVersions = []oc.Version{defVersion}
	defEnv      = oc.NewEnvironment(
		map[string]string{
			"ATC_EXTERNAL_URL": "https://cogito.invalid",
			"BUILD_JOB_NAME":   "a-job"})
)

func TestCheckSuccess(t *testing.T) {
	cfg := help.FakeTestCfg

	testCases := []struct {
		name         string
		inSource     oc.Source
		inVersion    oc.Version
		wantVersions []oc.Version
	}{
		{
			name: "happy path",
			inSource: oc.Source{
				"access_token": cfg.Token,
				"owner":        cfg.Owner,
				"repo":         cfg.Repo,
			},
			inVersion:    defVersion,
			wantVersions: defVersions,
		},
		{
			name: "do not return a nil version the first time it runs (see Concourse PR #4442)",
			inSource: oc.Source{
				"access_token": cfg.Token,
				"owner":        cfg.Owner,
				"repo":         cfg.Repo,
			},
			inVersion:    oc.Version{},
			wantVersions: defVersions,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Resource{}

			versions, err := r.Check(tc.inSource, tc.inVersion, defEnv, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}

			if diff := cmp.Diff(tc.wantVersions, versions); diff != "" {
				t.Fatalf("version: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestCheckFailure(t *testing.T) {
	testCases := []struct {
		name      string
		inSource  oc.Source
		inVersion oc.Version
		wantErr   string
	}{
		{
			name:      "missing mandatory source keys",
			inSource:  oc.Source{},
			inVersion: defVersion,
			wantErr:   "missing source keys: [access_token owner repo]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := Resource{}

			_, err := res.Check(tc.inSource, tc.inVersion, defEnv, silentLog)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Errorf("error message mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestIn(t *testing.T) {
	defSource := oc.Source{
		"access_token": "dummy",
		"owner":        "dummy",
		"repo":         "dummy",
	}

	var testCases = []struct {
		name      string
		inVersion oc.Version
	}{
		{
			name:      "happy path",
			inVersion: defVersion,
		},
		{
			name:      "do not return a nil version the first time it runs (see Concourse PR #4442)",
			inVersion: oc.Version{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Resource{}

			version, metadata, err := r.In(
				"/tmp", defSource, oc.Params{}, tc.inVersion, defEnv, silentLog,
			)

			if err != nil {
				t.Fatalf("err: have %v; want %v", err, nil)
			}
			if diff := cmp.Diff(defVersion, version); diff != "" {
				t.Errorf("version: (-want +have):\n%s", diff)
			}
			if diff := cmp.Diff(oc.Metadata{}, metadata); diff != "" {
				t.Errorf("metadata: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestOutMockSuccess(t *testing.T) {
	cfg := help.FakeTestCfg

	defSource := oc.Source{
		"access_token": cfg.Token,
		"owner":        cfg.Owner,
		"repo":         cfg.Repo,
	}
	defParams := oc.Params{
		"state": "error",
	}
	defMeta := oc.Metadata{oc.NameVal{
		Name: "state", Value: "error"},
	}
	defWantBody := map[string]string{
		"context": defEnv.Get("BUILD_JOB_NAME"),
	}

	testDir := "a-repo"

	testCases := []struct {
		name     string
		source   oc.Source
		params   oc.Params
		wantBody map[string]string
	}{
		{
			name: "valid mandatory source and params",
		},
		{
			name: "source: optional: context_prefix",
			source: help.MergeMap(defSource, oc.Source{
				"context_prefix": "cocco"},
			),
			wantBody: map[string]string{
				"context": "cocco/" + defEnv.Get("BUILD_JOB_NAME"),
			},
		},
		{
			name: "params: optional: context",
			params: help.MergeMap(defParams, oc.Params{
				"context": "bello",
			}),
			wantBody: map[string]string{
				"context": "bello",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			if tc.source == nil {
				tc.source = defSource
			}
			if tc.params == nil {
				tc.params = defParams
			}
			if tc.wantBody == nil {
				tc.wantBody = defWantBody
			}

			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintln(w, "Anything goes...")

					buf, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatalf("reading body: %v", err)
					}
					var bm map[string]string
					if err := json.Unmarshal(buf, &bm); err != nil {
						t.Fatalf("parsing JSON body: %v", err)
					}
					for k, v := range tc.wantBody {
						if bm[k] != v {
							t.Errorf("\nbody[%q]: have: %q; want: %q", k, bm[k], v)
						}
					}
				}),
			)

			savedAPI := github.API
			github.API = ts.URL
			defer func() {
				ts.Close()
				github.API = savedAPI
			}()

			res := Resource{}

			version, metadata, err := res.Out(
				inDir, tc.source, tc.params, defEnv, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}

			if diff := cmp.Diff(defVersion, version); diff != "" {
				t.Errorf("version: (-want +have):\n%s", diff)
			}

			if diff := cmp.Diff(defMeta, metadata); diff != "" {
				t.Errorf("metadata: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestOutMockFailure(t *testing.T) {
	cfg := help.FakeTestCfg

	defSource := oc.Source{
		"access_token": cfg.Token,
		"owner":        cfg.Owner,
		"repo":         cfg.Repo,
	}
	defParams := oc.Params{
		"state": "error",
	}

	testDir := "a-repo"

	testCases := []struct {
		name    string
		source  oc.Source
		params  oc.Params
		wantErr string
	}{
		{
			name:    "missing mandatory source keys",
			source:  oc.Source{},
			params:  defParams,
			wantErr: "missing source keys: [access_token owner repo]",
		},
		{
			name:    "missing mandatory parameters",
			source:  defSource,
			params:  oc.Params{},
			wantErr: "missing put parameter 'state'",
		},
		{
			name:   "invalid state parameter",
			source: defSource,
			params: oc.Params{
				"state": "hello",
			},
			wantErr: "invalid put parameter 'state: hello'",
		},
		{
			name:   "unknown parameter",
			source: defSource,
			params: oc.Params{
				"state": "pending",
				"pizza": "margherita",
			},
			wantErr: "unknown put parameter 'pizza'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintln(w, "Anything goes...")
				}),
			)

			savedAPI := github.API
			github.API = ts.URL
			defer func() {
				ts.Close()
				github.API = savedAPI
			}()

			res := Resource{}

			_, _, err := res.Out(
				inDir, tc.source, tc.params, defEnv, silentLog)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}

			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error msg mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestOutSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)

	defSource := oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo}
	defParams := oc.Params{"state": "error"}
	testDir := "a-repo"

	type in struct {
		source oc.Source
		params oc.Params
	}

	testCases := []struct {
		name string
		in   in
	}{
		{
			name: "backend reports success",
			in:   in{defSource, defParams},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			r := Resource{}
			_, _, err := r.Out(inDir, tc.in.source, tc.in.params, defEnv, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestOutFailureIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)

	defParams := oc.Params{"state": "error"}
	testDir := "a-repo"

	type in struct {
		source oc.Source
		params oc.Params
	}

	testCases := []struct {
		name    string
		in      in
		wantErr string
	}{
		{
			name: "local validations fail",
			in: in{
				oc.Source{
					"access_token": cfg.Token,
					"owner":        cfg.Owner,
					"repo":         "does-not-exists-really"},
				defParams,
			},
			wantErr: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: git@github.com:pix4d/cogito-test-read-write.git
    owner: pix4d
     repo: cogito-test-read-write

Cogito SOURCE configuration:
    owner: pix4d
     repo: does-not-exists-really`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			r := Resource{}
			_, _, err := r.Out(inDir, tc.in.source, tc.in.params, defEnv, silentLog)

			if err == nil {
				t.Fatalf("have: <no error>\nwant: %s", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error msg mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestGhTargetURL(t *testing.T) {
	testCases := []struct {
		name         string
		atc          string
		team         string
		pipeline     string
		job          string
		buildN       string
		instanceVars string
		want         string
	}{
		{
			name: "all defaults",
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42",
		},
		{
			name:         "instanced vars 1",
			instanceVars: `{"branch":"stable"}`,
			want:         "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%7D",
		},
		{
			name:         "instanced vars 2",
			instanceVars: `{"branch":"stable","foo":"bar"}`,
			want:         "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%2C%22foo%22%3A%22bar%22%7D",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == "" {
				t.Fatal("tc.want: empty")
			}

			atc := "https://ci.example.com"
			if tc.atc != "" {
				atc = tc.atc
			}
			team := "devs"
			if tc.team != "" {
				team = tc.team
			}
			pipeline := "magritte"
			if tc.pipeline != "" {
				pipeline = tc.pipeline
			}
			job := "paint"
			if tc.job != "" {
				job = tc.job
			}
			buildN := "42"
			if tc.buildN != "" {
				buildN = tc.buildN
			}

			have := ghTargetURL(atc, team, pipeline, job, buildN, tc.instanceVars)

			if have != tc.want {
				t.Fatalf("\nhave: %s\nwant: %s", have, tc.want)
			}
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

func TestCheckRepoDirSuccess(t *testing.T) {
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
			repoURL: sshRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTPS remote",
			dir:     "a-repo",
			repoURL: httpsRemote(wantOwner, wantRepo),
		},
		{
			name:    "repo with good HTTP remote",
			dir:     "a-repo",
			repoURL: httpRemote(wantOwner, wantRepo),
		},
		{
			name:    "PR resource but with basic auth in URL (see PR #46)",
			dir:     "a-repo",
			repoURL: "https://x-oauth-basic:ghp_XXX@github.com/smiling/butterfly.git",
		},
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc.dir, tc.repoURL, "dummySHA", "dummyHead")
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := checkRepoDir(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestCheckRepoDirFailure(t *testing.T) {
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
			repoURL: httpsRemote("owner", "repo"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: https://github.com/owner/repo.git
    owner: owner
     repo: repo

Cogito SOURCE configuration:
    owner: smiling
     repo: butterfly`,
		},
		{
			name:    "repo with unrelated SSH remote or wrong source config",
			dir:     "a-repo",
			repoURL: sshRemote("owner", "repo"),
			wantErrWild: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: git@github.com:owner/repo.git
    owner: owner
     repo: repo

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
		inDir, teardown := setup(t, tc.dir, tc.repoURL, "dummySHA", "dummyHead")
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := checkRepoDir(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)

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

func TestGitGetCommitSuccess(t *testing.T) {
	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"
	const defHead = "ref: refs/heads/a-branch-FIXME"

	testCases := []struct {
		name    string
		dir     string
		repoURL string
		head    string
	}{
		{
			name:    "happy path for branch checkout",
			dir:     "a-repo",
			repoURL: "dummy",
			head:    defHead,
		},
		{
			name:    "happy path for detached HEAD checkout",
			dir:     "a-repo",
			repoURL: "dummy",
			head:    wantSHA,
		},
	}

	for _, tc := range testCases {
		dir, teardown := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			sha, err := GitGetCommit(filepath.Join(dir, tc.dir))

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
			if sha != wantSHA {
				t.Fatalf("ref: have: %s; want: %s", sha, wantSHA)
			}
		})
	}
}

func TestGitGetCommitFailure(t *testing.T) {
	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"

	testCases := []struct {
		name        string
		dir         string
		repoURL     string
		head        string
		wantErrWild string // wildcard matching
	}{
		{
			name:        "missing HEAD",
			dir:         "not-a-repo",
			repoURL:     "dummy",
			head:        "dummy",
			wantErrWild: `git commit: read HEAD: open */not-a-repo/.git/HEAD: no such file or directory`,
		},
		{
			name:        "invalid format for HEAD",
			dir:         "a-repo",
			repoURL:     "dummyURL",
			head:        "this is a bad head",
			wantErrWild: `git commit: invalid HEAD format: "this is a bad head"`,
		},
	}

	for _, tc := range testCases {
		dir, teardown := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			_, err := GitGetCommit(filepath.Join(dir, tc.dir))

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

// setup creates a directory containing a git repository according to the parameters.
// It returns the path to the directory and a teardown function.
func setup(
	t *testing.T,
	dir string,
	inRepoURL string,
	inCommitSHA string,
	inHead string,
) (
	string,
	func(t *testing.T),
) {
	// Make a temp dir to be the resource work directory
	inDir, err := ioutil.TempDir("", "cogito-test-")
	if err != nil {
		t.Fatal("Temp dir", err)
	}
	tdata := make(help.TemplateData)
	tdata["repo_url"] = inRepoURL
	tdata["commit_sha"] = inCommitSHA
	tdata["head"] = inHead
	tdata["branch_name"] = "a-branch-FIXME"

	// Copy the testdata over
	err = help.CopyDir(inDir, filepath.Join("testdata", dir), help.DotRenamer, tdata)
	if err != nil {
		t.Fatal("CopyDir:", err)
	}

	teardown := func(t *testing.T) {
		defer os.RemoveAll(inDir)
	}
	return inDir, teardown
}

// sshRemote returns a github SSH URL
func sshRemote(owner, repo string) string {
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
}

// httpsRemote returns a github HTTPS URL
func httpsRemote(owner, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

// httpRemote returns a github HTTP URL
func httpRemote(owner, repo string) string {
	return fmt.Sprintf("http://github.com/%s/%s.git", owner, repo)
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseGitPseudoURL(tc.inURL)

			if err == nil {
				t.Fatalf("have: <no error>; want: %v", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Errorf("error message mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestValidateSourceSuccess(t *testing.T) {
	testCases := []struct {
		name   string
		source oc.Source
	}{
		{
			name: "all mandatory keys, no optional",
			source: oc.Source{
				"access_token": "dummy-token",
				"owner":        "dummy-owner",
				"repo":         "dummy-repo",
			},
		},
		// FIXME
		// {
		// 	name: "all mandatory and optional keys",
		// 	source: oc.Source{
		// 		"access_token": "dummy-token",
		// 		"owner":        "dummy-owner",
		// 		"repo":         "dummy-repo",
		// 	},
		// },
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := validateSource(tc.source)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestValidateSourceFailure(t *testing.T) {
	testCases := []struct {
		name    string
		source  oc.Source
		wantErr string
	}{
		{
			name:    "zero keys",
			source:  oc.Source{},
			wantErr: "missing source keys: [access_token owner repo]",
		},
		{
			name: "missing mandatory keys",
			source: oc.Source{
				"repo": "dummy-repo",
			},
			wantErr: "missing source keys: [access_token owner]",
		},
		{
			name: "all mandatory keys, one unknown key",
			source: oc.Source{
				"access_token": "dummy-token",
				"owner":        "dummy-owner",
				"repo":         "dummy-repo",

				"pizza": "napoli",
			},
			wantErr: "unknown source keys: [pizza]",
		},
		{
			name: "one missing mandatory key, one unknown key",
			source: oc.Source{
				"owner": "dummy-owner",
				"repo":  "dummy-repo",

				"pizza": "napoli",
			},
			wantErr: "missing source keys: [access_token]; unknown source keys: [pizza]",
		},
		{
			name: "wrong type is reported as missing (better than crashing)",
			source: oc.Source{
				"access_token": "dummy-token",
				"owner":        3,
				"repo":         "dummy-repo",
			},
			wantErr: "missing source keys: [owner]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := validateSource(tc.source)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Errorf("error message mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}
