package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/retry"
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

// Copied from ghcommitsink.go
// Note that sometimes in tests we override these values for practical reasons.
const (
	// retryUpTo is the total maximum duration of the retries.
	retryUpTo = 15 * time.Minute

	// retryFirstDelay is duration of the first backoff.
	retryFirstDelay = 2 * time.Second

	// retryBackoffLimit is the upper bound duration of a backoff.
	// That is, with an exponential backoff and a retryFirstDelay = 2s, the sequence will be:
	// 2s 4s 8s 16s 32s 60s ... 60s, until reaching a cumulative delay of retryUpTo.
	retryBackoffLimit = 1 * time.Minute
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
		log := testhelp.MakeTestLog()
		sleepSpy := SleepSpy{}
		target := &github.Target{
			Server: ts.URL,
			Retry: retry.Retry{
				FirstDelay:   retryFirstDelay,
				BackoffLimit: retryBackoffLimit,
				UpTo:         retryUpTo,
				SleepFn:      sleepSpy.Sleep,
				Log:          log,
			},
		}
		ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, log, false, cfg.PrivateKey, cfg.ApplicationId, cfg.InstallationId)

		err := ghStatus.Add(cfg.SHA, "success", targetURL, desc)

		assert.NilError(t, err)
		assert.DeepEqual(t, sleepSpy.sleeps, tc.wantSleeps)
	}

	testCases := []testCase{
		{
			name: "Success at first attempt",
			response: []mockedResponse{
				{status: http.StatusCreated},
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
			name: "retry also on server-side inconsistency (zero sleep time), repro of Pix4D/cogito#124",
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
			wantSleeps: []time.Duration{retryFirstDelay},
		},
		{
			name: "retry also on server-side inconsistency (negative sleep time), repro of Pix4D/cogito#124",
			response: []mockedResponse{
				{
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					// This causes sleep time to be < 0.
					rateLimitReset: now.Add(-30 * time.Minute).Unix(),
				},
				{
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					// This causes sleep time to be < 0.
					rateLimitReset: now.Add(-30 * time.Minute).Unix(),
				},
				{
					status:             http.StatusCreated,
					rateLimitRemaining: fullRateRemaining,
					rateLimitReset:     now.Add(1 * time.Hour).Unix(),
				},
			},
			wantSleeps: []time.Duration{retryFirstDelay, 2 * retryFirstDelay},
		},
		{
			name: "Github is flaky at the first attempt, success at 3rd attempt",
			response: []mockedResponse{
				{status: http.StatusGatewayTimeout},
				{status: http.StatusGatewayTimeout},
				{status: http.StatusCreated},
			},
			wantSleeps: []time.Duration{retryFirstDelay, 2 * retryFirstDelay},
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
		// wantSleeps:
		// - contains the durations we expect for the sleeps
		// - its size is the number of times we expect the sleep function to be called
		wantSleeps []time.Duration
		wantErr    string
	}

	cfg := testhelp.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	now := time.Now()
	desc := now.Format("15:04:05")
	upTo := 5 * time.Minute

	run := func(t *testing.T, tc testCase) {
		attempt := 0
		handler := func(w http.ResponseWriter, r *http.Request) {
			response := tc.response[attempt]
			if response.body == "" {
				response.body = "fake body"
			}
			w.Header().Set("x-ratelimit-remaining", response.rateLimitRemaining)
			w.Header().Set("x-ratelimit-reset", strconv.Itoa(int(response.rateLimitReset)))
			// The Date header is set automatically by default, but we override it
			// for better control.
			w.Header().Set("Date", now.Format(time.RFC1123))
			w.WriteHeader(response.status)
			fmt.Fprintln(w, response.body)
			attempt++
		}
		ts := httptest.NewServer(http.HandlerFunc(handler))
		defer ts.Close()
		wantErr := fmt.Sprintf(tc.wantErr, ts.URL)
		log := testhelp.MakeTestLog()
		sleepSpy := SleepSpy{}
		target := &github.Target{
			Server: ts.URL,
			Retry: retry.Retry{
				FirstDelay:   retryFirstDelay,
				BackoffLimit: retryBackoffLimit,
				UpTo:         upTo,
				SleepFn:      sleepSpy.Sleep,
				Log:          log,
			},
		}
		ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, log, false, cfg.PrivateKey, cfg.ApplicationId, cfg.InstallationId)

		err := ghStatus.Add(cfg.SHA, "success", targetURL, desc)

		assert.Error(t, err, wantErr)
		var ghError *github.StatusError
		if !errors.As(err, &ghError) {
			t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
		}
		assert.DeepEqual(t, sleepSpy.sleeps, tc.wantSleeps)
		wantStatus := tc.response[len(tc.response)-1].status
		assert.Equal(t, ghError.StatusCode, wantStatus)
	}

	testCases := []testCase{
		{
			name: "non transient error, stop at first attempt",
			response: []mockedResponse{
				{
					body:   "fake body",
					status: http.StatusNotFound,
				},
			},
			wantSleeps: nil,
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
			name: "transient error, consume all attempts",
			response: []mockedResponse{
				//                                                cumulative
				{status: http.StatusInternalServerError}, // 2s     2s
				{status: http.StatusInternalServerError}, // 4s     6s
				{status: http.StatusInternalServerError}, // 8s    14s
				{status: http.StatusInternalServerError}, // 16s   30s
				{status: http.StatusInternalServerError}, // 32s  1m2s
				{status: http.StatusInternalServerError}, // 1m   2m2s
				{status: http.StatusInternalServerError}, // 1m   3m2s
				{status: http.StatusInternalServerError}, // 1m   4m2s
				{status: http.StatusInternalServerError}, // 1m   5m2s too long
			},
			wantSleeps: []time.Duration{
				retryFirstDelay,
				4 * time.Second,
				8 * time.Second,
				16 * time.Second,
				32 * time.Second,
				1 * time.Minute,
				1 * time.Minute,
				1 * time.Minute,
			},
			wantErr: `failed to add state "success" for commit 0123456: 500 Internal Server Error
Body: fake body
Hint: Github API is down
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "Rate limited: wait time too long (> Retry.UpTo)",
			response: []mockedResponse{
				{
					body:               "API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]",
					status:             http.StatusForbidden,
					rateLimitRemaining: emptyRateRemaining,
					rateLimitReset:     now.Add(upTo + time.Second).Unix(),
				},
			},
			wantSleeps: nil,
			wantErr: `failed to add state "success" for commit 0123456: 403 Forbidden
Body: API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]
Hint: Rate limited but the wait time to reset would be longer than 5m0s (Retry.UpTo)
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

	useGithubApp := false
	cfg := testhelp.GitHubSecretsOrFail(t, useGithubApp)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"
	log := testhelp.MakeTestLog()
	target := &github.Target{
		Server: github.ApiRoot(github.GhDefaultHostname),
		Retry: retry.Retry{
			FirstDelay:   retryFirstDelay,
			BackoffLimit: retryBackoffLimit,
			UpTo:         retryUpTo,
			Log:          log,
		},
	}
	ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, log, cfg.UseGithubApp, cfg.PrivateKey, cfg.ApplicationId, cfg.InstallationId)

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

	cfg := testhelp.GitHubSecretsOrFail(t, false)
	state := "success"
	log := testhelp.MakeTestLog()

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

		target := &github.Target{
			Server: github.ApiRoot(github.GhDefaultHostname),
			Retry: retry.Retry{
				FirstDelay:   retryFirstDelay,
				BackoffLimit: retryBackoffLimit,
				UpTo:         retryUpTo,
				Log:          log,
			},
		}
		ghStatus := github.NewCommitStatus(target, tc.token, tc.owner, tc.repo, "dummy-context", log, false, "", int64(1), int64(1))
		err := ghStatus.Add(tc.sha, state, "dummy-url", "dummy-desc")

		assert.Assert(t, err != nil)
		haveErr := err.Error()
		assert.Equal(t, haveErr, tc.wantErr)
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
Body: {"message":"Bad credentials","documentation_url":"https://docs.github.com/rest","status":"401"}
Hint: Either wrong credentials or PAT expired (check your email for expiration notice)
Action: POST https://api.github.com/repos/pix4d/cogito-test-read-write/statuses/751affd155db7a00d936ee6e9f483deee69c5922
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "non existing repo: Not Found",
			repo: "non-existing-really",
			wantErr: `failed to add state "success" for commit 751affd: 404 Not Found
Body: {"message":"Not Found","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status","status":"404"}
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
Body: {"message":"No commit found for SHA: e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822","documentation_url":"https://docs.github.com/rest/commits/statuses#create-a-commit-status","status":"422"}
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

type SleepSpy struct {
	sleeps []time.Duration
}

func (spy *SleepSpy) Sleep(d time.Duration) {
	spy.sleeps = append(spy.sleeps, d)
}

func TestApiRoot(t *testing.T) {
	type testCase struct {
		name     string
		hostname string
		wantAPI  string
	}

	run := func(t *testing.T, tc testCase) {
		got := github.ApiRoot(tc.hostname)
		assert.Equal(t, got, tc.wantAPI)
	}

	testCases := []testCase{
		{
			name:     "hostname is localhost from http testserver",
			hostname: "127.0.0.1:5678",
			wantAPI:  "http://127.0.0.1:5678",
		},
		{
			name:     "default GitHub hostname",
			hostname: github.GhDefaultHostname,
			wantAPI:  "https://api.github.com",
		},
		{
			name:     "Github Enterprise hostname",
			hostname: "github.mycompany.org",
			wantAPI:  "https://github.mycompany.org/api/v3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestGithubAppAuthSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	cfg := testhelp.GitHubSecretsOrFail(t, true)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"
	log := testhelp.MakeTestLog()
	target := &github.Target{
		Server: github.ApiRoot(github.GhDefaultHostname),
		Retry: retry.Retry{
			FirstDelay:   retryFirstDelay,
			BackoffLimit: retryBackoffLimit,
			UpTo:         retryUpTo,
			Log:          log,
		},
	}
	ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, log, cfg.UseGithubApp, cfg.PrivateKey, cfg.ApplicationId, cfg.InstallationId)

	err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

	assert.NilError(t, err)
}
