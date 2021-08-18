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
		{"happy path",
			oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo},
			defVersion,
			defVersions,
			nil},
		{"do not return a nil version the first time it runs (see Concourse PR #4442)",
			oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo},
			oc.Version{},
			defVersions,
			nil},
		{"missing mandatory sources",
			oc.Source{},
			defVersion,
			nil,
			&missingSourceError{},
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
	defSource := oc.Source{"access_token": "dummy", "owner": "dummy", "repo": "dummy"}

	var testCases = []struct {
		name      string
		inVersion oc.Version
	}{
		{"happy path", defVersion},
		{"do not return a nil version the first time it runs (see Concourse PR #4442)",
			oc.Version{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Resource{}

			version, metadata, err := r.In("/tmp", defSource, oc.Params{}, tc.inVersion, defEnv, silentLog)

			if err != nil {
				t.Fatalf("err: got %v; want %v", err, nil)
			}
			if diff := cmp.Diff(defVersion, version); diff != "" {
				t.Errorf("version: (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(oc.Metadata{}, metadata); diff != "" {
				t.Errorf("metadata: (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOut(t *testing.T) {
	cfg := help.FakeTestCfg

	defSource := oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo}
	defParams := oc.Params{"state": "error"}
	defMeta := oc.Metadata{oc.NameVal{Name: "state", Value: "error"}}
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
		{"valid mandatory sources",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{"missing mandatory sources",
			in{oc.Source{}, defParams, defEnv},
			want{nil, nil, nil, &missingSourceError{}},
		},
		{"unknown source",
			in{oc.Source{"access_token": "x", "owner": "a", "repo": "b", "pizza": "napoli"},
				defParams, defEnv},
			want{nil, nil, nil, &unknownSourceError{}},
		},
		{"valid mandatory parameters",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{"completely missing mandatory parameters",
			in{defSource, oc.Params{}, defEnv},
			want{nil, nil, nil, &missingParamError{}},
		},
		{"invalid state parameter",
			in{defSource, oc.Params{"state": "hello"}, defEnv},
			want{nil, nil, nil, &invalidParamError{}},
		},
		{"unknown parameter",
			in{defSource, oc.Params{"state": "pending", "pizza": "margherita"}, defEnv},
			want{nil, nil, nil, &unknownParamError{}},
		},
		{"do not return a nil version the first time it runs (see Concourse PR #4442)",
			in{defSource, defParams, defEnv},
			want{defVersion, defMeta, nil, nil},
		},
		{"source: optional: context_prefix",
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
		{"put step: default context",
			in{defSource, defParams, defEnv},
			want{
				defVersion,
				defMeta,
				map[string]string{"context": defEnv.Get("BUILD_JOB_NAME")},
				nil,
			},
		},
		{"put step: optional: context",
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

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			}))
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
				t.Fatalf("\ngot: %v (%v)\nwant: %v (%v)", gotErrType, err, wantErrType, tc.want.err)
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

func TestOutE2E(t *testing.T) {
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
		{"backend reports success",
			in{defSource, defParams, defEnv},
			nil,
		},
		{"backend reports failure",
			in{
				oc.Source{
					"access_token": cfg.Token,
					"owner":        cfg.Owner,
					"repo":         "does-not-exists-really"},
				defParams,
				defEnv},
			errWrongRemote,
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

func TestCollectInputDirs(t *testing.T) {
	var testCases = []struct {
		name    string
		dir     string
		wantErr error
		wantN   int
	}{
		{"non existing base directory", "non-existing", os.ErrNotExist, 0},
		{"empty directory", "testdata/empty-dir", nil, 0},
		{"two directories and one file", "testdata/two-dirs", nil, 2},
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

func TestRepoDirMatches(t *testing.T) {
	const wantOwner = "smiling"
	const wantRepo = "butterfly"
	type testCase struct {
		name      string
		dir       string
		inRepoURL string
		wantErr   error
	}
	testCases := []testCase{
		{"dir is not a repo",
			"not-a-repo",
			"dummyurl",
			os.ErrNotExist,
		},
		{"bad .git/config",
			"repo-bad-git-config",
			"dummyurl",
			errKeyNotFound,
		},
		{"repo with wrong HTTPS remote",
			"a-repo",
			httpsRemote("owner", "repo"),
			errWrongRemote,
		},
		{"repo with wrong SSH remote or wrong source config",
			"a-repo",
			sshRemote("owner", "repo"),
			errWrongRemote,
		},
		{"repo with good SSH remote",
			"a-repo",
			sshRemote(wantOwner, wantRepo),
			nil,
		},
		{"repo with good HTTPS remote",
			"a-repo",
			httpsRemote(wantOwner, wantRepo),
			nil,
		},
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc.dir, tc.inRepoURL, "dummySHA", "dummyHead")
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := repodirMatches(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error: got %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGitCommit(t *testing.T) {
	const wantSHA = "af6cd86e98eb1485f04d38b78d9532e916bbff02"
	const defHead = "ref: refs/heads/a-branch-FIXME"
	type testCase struct {
		name      string
		dir       string
		inRepoURL string
		inHead    string
		wantErr   error
	}
	testCases := []testCase{
		{"missing HEAD", "not-a-repo", "dummy", "dummy", os.ErrNotExist},
		{"happy path for branch checkout", "a-repo", "dummy", defHead, nil},
		{"happy path for detached HEAD checkout", "a-repo", "dummy", wantSHA, nil},
		{"invalid format for HEAD", "a-repo", "dummyURL", "this is a bad head", errInvalidHead},
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc.dir, tc.inRepoURL, wantSHA, tc.inHead)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			gotRef, gotErr := GitCommit(filepath.Join(inDir, tc.dir))

			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("err: got %v; want %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if gotRef != wantSHA {
				t.Fatalf("ref: got %q; want %q", gotRef, wantSHA)
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
) (string, func(t *testing.T)) {
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

func TestParseGitPseudoURL(t *testing.T) {
	testCases := []struct {
		name    string
		inURL   string
		wantGU  gitURL
		wantErr error
	}{
		{"totally invalid URL",
			"hello",
			gitURL{},
			errInvalidURL,
		},
		{"valid SSH URL",
			"git@github.com:Pix4D/cogito.git",
			gitURL{"ssh", "github.com", "Pix4D", "cogito"},
			nil,
		},
		{"invalid SSH URL",
			"git@github.com/Pix4D/cogito.git",
			gitURL{},
			errInvalidURL,
		},
		{"valid HTTP URL",
			"https://github.com/Pix4D/cogito.git",
			gitURL{"https", "github.com", "Pix4D", "cogito"},
			nil,
		},
		{"invalid HTTP URL",
			"https://github.com:Pix4D/cogito.git",
			gitURL{},
			errInvalidURL,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gu, err := parseGitPseudoURL(tc.inURL)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v; want %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantGU, gu); diff != "" {
				t.Errorf("gitURL: (-want +got):\n%s", diff)
			}
		})
	}
}
