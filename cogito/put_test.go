package cogito_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

var (
	baseSource = cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	baseParams = cogito.PutParams{State: cogito.StatePending}

	basePutInput = cogito.PutInput{
		Source: baseSource,
		Params: baseParams,
	}
)

type MockPutter struct {
	loadConfigurationErr error
	processInputDirErr   error
	outputErr            error
	sinkers              []cogito.Sinker
}

func (mp MockPutter) LoadConfiguration(in io.Reader, args []string) error {
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
	in := bytes.NewReader(testhelp.ToJSON(t, basePutInput))
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, []string{"dummy-dir"})

	assert.NilError(t, err)
}

func TestPutterLoadConfigurationFailure(t *testing.T) {
	type testCase struct {
		name     string
		putInput cogito.PutInput
		args     []string
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(testhelp.ToJSON(t, tc.putInput))
		putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

		err := putter.LoadConfiguration(in, tc.args)

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:     "source: missing keys",
			putInput: cogito.PutInput{Source: cogito.Source{}},
			wantErr:  "put: source: missing keys: owner, repo, access_token",
		},
		{
			name: "source: invalid log_level",
			putInput: cogito.PutInput{
				Source: testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			},
			wantErr: "put: source: invalid log_level: pippo",
		},
		{
			name: "params: invalid",
			putInput: cogito.PutInput{
				Source: baseSource,
				Params: cogito.PutParams{State: "burnt-pizza"},
			},
			args:    []string{"dummy-dir"},
			wantErr: "put: params: invalid build state: burnt-pizza",
		},
		{
			name:     "arguments: missing input directory",
			putInput: cogito.PutInput{Source: baseSource},
			args:     []string{},
			wantErr:  "put: arguments: missing input directory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterLoadConfigurationSystemFailure(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(iotest.ErrReader(errors.New("test read error")), nil)

	assert.Error(t, err, "put: parsing JSON from stdin: test read error")
}

func TestPutterLoadConfigurationInvalidParamsFailure(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {},
  "params": {"pizza": "margherita"}
}`)
	wantErr := `put: parsing JSON from stdin: json: unknown field "pizza"`
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	err := putter.LoadConfiguration(in, nil)

	assert.Error(t, err, wantErr)
}

func TestPutterProcessInputDirSuccess(t *testing.T) {
	inputDir := "testdata/one-repo"
	tmpDir := testhelp.MakeGitRepoFromTestdata(t, inputDir,
		"https://github.com/dummy-owner/dummy-repo", "dummySHA", "banana")
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())
	putter.InputDir = filepath.Join(tmpDir, filepath.Base(inputDir))
	putter.Pi = cogito.PutInput{
		Source: cogito.Source{Owner: "dummy-owner", Repo: "dummy-repo"},
	}

	err := putter.ProcessInputDir()

	assert.NilError(t, err)
}

func TestPutterProcessInputDirFailure(t *testing.T) {
	type testCase struct {
		name     string
		inputDir string
		wantErr  string
	}

	test := func(t *testing.T, tc testCase) {
		tmpDir := testhelp.MakeGitRepoFromTestdata(t, tc.inputDir,
			"https://github.com/dummy-owner/dummy-repo", "dummySHA", "banana mango")
		putter := &cogito.ProdPutter{
			InputDir: filepath.Join(tmpDir, filepath.Base(tc.inputDir)),
			Pi: cogito.PutInput{
				Source: cogito.Source{Owner: "dummy-owner", Repo: "dummy-repo"},
			},
		}

		err := putter.ProcessInputDir()

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:     "two input dirs",
			inputDir: "testdata/two-dirs",
			wantErr:  "found 2 input dirs: [dir-1 dir-2]. Want exactly 1, corresponding to the GitHub repo dummy-owner/dummy-repo",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestPutterProcessInputDirNonExisting(t *testing.T) {
	putter := &cogito.ProdPutter{
		InputDir: "non-existing",
		Pi:       cogito.PutInput{Source: baseSource},
	}

	err := putter.ProcessInputDir()

	assert.ErrorContains(t, err,
		"collecting directories in non-existing: open non-existing: no such file or directory")
}

func TestPutterSinks(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())

	sinks := putter.Sinks()

	assert.Assert(t, len(sinks) == 1)
	_, ok := sinks[0].(cogito.GitHubCommitStatusSink)
	assert.Assert(t, ok)
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

func TestSinkGitHubCommitStatusSend(t *testing.T) {
	putter := cogito.NewPutter("dummy-API", hclog.NewNullLogger())
	sink := cogito.GitHubCommitStatusSink{Pu: putter}

	assert.NilError(t, sink.Send())
}
