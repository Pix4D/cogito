package resource

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Pix4D/cogito/github"
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
	defaultParams := oc.Params{"input-repo": cfg.Repo, "state": "error"}
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
			"partially missing mandatory parameters",
			in{defaultSource, oc.Params{"input-repo": cfg.Repo}, defaultEnv},
			want{nil, nil, &missingParamError{}},
		},
		{
			"completely missing mandatory parameters",
			in{defaultSource, oc.Params{}, defaultEnv},
			want{nil, nil, &missingParamError{}},
		},
		{
			"invalid state parameter",
			in{defaultSource, oc.Params{"input-repo": cfg.Repo, "state": "hello"}, defaultEnv},
			want{nil, nil, &invalidParamError{}},
		},
		{
			"unknown parameter",
			in{
				defaultSource,
				oc.Params{"input-repo": cfg.Repo, "state": "pending", "pizza": "margherita"},
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
		// Make a fake git repo to be the `input:` directory
		InRepoPath := filepath.Join(inDir, cfg.Repo, ".git")
		if err := os.MkdirAll(InRepoPath, 0770); err != nil {
			t.Fatal("input-repo dir", err)
		}
		// Make fake files normally added by the git resource
		RefPath := filepath.Join(InRepoPath, "ref")
		sha := []byte(cfg.Sha + "\n")
		if err := ioutil.WriteFile(RefPath, sha, 0660); err != nil {
			t.Fatal("Ref file", err)
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

func TestParseGitPseudoURL(t *testing.T) {
	testCases := []struct {
		name    string
		inURL   string
		wantGU  gitURL
		wantErr error
	}{
		{"invalid URL", "hello", gitURL{}, errInvalidURL},
		{"valid SSH URL", "git@github.com:Pix4D/cogito.git",
			gitURL{"ssh", "github.com", "Pix4D", "cogito"}, nil},
		{"valid HTTP URL", "https://github.com/Pix4D/cogito.git",
			gitURL{"https", "github.com", "Pix4D", "cogito"}, nil},
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
