package resource

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	oc "github.com/cloudboss/ofcourse/ofcourse"
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
	defDir := "a-repo"

	testCases := []struct {
		name         string
		source       oc.Source
		params       oc.Params
		env          oc.Environment
		wantVersion  oc.Version
		wantMetadata oc.Metadata
		wantBody     map[string]string
	}{
		{
			name:         "valid mandatory sources and parameters",
			source:       defSource,
			params:       defParams,
			env:          defEnv,
			wantVersion:  defVersion,
			wantMetadata: defMeta,
			wantBody:     nil,
		},
		{
			name:         "do not return a nil version the first time it runs (see Concourse PR #4442)",
			source:       defSource,
			params:       defParams,
			env:          defEnv,
			wantVersion:  defVersion,
			wantMetadata: defMeta,
			wantBody:     nil,
		},
		{
			name: "source: optional: context_prefix",
			source: oc.Source{
				"access_token":   cfg.Token,
				"owner":          cfg.Owner,
				"repo":           cfg.Repo,
				"context_prefix": "cocco"},
			params:       defParams,
			env:          defEnv,
			wantVersion:  defVersion,
			wantMetadata: defMeta,
			wantBody: map[string]string{
				"context": "cocco/" + defEnv.Get("BUILD_JOB_NAME"),
			},
		},
		{
			name:         "put step: default context",
			source:       defSource,
			params:       defParams,
			env:          defEnv,
			wantVersion:  defVersion,
			wantMetadata: defMeta,
			wantBody: map[string]string{
				"context": defEnv.Get("BUILD_JOB_NAME"),
			},
		},
		{
			name:   "put step: optional: context",
			source: defSource,
			params: oc.Params{
				"state":   "error",
				"context": "bello",
			},
			env:          defEnv,
			wantVersion:  defVersion,
			wantMetadata: defMeta,
			wantBody: map[string]string{
				"context": "bello",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, defDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintln(w, "Anything goes...")

					if tc.wantBody != nil {
						buf, _ := io.ReadAll(r.Body)
						var bm map[string]string
						if err := json.Unmarshal(buf, &bm); err != nil {
							t.Fatalf("parsing JSON body: %v", err)
						}
						for k, v := range tc.wantBody {
							if bm[k] != v {
								t.Errorf("\nbody[%q]: have: %q; want: %q", k, bm[k], v)
							}
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
				inDir, tc.source, tc.params, tc.env, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}

			if diff := cmp.Diff(tc.wantVersion, version); diff != "" {
				t.Errorf("version: (-want +have):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantMetadata, metadata); diff != "" {
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
	defDir := "a-repo"

	var testCases = []struct {
		name     string
		source   oc.Source
		params   oc.Params
		env      oc.Environment
		wantBody map[string]string
		wantErr  string
	}{
		{
			name:     "missing mandatory source keys",
			source:   oc.Source{},
			params:   defParams,
			env:      defEnv,
			wantBody: nil,
			wantErr:  "missing source keys: [access_token owner repo]",
		},
		{
			name:     "missing mandatory parameters",
			source:   defSource,
			params:   oc.Params{},
			env:      defEnv,
			wantBody: nil,
			wantErr:  "missing put parameter 'state'",
		},
		{
			name:   "invalid state parameter",
			source: defSource,
			params: oc.Params{
				"state": "hello",
			},
			env:      defEnv,
			wantBody: nil,
			wantErr:  "invalid put parameter 'state: hello'",
		},
		{
			name:   "unknown parameter",
			source: defSource,
			params: oc.Params{
				"state": "pending",
				"pizza": "margherita",
			},
			env:      defEnv,
			wantBody: nil,
			wantErr:  "unknown put parameter 'pizza'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, defDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			ts := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintln(w, "Anything goes...")

					if tc.wantBody != nil {
						buf, _ := io.ReadAll(r.Body)
						var bm map[string]string
						if err := json.Unmarshal(buf, &bm); err != nil {
							t.Fatalf("parsing JSON body: %v", err)
						}
						for k, v := range tc.wantBody {
							if bm[k] != v {
								t.Errorf("\nbody[%q]: have: %q; want: %q", k, bm[k], v)
							}
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
			_, _, err := res.Out(
				inDir, tc.source, tc.params, tc.env, silentLog)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}

			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error msg mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestOutIntegrationSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)

	defSource := oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo}
	defParams := oc.Params{"state": "error"}
	defDir := "a-repo"

	type in struct {
		source oc.Source
		params oc.Params
		env    oc.Environment
	}

	testCases := []struct {
		name string
		in   in
	}{
		{
			name: "backend reports success",
			in:   in{defSource, defParams, defEnv},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, defDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			r := Resource{}
			_, _, err := r.Out(inDir, tc.in.source, tc.in.params, tc.in.env, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestOutIntegrationFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)

	defParams := oc.Params{"state": "error"}
	defDir := "a-repo"

	type in struct {
		source oc.Source
		params oc.Params
		env    oc.Environment
	}

	testCases := []struct {
		name    string
		in      in
		wantErr string
	}{
		{
			name: "backend reports failure",
			in: in{
				oc.Source{
					"access_token": cfg.Token,
					"owner":        cfg.Owner,
					"repo":         "does-not-exists-really"},
				defParams,
				defEnv},
			wantErr: `resource source configuration and git repository are incompatible.
Git remote: "git@github.com:pix4d/cogito-test-read-write.git"
Resource config: host: github.com, owner: "pix4d", repo: "does-not-exists-really". wrong git remote`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, defDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			r := Resource{}
			_, _, err := r.Out(inDir, tc.in.source, tc.in.params, tc.in.env, silentLog)

			if err == nil {
				t.Fatalf("have: <no error>\nwant: %s", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error msg mismatch: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestTargetURL(t *testing.T) {
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

			if have := targetURL(atc, team, pipeline, job, buildN, tc.instanceVars); have != tc.want {
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
		dir     string
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
		name      string
		dir       string
		repoURL   string // repoURL to put in file <dir>/.git/config
		wantErrRe string // regexp
	}{
		{
			name:      "dir is not a repo",
			dir:       "not-a-repo",
			repoURL:   "dummyurl",
			wantErrRe: `parsing .git/config: open (\S+)/not-a-repo/.git/config: no such file or directory`,
		},
		{
			name:      "bad file .git/config",
			dir:       "repo-bad-git-config",
			repoURL:   "dummyurl",
			wantErrRe: `.git/config: key \[remote "origin"\]/url: not found`,
		},
		{
			name:    "repo with unrelated HTTPS remote",
			dir:     "a-repo",
			repoURL: httpsRemote("owner", "repo"),
			wantErrRe: `resource source configuration and git repository are incompatible.
Git remote: "https://github.com/owner/repo.git"
Resource config: host: github.com, owner: "smiling", repo: "butterfly". wrong git remote`,
		},
		{
			name:    "repo with unrelated SSH remote or wrong source config",
			dir:     "a-repo",
			repoURL: sshRemote("owner", "repo"),
			wantErrRe: `resource source configuration and git repository are incompatible.
Git remote: "git@github.com:owner/repo.git"
Resource config: host: github.com, owner: "smiling", repo: "butterfly". wrong git remote`,
		},
		{
			name:      "invalid git pseudo URL in .git/config",
			dir:       "a-repo",
			repoURL:   "foo://bar",
			wantErrRe: `.git/config: remote: invalid git URL foo://bar: no valid scheme`,
		},
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc.dir, tc.repoURL, "dummySHA", "dummyHead")
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := checkRepoDir(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErrRe)
			}

			have := err.Error()
			re := regexp.MustCompile(tc.wantErrRe)
			if !re.MatchString(have) {
				if diff := cmp.Diff(tc.wantErrRe, have); diff != "" {
					t.Fatalf("error msg regexp mismatch: (-want +have):\n%s", diff)
				}
				t.Fatalf("error msg regexp\nhave: %s\nwant: %s", have, tc.wantErrRe)
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
		name      string
		dir       string
		repoURL   string
		head      string
		wantErrRe string // regexp
	}{
		{
			name:      "missing HEAD",
			dir:       "not-a-repo",
			repoURL:   "dummy",
			head:      "dummy",
			wantErrRe: `git commit: read HEAD: open (\S+)/not-a-repo/.git/HEAD: no such file or directory`,
		},
		{
			name:      "invalid format for HEAD",
			dir:       "a-repo",
			repoURL:   "dummyURL",
			head:      "this is a bad head",
			wantErrRe: `git commit: invalid HEAD format: "this is a bad head"`,
		},
	}

	for _, tc := range testCases {
		dir, teardown := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			_, err := GitGetCommit(filepath.Join(dir, tc.dir))

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErrRe)
			}

			have := err.Error()
			re := regexp.MustCompile(tc.wantErrRe)
			if !re.MatchString(have) {
				diff := cmp.Diff(tc.wantErrRe, have)
				t.Fatalf("error msg regexp mismatch: (-want +have):\n%s", diff)
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
			name:   "valid SSH URL",
			inURL:  "git@github.com:Pix4D/cogito.git",
			wantGU: gitURL{"ssh", "github.com", "Pix4D", "cogito"},
		},
		{
			name:   "valid HTTPS URL",
			inURL:  "https://github.com/Pix4D/cogito.git",
			wantGU: gitURL{"https", "github.com", "Pix4D", "cogito"},
		},
		{
			name:   "valid HTTP URL",
			inURL:  "http://github.com/Pix4D/cogito.git",
			wantGU: gitURL{"http", "github.com", "Pix4D", "cogito"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gitUrl, err := parseGitPseudoURL(tc.inURL)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
			if diff := cmp.Diff(tc.wantGU, gitUrl); diff != "" {
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
			wantErr: "invalid git URL hello: no valid scheme",
		},
		{
			name:    "invalid SSH URL",
			inURL:   "git@github.com/Pix4D/cogito.git",
			wantErr: "invalid git SSH URL git@github.com/Pix4D/cogito.git: want exactly one ':'",
		},
		{
			name:    "invalid HTTPS URL",
			inURL:   "https://github.com:Pix4D/cogito.git",
			wantErr: "invalid git URL: path: want: 3 components; have: 2 [github.com:Pix4D cogito.git]",
		},
		{
			name:    "invalid HTTP URL",
			inURL:   "http://github.com:Pix4D/cogito.git",
			wantErr: "invalid git URL: path: want: 3 components; have: 2 [github.com:Pix4D cogito.git]",
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
