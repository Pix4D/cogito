package cogito_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

// Note that all input tests are performed by [TestNewCheckInputSuccess]
func TestCheckSuccess(t *testing.T) {
	type testCase struct {
		name    string
		in      cogito.CheckInput
		wantOut []cogito.Version
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(toJSON(t, tc.in))
		var out bytes.Buffer
		log := hclog.NewNullLogger()

		err := cogito.Check(log, in, &out, nil)

		assert.NilError(t, err)
		var have []cogito.Version
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
			name: "first request (Concourse omits the version field)",
			in: cogito.CheckInput{
				Source: baseSource,
			},
			wantOut: []cogito.Version{{Ref: "dummy"}},
		},
		{
			name: "subsequent requests (Concourse adds the version field)",
			in: cogito.CheckInput{
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
		in      cogito.CheckInput
		writer  io.Writer
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := bytes.NewReader(toJSON(t, tc.in))
		log := hclog.NewNullLogger()

		err := cogito.Check(log, in, tc.writer, nil)

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "missing keys",
			in:      cogito.CheckInput{},
			writer:  io.Discard,
			wantErr: "missing keys",
		},
		{
			name: "write error",
			in: cogito.CheckInput{
				Source: cogito.Source{
					Owner:       "the-owner",
					Repo:        "the-repo",
					AccessToken: "the-token",
				},
			},
			writer:  &failingWriter{},
			wantErr: "no bananas",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}
