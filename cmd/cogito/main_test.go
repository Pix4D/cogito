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
	"github.com/Pix4D/cogito/testhelp"
	"gotest.tools/v3/assert"
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
	wantState := cogito.StatePending
	wantGitRef := "dummyHead"
	var ghReq github.AddRequest
	var URL *url.URL
	ts := testhelp.SpyHttpServer(&ghReq, &URL, http.StatusCreated)
	in := strings.NewReader(`
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-secret",
    "log_level": "debug"
  },
  "params": {"state": "pending"}
}`)
	var out bytes.Buffer
	var logOut bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote("the-owner", "the-repo"), "dummySHA", wantGitRef)
	t.Setenv("COGITO_GITHUB_API", ts.URL)

	err := run(in, &out, &logOut, []string{"out", inputDir})

	assert.NilError(t, err, "\nout: %s\nlogOut: %s", out.String(), logOut.String())
	ts.Close() // Avoid races before the following asserts.
	assert.Equal(t, ghReq.State, string(wantState), "", ghReq.State, string(wantState))
	assert.Equal(t, path.Base(URL.Path), wantGitRef)
}

func TestRunPutSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	cfg := testhelp.SkipTestIfNoEnvVars(t)
	in := bytes.NewReader(testhelp.ToJSON(t, cogito.PutRequest{
		Source: cogito.Source{
			Owner:       cfg.Owner,
			Repo:        cfg.Repo,
			AccessToken: cfg.Token,
			LogLevel:    "debug",
		},
		Params: cogito.PutParams{State: cogito.StatePending},
	}))
	var out bytes.Buffer
	var logOut bytes.Buffer
	inputDir := testhelp.MakeGitRepoFromTestdata(t, "../../cogito/testdata/one-repo/a-repo",
		testhelp.HttpsRemote(cfg.Owner, cfg.Repo), cfg.SHA, "ref: refs/heads/a-branch-FIXME")
	t.Setenv("BUILD_JOB_NAME", "TestRunPutSuccessIntegration")
	t.Setenv("ATC_EXTERNAL_URL", "https://cogito.invalid")

	err := run(in, &out, &logOut, []string{"out", inputDir})

	assert.NilError(t, err, "\nout: %s\nlogOut: %s", out.String(), logOut.String())
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
