package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/googlechat"
	"github.com/Pix4D/cogito/testhelp"
)

func TestRunCheckSuccess(t *testing.T) {
	stdin := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret",
    "log_level": "debug"
  }
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := mainErr(stdin, &stdout, &stderr, []string{"check"})

	assert.NilError(t, err, "\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
}

func TestRunGetSuccess(t *testing.T) {
	stdin := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret",
    "log_level": "debug"
  },
  "version": {"ref": "pizza"}
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := mainErr(stdin, &stdout, &stderr, []string{"in", "dummy-dir"})

	assert.NilError(t, err, "\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
}

func TestRunPutSuccess(t *testing.T) {
	wantState := cogito.StateError
	wantGitRef := "dummyHead"
	var ghReq github.AddRequest
	var ghUrl *url.URL
	gitHubSpy := testhelp.SpyHttpServer(&ghReq, nil, &ghUrl, http.StatusCreated)
	gitHubSpyURL, err := url.Parse(gitHubSpy.URL)
	assert.NilError(t, err, "error parsing SpyHttpServer URL: %s", err)
	var chatMsg googlechat.BasicMessage
	chatReply := googlechat.MessageReply{}
	var gchatUrl *url.URL
	googleChatSpy := testhelp.SpyHttpServer(&chatMsg, chatReply, &gchatUrl, http.StatusOK)
	stdin := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner:        "the-owner",
			Repo:         "the-repo",
			AccessToken:  "the-secret",
			GhHostname:   gitHubSpyURL.Host,
			GChatWebHook: googleChatSpy.URL,
			LogLevel:     "debug",
		},
		Params: cogito.PutParams{State: wantState},
	}))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote(gitHubSpyURL.Host, "the-owner", "the-repo"), "dummySHA", wantGitRef)

	err = mainErr(stdin, &stdout, &stderr, []string{"out", inputDir})

	assert.NilError(t, err, "\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
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
	stdin := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner:        gitHubCfg.Owner,
			Repo:         gitHubCfg.Repo,
			AccessToken:  gitHubCfg.Token,
			GChatWebHook: googleChatCfg.Hook,
			LogLevel:     "debug",
		},
		Params: cogito.PutParams{State: cogito.StateError},
	}))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote(github.GhDefaultHostname, gitHubCfg.Owner, gitHubCfg.Repo), gitHubCfg.SHA,
		"ref: refs/heads/a-branch-FIXME")
	t.Setenv("BUILD_JOB_NAME", "TestRunPutSuccessIntegration")
	t.Setenv("ATC_EXTERNAL_URL", "https://cogito.example")
	t.Setenv("BUILD_PIPELINE_NAME", "the-test-pipeline")
	t.Setenv("BUILD_TEAM_NAME", "the-test-team")
	t.Setenv("BUILD_NAME", "42")

	err := mainErr(stdin, &stdout, &stderr, []string{"out", inputDir})

	assert.NilError(t, err, "\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	assert.Assert(t, cmp.Contains(stderr.String(),
		`level=INFO msg="commit status posted successfully" name=cogito.put name=ghCommitStatus state=error`))
	assert.Assert(t, cmp.Contains(stderr.String(),
		`level=INFO msg="state posted successfully to chat" name=cogito.put name=gChat state=error`))
}

func TestRunPutGhAppSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	gitHubCfg := testhelp.GitHubSecretsOrFail(t)
	ghAppInstallationID, err := strconv.ParseInt(gitHubCfg.GhAppInstallationID, 10, 32)
	assert.NilError(t, err)

	stdin := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner: gitHubCfg.Owner,
			Repo:  gitHubCfg.Repo,
			GitHubApp: github.GitHubApp{
				ClientId:       gitHubCfg.GhAppClientID,
				InstallationId: int(ghAppInstallationID),
				PrivateKey:     gitHubCfg.GhAppPrivateKey,
			},
			LogLevel: "debug",
		},
		Params: cogito.PutParams{State: cogito.StateError},
	}))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote(github.GhDefaultHostname, gitHubCfg.Owner, gitHubCfg.Repo), gitHubCfg.SHA,
		"ref: refs/heads/a-branch-FIXME")
	t.Setenv("BUILD_JOB_NAME", "TestRunPutGhAppSuccessIntegration")
	t.Setenv("ATC_EXTERNAL_URL", "https://cogito.example")
	t.Setenv("BUILD_PIPELINE_NAME", "the-test-pipeline")
	t.Setenv("BUILD_TEAM_NAME", "the-test-team")
	t.Setenv("BUILD_NAME", "42")

	err = mainErr(stdin, &stdout, &stderr, []string{"out", inputDir})

	assert.NilError(t, err, "\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	assert.Assert(t, cmp.Contains(stderr.String(),
		`level=INFO msg="commit status posted successfully" name=cogito.put name=ghCommitStatus state=error`))
}

func TestRunFailure(t *testing.T) {
	type testCase struct {
		name    string
		args    []string
		stdin   string
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		stdin := strings.NewReader(tc.stdin)

		err := mainErr(stdin, nil, io.Discard, tc.args)

		assert.ErrorContains(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "unknown command",
			args:    []string{"foo"},
			wantErr: `invoked as 'foo'; want: one of [check in out]`,
		},
		{
			name: "check, wrong stdin",
			args: []string{"check"},
			stdin: `
{
  "fruit": "banana"
}`,
			wantErr: `check: parsing request: json: unknown field "fruit"`,
		},
		{
			name:    "peeking for log_level",
			args:    []string{"check"},
			stdin:   "",
			wantErr: "peeking into JSON for log_level: unexpected end of JSON input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Assert(t, tc.wantErr != "")
			test(t, tc)
		})
	}
}

func TestRunSystemFailure(t *testing.T) {
	stdin := iotest.ErrReader(errors.New("test read error"))

	err := mainErr(stdin, nil, io.Discard, []string{"check"})

	assert.ErrorContains(t, err, "test read error")
}

func TestRunPrintsBuildInformation(t *testing.T) {
	stdin := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret"
  }
}`)
	var stderr bytes.Buffer
	wantLog := "This is the Cogito GitHub status resource. unknown"

	err := mainErr(stdin, io.Discard, &stderr, []string{"check"})
	assert.NilError(t, err)
	haveLog := stderr.String()

	assert.Assert(t, strings.Contains(haveLog, wantLog),
		"\nhave: %s\nwant: %s", haveLog, wantLog)
}
