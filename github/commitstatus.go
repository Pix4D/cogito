// Package github implements the GitHub APIs used by Cogito (Commit status API).
//
// See the README and CONTRIBUTING files for additional information, caveats about GitHub
// API and imposed limits, and reference to official documentation.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"
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

// API is the GitHub API endpoint.
const API = "https://api.github.com"

type CommitStatus struct {
	server  string
	token   string
	owner   string
	repo    string
	context string
}

// NewCommitStatus returns a CommitStatus object associated to a specific GitHub owner and repo.
// Parameter token is the personal OAuth token of a user that has write access to the repo. It
// only needs the repo:status scope.
// Parameter context is what created the status, for example "JOBNAME", or "PIPELINENAME/JOBNAME".
// Be careful when using PIPELINENAME: if that name is ephemeral, it will make it impossible to
// use GitHub repository branch protection rules.
//
// See also:
// https://docs.github.com/en/rest/commits/statuses#about-the-commit-statuses-api
func NewCommitStatus(server, token, owner, repo, context string) CommitStatus {
	return CommitStatus{server, token, owner, repo, context}
}

// Add adds a commit state to the given sha, decorating it with targetURL and optional description.
// Parameter sha is the 40 hexadecimal digit sha associated to the commit to decorate.
// Parameter state is one of error, failure, pending, success.
// Parameter targetURL (optional) points to the specific process (for example, a CI build)
// that generated this state.
// Parameter description (optional) gives more information about the status.
// The returned error contains some diagnostic information to help troubleshooting.
//
// See also: https://docs.github.com/en/rest/commits/statuses#create-a-commit-status
func (s CommitStatus) Add(sha, state, targetURL, description string) error {
	// API: POST /repos/{owner}/{repo}/statuses/{sha}
	url := s.server + path.Join("/repos", s.owner, s.repo, "statuses", sha)

	// Field names must be uppercase (that is, exported) for the JSON encoder to consider
	// them, but the GitHub API wants lowercase names, so we use the optional `json:...`
	// tag to override the case.
	reqBody := struct {
		State       string `json:"state"`
		TargetURL   string `json:"target_url"`
		Description string `json:"description"`
		Context     string `json:"context"`
	}{state, targetURL, description, s.context}

	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("JSON encode: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Authorization", "token "+s.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// By default there is no timeout, so the call could hang forever.
	client := &http.Client{Timeout: time.Second * 30}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http client Do: %w", err)
	}
	defer resp.Body.Close()

	// GH API BUG
	// According to
	// https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/
	// each reply to a GH API action will return these entries in the header:
	//
	// X-Accepted-OAuth-Scopes:  Lists the scopes that the action checks for.
	// X-OAuth-Scopes:           Lists the scopes your token has authorized.
	//
	// But the API action we are using here: POST /repos/:owner/:repo/statuses/:sha
	//
	// returns an empty list for X-Accepted-Oauth-Scopes, while the API documentation
	// https://developer.github.com/v3/repos/statuses/ says:
	//
	//     Note that the repo:status OAuth scope grants targeted access to statuses without
	//     also granting access to repository code, while the repo scope grants permission
	//     to code as well as statuses.
	//
	// So X-Accepted-Oauth-Scopes cannot be empty, because it is a privileged operation, and
	// should be at least repo:status.
	//
	// Since we cannot use this information to detect configuration errors, for the time being
	// we report it in the error message.

	XAcceptedOauthScope := resp.Header["X-Accepted-Oauth-Scopes"]
	XOauthScopes := resp.Header["X-Oauth-Scopes"]
	OAuthInfo := fmt.Sprintf("X-Accepted-Oauth-Scopes: %v, X-Oauth-Scopes: %v",
		XAcceptedOauthScope, XOauthScopes)

	respBody, _ := ioutil.ReadAll(resp.Body)
	var hint string

	switch resp.StatusCode {
	case http.StatusCreated:
		// Happy path
		return nil
	case http.StatusNotFound:
		hint = fmt.Sprintf(`one of the following happened:
    1. The repo https://github.com/%s doesn't exist
    2. The user who issued the token doesn't have write access to the repo
    3. The token doesn't have scope repo:status`,
			path.Join(s.owner, s.repo))
	case http.StatusInternalServerError:
		hint = "Github API is down"
	default:
		// Any other error
		hint = "none"
	}
	return &StatusError{
		What: fmt.Sprintf("Failed to add state %q for commit %s: %d %s",
			state, sha[0:min(len(sha), 7)], resp.StatusCode, http.StatusText(resp.StatusCode)),
		StatusCode: resp.StatusCode,
		Details: fmt.Sprintf(`Body: %s
Hint: %s
Action: %s %s
OAuth: %s`,
			strings.TrimSpace(string(respBody)), hint, req.Method, url, OAuthInfo),
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
