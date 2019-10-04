package resource

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/help"
	oc "github.com/cloudboss/ofcourse/ofcourse"
	"github.com/google/go-cmp/cmp"
)

var (
	silentLog = oc.NewLogger(oc.SilentLevel)

	defaultVersion  = oc.Version{"ref": "dummy"}
	defaultversions = []oc.Version{defaultVersion}
	defaultEnv      = oc.NewEnvironment(
		map[string]string{"ATC_EXTERNAL_URL": "https://cogito.invalid"})
)

func TestCheck(t *testing.T) {
	r := Resource{}
	versions, err := r.Check(oc.Source{}, oc.Version{}, defaultEnv, silentLog)

	if diff := cmp.Diff(defaultversions, versions); diff != "" {
		t.Errorf("version: (-want +got):\n%s", diff)
	}
	if err != nil {
		t.Errorf("err: got %v; want %v", err, nil)
	}
}

func TestIn(t *testing.T) {
	r := Resource{}
	version, metadata, err := r.In(
		"/tmp", oc.Source{}, oc.Params{}, defaultVersion, defaultEnv, silentLog)

	if diff := cmp.Diff(defaultVersion, version); diff != "" {
		t.Errorf("version: (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(oc.Metadata{}, metadata); diff != "" {
		t.Errorf("metadata: (-want +got):\n%s", diff)
	}
	if err != nil {
		t.Errorf("err: got %v; want %v", err, nil)
	}
}

// For the time being this is an end-to-end test only. Will add a fake version soon.
// See README for how to enable end-to-end tests.
func TestOut(t *testing.T) {
	cfg := github.SkipTestIfNoEnvVars(t)

	defaultSource := oc.Source{"access_token": cfg.Token, "owner": cfg.Owner, "repo": cfg.Repo}
	defaultParams := oc.Params{"state": "error"}
	defaultMeta := oc.Metadata{oc.NameVal{Name: "state", Value: "error"}}

	type in struct {
		source oc.Source
		params oc.Params
		env    oc.Environment
	}
	type want struct {
		version  oc.Version
		metadata oc.Metadata
		err      error
	}
	var testCases = []struct {
		name string
		in   in
		want want
	}{
		{
			"valid mandatory sources",
			in{defaultSource, defaultParams, defaultEnv},
			want{defaultVersion, defaultMeta, nil},
		},
		{
			"missing mandatory sources",
			in{oc.Source{}, defaultParams, defaultEnv},
			want{nil, nil, &missingSourceError{}},
		},
		{
			"unknown source",
			in{oc.Source{
				"access_token": "x", "owner": "a", "repo": "b", "pizza": "napoli"},
				defaultParams,
				defaultEnv},
			want{nil, nil, &unknownSourceError{}},
		},

		{
			"valid mandatory parameters",
			in{defaultSource, defaultParams, defaultEnv},
			want{defaultVersion, defaultMeta, nil},
		},
		{
			"completely missing mandatory parameters",
			in{defaultSource, oc.Params{}, defaultEnv},
			want{nil, nil, &missingParamError{}},
		},
		{
			"invalid state parameter",
			in{defaultSource, oc.Params{"state": "hello"}, defaultEnv},
			want{nil, nil, &invalidParamError{}},
		},
		{
			"unknown parameter",
			in{
				defaultSource,
				oc.Params{"state": "pending", "pizza": "margherita"},
				defaultEnv,
			},
			want{nil, nil, &unknownParamError{}},
		},
	}

	// Per-subtest setup and teardown.
	setup := func(t *testing.T) (string, func(t *testing.T)) {
		// Make a temp dir to be the resource work directory
		inDir, err := ioutil.TempDir("", "cogito-test-")
		if err != nil {
			t.Fatal("Temp dir", err)
		}
		// Copy the testdata over
		const repo = "repo-with-ssh-remote"
		err = help.CopyDir(inDir, filepath.Join("testdata", repo), help.DotRenamer, nil)
		if err != nil {
			t.Fatal("CopyDir:", err)
		}
		// Make fake file '.git/ref' normally added by the git resource
		refPath := filepath.Join(inDir, repo, ".git/ref")
		sha := []byte(cfg.Sha + "\n")
		if err := ioutil.WriteFile(refPath, sha, 0660); err != nil {
			t.Fatal("setup: writing ref file", err)
		}

		teardown := func(t *testing.T) {
			defer os.RemoveAll(inDir)
		}
		return inDir, teardown
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inDir, teardown := setup(t)
			defer teardown(t)

			r := Resource{}
			version, metadata, err := r.Out(
				inDir, tc.in.source, tc.in.params, tc.in.env, silentLog)

			gotErrType := reflect.TypeOf(err)
			wantErrType := reflect.TypeOf(tc.want.err)
			if gotErrType != wantErrType {
				t.Fatalf("err: got %v (%v);\nwant %v (%v)", gotErrType, err, wantErrType, tc.want.err)
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
		{"dir is not a repo", "not-a-repo", "dummyurl", os.ErrNotExist},
		{"bad .git/config", "repo-bad-git-config", "dummyurl", errKeyNotFound},
		{"repo with wrong HTTPS remote", "a-repo", https_remote("owner", "repo"), errWrongRemote},
		{"repo with wrong SSH remote", "a-repo", ssh_remote("owner", "repo"), errWrongRemote},
		{"repo with good SSH remote", "a-repo", ssh_remote(wantOwner, wantRepo), nil},
		{"repo with good HTTPS remote", "a-repo", https_remote(wantOwner, wantRepo), nil},
	}

	// Per-subtest setup and teardown.
	setup := func(t *testing.T, tc testCase) (string, func(t *testing.T)) {
		// Make a temp dir to be the resource work directory
		inDir, err := ioutil.TempDir("", "cogito-test-")
		if err != nil {
			t.Fatal("Temp dir", err)
		}
		tdata := make(help.TemplateData)
		tdata["repo_url"] = tc.inRepoURL

		// Copy the testdata over
		err = help.CopyDir(inDir, filepath.Join("testdata", tc.dir), help.DotRenamer, tdata)
		if err != nil {
			t.Fatal("CopyDir:", err)
		}

		teardown := func(t *testing.T) {
			defer os.RemoveAll(inDir)
		}
		return inDir, teardown
	}

	for _, tc := range testCases {
		inDir, teardown := setup(t, tc)
		defer teardown(t)

		t.Run(tc.name, func(t *testing.T) {
			err := repodirMatches(filepath.Join(inDir, tc.dir), wantOwner, wantRepo)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error: got %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func ssh_remote(owner, repo string) string {
	return fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
}

func https_remote(owner, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

func TestParseGitPseudoURL(t *testing.T) {
	testCases := []struct {
		name    string
		inURL   string
		wantGU  gitURL
		wantErr error
	}{
		{"totally invalid URL", "hello", gitURL{}, errInvalidURL},
		{"valid SSH URL", "git@github.com:Pix4D/cogito.git",
			gitURL{"ssh", "github.com", "Pix4D", "cogito"}, nil},
		{"invalid SSH URL", "git@github.com/Pix4D/cogito.git", gitURL{}, errInvalidURL},
		{"valid HTTP URL", "https://github.com/Pix4D/cogito.git",
			gitURL{"https", "github.com", "Pix4D", "cogito"}, nil},
		{"invalid HTTP URL", "https://github.com:Pix4D/cogito.git", gitURL{}, errInvalidURL},
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

func TestParseGitRef(t *testing.T) {

	var testCases = []struct {
		name    string
		in      string
		wantRef string
		wantTag string
		wantErr error
	}{
		{"only ref present (no tag)",
			"af6cd86e98eb1485f04d38b78d9532e916bbff02\n",
			"af6cd86e98eb1485f04d38b78d9532e916bbff02",
			"",
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotRef, gotTag, gotErr := parseGitRef(tc.in)
			if gotErr != tc.wantErr {
				t.Errorf("err: got %v; want %v", gotErr, tc.wantErr)
			}
			if gotRef != tc.wantRef {
				t.Errorf("ref: got %q; want %q", gotRef, tc.wantRef)
			}
			if gotTag != tc.wantTag {
				t.Errorf("tag: got %q; want %q", gotTag, tc.wantTag)
			}
		})
	}
}
