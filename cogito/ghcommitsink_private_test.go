package cogito

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGhMakeContext(t *testing.T) {
	type testCase struct {
		name        string
		request     PutRequest
		wantContext string
	}

	test := func(t *testing.T, tc testCase) {
		have := ghMakeContext(tc.request)

		assert.Equal(t, have, tc.wantContext)
	}

	testCases := []testCase{
		{
			name: "default: context taken from job name",
			request: PutRequest{
				Env: Environment{BuildJobName: "the-job"},
			},
			wantContext: "the-job",
		},
		{
			name: "context_prefix",
			request: PutRequest{
				Source: Source{ContextPrefix: "the-prefix"},
				Env:    Environment{BuildJobName: "the-job"},
			},
			wantContext: "the-prefix/the-job",
		},
		{
			name: "explicit context overrides job name",
			request: PutRequest{
				Params: PutParams{Context: "the-context"},
				Env:    Environment{BuildJobName: "the-job"},
			},
			wantContext: "the-context",
		},
		{
			name: "prefix and override",
			request: PutRequest{
				Source: Source{ContextPrefix: "the-prefix"},
				Params: PutParams{Context: "the-context"},
				Env:    Environment{BuildJobName: "the-job"},
			},
			wantContext: "the-prefix/the-context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestGhAdaptState(t *testing.T) {
	type testCase struct {
		name  string
		state BuildState
		want  BuildState
	}

	test := func(t *testing.T, tc testCase) {
		have := ghAdaptState(tc.state)

		assert.Equal(t, BuildState(have), tc.want)
	}

	testCases := []testCase{
		{
			name:  "no conversion",
			state: StatePending,
			want:  StatePending,
		},
		{
			name:  "abort converted to error",
			state: StateAbort,
			want:  StateError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}
