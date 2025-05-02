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

func TestCheckSuccess(t *testing.T) {
	type testCase struct {
		name    string
		request cogito.CheckRequest
		wantOut []cogito.Version
	}

	test := func(t *testing.T, tc testCase) {
		in := testhelp.ToJSON(t, tc.request)
		var out bytes.Buffer
		log := testhelp.MakeTestLog()

		err := cogito.Check(log, in, &out, nil)

		assert.NilError(t, err)
		var have []cogito.Version
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
			name: "first request (Concourse omits the version field)",
			request: cogito.CheckRequest{
				Source: baseGithubSource,
			},
			wantOut: []cogito.Version{{Ref: "dummy"}},
		},
		{
			name: "subsequent requests (Concourse adds the version field)",
			request: cogito.CheckRequest{
				Source:  baseGithubSource,
				Version: cogito.Version{Ref: "dummy"},
			},
			wantOut: []cogito.Version{{Ref: "dummy"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestCheckFailure(t *testing.T) {
	type testCase struct {
		name    string
		source  cogito.Source // will be embedded in cogito.CheckRequest
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := testhelp.ToJSON(t, cogito.CheckRequest{Source: tc.source})
		log := testhelp.MakeTestLog()

		err := cogito.Check(log, in, tc.writer, nil)

		assert.Error(t, err, tc.wantErr)
	}

	baseGithubSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:    "validation failure: missing access token or github_app",
			source:  cogito.Source{},
			writer:  io.Discard,
			wantErr: "check: source: one of access_token or github_app must be specified",
		},
		{
			name: "validation failure: both access_key and github_app are set",
			source: cogito.Source{
				AccessToken: "dummy-token",
				GitHubApp:   github.GitHubApp{ClientId: "client-id"},
			},
			writer:  io.Discard,
			wantErr: "check: source: cannot specify both github_app and access_token",
		},
		{
			name:    "validation failure: missing repo keys",
			source:  cogito.Source{AccessToken: "dummy-token"},
			writer:  io.Discard,
			wantErr: "check: source: missing keys: owner, repo",
		},
		{
			name: "validation failure: missing gchat keys",
			source: cogito.Source{
				Sinks: []string{"gchat"},
			},
			writer:  io.Discard,
			wantErr: "check: source: missing keys: gchat_webhook",
		},
		{
			name: "validation failure: wrong sink key",
			source: cogito.Source{
				Sinks: []string{"ghost", "gchat"},
			},
			writer:  io.Discard,
			wantErr: "check: source: invalid sink(s): [ghost]",
		},
		{
			name:    "write error",
			source:  baseGithubSource,
			writer:  &testhelp.FailingWriter{},
			wantErr: "check: preparing output: test write error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestCheckInputFailure(t *testing.T) {
	log := testhelp.MakeTestLog()

	err := cogito.Check(log, nil, io.Discard, nil)

	assert.Error(t, err, "check: parsing request: EOF")
}
