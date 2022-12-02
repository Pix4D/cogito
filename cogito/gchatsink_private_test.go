package cogito

import (
	"strings"
	"testing"
	"testing/fstest"

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

func TestPrepareChatMessageOnlyChatSuccess(t *testing.T) {
	have, err := prepareChatMessage(nil, PutRequest{}, "")

	assert.NilError(t, err)
	assert.Check(t, !strings.Contains(have, "commit"), "not wanted: commit")
}

func TestPrepareChatMessageSuccess(t *testing.T) {
	type testCase struct {
		name        string
		makeReq     func() PutRequest
		inputDir    fstest.MapFS
		wantPresent []string
		wantAbsent  []string
	}

	baseRequest := PutRequest{
		Source: Source{
			Owner:             "the-owner",
			ChatAppendSummary: true, // the default
		},
		Params: PutParams{
			State:             StateError,
			ChatAppendSummary: true, // the default
		},
		Env: Environment{BuildJobName: "the-job"},
	}

	baseGitRef := "deadbeef"

	buildSummary := []string{
		baseRequest.Source.Owner, baseRequest.Env.BuildJobName, baseGitRef}
	customMessage := "the-custom-message"
	customFile := "from-custom-file"

	test := func(t *testing.T, tc testCase) {
		have, err := prepareChatMessage(tc.inputDir, tc.makeReq(), baseGitRef)

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
			name:        "build summary only",
			makeReq:     func() PutRequest { return baseRequest },
			wantPresent: buildSummary,
			wantAbsent:  []string{customMessage},
		},
		{
			name: "chat_message, all defaults",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessage = customMessage
				return req
			},
			wantPresent: append([]string{customMessage}, buildSummary...),
		},
		{
			name: "chat_message, params.append false",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessage = customMessage
				req.Params.ChatAppendSummary = false
				return req
			},
			wantPresent: []string{customMessage},
			wantAbsent:  buildSummary,
		},
		{
			name: "chat_message_file, all defaults",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessageFile = "registration/msg.txt"
				return req
			},
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte(customFile)},
			},
			wantPresent: append([]string{customFile}, buildSummary...),
			wantAbsent:  []string{customMessage},
		},
		{
			name: "chat_message_file, params.append false",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessageFile = "registration/msg.txt"
				req.Params.ChatAppendSummary = false
				return req
			},
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte(customFile)},
			},
			wantPresent: []string{customFile},
			wantAbsent:  append([]string{customMessage}, buildSummary...),
		},
		{
			name: "chat_message and chat_message_file, all defaults",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessage = customMessage
				req.Params.ChatMessageFile = "registration/msg.txt"
				return req
			},
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte(customFile)},
			},
			wantPresent: append([]string{customMessage, customFile}, buildSummary...),
		},
		{
			name: "chat_message and chat_message_file, params.append false",
			makeReq: func() PutRequest {
				req := baseRequest
				req.Params.ChatMessage = customMessage
				req.Params.ChatMessageFile = "registration/msg.txt"
				req.Params.ChatAppendSummary = false
				return req
			},
			inputDir: fstest.MapFS{
				"registration/msg.txt": {Data: []byte(customFile)},
			},
			wantPresent: []string{customMessage, customFile},
			wantAbsent:  buildSummary,
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
