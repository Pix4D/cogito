package cogito_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

func TestCheckSuccess(t *testing.T) {
	type testCase struct {
		name    string
		request cogito.CheckRequest
		wantOut []cogito.Version
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(testhelp.ToJSON(t, tc.request))
		var out bytes.Buffer
		log := hclog.NewNullLogger()

		err := cogito.Check(log, in, &out, nil)

		assert.NilError(t, err)
		var have []cogito.Version
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
			name: "first request (Concourse omits the version field)",
			request: cogito.CheckRequest{
				Source: baseSource,
			},
			wantOut: []cogito.Version{{Ref: "dummy"}},
		},
		{
			name: "subsequent requests (Concourse adds the version field)",
			request: cogito.CheckRequest{
				Source:  baseSource,
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
		reader  io.Reader     // if set, will override field `source`.
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := tc.reader
		if in == nil {
			in = bytes.NewReader(testhelp.ToJSON(t, cogito.CheckRequest{Source: tc.source}))
		}
		log := hclog.NewNullLogger()

		err := cogito.Check(log, in, tc.writer, nil)

		assert.Error(t, err, tc.wantErr)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:    "validation failure: missing keys",
			source:  cogito.Source{},
			writer:  io.Discard,
			wantErr: "check: source: missing keys: owner, repo, access_token",
		},
		{
			name:    "validation failure: log",
			source:  testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			writer:  io.Discard,
			wantErr: "check: source: invalid log_level: pippo",
		},
		{
			name:    "write error",
			source:  baseSource,
			writer:  &testhelp.FailingWriter{},
			wantErr: "check: test write error",
		},
		{
			name:    "read error",
			source:  cogito.Source{},
			reader:  iotest.ErrReader(errors.New("test read error")),
			writer:  io.Discard,
			wantErr: "check: parsing request: test read error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}
