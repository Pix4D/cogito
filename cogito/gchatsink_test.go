package cogito_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/googlechat"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestSinkGoogleChatSendSuccess(t *testing.T) {
	wantGitRef := "deadbeef"
	wantState := cogito.StateError // We want a state that is sent by default
	var message googlechat.BasicMessage
	var URL *url.URL
	ts := testhelp.SpyHttpServer(&message, &URL, http.StatusOK)
	sink := cogito.GoogleChatSink{
		Log:    hclog.NewNullLogger(),
		GitRef: wantGitRef,
		Request: cogito.PutRequest{
			Source: cogito.Source{GChatWebHook: ts.URL},
			Params: cogito.PutParams{State: wantState},
			Env: cogito.Environment{
				BuildPipelineName: "the-test-pipeline",
				BuildJobName:      "the-test-job",
			},
		},
	}

	err := sink.Send()

	assert.NilError(t, err)
	ts.Close() // Avoid races before the following asserts.
	assert.Assert(t, cmp.Contains(message.Text, "*state* 🟠 error"))
	assert.Assert(t, cmp.Contains(message.Text, "*pipeline* the-test-pipeline"))
	assert.Assert(t, cmp.Contains(message.Text, "*job* the-test-job"))
	assert.Assert(t, cmp.Contains(URL.String(), "/?threadKey=the-test-pipeline+deadbeef"))
}

func TestSinkGoogleChatDoesNotSendSuccess(t *testing.T) {
	type testCase struct {
		name    string
		request cogito.PutRequest
	}

	test := func(t *testing.T, tc testCase) {
		sink := cogito.GoogleChatSink{
			Log:     hclog.NewNullLogger(),
			Request: tc.request,
		}

		err := sink.Send()

		assert.NilError(t, err)
	}

	testCases := []testCase{
		{
			name: "feature not enabled",
			request: cogito.PutRequest{
				Source: cogito.Source{GChatWebHook: ""},            // empty
				Params: cogito.PutParams{State: cogito.StateError}, // sent by default
			},
		},
		{
			name: "state not in enabled states",
			request: cogito.PutRequest{
				Source: cogito.Source{GChatWebHook: "https://cogito.invalid"},
				Params: cogito.PutParams{State: cogito.StatePending}, // not sent by default
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}

func TestSinkGoogleChatSendFailure(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
	sink := cogito.GoogleChatSink{
		Log: hclog.NewNullLogger(),
		Request: cogito.PutRequest{
			Source: cogito.Source{GChatWebHook: ts.URL},
			Params: cogito.PutParams{State: cogito.StateError}, // sent by default
		},
	}

	err := sink.Send()

	assert.ErrorContains(t, err, "GoogleChatSink: TextMessage: status: 418 I'm a teapot")
	ts.Close()
}
