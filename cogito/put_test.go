package cogito_test

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

var (
	baseGithubSource = cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	// StateError is sent to notification sinks by default.
	baseParams = cogito.PutParams{State: cogito.StateError}

	basePutRequest = cogito.PutRequest{
		Source: baseGithubSource,
		Params: baseParams,
	}
)

type MockPutter struct {
	loadConfigurationErr error
	processInputDirErr   error
	outputErr            error
	sinkers              []cogito.Sinker
}

func (mp MockPutter) LoadConfiguration(input []byte, args []string) error {
	return mp.loadConfigurationErr
}

func (mp MockPutter) ProcessInputDir() error {
	return mp.processInputDirErr
}

func (mp MockPutter) Sinks() []cogito.Sinker {
	return mp.sinkers
}

func (mp MockPutter) Output(out io.Writer) error {
	return mp.outputErr
}

type MockSinker struct {
	sendError error
}

func (ms MockSinker) Send() error {
	return ms.sendError
}

func TestPutSuccess(t *testing.T) {
	putter := MockPutter{sinkers: []cogito.Sinker{MockSinker{}}}

	err := cogito.Put(hclog.NewNullLogger(), nil, nil, nil, putter)

	assert.NilError(t, err)
}

func TestPutFailure(t *testing.T) {
	type testCase struct {
		name    string
		putter  cogito.Putter
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		err := cogito.Put(hclog.NewNullLogger(), nil, nil, nil, tc.putter)

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name: "load configuration error",
			putter: MockPutter{
				loadConfigurationErr: errors.New("mock: load configuration"),
			},
			wantErr: "put: mock: load configuration",
		},
		{
			name: "process input dir error",
			putter: MockPutter{
				processInputDirErr: errors.New("mock: process input dir"),
			},
			wantErr: "put: mock: process input dir",
		},
		{
			name: "sink errors",
			putter: MockPutter{
				sinkers: []cogito.Sinker{
					MockSinker{sendError: errors.New("mock: send error 1")},
					MockSinker{sendError: errors.New("mock: send error 2")},
				},
			},
			wantErr: "put: multiple errors:\n\tmock: send error 1\n\tmock: send error 2",
		},
		{
			name: "output error",
			putter: MockPutter{
				outputErr: errors.New("mock: output error"),
			},
			wantErr: "put: mock: output error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterLoadConfigurationSuccess(t *testing.T) {
	in := testhelp.ToJSON(t, basePutRequest)
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, []string{"dummy-dir"})

	assert.NilError(t, err)
}

func TestPutterLoadConfigurationSinksOverrideSuccess(t *testing.T) {
	in := []byte(`
	{
	  "source": {
		"owner": "the-owner",
		"repo": "the-repo",
		"access_token": "the-token",
		"gchat_webhook": "sensitive-webhook",
		"sinks": ["github"]
	},
	  "params": {"sinks": ["gchat"]}
	}`)
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())
	inputDir := []string{""}
	err := putter.LoadConfiguration(in, inputDir)
	assert.NilError(t, err)

	if putter.Request.Params.Sinks[0] != "gchat" || len(putter.Request.Params.Sinks) != 1 {
		err = fmt.Errorf("expected sinks overridden: want [gchat] got %s", putter.Request.Params.Sinks)
	}
	assert.NilError(t, err)
}

func TestPutterLoadConfigurationFailure(t *testing.T) {
	type testCase struct {
		name     string
		putInput cogito.PutRequest
		args     []string
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		in := testhelp.ToJSON(t, tc.putInput)
		putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

		err := putter.LoadConfiguration(in, tc.args)

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:     "source: missing keys",
			putInput: cogito.PutRequest{Source: cogito.Source{}, Params: baseParams},
			wantErr:  "put: source: missing keys: owner, repo, access_token",
		},
		{
			name: "params: invalid",
			putInput: cogito.PutRequest{
				Source: baseGithubSource,
				Params: cogito.PutParams{State: "burnt-pizza"},
			},
			wantErr: "put: parsing request: invalid build state: burnt-pizza",
		},
		{
			name:     "arguments: missing input directory",
			putInput: basePutRequest,
			args:     []string{},
			wantErr:  "put: concourse resource protocol violation: missing input directory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterLoadConfigurationInvalidParamsFailure(t *testing.T) {
	in := []byte(`
{
  "source": {},
  "params": {"pizza": "margherita"}
}`)
	wantErr := `put: parsing request: json: unknown field "pizza"`
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, nil)

	assert.Error(t, err, wantErr)
}

func TestPutterLoadConfigurationMissingGchatwebHook(t *testing.T) {
	in := []byte(`
{
  "source": {"sinks": ["gchat"]},
  "params": {}
}`)
	wantErr := `put: source: missing keys: gchat_webhook`
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, nil)

	assert.Error(t, err, wantErr)
}

func TestPutterLoadConfigurationUnknownSink(t *testing.T) {
	in := []byte(`
{
  "source": {"sinks": ["pizza"]},
  "params": {}
}`)
	wantErr := `put: source: invalid sink(s): [pizza]`
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, nil)

	assert.Error(t, err, wantErr)
}

func TestPutterLoadConfigurationUnknownSinkPutParams(t *testing.T) {
	in := []byte(`
{
  "source": {"sinks": ["gchat"], "gchat_webhook": "dummy-webhook"},
  "params": {"sinks": ["pizza"]}
}`)
	wantErr := `put: arguments: unsupported sink(s): [pizza]`
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, nil)

	assert.Error(t, err, wantErr)
}

func TestPutterProcessInputDirSuccess(t *testing.T) {
	type testCase struct {
		name     string
		inputDir string
		params   cogito.PutParams
		sink     string
	}

	test := func(t *testing.T, tc testCase) {
		putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.inputDir,
			"https://github.com/dummy-owner/dummy-repo", "dummySHA", "banana")
		putter.InputDir = filepath.Join(tmpDir, filepath.Base(tc.inputDir))
		putter.Request = cogito.PutRequest{
			Source: cogito.Source{Owner: "dummy-owner", Repo: "dummy-repo", Sinks: []string{tc.sink}},
			Params: tc.params,
		}

		err := putter.ProcessInputDir()

		assert.NilError(t, err)
	}

	testCases := []testCase{
		{
			name:     "one dir with a repo",
			inputDir: "testdata/one-repo",
		},
		{
			name:     "two dirs: repo and msg file",
			inputDir: "testdata/repo-and-msgdir",
			params:   cogito.PutParams{ChatMessageFile: "msgdir/msg.txt"},
		},
		{
			name:     "only msg dir, but gchat is set",
			inputDir: "testdata/repo-and-msgdir/msgdir",
			sink:     "gchat",
		},
		{
			name:     "only msg dir and message file, but gchat is set",
			inputDir: "testdata/repo-and-msgdir/msgdir",
			sink:     "gchat",
			params:   cogito.PutParams{ChatMessageFile: "msgdir/msg.txt"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterProcessInputDirFailure(t *testing.T) {
	type testCase struct {
		name     string
		inputDir string
		params   cogito.PutParams
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.inputDir,
			"https://github.com/dummy-owner/dummy-repo", "dummySHA", "banana mango")
		putter := cogito.NewPutter("dummy-api", hclog.NewNullLogger())
		putter.Request = cogito.PutRequest{
			Source: cogito.Source{Owner: "dummy-owner", Repo: "dummy-repo"},
			Params: tc.params,
		}
		putter.InputDir = filepath.Join(tmpDir, filepath.Base(tc.inputDir))

		err := putter.ProcessInputDir()

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:     "no input dirs",
			inputDir: "testdata/empty-dir",
			wantErr:  "put:inputs: missing directory for GitHub repo: have: [], GitHub: dummy-owner/dummy-repo",
		},
		{
			name:     "no input dirs and sink 'github' is set",
			inputDir: "testdata/empty-dir",
			params:   cogito.PutParams{Sinks: []string{"github"}},
			wantErr:  "put:inputs: missing directory for GitHub repo: have: [], GitHub: dummy-owner/dummy-repo",
		},
		{
			name:     "two input dirs",
			inputDir: "testdata/two-dirs",
			wantErr:  "put:inputs: want only directory for GitHub repo: have: [dir-1 dir-2], GitHub: dummy-owner/dummy-repo",
		},
		{
			name:     "one input dir but not a repo",
			inputDir: "testdata/not-a-repo",
			wantErr:  "parsing .git/config: open ",
		},
		{
			name:     "git repo, but something wrong",
			inputDir: "testdata/one-repo",
			wantErr:  "git commit: branch checkout: read SHA file: open ",
		},
		{
			name:     "repo and msgdir, but missing dir in chat_message_file",
			inputDir: "testdata/repo-and-msgdir",
			params:   cogito.PutParams{ChatMessageFile: "msg.txt"},
			wantErr:  "chat_message_file: wrong format: have: msg.txt, want: path of the form: <dir>/<file>",
		},
		{
			name:     "chat_message_file specified but different put:inputs",
			inputDir: "testdata/repo-and-msgdir",
			params:   cogito.PutParams{ChatMessageFile: "banana/msg.txt"},
			wantErr:  "put:inputs: directory for chat_message_file not found: have: [a-repo msgdir], chat_message_file: banana/msg.txt",
		},
		{
			name:     "chat_message_file specified but too few put:inputs",
			inputDir: "testdata/one-repo",
			params:   cogito.PutParams{ChatMessageFile: "banana/msg.txt"},
			wantErr:  "put:inputs: directory for chat_message_file not found: have: [a-repo], chat_message_file: banana/msg.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterProcessInputDirNonExisting(t *testing.T) {
	putter := &cogito.ProdPutter{
		InputDir: "non-existing",
		Request:  cogito.PutRequest{Source: baseGithubSource},
	}

	err := putter.ProcessInputDir()

	assert.ErrorContains(t, err,
		"collecting directories in non-existing: open non-existing: no such file or directory")
}

func TestPutterSinks(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	sinks := putter.Sinks()
	assert.Assert(t, len(sinks) == 2)
	_, ok1 := sinks[0].(cogito.GitHubCommitStatusSink)
	assert.Assert(t, ok1)
	_, ok2 := sinks[1].(cogito.GoogleChatSink)
	assert.Assert(t, ok2)
}

func TestPutterCustomSinks(t *testing.T) {
	putter := cogito.NewPutter("dummy-api", hclog.NewNullLogger())
	putter.Request = cogito.PutRequest{
		Params: cogito.PutParams{Sinks: []string{"gchat"}},
	}
	sinks := putter.Sinks()
	assert.Assert(t, len(sinks) == 1)
	_, ok1 := sinks[0].(cogito.GoogleChatSink)
	assert.Assert(t, ok1)
}

func TestPutterOutputSuccess(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.Output(io.Discard)

	assert.NilError(t, err)
}

func TestPutterOutputFailure(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.Output(&testhelp.FailingWriter{})

	assert.Error(t, err, "put: test write error")
}
