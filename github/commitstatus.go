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
	"golang.org/x/exp/slices"
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
	Server string

	// Maximum number of retries for the retryable http request
	MaxRetries int
	// Default wait time between two http requests
	WaitTime time.Duration
	// Maximum sleep time allowed
	MaxSleepTime time.Duration
	// adds some randomness to sleep time to prevent creating a Thundering Herd
	Jitter time.Duration
}

type CommitStatus struct {
	target  Target
	token   string
	owner   string
	repo    string
	context string

	log hclog.Logger
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
func NewCommitStatus(target Target, token, owner, repo, context string, log hclog.Logger) CommitStatus {
	return CommitStatus{target, token, owner, repo, context, log}
}

// AddRequest is the JSON object sent to the API.
type AddRequest struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

// Add adds a commit state to the given sha, decorating it with targetURL and optional
// description.
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
	url := s.target.Server + path.Join("/repos", s.owner, s.repo, "statuses", sha)

	reqBody := AddRequest{
		State:       state,
		TargetURL:   targetURL,
		Description: description,
		Context:     s.context,
	}

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

	var response httpResponse
	timeToSleep := 0 * time.Second

	for attempt := 1; attempt <= s.target.MaxRetries; attempt++ {
		time.Sleep(timeToSleep)
		s.log.Info("GitHub HTTP request", "attempt", attempt, "max", s.target.MaxRetries)
		response, err = httpRequestDo(req)
		if err != nil {
			return err
		}
		timeToSleep, reason, err := checkForRetry(response, s.target.WaitTime,
			s.target.MaxSleepTime, s.target.Jitter)
		if err != nil {
			return fmt.Errorf("internal error: %s", err)
		}
		if timeToSleep == 0 {
			break
		}
		s.log.Info("Sleeping for", "duration", timeToSleep, "reason", reason)
	}

	return s.checkStatus(response, state, sha, url)
}

type httpResponse struct {
	statusCode         int
	body               string
	oauthInfo          string
	rateLimitRemaining int
	rateLimitReset     time.Time
	date               time.Time
}

func httpRequestDo(req *http.Request) (httpResponse, error) {
	var response httpResponse
	// By default, there is no timeout, so the call could hang forever.
	client := &http.Client{Timeout: time.Second * 30}
	resp, err := client.Do(req)
	if err != nil {
		return httpResponse{}, fmt.Errorf("http client Do: %w", err)
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
	//     Note that the repo:status OAuth scope grants targeted access to statuses without
	//     also granting access to repository code, while the repo scope grants permission
	//     to code as well as statuses.
	//
	// So X-Accepted-Oauth-Scopes cannot be empty, because it is a privileged operation, and
	// should be at least repo:status.
	//
	// Since we cannot use this information to detect configuration errors, for the time being
	// we report it in the error message.
	response.oauthInfo = fmt.Sprintf("X-Accepted-Oauth-Scopes: %v, X-Oauth-Scopes: %v",
		resp.Header.Get("X-Accepted-Oauth-Scopes"), resp.Header.Get("X-Oauth-Scopes"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty (both are standard behaviors)
	response.rateLimitRemaining, _ = strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty (both are standard behaviors)
	limit, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Reset"))
	response.rateLimitReset = time.Unix(int64(limit), 0)

	// time.RFC1123 is the format of the HTTP Date header
	// example: Date:[ Mon, 02 Jan 2006 15:04:05 MST ]
	// https://datatracker.ietf.org/doc/html/rfc2616#section-14.18
	date, err := time.Parse(time.RFC1123, resp.Header.Get("Date"))
	if err != nil {
		return httpResponse{}, fmt.Errorf("failed to parse the date header: %s", err)
	}
	response.date = date

	return response, nil
}

func (s CommitStatus) checkStatus(resp httpResponse, state, sha, url string) error {
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
			path.Join(s.owner, s.repo))
	case http.StatusInternalServerError:
		hint = "Github API is down"
	case http.StatusUnauthorized:
		hint = "Either wrong credentials or PAT expired (check your email for expiration notice)"
	case http.StatusForbidden:
		if resp.rateLimitRemaining == 0 {
			hint = fmt.Sprintf("Rate limited but the wait time to reset would be longer than %v (MaxSleepTime)", s.target.MaxSleepTime)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// checkForRetry determines if the HTTP request should be retried.
// If yes, checkForRetry returns a positive duration.
// If no, checkForRetry returns a 0 duration.
//
// It considers two different reasons for a retry:
//  1. The request encountered a GitHub-specific rate limit.
//     In this case, it considers parameters maxSleepTime and jitter.
//  2. The HTTP status code is in a retryable subset of the 5xx status codes.
//     In this case, it returns the same as the input parameter waitTime.
func checkForRetry(res httpResponse, waitTime, maxSleepTime, jitter time.Duration,
) (time.Duration, string, error) {
	retryableStatusCodes := []int{
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	switch {
	// If the request exceeds the rate limit, the response will have status 403 Forbidden
	// and the x-ratelimit-remaining header will be 0
	// https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
	case res.statusCode == http.StatusForbidden && res.rateLimitRemaining == 0:
		// Calculate the sleep time based solely on the server clock. This is unaffected
		// by the inevitable clock drift between server and client.
		sleepTime := res.rateLimitReset.Sub(res.date)
		// Be a good netizen by adding some jitter to the time we sleep.
		sleepTime += jitter
		switch {
		case sleepTime > maxSleepTime:
			return 0, "", nil
		case sleepTime > 0 && sleepTime < maxSleepTime:
			return sleepTime, "rate limited", nil
		default:
			return 0, "", fmt.Errorf("unexpected: negative sleep time: %s", sleepTime)
		}
	case slices.Contains(retryableStatusCodes, res.statusCode):
		return waitTime, http.StatusText(res.statusCode), nil
	default:
		return 0, "", nil
	}
}
