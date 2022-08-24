package cogito

import (
	"strings"
	"testing"

	"github.com/Pix4D/cogito/testhelp"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestShouldSendToChatDefaultConfig(t *testing.T) {
	type testCase struct {
		state BuildState
		want  bool
	}

	test := func(t *testing.T, tc testCase) {
		assert.Equal(t, shouldSendToChat(tc.state, defaultNotifyStates), tc.want)
	}

	testCases := []testCase{
		{state: StateAbort, want: true},
		{state: StateError, want: true},
		{state: StateFailure, want: true},
		{state: StatePending, want: false},
		{state: StateSuccess, want: false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) { test(t, tc) })
	}
}

func TestShouldSendToChatCustomConfig(t *testing.T) {
	type testCase struct {
		state BuildState
		want  bool
	}

	baseNotifyStates := []BuildState{StatePending, StateSuccess}

	test := func(t *testing.T, tc testCase) {
		assert.Equal(t, shouldSendToChat(tc.state, baseNotifyStates), tc.want)
	}

	testCases := []testCase{
		{state: StateAbort, want: false},
		{state: StateError, want: false},
		{state: StateFailure, want: false},
		{state: StatePending, want: true},
		{state: StateSuccess, want: true},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) { test(t, tc) })
	}
}

func TestPrepareChatMessage(t *testing.T) {
	type testCase struct {
		name        string
		request     PutRequest
		gitRef      string
		wantPresent []string
		wantAbsent  []string
	}

	test := func(t *testing.T, tc testCase) {
		have := prepareChatMessage(tc.request, tc.gitRef)

		for _, elem := range tc.wantPresent {
			assert.Check(t, strings.Contains(have, elem))
		}
		for _, elem := range tc.wantAbsent {
			assert.Check(t, !strings.Contains(have, elem))
		}
	}

	baseRequest := PutRequest{
		Source: Source{Owner: "the-owner"},
		Params: PutParams{State: StateError},
		Env:    Environment{BuildJobName: "the-job"},
	}

	testCases := []testCase{
		{
			name:    "default build summary",
			request: baseRequest,
			gitRef:  "deadbeef",
			wantPresent: []string{
				baseRequest.Source.Owner, baseRequest.Env.BuildJobName, "deadbeef"},
		},
		{
			name: "chat_message overrides default",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{Params: PutParams{ChatMessage: "the-custom-message"}}),
			gitRef:      "deadbeef",
			wantPresent: []string{"the-custom-message"},
			wantAbsent: []string{
				baseRequest.Source.Owner, baseRequest.Env.BuildJobName, "deadbeef"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestGChatBuildSummaryText(t *testing.T) {
	commit := "deadbeef"
	state := StatePending
	src := Source{
		Owner: "the-owner",
		Repo:  "the-repo",
	}
	env := Environment{
		BuildName:         "42",
		BuildJobName:      "the-job",
		BuildPipelineName: "the-pipeline",
		AtcExternalUrl:    "https://cogito.invalid",
	}

	have := gChatBuildSummaryText(commit, state, src, env)

	assert.Assert(t, cmp.Contains(have, "*pipeline* the-pipeline"))
	assert.Assert(t, cmp.Regexp(`\*job\* <https:.+\|the-job\/42>`, have))
	assert.Assert(t, cmp.Contains(have, "*state* 🟡 pending"))
	assert.Assert(t, cmp.Regexp(
		`\*commit\* <https:.+\/commit\/deadbeef\|deadbeef> \(repo: the-owner\/the-repo\)`,
		have))
}

func TestStateToIcon(t *testing.T) {
	type testCase struct {
		state BuildState
		want  string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Equal(t, decorateState(tc.state), tc.want)
	}

	testCases := []testCase{
		{state: StateAbort, want: "🟤 abort"},
		{state: StateError, want: "🟠 error"},
		{state: StateFailure, want: "🔴 failure"},
		{state: StatePending, want: "🟡 pending"},
		{state: StateSuccess, want: "🟢 success"},
		{state: BuildState("impossible"), want: "❓ impossible"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.state), func(t *testing.T) { test(t, tc) })
	}
}
