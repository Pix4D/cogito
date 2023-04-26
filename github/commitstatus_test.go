package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-hclog"
)

type mockedResponse struct {
	body               string
	statuses           []int
	rateLimitRemaining []string
	rateLimitReset     []int
}

func TestGitHubStatusSuccessMockAPI(t *testing.T) {
	cfg := testhelp.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"
	now := int(time.Now().Unix())
	testCases := []struct {
		name     string
		response mockedResponse
	}{
		{
			name: "No errors",
			response: mockedResponse{
				body:               "Anything goes...",
				statuses:           []int{http.StatusCreated},
				rateLimitRemaining: []string{"5000"},
				rateLimitReset:     []int{now},
			},
		},
		{
			name: "Rate limited in the first attempt, success in second attempt",
			response: mockedResponse{
				body:     "Anything goes...",
				statuses: []int{http.StatusForbidden, http.StatusCreated},
				// In the first request there is remaining rate is 0, for the second request
				// it resets to 5000 (default Github rate limit for authenticated users)
				rateLimitRemaining: []string{"0", "5000"},
				// In first request, rate limit will reset 1s after the attempt.
				// Keep this value low to prevent tests to sleep for too long
				// note that 'now' is already a UNIX time in seconds (integer)
				rateLimitReset: []int{now + 1, now + 3600},
			},
		},
		{
			name: "Github is flaky (Gateway timeout) in the first attempt, success in second attempt",
			response: mockedResponse{
				body:               "Anything goes...",
				statuses:           []int{http.StatusGatewayTimeout, http.StatusCreated},
				rateLimitRemaining: []string{"5000", "5000"},
				rateLimitReset:     []int{now + 1, now + 1},
			},
		},
	}

	for _, tc := range testCases {
		attempt := 0
		mockedResponse := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("x-ratelimit-remaining", tc.response.rateLimitRemaining[attempt])
			w.Header().Set("x-ratelimit-reset", strconv.Itoa(tc.response.rateLimitReset[attempt]))
			w.WriteHeader(tc.response.statuses[attempt])
			fmt.Fprintln(w, tc.response.body)
			attempt++
		}
		ts := httptest.NewServer(http.HandlerFunc(mockedResponse))
		defer ts.Close()

		target := github.Target{
			Server:       ts.URL,
			MaxRetries:   2,
			WaitTime:     time.Second,
			MaxSleepTime: 5 * time.Second,
		}
		t.Run(tc.name, func(t *testing.T) {
			ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())
			err := ghStatus.Add(cfg.SHA, state, targetURL, desc)
			if err != nil {
				t.Fatalf("\nhave: %s\nwant: <no error>", err)
			}
		})
	}
}

func TestGitHubStatusFailureMockAPI(t *testing.T) {
	cfg := testhelp.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"
	now := int(time.Now().Unix())

	testCases := []struct {
		name     string
		response mockedResponse
		wantErr  string
	}{
		{
			name: "404 Not Found (multiple causes)",
			response: mockedResponse{
				body:               "fake body",
				statuses:           []int{http.StatusNotFound},
				rateLimitRemaining: []string{"5000"},
				rateLimitReset:     []int{now},
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
			response: mockedResponse{
				body:               "fake body",
				statuses:           []int{http.StatusServiceUnavailable, http.StatusInternalServerError},
				rateLimitRemaining: []string{"5000", "5000"},
				rateLimitReset:     []int{now, now},
			},
			wantErr: `failed to add state "success" for commit 0123456: 500 Internal Server Error
Body: fake body
Hint: Github API is down
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "Any other error",
			response: mockedResponse{
				body:               "fake body",
				statuses:           []int{http.StatusTeapot},
				rateLimitRemaining: []string{"5000"},
				rateLimitReset:     []int{now},
			},
			wantErr: `failed to add state "success" for commit 0123456: 418 I'm a teapot
Body: fake body
Hint: none
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
		{
			name: "Rate limited: wait time too long (> MaxSleepTime)",
			response: mockedResponse{
				body:               "API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]",
				statuses:           []int{http.StatusForbidden},
				rateLimitRemaining: []string{"0"},
				// the value must be higher than what is configured target.MaxSleepTime
				rateLimitReset: []int{now + 5*int(time.Minute.Seconds())},
			},
			wantErr: `failed to add state "success" for commit 0123456: 403 Forbidden
Body: API rate limit exceeded for user ID 123456789. [rate reset in XXmXXs]
Hint: Rate limited but the wait time to reset would be longer than 1m0s (MaxSleepTime)
Action: POST %s/repos/fakeOwner/fakeRepo/statuses/0123456789012345678901234567890123456789
OAuth: X-Accepted-Oauth-Scopes: , X-Oauth-Scopes: `,
		},
	}

	for _, tc := range testCases {
		attempt := 0
		mockedResponse := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("x-ratelimit-remaining", tc.response.rateLimitRemaining[attempt])
			w.Header().Set("x-ratelimit-reset", strconv.Itoa(tc.response.rateLimitReset[attempt]))
			w.WriteHeader(tc.response.statuses[attempt])
			fmt.Fprintln(w, tc.response.body)
			attempt++
		}
		ts := httptest.NewServer(http.HandlerFunc(mockedResponse))
		defer ts.Close()

		t.Run(tc.name, func(t *testing.T) {
			wantErr := fmt.Sprintf(tc.wantErr, ts.URL)
			target := github.Target{
				Server:       ts.URL,
				MaxRetries:   2,
				WaitTime:     time.Second,
				MaxSleepTime: time.Minute,
			}
			ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())
			err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

			if err == nil {
				t.Fatalf("\nhave: <no error>\nwant: %s", wantErr)
			}
			var ghError *github.StatusError
			if !errors.As(err, &ghError) {
				t.Fatalf("\nhave: %s\nwant: type github.StatusError", err)
			}
			wantStatus := tc.response.statuses[len(tc.response.statuses)-1]
			if have, want := ghError.StatusCode, wantStatus; have != want {
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
		t.Skip("Skipping integration test (reason: -short)")
	}

	cfg := testhelp.GitHubSecretsOrFail(t)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	target := github.Target{
		Server:       github.API,
		MaxRetries:   2,
		WaitTime:     time.Second,
		MaxSleepTime: 5 * time.Second,
	}
	ghStatus := github.NewCommitStatus(target, cfg.Token, cfg.Owner, cfg.Repo, context, hclog.NewNullLogger())
	err := ghStatus.Add(cfg.SHA, state, targetURL, desc)

	if err != nil {
		t.Fatalf("\nhave: %s\nwant: <no error>", err)
	}
}

func TestGitHubStatusFailureIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test (reason: -short)")
	}

	cfg := testhelp.GitHubSecretsOrFail(t)
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

			target := github.Target{
				Server:       github.API,
				MaxRetries:   2,
				WaitTime:     time.Second,
				MaxSleepTime: 5 * time.Second,
			}
			ghStatus := github.NewCommitStatus(target, tc.token, tc.owner, tc.repo, "dummy-context", hclog.NewNullLogger())
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
