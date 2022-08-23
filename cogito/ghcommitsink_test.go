package cogito_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

func TestSinkGitHubCommitStatusSendSuccess(t *testing.T) {
	wantGitRef := "deadbeefdeadbeef"
	wantState := cogito.StatePending
	jobName := "the-job"
	wantContext := jobName
	var ghReq github.AddRequest
	var URL *url.URL
	ts := testhelp.SpyHttpServer(&ghReq, nil, &URL, http.StatusCreated)
	sink := cogito.GitHubCommitStatusSink{
		Log:    hclog.NewNullLogger(),
		GhAPI:  ts.URL,
		GitRef: wantGitRef,
		Request: cogito.PutRequest{
			Params: cogito.PutParams{State: wantState},
			Env:    cogito.Environment{BuildJobName: jobName},
		},
	}

	err := sink.Send()

	assert.NilError(t, err)
	ts.Close() // Avoid races before the following asserts.
	assert.Equal(t, path.Base(URL.Path), wantGitRef)
	assert.Equal(t, ghReq.State, string(wantState))
	assert.Equal(t, ghReq.Context, wantContext)
}

func TestSinkGitHubCommitStatusSendFailure(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
	defer ts.Close()
	sink := cogito.GitHubCommitStatusSink{
		Log:    hclog.NewNullLogger(),
		GhAPI:  ts.URL,
		GitRef: "deadbeefdeadbeef",
		Request: cogito.PutRequest{
			Params: cogito.PutParams{State: cogito.StatePending},
		},
	}

	err := sink.Send()

	assert.ErrorContains(t, err,
		`failed to add state "pending" for commit deadbee: 418 I'm a teapot`)
}
