package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	testCases := []struct {
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
			ghStatus := github.NewCommitStatus(ts.URL, cfg.Token, cfg.Owner, cfg.Repo, context)
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

	testCases := []struct {
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
			ghStatus := github.NewCommitStatus(ts.URL, cfg.Token, cfg.Owner, cfg.Repo, context)
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

func TestGitHubStatusSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	ghStatus := github.NewCommitStatus(github.API, cfg.Token, cfg.Owner, cfg.Repo, context)
	err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

	if err != nil {
		t.Fatalf("\nhave: %s\nwant: <no error>", err)
	}
}

func TestGitHubStatusFailureIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	cfg := help.SkipTestIfNoEnvVars(t)
	state := "success"

	testCases := []struct {
		name       string
		token      string // default: cfg.Token
		owner      string // default: cfg.Owner
		repo       string // default: cfg.Repo
		sha        string // default: cfg.SHA
		wantErr    string
		wantStatus int
	}{
		{
			name:  "bad token: Unauthorized",
			token: "bad-token",
			wantErr: `Failed to add state "success" for commit 32e4b4f: 401 Unauthorized
Body: {"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}
Hint: none
Action: POST https://api.github.com/repos/pix4d/cogito-test-read-write/statuses/32e4b4f91bb8de500f6a7aa2011f93c3f322381c
OAuth: X-Accepted-Oauth-Scopes: [], X-Oauth-Scopes: []`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "non existing repo: Not Found",
			repo: "non-existing-really",
			wantErr: `Failed to add state "success" for commit 32e4b4f: 404 Not Found
Body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status"}
Hint: one of the following happened:
    1. The repo https://github.com/pix4d/non-existing-really doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status
Action: POST https://api.github.com/repos/pix4d/non-existing-really/statuses/32e4b4f91bb8de500f6a7aa2011f93c3f322381c
OAuth: X-Accepted-Oauth-Scopes: [repo], X-Oauth-Scopes: [repo:status]`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "non existing SHA: Unprocessable Entity",
			sha:  "e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822",
			wantErr: `Failed to add state "success" for commit e576e3a: 422 Unprocessable Entity
Body: {"message":"No commit found for SHA: e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status"}
Hint: none
Action: POST https://api.github.com/repos/pix4d/cogito-test-read-write/statuses/e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822
OAuth: X-Accepted-Oauth-Scopes: [], X-Oauth-Scopes: [repo:status]`,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// zero values are defaults
			if tc.token == "" {
				tc.token = cfg.Token
			}
			if tc.owner == "" {
				tc.owner = cfg.Owner
			}
			if tc.repo == "" {
				tc.repo = cfg.Repo
			}
			if tc.sha == "" {
				tc.sha = cfg.SHA
			}

			ghStatus := github.NewCommitStatus(github.API, tc.token, tc.owner, tc.repo,
				"dummy-context")
			err := ghStatus.Add(tc.sha, state, "dummy-url", "dummy-desc")

			if err == nil {
				t.Fatal("\nhave: <no error>\nwant: <some error>")
			}
			var ghError *github.StatusError
			if !errors.As(err, &ghError) {
				t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
			}
			if have, want := ghError.StatusCode, tc.wantStatus; have != want {
				t.Fatalf("status code: have: %d; want: %d", have, want)
			}

			if diff := cmp.Diff(tc.wantErr, err.Error()); diff != "" {
				t.Fatalf("error: (+have -want):\n%s", diff)
			}
		})
	}
}
