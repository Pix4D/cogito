package cogito_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/Pix4D/cogito/cogito"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

func TestPutSuccess(t *testing.T) {
	type testCase struct {
		name string
		in   cogito.PutInput
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(toJSON(t, tc.in))
		var out bytes.Buffer
		log := hclog.NewNullLogger()

		err := cogito.Put(log, in, &out, []string{"dummy-dir"})

		assert.NilError(t, err)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name: "minimal smoke",
			in: cogito.PutInput{
				Source: baseSource,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestPutFailure(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		source  cogito.Source // will be embedded in cogito.PutInput
		reader  io.Reader     // if set, will override fields source above.
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := tc.reader
		if in == nil {
			in = bytes.NewReader(toJSON(t,
				cogito.PutInput{
					Source: tc.source,
				}))
		}
		log := hclog.NewNullLogger()

		err := cogito.Put(log, in, tc.writer, tc.args)

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
			wantErr: "put: source: missing keys: owner, repo, access_token",
		},
		{
			name:    "user validation failure: log_level",
			source:  mergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			writer:  io.Discard,
			wantErr: "put: source: invalid log_level: pippo",
		},
		{
			name:    "concourse validation failure: missing input directory",
			source:  baseSource,
			writer:  io.Discard,
			wantErr: "put: missing input directory from arguments",
		},
		{
			name:    "system write error",
			args:    []string{"dummy-dir"},
			source:  baseSource,
			writer:  &failingWriter{},
			wantErr: "put: test write error",
		},
		{
			name:    "system read error",
			reader:  iotest.ErrReader(errors.New("test read error")),
			writer:  io.Discard,
			wantErr: "put: parsing JSON from stdin: test read error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}
