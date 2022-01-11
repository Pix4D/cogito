package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/help"
	"github.com/google/go-cmp/cmp"
)

func TestGitHubStatusSuccessMockAPI(t *testing.T) {
	cfg := help.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	var testCases = []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "No errors",
			body:   "Anything goes...",
			status: http.StatusCreated,
		},
	}

	for _, tc := range testCases {
		ts := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprintln(w, tc.body)
			}))
		defer ts.Close()

		t.Run(tc.name, func(t *testing.T) {
			ghStatus := github.NewStatus(ts.URL, cfg.Token, cfg.Owner, cfg.Repo, context)
			err := ghStatus.Add(cfg.SHA, state, targetURL, desc)
			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestGitHubStatusFailureMockAPI(t *testing.T) {
	cfg := help.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	var testCases = []struct {
		name       string
		body       string
		wantErr    string
		wantStatus int
	}{
		{
			name: "404 Not Found (multiple causes)",
			body: "fake body",
			wantErr: `Failed to add state "success" for commit 0123456: 404 Not Found
Body: fake body
Hint: one of the following happened:
    1. The repo https://github.com/fakeOwner/fakeRepo doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: [], X-Oauth-Scopes: []`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "500 Internal Server Error",
			body: "fake body",
			wantErr: `Failed to add state "success" for commit 0123456: 500 Internal Server Error
Body: fake body
Hint: Github API is down
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: [], X-Oauth-Scopes: []`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "Any other error",
			body: "fake body",
			wantErr: `Failed to add state "success" for commit 0123456: 418 I'm a teapot
Body: fake body
Hint: none
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: [], X-Oauth-Scopes: []`,
			wantStatus: http.StatusTeapot,
		},
	}

	for _, tc := range testCases {
		ts := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.wantStatus)
				fmt.Fprintln(w, tc.body)
			}))
		defer ts.Close()

		t.Run(tc.name, func(t *testing.T) {
			wantErr := fmt.Sprintf(tc.wantErr, ts.URL)
			ghStatus := github.NewStatus(ts.URL, cfg.Token, cfg.Owner, cfg.Repo, context)
			err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", wantErr)
			}
			var ghError *github.StatusError
			if !errors.As(err, &ghError) {
				t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
			}
			if have, want := ghError.StatusCode, tc.wantStatus; have != want {
				t.Fatalf("status code: have: %d; want: %d", have, want)
			}

			if diff := cmp.Diff(wantErr, err.Error()); diff != "" {
				t.Fatalf("error: (+have -want):\n%s", diff)
			}
		})
	}
}

func TestGitHubStatusIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	status := github.NewStatus(github.API, cfg.Token, cfg.Owner, cfg.Repo, context)
	err := status.Add(cfg.SHA, state, targetURL, desc)

	if err != nil {
		t.Fatalf("\nhave: %v\nwant: no error", err)
	}
}

func TestUnderstandGitHubStatusFailuresIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)

	var testCases = []struct {
		name       string
		token      string
		owner      string
		repo       string
		sha        string
		wantStatus int
	}{
		{"bad token: Unauthorized",
			"bad-token", cfg.Owner, cfg.Repo, "dummy-sha", http.StatusUnauthorized},
		{"non existing repo: Not Found",
			cfg.Token, cfg.Owner, "non-existing-really", "dummy-sha", http.StatusNotFound},
		{"bad SHA: Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "dummy-sha", http.StatusUnprocessableEntity},
		{"tag instead of SHA: Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "v0.0.2", http.StatusUnprocessableEntity},
		{"non existing SHA: Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822", http.StatusUnprocessableEntity},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := github.NewStatus(github.API, tc.token, tc.owner, tc.repo, "dummy")
			err := status.Add(tc.sha, "dummy", "dummy", "dummy")

			var statusErr *github.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.StatusCode != tc.wantStatus {
					t.Fatalf("status code: have %v (%v); want %v (%v)\n%v",
						statusErr.StatusCode, http.StatusText(statusErr.StatusCode),
						tc.wantStatus, http.StatusText(tc.wantStatus), err)
				}
			} else {
				t.Fatalf("have %v; want *github.StatusError", reflect.TypeOf(err))
			}
		})
	}
}
