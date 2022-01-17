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
	"reflect"
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

func TestCheck(t *testing.T) {
	cfg := help.FakeTestCfg

	var testCases = []struct {
		name         string
		inSource     oc.Source
		inVersion    oc.Version
		wantVersions []oc.Version
		wantErr      error
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
			wantErr:      nil,
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
			wantErr:      nil,
		},
		{
			name:         "missing mandatory sources",
			inSource:     oc.Source{},
			inVersion:    defVersion,
			wantVersions: nil,
			wantErr:      &missingSourceError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Resource{}

			versions, err := r.Check(tc.inSource, tc.inVersion, defEnv, silentLog)

			gotErrType := reflect.TypeOf(err)
			wantErrType := reflect.TypeOf(tc.wantErr)
			if gotErrType != wantErrType {
				t.Fatalf("err: got %v (%v);\nwant %v (%v)",
					gotErrType, err, wantErrType, tc.wantErr)
			}

			if diff := cmp.Diff(tc.wantVersions, versions); diff != "" {
				t.Fatalf("version: (-want +got):\n%s", diff)
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

func TestOut(t *testing.T) {
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

	type in struct {
		source oc.Source
		params oc.Params
		env    oc.Environment
	}
	type want struct {
		version  oc.Version
		metadata oc.Metadata
		body     map[string]string
		err      error
	}
	var testCases = []struct {
		name string
		in   in
		want want
	}{
		{
			"valid mandatory sources",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{
			"missing mandatory sources",
			in{oc.Source{}, defParams, defEnv},
			want{nil, nil, nil, &missingSourceError{}},
		},
		{
			"unknown source",
			in{oc.Source{"access_token": "x", "owner": "a", "repo": "b", "pizza": "napoli"},
				defParams, defEnv},
			want{nil, nil, nil, &unknownSourceError{}},
		},
		{
			"valid mandatory parameters",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{
			"completely missing mandatory parameters",
			in{defSource, oc.Params{}, defEnv},
			want{nil, nil, nil, &missingParamError{}},
		},
		{
			"invalid state parameter",
			in{defSource, oc.Params{"state": "hello"}, defEnv},
			want{nil, nil, nil, &invalidParamError{}},
		},
		{
			"unknown parameter",
			in{defSource, oc.Params{"state": "pending", "pizza": "margherita"}, defEnv},
			want{nil, nil, nil, &unknownParamError{}},
		},
		{
			"do not return a nil version the first time it runs (see Concourse PR #4442)",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{
			"source: optional: context_prefix",
			in{
				oc.Source{
					"access_token":   cfg.Token,
					"owner":          cfg.Owner,
					"repo":           cfg.Repo,
					"context_prefix": "cocco"},
				defParams,
				defEnv,
			},
			want{
				defVersion,
				defMeta,
				map[string]string{"context": "cocco/" + defEnv.Get("BUILD_JOB_NAME")},
				nil,
			},
		},
		{
			"put step: default context",
			in{defSource, defParams, defEnv},
			want{
				defVersion,
				defMeta,
				map[string]string{"context": defEnv.Get("BUILD_JOB_NAME")},
				nil,
			},
		},
		{
			"put step: optional: context",
			in{defSource, oc.Params{"state": "error", "context": "bello"}, defEnv},
			want{
				defVersion,
				defMeta,
				map[string]string{"context": "bello"},
				nil,
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

					if tc.want.body != nil {
						buf, _ := io.ReadAll(r.Body)
						var bm map[string]string
						if err := json.Unmarshal(buf, &bm); err != nil {
							t.Fatalf("parsing JSON body: %v", err)
						}
						for k, v := range tc.want.body {
							if bm[k] != v {
								t.Errorf("\nbody[%q]: got: %q; want: %q", k, bm[k], v)
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

			r := Resource{}
			version, metadata, err := r.Out(
				inDir, tc.in.source, tc.in.params, tc.in.env, silentLog)

			gotErrType := reflect.TypeOf(err)
			wantErrType := reflect.TypeOf(tc.want.err)
			if gotErrType != wantErrType {
				t.Fatalf("\ngot: %v (%v)\nwant: %v (%v)",
					gotErrType, err, wantErrType, tc.want.err)
			}

			if diff := cmp.Diff(tc.want.version, version); diff != "" {
				t.Errorf("version: (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.want.metadata, metadata); diff != "" {
				t.Errorf("metadata: (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOutIntegration(t *testing.T) {
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
	var testCases = []struct {
		name    string
		in      in
		wantErr error
	}{
		{
			name:    "backend reports success",
			in:      in{defSource, defParams, defEnv},
			wantErr: nil,
		},
		{
			name: "backend reports failure",
			in: in{
				oc.Source{
					"access_token": cfg.Token,
					"owner":        cfg.Owner,
					"repo":         "does-not-exists-really"},
				defParams,
				defEnv},
			wantErr: errWrongRemote,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t, defDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)
			defer teardown(t)

			r := Resource{}
			_, _, err := r.Out(inDir, tc.in.source, tc.in.params, tc.in.env, silentLog)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("\ngot:  %v\nwant: no error", err)
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("\ngot:  %v\nwant: %v", err, tc.wantErr)
				}
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

			if got := targetURL(atc, team, pipeline, job, buildN, tc.instanceVars); got != tc.want {
				t.Fatalf("\ngot:  %s\nwant: %s", got, tc.want)
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
				t.Errorf("sut(%v): error: got %v; want %v", tc.dir, err, tc.wantErr)
			}
			gotN := len(dirs)
			if gotN != tc.wantN {
				t.Errorf("sut(%v): len(dirs): got %v; want %v", tc.dir, gotN, tc.wantN)
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
		name    string
		dir     string
		repoURL string
		wantErr error
	}{
		{
			name:    "dir is not a repo",
			dir:     "not-a-repo",
			repoURL: "dummyurl",
			wantErr: os.ErrNotExist,
		},
		{
			name:    "bad .git/config",
			dir:     "repo-bad-git-config",
			repoURL: "dummyurl",
			wantErr: errKeyNotFound,
		},
		{
			name:    "repo with wrong HTTPS remote",
			dir:     "a-repo",
			repoURL: httpsRemote("owner", "repo"),
			wantErr: errWrongRemote,
		},
		{
			name:    "repo with wrong SSH remote or wrong source config",
			dir:     "a-repo",
			repoURL: sshRemote("owner", "repo"),
			wantErr: errWrongRemote,
		},
		{
			name:    "invalid git pseudo URL in .git/config",
			dir:     "a-repo",
			repoURL: "foo://bar",
			wantErr: errInvalidURL,
		},
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc.dir, tc.repoURL, "dummySHA", "dummyHead")
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := checkRepoDir(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error: got %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGitCommitSuccess(t *testing.T) {
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
			sha, err := GitCommit(filepath.Join(dir, tc.dir))

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
			if sha != wantSHA {
				t.Fatalf("ref: have: %s; want: %s", sha, wantSHA)
			}
		})
	}
}

func TestGitCommitFailure(t *testing.T) {
	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"

	testCases := []struct {
		name    string
		dir     string
		repoURL string
		head    string
		wantErr error
	}{
		{
			name:    "missing HEAD",
			dir:     "not-a-repo",
			repoURL: "dummy",
			head:    "dummy",
			wantErr: os.ErrNotExist,
		},
		{
			name:    "invalid format for HEAD",
			dir:     "a-repo",
			repoURL: "dummyURL",
			head:    "this is a bad head",
			wantErr: errInvalidHead,
		},
	}

	for _, tc := range testCases {
		dir, teardown := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			_, err := GitCommit(filepath.Join(dir, tc.dir))

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", tc.wantErr)
			}

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v; want %v", err, tc.wantErr)
			}
		})
	}
}

// Per-subtest setup and teardown.
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

func sshRemote(owner, repo string) string {
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
}

func httpsRemote(owner, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

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
				t.Errorf("gitURL: (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseGitPseudoURLFailure(t *testing.T) {
	testCases := []struct {
		name    string
		inURL   string
		wantGU  gitURL
		wantErr error
	}{
		{
			name:    "totally invalid URL",
			inURL:   "hello",
			wantGU:  gitURL{},
			wantErr: errInvalidURL,
		},
		{
			name:    "invalid SSH URL",
			inURL:   "git@github.com/Pix4D/cogito.git",
			wantGU:  gitURL{},
			wantErr: errInvalidURL,
		},
		{
			name:    "invalid HTTPS URL",
			inURL:   "https://github.com:Pix4D/cogito.git",
			wantGU:  gitURL{},
			wantErr: errInvalidURL,
		},
		{
			name:    "invalid HTTP URL",
			inURL:   "http://github.com:Pix4D/cogito.git",
			wantGU:  gitURL{},
			wantErr: errInvalidURL,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseGitPseudoURL(tc.inURL)

			if err == nil {
				t.Fatalf("have: <no error>; want: %v", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v; want %v", err, tc.wantErr)
			}
		})
	}
}
