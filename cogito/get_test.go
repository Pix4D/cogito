package cogito_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
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
		log := hclog.NewNullLogger()

		err := cogito.Get(log, in, &out, []string{"dummy-dir"})

		assert.NilError(t, err)
		var have cogito.Output
		testhelp.FromJSON(t, out.Bytes(), &have)
		assert.DeepEqual(t, have, tc.wantOut)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name: "returns requested version",
			request: cogito.GetRequest{
				Source:  baseSource,
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
		log := hclog.NewNullLogger()

		err := cogito.Get(log, in, tc.writer, tc.args)

		assert.Error(t, err, tc.wantErr)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:    "user validation failure: missing keys",
			source:  cogito.Source{},
			writer:  io.Discard,
			wantErr: "get: source: missing keys: owner, repo, access_token",
		},
		{
			name:    "concourse validation failure: empty version field",
			source:  baseSource,
			writer:  io.Discard,
			wantErr: "get: empty 'version' field",
		},
		{
			name:    "concourse validation failure: missing output directory",
			source:  baseSource,
			version: cogito.Version{Ref: "dummy"},
			writer:  io.Discard,
			wantErr: "get: arguments: missing output directory",
		},
		{
			name:    "system write error",
			args:    []string{"dummy-dir"},
			source:  baseSource,
			version: cogito.Version{Ref: "dummy"},
			writer:  &testhelp.FailingWriter{},
			wantErr: "get: preparing output: test write error",
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

	err := cogito.Get(hclog.NewNullLogger(), in, io.Discard, []string{})

	assert.Error(t, err, wantErr)
}
