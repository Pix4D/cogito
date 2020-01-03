// Package github implements the GitHub status API, following the GitHub REST API v3.
//
// See the README file for additional information, caveats about GitHub API and imposed limits,
// and reference to official documentation.

package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"time"
)

type StatusError struct {
	What       string
	StatusCode int
	Details    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%v, status %v (%v)", e.What, e.StatusCode, e.Details)
}

const API = "https://api.github.com"

type status struct {
	server  string
	token   string
	owner   string
	repo    string
	context string
}

// NewStatus returns a status object associated to a specific GitHub owner and repo.
// Parameter token is the personal OAuth token of a user that has write access to the repo. It
// only needs the repo:status scope.
// Parameter context is what created the status, for example "JOBNAME", or "PIPELINENAME/JOBNAME".
// Be careful when using PIPELINENAME: if that name is ephemeral, it will make it impossible to
// use GitHub repository branch protection rules.
// See also:
// * https://developer.github.com/v3/repos/statuses/
// * README file
func NewStatus(server, token, owner, repo, context string) status {
	return status{server, token, owner, repo, context}
}

// Add adds state to the given sha, decorating it with target_url and optional description.
// Parameter sha is the 40 hexadecimal digit sha associated to the commit to decorate.
// Parameter state is one of error, failure, pending, success.
// Parameter target_url (optional) points to the specific process (for example, a CI build)
// that generated this state.
// Parameter description (optional) gives more information about the status.
// The returned error contains some diagnostic information to troubleshoot.
func (s status) Add(sha, state, target_url, description string) error {
	// API: POST /repos/:owner/:repo/statuses/:sha
	url := s.server + path.Join("/repos", s.owner, s.repo, "statuses", sha)

	// Field names must be uppercase (that is, exported) for the JSON encoder to consider
	// them, but the GitHub API wants lowercase names, so we use the optional `json:...`
	// tag to override the case.
	reqBody := struct {
		State       string `json:"state"`
		Target_url  string `json:"target_url"`
		Description string `json:"description"`
		Context     string `json:"context"`
	}{state, target_url, description, s.context}

	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("JSON encode: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBodyJson))
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
	OAuthInfo := fmt.Sprintf(" X-Accepted-Oauth-Scopes: %v, X-Oauth-Scopes: %v",
		XAcceptedOauthScope, XOauthScopes)

	respBody, _ := ioutil.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		// Happy path
		return nil
	case http.StatusNotFound:
		msg := disambiguateError(s)
		return &StatusError{req.Method + " " + url + msg + OAuthInfo, resp.StatusCode, string(respBody)}
	default:
		// Any other error
		return &StatusError{req.Method + " " + url + OAuthInfo, resp.StatusCode, string(respBody)}
	}
}

// We might get 404 Not Found for two completely different reasons:
// 1. The repository doesn't exist. This makes sense.
// 2. The user whose token we are using doesn't have write access to the repo.
//    This is a bug, the API should return 401 Unauthorized instead. Doing so would not be a leak,
//    since we reach this point ONLY if we are authorized.
// So we go through this machinery to provide better feedback to the user, attempting to
// disambiguate.
func disambiguateError(s status) string {
	err := s.CanReadRepo()
	if err == nil {
		return ": The user with this token doesn't have write access to the repo."
	}
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode == http.StatusNotFound {
			return ": The repo doesn't exist."
		}
	}
	return ": Could not disambiguate this error."
}

// CanReadRepo validates if the token has read access to the repo.
// This is a workaround to troubleshoot certain errors.
func (s status) CanReadRepo() error {
	// API: GET /repos/:owner/:repo
	url := s.server + path.Join("/repos", s.owner, s.repo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
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

	respBody, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &StatusError{req.Method + " " + url, resp.StatusCode, string(respBody)}
	}
	return nil
}
