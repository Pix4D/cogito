package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/help"
)

func TestGitHubStatusUseMockAPI(t *testing.T) {
	cfg := help.FakeTestCfg
	context := "cogito/test"
	targetURL := "https://cogito.invalid/builds/job/42"
	desc := time.Now().Format("15:04:05")
	state := "success"
	var testCases = []struct {
		name       string
		wantErr    string
		wantStatus int
		write      string
	}{
		{"No errors",
			"", http.StatusCreated, "Anything goes..."},
		{"Server error",
			http.StatusText(500), http.StatusInternalServerError, "Something bad happened!"},
		{"Repo not found",
			fmt.Sprintf(
				"\nOne of the following happened:\n"+
					"\t1. The repo https://github.com/%s doesn't exist\n"+
					"\t2. The user who issued the token doesn't have write access to the repo\n"+
					"\t3. The token doesn't have scope repo:status\n", path.Join(cfg.Owner, cfg.Repo),
			),
			http.StatusNotFound, "Repo not found"},
	}
	for _, tc := range testCases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.wantStatus)
			fmt.Fprintln(w, tc.write)
		}))
		defer ts.Close()
		t.Run(tc.name, func(t *testing.T) {
			status := github.NewStatus(ts.URL, cfg.Token, cfg.Owner, cfg.Repo, context)
			err := status.Add(cfg.SHA, state, targetURL, desc)
			if err != nil {
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("\ngot:  %v\nwant: %v", err, tc.wantErr)
				}
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
		t.Fatalf("\ngot:  %v\nwant: no error", err)
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
					t.Fatalf("status code: got %v (%v); want %v (%v)\n%v",
						statusErr.StatusCode, http.StatusText(statusErr.StatusCode),
						tc.wantStatus, http.StatusText(tc.wantStatus), err)
				}
			} else {
				t.Fatalf("got %v; want *github.StatusError", reflect.TypeOf(err))
			}
		})
	}
}
