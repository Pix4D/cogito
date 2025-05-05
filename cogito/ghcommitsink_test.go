package cogito_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
)

func TestSinkGitHubCommitStatusSendSuccess(t *testing.T) {
	wantGitRef := "deadbeefdeadbeef"
	wantState := cogito.StatePending
	jobName := "the-job"
	wantContext := jobName
	var ghReq github.AddRequest
	var URL *url.URL
	ts := testhelp.SpyHttpServer(&ghReq, nil, &URL, http.StatusCreated)
	gitHubSpyURL, err := url.Parse(ts.URL)
	assert.NilError(t, err, "error parsing SpyHttpServer URL: %s", err)
	sink := cogito.GitHubCommitStatusSink{
		Log:    testhelp.MakeTestLog(),
		GitRef: wantGitRef,
		Request: cogito.PutRequest{
			Source: cogito.Source{GhHostname: gitHubSpyURL.Host, AccessToken: "dummy-token"},
			Params: cogito.PutParams{State: wantState},
			Env:    cogito.Environment{BuildJobName: jobName},
		},
	}

	err = sink.Send()

	assert.NilError(t, err)
	ts.Close() // Avoid races before the following asserts.
	assert.Equal(t, path.Base(URL.Path), wantGitRef)
	assert.Equal(t, ghReq.State, string(wantState))
	assert.Equal(t, ghReq.Context, wantContext)
}

func TestSinkGitHubCommitStatusSendGhAppSuccess(t *testing.T) {
	wantGitRef := "deadbeefdeadbeef"
	wantState := cogito.StatePending
	jobName := "the-job"
	wantContext := jobName
	var ghReq github.AddRequest

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/repos/statuses/deadbeefdeadbeef" {
			dec := json.NewDecoder(r.Body)
			if err := dec.Decode(&ghReq); err != nil {
				w.WriteHeader(http.StatusTeapot)
				fmt.Fprintln(w, "test: decoding request:", err)
				return
			}
		}

		w.WriteHeader(http.StatusCreated)
		if r.URL.String() == "/app/installations/12345/access_tokens" {
			fmt.Fprintln(w, `{"token": "dummy_installation_token"}`)
			return
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	gitHubSpyURL, err := url.Parse(ts.URL)
	assert.NilError(t, err, "error parsing SpyHttpServer URL: %s", err)

	privateKey, err := testhelp.GeneratePrivateKey(t, 2048)
	assert.NilError(t, err)

	app := github.GitHubApp{
		ClientId:       "client-id",
		InstallationId: 12345,
		PrivateKey:     testhelp.EncodePrivateKeyToPEM(privateKey),
	}
	err = app.Validate()
	assert.NilError(t, err)

	sink := cogito.GitHubCommitStatusSink{
		Log:    testhelp.MakeTestLog(),
		GitRef: wantGitRef,
		Request: cogito.PutRequest{
			Source: cogito.Source{
				GhHostname: gitHubSpyURL.Host,
				GitHubApp:  app,
			},
			Params: cogito.PutParams{State: wantState},
			Env:    cogito.Environment{BuildJobName: jobName},
		},
	}

	err = sink.Send()

	assert.NilError(t, err)
	assert.Equal(t, ghReq.State, string(wantState))
	assert.Equal(t, ghReq.Context, wantContext)
}

func TestSinkGitHubCommitStatusSendFailure(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
	gitHubSpyURL, err := url.Parse(ts.URL)
	assert.NilError(t, err, "error parsing SpyHttpServer URL: %s", err)
	defer ts.Close()
	sink := cogito.GitHubCommitStatusSink{
		Log:    testhelp.MakeTestLog(),
		GitRef: "deadbeefdeadbeef",
		Request: cogito.PutRequest{
			Source: cogito.Source{GhHostname: gitHubSpyURL.Host, AccessToken: "dummy-token"},
			Params: cogito.PutParams{State: cogito.StatePending},
		},
	}

	err = sink.Send()

	assert.ErrorContains(t, err,
		`failed to add state "pending" for commit deadbee: 418 I'm a teapot`)
}
