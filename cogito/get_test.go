package cogito_test

import (
	"bytes"
	"io"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
)

func TestGetSuccess(t *testing.T) {
	type testCase struct {
		name    string
		request cogito.GetRequest
		wantOut cogito.Output
	}

	test := func(t *testing.T, tc testCase) {
		in := testhelp.ToJSON(t, tc.request)
		var out bytes.Buffer
		log := testhelp.MakeTestLog()

		err := cogito.Get(log, in, &out, []string{"dummy-dir"})

		assert.NilError(t, err)
		var have cogito.Output
		testhelp.FromJSON(t, out.Bytes(), &have)
		assert.DeepEqual(t, have, tc.wantOut)
	}

	baseGithubSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name: "returns requested version",
			request: cogito.GetRequest{
				Source:  baseGithubSource,
				Version: cogito.Version{Ref: "banana"},
			},
			wantOut: cogito.Output{Version: cogito.Version{Ref: "banana"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestGetFailure(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		source  cogito.Source  // will be embedded in cogito.GetRequest
		version cogito.Version // will be embedded in cogito.GetRequest
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := testhelp.ToJSON(t,
			cogito.GetRequest{
				Source:  tc.source,
				Version: tc.version,
			})
		log := testhelp.MakeTestLog()

		err := cogito.Get(log, in, tc.writer, tc.args)

		assert.Error(t, err, tc.wantErr)
	}

	baseGithubSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:    "user validation failure: missing access token or github_app",
			source:  cogito.Source{},
			writer:  io.Discard,
			wantErr: "get: source: one of access_token or github_app must be specified",
		},
		{
			name: "user validation failure: both access_key and github_app are set",
			source: cogito.Source{
				AccessToken: "dummy-token",
				GitHubApp:   github.GitHubApp{ClientId: "client-id"},
			},
			writer:  io.Discard,
			wantErr: "get: source: cannot specify both github_app and access_token",
		},
		{
			name:    "user validation failure: missing keys",
			source:  cogito.Source{AccessToken: "dummy-token"},
			writer:  io.Discard,
			wantErr: "get: source: missing keys: owner, repo",
		},
		{
			name:    "concourse validation failure: empty version field",
			source:  baseGithubSource,
			writer:  io.Discard,
			wantErr: "get: empty 'version' field",
		},
		{
			name:    "concourse validation failure: missing output directory",
			source:  baseGithubSource,
			version: cogito.Version{Ref: "dummy"},
			writer:  io.Discard,
			wantErr: "get: arguments: missing output directory",
		},
		{
			name:    "system write error",
			args:    []string{"dummy-dir"},
			source:  baseGithubSource,
			version: cogito.Version{Ref: "dummy"},
			writer:  &testhelp.FailingWriter{},
			wantErr: "get: preparing output: test write error",
		},
		{
			name:    "user missing gchat webhook",
			source:  cogito.Source{Sinks: []string{"gchat"}},
			version: cogito.Version{Ref: "dummy"},
			writer:  &testhelp.FailingWriter{},
			wantErr: "get: source: missing keys: gchat_webhook",
		},
		{
			name:    "user validation failure: wrong sink key",
			source:  cogito.Source{Sinks: []string{"ghost", "gchat"}},
			writer:  io.Discard,
			wantErr: "get: source: invalid sink(s): [ghost]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestGetNonEmptyParamsFailure(t *testing.T) {
	in := []byte(`
{
  "source": {},
  "params": {"pizza": "margherita"}
}`)
	wantErr := `get: parsing request: json: unknown field "params"`

	err := cogito.Get(testhelp.MakeTestLog(), in, io.Discard, []string{})

	assert.Error(t, err, wantErr)
}
