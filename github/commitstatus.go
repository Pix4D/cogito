// Package github implements the GitHub APIs used by Cogito (Commit status API).
//
// See the README and CONTRIBUTING files for additional information, caveats about GitHub
// API and imposed limits, and reference to official documentation.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

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

// API is the GitHub API endpoint.
const API = "https://api.github.com"

type Target struct {
	// Server is the GitHub API server.
	// Currently, hardcoded to https://api.github.com
	Server string
	Retry  retry.Retry
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

	log hclog.Logger
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
func NewCommitStatus(target *Target, token, owner, repo, context string, log hclog.Logger) CommitStatus {
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
// number of attempts before giving up. The retry logic is configured in the target
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

	response := &HttpResponse{}

	workFn := func(userCtx any) error {
		response := userCtx.(*HttpResponse)
		if err := httpRequestDo(req, response); err != nil {
			return err
		}
		return nil
	}

	retryErr := cs.target.Retry.Do(ExponentialBackoff, Classifier, workFn, response)
	if retryErr != nil {
		return fmt.Errorf("commit status: %s", retryErr)
	}

	return cs.checkStatus(*response, state, sha, url)
}

func httpRequestDo(req *http.Request, response *HttpResponse) error {
	// By default, there is no timeout, so the call could hang forever.
	client := &http.Client{Timeout: time.Second * 30}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http client Do: %w", err)
	}
	defer resp.Body.Close()

	response.statusCode = resp.StatusCode
	respBody, _ := io.ReadAll(resp.Body)
	response.body = strings.TrimSpace(string(respBody))

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
	//     Note that the repo:status OAuth scope grants targeted access to statuses
	//     without also granting access to repository code, while the repo scope grants
	//     permission to code as well as statuses.
	//
	// So X-Accepted-Oauth-Scopes cannot be empty, because it is a privileged operation,
	// and should be at least repo:status.
	//
	// Since we cannot use this information to detect configuration errors, for the time
	// being we report it in the error message.
	response.oauthInfo = fmt.Sprintf("X-Accepted-Oauth-Scopes: %v, X-Oauth-Scopes: %v",
		resp.Header.Get("X-Accepted-Oauth-Scopes"), resp.Header.Get("X-Oauth-Scopes"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty.
	response.rateLimitRemaining, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty.
	limit, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Reset"))
	response.rateLimitReset = time.Unix(int64(limit), 0)

	// The HTTP Date header is formatted according to RFC1123.
	// (https://datatracker.ietf.org/doc/html/rfc2616#section-14.18)
	// Example:
	//   Date: Mon, 02 Jan 2006 15:04:05 MST
	date, err := time.Parse(time.RFC1123, resp.Header.Get("Date"))
	if err != nil {
		return fmt.Errorf("failed to parse the date header: %s", err)
	}
	response.date = date

	return nil
}

func (cs CommitStatus) checkStatus(resp HttpResponse, state, sha, url string) error {
	var hint string

	switch resp.statusCode {
	case http.StatusCreated:
		// Happy path
		return nil
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
		if resp.rateLimitRemaining == 0 {
			hint = fmt.Sprintf("Rate limited but the wait time to reset would be longer than %v (Retry.UpTo)", cs.target.Retry.UpTo)
		} else {
			hint = "none"
		}
	default:
		// Any other error
		hint = "none"
	}
	return &StatusError{
		What: fmt.Sprintf("failed to add state %q for commit %s: %d %s",
			state, sha[0:min(len(sha), 7)], resp.statusCode, http.StatusText(resp.statusCode)),
		StatusCode: resp.statusCode,
		Details: fmt.Sprintf(`Body: %s
Hint: %s
Action: %s %s
OAuth: %s`,
			resp.body,
			hint,
			http.MethodPost,
			url,
			resp.oauthInfo),
	}
}
