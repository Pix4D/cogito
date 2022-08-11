package cogito

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestGChatFormatText(t *testing.T) {
	type testCase struct {
		state BuildState
		want  string
	}

	test := func(t *testing.T, tc testCase) {
		have := gChatFormatText("deadbeef", "a-pipeline", "a-job", tc.state, "a-url")

		assert.Assert(t, cmp.Contains(have, tc.want))
	}

	testCases := []testCase{
		{state: StateAbort, want: "*state* 🟤 abort\n"},
		{state: StateError, want: "*state* 🟠 error\n"},
		{state: StateFailure, want: "*state* 🔴 failure\n"},
		{state: StatePending, want: "*state* 🟡 pending\n"},
		{state: StateSuccess, want: "*state* 🟢 success\n"},
		{state: BuildState("impossible"), want: "*state* ❓ impossible\n"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) { test(t, tc) })
	}
}
