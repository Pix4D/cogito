package resource

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	oc "github.com/cloudboss/ofcourse/ofcourse"
	"github.com/gertd/wild"
	"github.com/google/go-cmp/cmp"

	"github.com/Pix4D/cogito/help"
)

var (
	silentLog = oc.NewLogger(oc.SilentLevel)

	defVersion = oc.Version{"ref": "dummy"}
	defEnv     = oc.NewEnvironment(
		map[string]string{
			"ATC_EXTERNAL_URL": "https://cogito.invalid",
			"BUILD_JOB_NAME":   "a-job"})
)

func TestOutMockSuccess(t *testing.T) {
	cfg := help.FakeTestCfg

	defSource := oc.Source{
		accessTokenKey: cfg.Token,
		ownerKey:       cfg.Owner,
		repoKey:        cfg.Repo,
	}
	defParams := oc.Params{
		stateKey: errorState,
	}
	defMeta := oc.Metadata{oc.NameVal{
		Name: stateKey, Value: errorState},
	}
	defWantBody := map[string]string{
		contextKey: defEnv.Get("BUILD_JOB_NAME"),
	}

	testDir := "a-repo"

	testCases := []struct {
		name     string
		source   oc.Source
		params   oc.Params
		wantMeta oc.Metadata
		wantBody map[string]string
	}{
		{
			name: "source: optional: context_prefix",
			source: help.MergeMap(defSource, oc.Source{
				contextPrefixKey: "cocco"},
			),
			wantBody: map[string]string{
				contextKey: "cocco/" + defEnv.Get("BUILD_JOB_NAME"),
			},
		},
		{
			name: "params: optional: context",
			params: help.MergeMap(defParams, oc.Params{
				contextKey: "bello",
			}),
			wantBody: map[string]string{
				contextKey: "bello",
			},
		},
		{
			name: "cogito states are converted to gh commit states",
			params: oc.Params{
				stateKey: abortState,
			},
			wantMeta: oc.Metadata{oc.NameVal{
				Name: stateKey, Value: abortState},
			},
			wantBody: map[string]string{
				contextKey: defEnv.Get("BUILD_JOB_NAME"),
				stateKey:   errorState,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)

			if tc.source == nil {
				tc.source = defSource
			}
			if tc.params == nil {
				tc.params = defParams
			}
			if tc.wantMeta == nil {
				tc.wantMeta = defMeta
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

			defer func() {
				ts.Close()
			}()

			res := NewWith(ts.URL)

			version, metadata, err := res.Out(
				inDir, tc.source, tc.params, defEnv, silentLog)

			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}

			if diff := cmp.Diff(defVersion, version); diff != "" {
				t.Errorf("version: (-want +have):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantMeta, metadata); diff != "" {
				t.Errorf("metadata: (-want +have):\n%s", diff)
			}
		})
	}
}

func TestOutSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)
	gchatHook := os.Getenv("COGITO_TEST_GCHAT_HOOK")

	defSource := oc.Source{
		accessTokenKey: cfg.Token,
		ownerKey:       cfg.Owner,
		repoKey:        cfg.Repo,
	}
	defParams := oc.Params{
		stateKey: errorState,
	}
	testDir := "a-repo"

	testCases := []struct {
		name   string
		source oc.Source
		params oc.Params
	}{
		{
			name: "github backend reports success",
		},
		{
			name: "github and gchat backends report success",
			source: help.MergeMap(defSource, oc.Source{
				gchatWebhookKey: gchatHook,
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)

			if tc.source == nil {
				tc.source = defSource
			}
			if tc.params == nil {
				tc.params = defParams
			}

			r := New()
			_, _, err := r.Out(inDir, tc.source, tc.params, defEnv, silentLog)

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

	defParams := oc.Params{
		stateKey: errorState,
	}
	testDir := "a-repo"

	testCases := []struct {
		name    string
		source  oc.Source
		params  oc.Params
		wantErr string
	}{
		{
			name: "local validations fail",
			source: oc.Source{
				accessTokenKey: cfg.Token,
				ownerKey:       cfg.Owner,
				repoKey:        "does-not-exist-really",
			},
			wantErr: `the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: git@github.com:pix4d/cogito-test-read-write.git
    owner: pix4d
     repo: cogito-test-read-write

Cogito SOURCE configuration:
    owner: pix4d
     repo: does-not-exist-really`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir := setup(t, testDir, sshRemote(cfg.Owner, cfg.Repo), cfg.SHA, cfg.SHA)

			if tc.params == nil {
				tc.params = defParams
			}

			r := New()
			_, _, err := r.Out(inDir, tc.source, tc.params, defEnv, silentLog)

			if err == nil {
				t.Fatalf("have: <no error>\nwant: %s", tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error msg mismatch: (-want +have):\n%s", diff)
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
		dir := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)

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
		dir := setup(t, tc.dir, tc.repoURL, wantSHA, tc.head)

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
