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

func TestPutSuccess(t *testing.T) {
	type testCase struct {
		name    string
		in      cogito.PutInput
		wantOut cogito.Output
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(toJSON(t, tc.in))
		var out bytes.Buffer
		log := hclog.NewNullLogger()

		err := cogito.Put(log, in, &out, []string{"dummy-dir"})

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

	baseParam := cogito.PutParams{State: cogito.StatePending}

	testCases := []testCase{
		{
			name: "returns correct version and metadata",
			in: cogito.PutInput{
				Source: baseSource,
				Params: baseParam,
			},
			wantOut: cogito.Output{
				Version:  cogito.DummyVersion,
				Metadata: []cogito.Metadata{{Name: cogito.KeyState, Value: cogito.StatePending}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestPutPipelineValidationFailure(t *testing.T) {
	type testCase struct {
		name     string
		putInput cogito.PutInput
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		log := hclog.NewNullLogger()
		in := bytes.NewReader(toJSON(t, tc.putInput))

		err := cogito.Put(log, in, io.Discard, []string{"dummy-dir"})

		assert.Error(t, err, tc.wantErr)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name: "missing keys",
			putInput: cogito.PutInput{
				Source: cogito.Source{},
			},
			wantErr: "put: source: missing keys: owner, repo, access_token",
		},
		{
			name: "invalid log_level",
			putInput: cogito.PutInput{
				Source: mergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			},
			wantErr: "put: source: invalid log_level: pippo",
		},
		{
			name: "invalid params",
			putInput: cogito.PutInput{
				Source: baseSource,
				Params: cogito.PutParams{State: "burnt-pizza"},
			},
			wantErr: "put: params: invalid build state: burnt-pizza",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}

}

func TestPutProtocolFailure(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		log := hclog.NewNullLogger()
		in := bytes.NewReader(toJSON(t, cogito.PutInput{
			Source: cogito.Source{
				Owner:       "the-owner",
				Repo:        "the-repo",
				AccessToken: "the-token",
			},
			Params: cogito.PutParams{State: cogito.StatePending},
		}))

		err := cogito.Put(log, in, io.Discard, tc.args)

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "missing input directory from arguments",
			args:    []string{},
			wantErr: "put: arguments: missing input directory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestPutSystemFailure(t *testing.T) {
	type testCase struct {
		name    string
		reader  io.Reader
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		log := hclog.NewNullLogger()

		err := cogito.Put(log, tc.reader, tc.writer, []string{"dummy-dir"})

		assert.Error(t, err, tc.wantErr)
	}

	baseReader := bytes.NewReader(toJSON(t,
		cogito.PutInput{
			Source: cogito.Source{
				Owner:       "the-owner",
				Repo:        "the-repo",
				AccessToken: "the-token",
			},
			Params: cogito.PutParams{State: cogito.StatePending},
		}))

	testCases := []testCase{
		{
			name:    "system read error",
			reader:  iotest.ErrReader(errors.New("test read error")),
			writer:  io.Discard,
			wantErr: "put: parsing JSON from stdin: test read error",
		},
		{
			name:    "system write error",
			reader:  baseReader,
			writer:  &failingWriter{},
			wantErr: "put: test write error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

// TODO: give a more helpful error message!
//  The JSON parser does not mention the outer object ("params") :-(
func TestPutInvalidParamsFailure(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {},
  "params": {"pizza": "margherita"}
}`)
	wantErr := `put: parsing JSON from stdin: json: unknown field "pizza"`

	err := cogito.Put(hclog.NewNullLogger(), in, io.Discard, []string{})

	assert.Error(t, err, wantErr)
}
