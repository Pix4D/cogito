// Package github implements the GitHub APIs used by Cogito (Commit status API).
//
// See the README and CONTRIBUTING files for additional information, caveats about GitHub
// API and imposed limits, and reference to official documentation.
package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Pix4D/cogito/retry"
)

// StatusError is one of the possible errors returned by the github package.
type StatusError struct {
	What       string
	StatusCode int
	Details    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s\n%s", e.What, e.Details)
}

// GhDefaultHostname is the default GitHub hostname (used for git but not for the API)
const GhDefaultHostname = "github.com"

var localhostRegexp = regexp.MustCompile(`^127.0.0.1:[0-9]+$`)

type Target struct {
	// Server is the GitHub API server.
	Server string
	// Retry controls the retry logic.
	Retry retry.Retry
}

// CommitStatus is a wrapper to the GitHub API to set the commit status for a specific
// GitHub owner and repo.
// See also:
// - NewCommitStatus
// - https://docs.github.com/en/rest/commits/statuses
type CommitStatus struct {
	target  *Target
	token   string
	owner   string
	repo    string
	context string

	log *slog.Logger
}

// NewCommitStatus returns a CommitStatus object associated to a specific GitHub owner
// and repo.
// Parameter token is the personal OAuth token of a user that has write access to the
// repo. It only needs the repo:status scope.
// Parameter context is what created the status, for example "JOBNAME", or
// "PIPELINENAME/JOBNAME". The name comes from the GitHub API.
// Be careful when using PIPELINENAME: if that name is ephemeral, it will make it
// impossible to use GitHub repository branch protection rules.
//
// See also:
// - https://docs.github.com/en/rest/commits/statuses
func NewCommitStatus(target *Target, token, owner, repo, context string, log *slog.Logger) CommitStatus {
	return CommitStatus{
		target:  target,
		token:   token,
		owner:   owner,
		repo:    repo,
		context: context,
		log:     log,
	}
}

// AddRequest is the JSON object sent to the API.
type AddRequest struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

// Add sets the commit state to the given sha, decorating it with targetURL and optional
// description.
// In case of transient errors or rate limiting by the backend, Add performs a certain
// number of attempts before giving up. The retry logic is configured in the Target.Retry
// parameter of NewCommitStatus.
// Parameter sha is the 40 hexadecimal digit sha associated to the commit to decorate.
// Parameter state is one of error, failure, pending, success.
// Parameter targetURL (optional) points to the specific process (for example, a CI build)
// that generated this state.
// Parameter description (optional) gives more information about the status.
// The returned error contains some diagnostic information to help troubleshooting.
//
// See also: https://docs.github.com/en/rest/commits/statuses#create-a-commit-status
func (cs CommitStatus) Add(sha, state, targetURL, description string) error {
	// API: POST /repos/{owner}/{repo}/statuses/{sha}
	url := cs.target.Server + path.Join("/repos", cs.owner, cs.repo, "statuses", sha)

	reqBody := AddRequest{
		State:       state,
		TargetURL:   targetURL,
		Description: description,
		Context:     cs.context,
	}

	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("JSON encode: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Authorization", "token "+cs.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// The retryable unit of work.
	workFn := func() error {
		// By default, there is no timeout, so the call could hang forever.
		client := &http.Client{Timeout: time.Second * 30}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("http client Do: %w", err)
		}
		defer resp.Body.Close()

		elapsed := time.Since(start)
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		limit := resp.Header.Get("X-RateLimit-Limit")
		reset := resp.Header.Get("X-RateLimit-Reset")
		cs.log.Debug("http-request", "method", req.Method, "url", req.URL, "status", resp.StatusCode, "duration", elapsed, "rate-limit", limit, "rate-limit-remaining", remaining, "rate-limit-reset", reset)

		if resp.StatusCode == http.StatusCreated {
			return nil
		}

		body, _ := io.ReadAll(resp.Body)
		return NewGitHubError(resp, errors.New(strings.TrimSpace(string(body))))
	}

	if err := cs.target.Retry.Do(Backoff, Classifier, workFn); err != nil {
		return cs.explainError(err, state, sha, url)
	}

	return nil
}

// TODO: can we merge (at least partially) this function in GitHubError.Error ?
// As-is, it is redundant. On the other hand, GitHubError.Error is now public
// and used by other tools, so we must not merge hints specific to the
// Commit Status API.
func (cs CommitStatus) explainError(err error, state, sha, url string) error {
	commonWhat := fmt.Sprintf("failed to add state %q for commit %s", state, sha[0:min(len(sha), 7)])
	var ghErr GitHubError
	if errors.As(err, &ghErr) {
		hint := "none"
		switch ghErr.StatusCode {
		case http.StatusNotFound:
			hint = fmt.Sprintf(`one of the following happened:
    1. The repo https://github.com/%s doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status`,
				path.Join(cs.owner, cs.repo))
		case http.StatusInternalServerError:
			hint = "Github API is down"
		case http.StatusUnauthorized:
			hint = "Either wrong credentials or PAT expired (check your email for expiration notice)"
		case http.StatusForbidden:
			if ghErr.RateLimitRemaining == 0 {
				hint = fmt.Sprintf("Rate limited but the wait time to reset would be longer than %v (Retry.UpTo)", cs.target.Retry.UpTo)
			}
		}
		return &StatusError{
			What: fmt.Sprintf("%s: %d %s", commonWhat, ghErr.StatusCode,
				http.StatusText(ghErr.StatusCode)),
			StatusCode: ghErr.StatusCode,
			Details: fmt.Sprintf("Body: %s\nHint: %s\nAction: %s %s\nOAuth: %s",
				ghErr, hint, http.MethodPost, url, ghErr.OauthInfo),
		}
	}

	return &StatusError{
		What:    fmt.Sprintf("%s: %s", commonWhat, err),
		Details: fmt.Sprintf("Action: %s %s", http.MethodPost, url),
	}
}

// ApiRoot constructs the root part of the GitHub API URL for a given hostname.
// Example:
// if hostname is github.com it returns https://api.github.com
// if hostname looks like a httptest server, it returns http://127.0.0.1:PORT
// otherwise, hostname is assumed to be of a Github Enterprise instance.
// For example, github.mycompany.org returns https://github.mycompany.org/api/v3
func ApiRoot(h string) string {
	hostname := strings.ToLower(h)
	if hostname == GhDefaultHostname {
		return "https://api.github.com"
	}
	if localhostRegexp.MatchString(hostname) {
		return fmt.Sprintf("http://%s", hostname)
	}
	return fmt.Sprintf("https://%s/api/v3", hostname)
}
