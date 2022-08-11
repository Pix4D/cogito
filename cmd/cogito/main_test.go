package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/googlechat"
	"github.com/Pix4D/cogito/testhelp"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestRunCheckSuccess(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret",
    "log_level": "debug"
  }
}`)
	var out bytes.Buffer
	var logOut bytes.Buffer

	err := run(in, &out, &logOut, []string{"check"})

	assert.NilError(t, err, "\nout: %s\nlogOut: %s", out.String(), logOut.String())
}

func TestRunGetSuccess(t *testing.T) {
	in := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret",
    "log_level": "debug"
  },
  "version": {"ref": "pizza"}
}`)
	var out bytes.Buffer
	var logOut bytes.Buffer

	err := run(in, &out, &logOut, []string{"in", "dummy-dir"})

	assert.NilError(t, err, "\nout: %s\nlogOut: %s", out.String(), logOut.String())
}

func TestRunPutSuccess(t *testing.T) {
	wantState := cogito.StateError
	wantGitRef := "dummyHead"
	var ghReq github.AddRequest
	var ghUrl *url.URL
	gitHubSpy := testhelp.SpyHttpServer(&ghReq, &ghUrl, http.StatusCreated)
	var chatMsg googlechat.BasicMessage
	var gchatUrl *url.URL
	googleChatSpy := testhelp.SpyHttpServer(&chatMsg, &gchatUrl, http.StatusOK)
	in := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner:        "the-owner",
			Repo:         "the-repo",
			AccessToken:  "the-secret",
			GChatWebHook: googleChatSpy.URL,
			LogLevel:     "debug",
		},
		Params: cogito.PutParams{State: wantState},
	}))
	var out bytes.Buffer
	var logOut bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote("the-owner", "the-repo"), "dummySHA", wantGitRef)
	t.Setenv("COGITO_GITHUB_API", gitHubSpy.URL)

	err := run(in, &out, &logOut, []string{"out", inputDir})

	assert.NilError(t, err, "\nout: %s\nlogOut: %s", out.String(), logOut.String())
	//
	gitHubSpy.Close() // Avoid races before the following asserts.
	assert.Equal(t, ghReq.State, string(wantState))
	assert.Equal(t, path.Base(ghUrl.Path), wantGitRef)
	//
	googleChatSpy.Close() // Avoid races before the following asserts.
	assert.Assert(t, cmp.Contains(chatMsg.Text, "*state* 🟠 error"))
}

func TestRunPutSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	gitHubCfg := testhelp.GitHubSecretsOrFail(t)
	googleChatCfg := testhelp.GoogleChatSecretsOrFail(t)
	in := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner:        gitHubCfg.Owner,
			Repo:         gitHubCfg.Repo,
			AccessToken:  gitHubCfg.Token,
			GChatWebHook: googleChatCfg.Hook,
			LogLevel:     "debug",
		},
		Params: cogito.PutParams{State: cogito.StateError},
	}))
	var out bytes.Buffer
	var logOut bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote(gitHubCfg.Owner, gitHubCfg.Repo), gitHubCfg.SHA,
		"ref: refs/heads/a-branch-FIXME")
	t.Setenv("BUILD_JOB_NAME", "TestRunPutSuccessIntegration")
	t.Setenv("ATC_EXTERNAL_URL", "https://cogito.invalid")
	t.Setenv("BUILD_PIPELINE_NAME", "the-test-pipeline")
	t.Setenv("BUILD_TEAM_NAME", "the-test-team")

	err := run(in, &out, &logOut, []string{"out", inputDir})

	assert.NilError(t, err, "\nout:\n%s\nlogOut:\n%s", out.String(), logOut.String())
	assert.Assert(t, cmp.Contains(logOut.String(),
		"cogito.put.ghCommitStatus: commit status posted successfully"))
	assert.Assert(t, cmp.Contains(logOut.String(),
		"cogito.put.gChat: state posted successfully to chat"))
}

func TestRunFailure(t *testing.T) {
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
