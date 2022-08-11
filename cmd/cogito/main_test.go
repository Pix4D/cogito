package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRunSmokeSuccess(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		in      string
		wantOut string
	}

	test := func(t *testing.T, tc testCase) {
		in := strings.NewReader(tc.in)
		var out bytes.Buffer

		err := run(in, &out, io.Discard, tc.args)

		assert.NilError(t, err)
		have := out.String()
		assert.Equal(t, have, tc.wantOut)
	}

	testCases := []testCase{
		{
			name: "check",
			args: []string{"check"},
			in: `
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret"
  } 
}`,
			wantOut: `[{"ref":"dummy"}]
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestRunSmokeFailure(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		in      string
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		in := strings.NewReader(tc.in)

		err := run(in, nil, io.Discard, tc.args)

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "unknown command",
			args:    []string{"foo"},
			wantErr: `cogito: unexpected invocation as 'foo'; want: one of 'check', 'in', 'out'; (command-line: [foo])`,
		},
		{
			name: "check, wrong in",
			args: []string{"check"},
			in: `
{
  "fruit": "banana" 
}`,
			wantErr: `check: parsing JSON from stdin: json: unknown field "fruit"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Assert(t, tc.wantErr != "")
			test(t, tc)
		})
	}
}

func TestRunPrintsBuildInformation(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret"
  } 
}`)
	var logBuf bytes.Buffer
	wantLog := "cogito: This is the Cogito GitHub status resource. unknown"

	err := run(in, io.Discard, &logBuf, []string{"check"})
	assert.NilError(t, err)
	haveLog := logBuf.String()

	assert.Assert(t, strings.Contains(haveLog, wantLog), haveLog)
}
