package cogito_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Pix4D/cogito/cogito"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

func TestGetSuccess(t *testing.T) {
	type testCase struct {
		name    string
		in      cogito.GetInput
		wantOut cogito.Output
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(toJSON(t, tc.in))
		var out bytes.Buffer
		log := hclog.NewNullLogger()

		err := cogito.Get(log, in, &out, []string{"dummy-dir"})

		assert.NilError(t, err)
		var have cogito.Output
		fromJSON(t, out.Bytes(), &have)
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
			in: cogito.GetInput{
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
		source  cogito.Source  // will be embedded in cogito.GetInput
		version cogito.Version // will be embedded in cogito.GetInput
		reader  io.Reader      // if set, will override fields source and version above.
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := tc.reader
		if in == nil {
			in = bytes.NewReader(toJSON(t,
				cogito.GetInput{
					Source:  tc.source,
					Version: tc.version,
				}))
		}
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
			name:    "user validation failure: log_level",
			source:  mergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			writer:  io.Discard,
			wantErr: "get: source: invalid log_level: pippo",
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
			wantErr: "get: missing output directory from arguments",
		},
		{
			name:    "system write error",
			args:    []string{"dummy-dir"},
			source:  baseSource,
			version: cogito.Version{Ref: "dummy"},
			writer:  &failingWriter{},
			wantErr: "get: test write error",
		},
		{
			name:    "system read error",
			source:  cogito.Source{},
			reader:  iotest.ErrReader(errors.New("test read error")),
			writer:  io.Discard,
			wantErr: "get: parsing JSON from stdin: test read error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestGetNonEmptyParamsFailure(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {},
  "params": {"pizza": "margherita"}
}`)
	wantErr := `get: parsing JSON from stdin: json: unknown field "params"`

	err := cogito.Get(hclog.NewNullLogger(), in, io.Discard, []string{})

	assert.Error(t, err, wantErr)
}
