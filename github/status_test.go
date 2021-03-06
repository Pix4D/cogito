package github_test

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gh "github.com/Pix4D/cogito/github"
)

func TestGitHubStatusE2E(t *testing.T) {
	cfg := gh.SkipTestIfNoEnvVars(t)
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"

	status := gh.NewStatus(gh.API, cfg.Token, cfg.Owner, cfg.Repo, context)
	err := status.Add(cfg.SHA, state, targetURL, desc)

	if err != nil {
		t.Fatalf("\ngot:  %v\nwant: no error", err)
	}
}

func TestGitHubStatusCanDiagnoseReadOnlyUser(t *testing.T) {
	cfg := gh.SkipTestIfNoEnvVars(t)
	const readOnlyOwner = "octocat"
	const readOnlyRepo = "Spoon-Knife"
	const readOnlySHA = "d0dd1f61b33d64e29d8bc1372a94ef6a2fee76a9"
	const context = "dummy"
	const targetURL = "dummy"
	desc := time.Now().Format("15:04:05")
	const state = "success"

	status := gh.NewStatus(gh.API, cfg.Token, readOnlyOwner, readOnlyRepo, context)

	if err := status.CanReadRepo(); err != nil {
		t.Fatalf("\ngot:  %v\nwant: no error", err)
	}

	err := status.Add(readOnlySHA, state, targetURL, desc)

	var statusErr *gh.StatusError
	wantStatusCode := http.StatusNotFound
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode != wantStatusCode {
			t.Fatalf("\ngot:  %v %v\nwant: %v %v\ndetails: %v",
				statusErr.StatusCode, http.StatusText(statusErr.StatusCode),
				wantStatusCode, http.StatusText(wantStatusCode),
				err)
		}
		// As ugly as it is, I have to read into the error message :-(
		const wantDiagnose = "The user with this token doesn't have write access to the repo"
		if !strings.Contains(statusErr.Error(), wantDiagnose) {
			t.Fatalf("Error message (%v) does not contain expected diagnosis (%v)",
				statusErr.Error(), wantDiagnose)
		}
	} else {
		t.Fatalf("got %v; want *gh.StatusError", reflect.TypeOf(err))
	}
}

func TestUnderstandGitHubStatusFailures(t *testing.T) {
	cfg := gh.SkipTestIfNoEnvVars(t)

	var testCases = []struct {
		name       string
		token      string
		owner      string
		repo       string
		sha        string
		wantStatus int
	}{
		{"bad token -> Unauthorized",
			"bad-token", cfg.Owner, cfg.Repo, "dummy-sha", http.StatusUnauthorized},
		{"non existing repo -> Not Found",
			cfg.Token, cfg.Owner, "non-existing-really", "dummy-sha", http.StatusNotFound},
		{"bad SHA -> Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "dummy-sha", http.StatusUnprocessableEntity},
		{"tag instead of SHA -> Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "v0.0.2", http.StatusUnprocessableEntity},
		{"non existing SHA -> Unprocessable Entity",
			cfg.Token, cfg.Owner, cfg.Repo, "e576e3aa7aaaa048b396e2f34fa24c9cf4d1e822", http.StatusUnprocessableEntity},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := gh.NewStatus(gh.API, tc.token, tc.owner, tc.repo, "dummy")
			err := status.Add(tc.sha, "dummy", "dummy", "dummy")

			var statusErr *gh.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.StatusCode != tc.wantStatus {
					t.Fatalf("status code: got %v (%v); want %v (%v)\n%v",
						statusErr.StatusCode, http.StatusText(statusErr.StatusCode),
						tc.wantStatus, http.StatusText(tc.wantStatus), err)
				}
			} else {
				t.Fatalf("got %v; want *gh.StatusError", reflect.TypeOf(err))
			}
		})
	}
}

func TestStatusValidate(t *testing.T) {
	cfg := gh.SkipTestIfNoEnvVars(t)

	var testCases = []struct {
		name       string
		token      string
		owner      string
		repo       string
		wantStatus int
	}{
		{"bad token -> Unauthorized",
			"bad-token", cfg.Owner, cfg.Repo, http.StatusUnauthorized},
		{"non existing repo -> Not Found",
			cfg.Token, cfg.Owner, "non-existing-really", http.StatusNotFound},
	}

	t.Run("happy path", func(t *testing.T) {
		status := gh.NewStatus(gh.API, cfg.Token, cfg.Owner, cfg.Repo, "dummy")

		if err := status.CanReadRepo(); err != nil {
			t.Fatalf("got: %v; want: no error", err)
		}
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := gh.NewStatus(gh.API, tc.token, tc.owner, tc.repo, "dummy")

			err := status.CanReadRepo()

			var statusErr *gh.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.StatusCode != tc.wantStatus {
					t.Fatalf("status code: got %v (%v); want %v (%v)\nerror: %v",
						statusErr.StatusCode, http.StatusText(statusErr.StatusCode),
						tc.wantStatus, http.StatusText(tc.wantStatus), err)
				}
			} else {
				t.Fatalf("got %v; want gh.StatusError", reflect.TypeOf(err))
			}
		})
	}
}
