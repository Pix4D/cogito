package cogito

import (
	"strings"
	"testing"
	"testing/fstest"

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
		request := PutRequest{}
		request.Source.ChatNotifyOnStates = defaultNotifyStates
		request.Params.State = tc.state

		assert.Equal(t, shouldSendToChat(request), tc.want)
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

	test := func(t *testing.T, tc testCase) {
		request := PutRequest{}
		request.Source.ChatNotifyOnStates = []BuildState{StatePending, StateSuccess}
		request.Params.State = tc.state

		assert.Equal(t, shouldSendToChat(request), tc.want)
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

func TestPrepareChatMessageSuccess(t *testing.T) {
	type testCase struct {
		name        string
		request     PutRequest
		inputDir    fstest.MapFS
		wantPresent []string
		wantAbsent  []string
	}

	baseRequest := PutRequest{
		Source: Source{Owner: "the-owner"},
		Params: PutParams{State: StateError},
		Env:    Environment{BuildJobName: "the-job"},
	}

	baseGitRef := "deadbeef"

	basePresent := []string{
		baseRequest.Source.Owner, baseRequest.Env.BuildJobName, baseGitRef}

	test := func(t *testing.T, tc testCase) {
		have, err := prepareChatMessage(tc.inputDir, tc.request, baseGitRef)

		assert.NilError(t, err)
		for _, elem := range tc.wantPresent {
			assert.Check(t, strings.Contains(have, elem), "wanted: %s", elem)
		}
		for _, elem := range tc.wantAbsent {
			assert.Check(t, !strings.Contains(have, elem), "not wanted: %s", elem)
		}
	}

	testCases := []testCase{
		{
			name:        "default build summary",
			request:     baseRequest,
			wantPresent: basePresent,
		},
		{
			name: "chat_message overrides default",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{Params: PutParams{ChatMessage: "the-custom-message"}}),
			wantPresent: []string{"the-custom-message"},
			wantAbsent:  basePresent,
		},
		{
			name: "chat_message and append",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{
					Params: PutParams{
						ChatMessage:       "the-custom-message",
						ChatMessageAppend: true,
					},
				}),
			wantPresent: append(basePresent, "the-custom-message"),
		},
		{
			name: "chat message file",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{Params: PutParams{ChatMessageFile: "registration/msg.txt"}}),
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte("from-custom-file")},
			},
			wantPresent: []string{"from-custom-file"},
			wantAbsent:  basePresent,
		},
		{
			name: "chat message and chat message file",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{Params: PutParams{
					ChatMessage:     "the-chat-message",
					ChatMessageFile: "registration/msg.txt"}}),
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte("from-custom-file")},
			},
			wantPresent: []string{"the-chat-message", "from-custom-file"},
			wantAbsent:  basePresent,
		},
		{
			name: "chat message file and append",
			request: testhelp.MergeStructs(
				baseRequest,
				PutRequest{Params: PutParams{
					ChatMessageAppend: true,
					ChatMessageFile:   "registration/msg.txt"}}),
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte("from-custom-file")},
			},
			wantPresent: append(basePresent, "from-custom-file"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPrepareChatMessageFailure(t *testing.T) {
	request := PutRequest{Params: PutParams{ChatMessageFile: "foo/msg.txt"}}
	inputDir := fstest.MapFS{"bar/msg.txt": {Data: []byte("from-custom-file")}}

	_, err := prepareChatMessage(inputDir, request, "deadbeef")

	assert.Error(t, err,
		"reading chat_message_file: open foo/msg.txt: file does not exist")
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
