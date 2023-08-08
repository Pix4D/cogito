package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
)

type mockedResponse struct {
	body               string
	status             int
	rateLimitRemaining string
	rateLimitReset     int64
}

const (
	emptyRateRemaining = "0"    // From the GitHub API.
	fullRateRemaining  = "5000" // From the GitHub API.
)

func TestGitHubStatusSuccessMockAPI(t *testing.T) {
	type testCase struct {
		name     string
		response []mockedResponse
		// wantSleeps:
		// - contains the durations we expect for the sleeps
		// - its size is the number of times we expect the sleep function to be called
		wantSleeps []time.Duration
	}

	cfg := testhelp.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	now := time.Now()
	desc := now.Format("15:04:05")

	run := func(t *testing.T, tc testCase) {
		attempt := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			response := tc.response[attempt]
			if response.body == "" { // default
				response.body = "Anything goes..."
			}
			w.Header().Set("x-ratelimit-remaining", response.rateLimitRemaining)
			w.Header().Set("x-ratelimit-reset", strconv.Itoa(int(response.rateLimitReset)))
			w.WriteHeader(response.status)
			fmt.Fprintln(w, response.body)
			attempt++
		}
		ts := httptest.NewServer(http.HandlerFunc(handler))
		defer ts.Close()
		target := github.Target{
			Server:              ts.URL,
			MaxAttempts:         2,
			WaitTransient:       time.Second,
			MaxSleepRateLimited: 5 * time.Second,
		}
		var haveSleeps []time.Duration
		sleepSpy := func(d time.Duration) {
			haveSleeps = append(haveSleeps, d)
		}
		ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())
		ghStatus.SetSleepFn(sleepSpy)

		err := ghStatus.Add(cfg.SHA, "success", targetURL, desc)

		assert.NilError(t, err)
		assert.DeepEqual(t, haveSleeps, tc.wantSleeps)
	}

	testCases := []testCase{
		{
			name: "Success at first attempt",
			response: []mockedResponse{
				{
					status:             http.StatusCreated,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Unix(),
				},
			},
			wantSleeps: nil,
		},
		{
			name: "Rate limited at the first attempt, success at the second attempt",
			response: []mockedResponse{
				{
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					rateLimitReset:     now.Add(42 * time.Second).Unix(),
				},
				{
					status:             http.StatusCreated,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Add(1 * time.Hour).Unix(),
				},
			},
			wantSleeps: []time.Duration{42 * time.Second},
		},
		{
			name: "retry also on server-side inconsistency (zero or negative sleep time), repro of Pix4D/cogito#124",
			response: []mockedResponse{
				{
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					// This causes sleep time to be 0: it would be silly to fail,
					// we should instead attempt once more. Depending on the problem
					// server-side, the next request might also fail, but at least we
					// did everything we could.
					rateLimitReset: now.Unix(),
				},
				{
					status:             http.StatusCreated,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Add(1 * time.Hour).Unix(),
				},
			},
			wantSleeps: []time.Duration{0 * time.Second},
		},
		{
			name: "Github is flaky (Gateway timeout) at the first attempt, success at second attempt",
			response: []mockedResponse{
				{
					status:             http.StatusGatewayTimeout,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Add(1 * time.Second).Unix(),
				},
				{
					status:             http.StatusCreated,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Add(1 * time.Second).Unix(),
				},
			},
			wantSleeps: []time.Duration{1 * time.Second},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestGitHubStatusFailureMockAPI(t *testing.T) {
	type testCase struct {
		name     string
		response []mockedResponse
		wantErr  string
	}

	cfg := testhelp.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	now := time.Now()
	desc := now.Format("15:04:05")
	maxSleepTime := 1 * time.Minute

	run := func(t *testing.T, tc testCase) {
		attempt := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			response := tc.response[attempt]
			w.Header().Set("x-ratelimit-remaining", response.rateLimitRemaining)
			w.Header().Set("x-ratelimit-reset", strconv.Itoa(int(response.rateLimitReset)))
			w.WriteHeader(response.status)
			fmt.Fprintln(w, response.body)
			attempt++
		}
		ts := httptest.NewServer(http.HandlerFunc(handler))
		defer ts.Close()

		wantErr := fmt.Sprintf(tc.wantErr, ts.URL)
		target := github.Target{
			Server:              ts.URL,
			MaxAttempts:         2,
			WaitTransient:       time.Second,
			MaxSleepRateLimited: maxSleepTime,
		}
		ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())

		err := ghStatus.Add(cfg.SHA, "success", targetURL, desc)

		assert.Error(t, err, wantErr)
		var ghError *github.StatusError
		if !errors.As(err, &ghError) {
			t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
		}
		wantStatus := tc.response[len(tc.response)-1].status
		assert.Equal(t, ghError.StatusCode, wantStatus)
	}

	testCases := []testCase{
		{
			name: "404 Not Found (multiple causes)",
			response: []mockedResponse{
				{
					body:               "fake body",
					status:             http.StatusNotFound,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Unix(),
				},
			},
			wantErr: `failed to add state "success" for commit 0123456: 404 Not Found
Body: fake body
Hint: one of the following happened:
    1. The repo https://github.com/fakeOwner/fakeRepo doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "500 Internal Server Error after 2 attempts",
			response: []mockedResponse{
				{
					body:               "fake body",
					status:             http.StatusServiceUnavailable,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Unix(),
				},
				{
					body:               "fake body",
					status:             http.StatusInternalServerError,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Unix(),
				},
			},
			wantErr: `failed to add state "success" for commit 0123456: 500 Internal Server Error
Body: fake body
Hint: Github API is down
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "Any other error",
			response: []mockedResponse{
				{
					body:               "fake body",
					status:             http.StatusTeapot,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Unix(),
				},
			},
			wantErr: `failed to add state "success" for commit 0123456: 418 I'm a teapot
Body: fake body
Hint: none
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "Rate limited: wait time too long (> MaxSleepRateLimited)",
			response: []mockedResponse{
				{
					body:               "API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]",
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					rateLimitReset:     now.Add(5 * maxSleepTime).Unix(),
				},
			},
			wantErr: `failed to add state "success" for commit 0123456: 403 Forbidden
Body: API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]
Hint: Rate limited but the wait time to reset would be longer than 1m0s (MaxSleepRateLimited)
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestGitHubStatusSuccessIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	cfg := testhelp.GitHubSecretsOrFail(t)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	target := github.Target{
		Server:              github.API,
		MaxAttempts:         2,
		WaitTransient:       time.Second,
		MaxSleepRateLimited: 5 * time.Second,
	}
	ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())

	err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

	assert.NilError(t, err)
}

func TestGitHubStatusFailureIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	type testCase struct {
		name       string
		token      string // default: cfg.Token
		owner      string // default: cfg.Owner
		repo       string // default: cfg.Repo
		sha        string // default: cfg.SHA
		wantErr    string
		wantStatus int
	}

	cfg := testhelp.GitHubSecretsOrFail(t)
	state := "success"

	run := func(t *testing.T, tc testCase) {
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

		target := github.Target{
			Server:              github.API,
			MaxAttempts:         2,
			WaitTransient:       time.Second,
			MaxSleepRateLimited: 5 * time.Second,
		}
		ghStatus := github.NewCommitStatus(target, tc.token, tc.owner, tc.repo, "dummy-context", hclog.NewNullLogger())
		err := ghStatus.Add(tc.sha, state, "dummy-url", "dummy-desc")

		assert.Error(t, err, tc.wantErr)
		var ghError *github.StatusError
		if !errors.As(err, &ghError) {
			t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
		}
		assert.Equal(t, ghError.StatusCode, tc.wantStatus)
	}

	testCases := []testCase{
		{
			name:  "bad token: Unauthorized",
			token: "bad-token",
			wantErr: `failed to add state "success" for commit 751affd: 401 Unauthorized
Body: {"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}
Hint: Either wrong credentials or PAT expired (check your email for expiration notice)
Action: POST https://api.github.com/repos/pix4d/cogito-test-read-write/statuses/751affd155db7a00d936ee6e9f483deee69c5922
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "non existing repo: Not Found",
			repo: "non-existing-really",
			wantErr: `failed to add state "success" for commit 751affd: 404 Not Found
Body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status"}
Hint: one of the following happened:
    1. The repo https://github.com/pix4d/non-existing-really doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status
Action: POST https://api.github.com/repos/pix4d/non-existing-really/statuses/751affd155db7a00d936ee6e9f483deee69c5922
OAuth: X-Accepted-Oauth-Scopes: repo, X-Oauth-Scopes: repo:status`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "non existing SHA: Unprocessable Entity",
			sha:  "e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822",
			wantErr: `failed to add state "success" for commit e576e3a: 422 Unprocessable Entity
Body: {"message":"No commit found for SHA: e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status"}
Hint: none
Action: POST https://api.github.com/repos/pix4d/cogito-test-read-write/statuses/e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: repo:status`,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}
