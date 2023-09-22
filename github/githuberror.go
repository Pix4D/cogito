package github

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type GitHubError struct {
	StatusCode         int
	OauthInfo          string
	Date               time.Time
	RateLimitRemaining int
	RateLimitReset     time.Time
	innerErr           error
}

func NewGitHubError(httpResp *http.Response, innerErr error) error {
	ghErr := GitHubError{
		innerErr:   innerErr,
		StatusCode: httpResp.StatusCode,
	}

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
	ghErr.OauthInfo = fmt.Sprintf("X-Accepted-Oauth-Scopes: %v, X-Oauth-Scopes: %v",
		httpResp.Header.Get("X-Accepted-Oauth-Scopes"), httpResp.Header.Get("X-Oauth-Scopes"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty.
	ghErr.RateLimitRemaining, _ = strconv.Atoi(httpResp.Header.Get("X-RateLimit-Remaining"))

	// strconv.Atoi returns 0 in case of error, Get returns "" if empty.
	limit, _ := strconv.Atoi(httpResp.Header.Get("X-RateLimit-Reset"))
	// WARNING
	// If the parsing fails for any reason, limit will be set to  0. In Unix
	// time, 0 is the epoch, 1970-01-01, so ghErr.RateLimitReset will be set to
	// that date. This will cause the [Backoff] function to calculate a negative
	// delay. This is properly taken care of by [Backoff].
	ghErr.RateLimitReset = time.Unix(int64(limit), 0)

	// The HTTP Date header is formatted according to RFC1123.
	// (https://datatracker.ietf.org/doc/html/rfc2616#section-14.18)
	// Example:
	//   Date: Mon, 02 Jan 2006 15:04:05 MST
	date, err := time.Parse(time.RFC1123, httpResp.Header.Get("Date"))
	// FIXME this is not robust. Maybe log instead and put a best effort date instead?
	if err != nil {
		return fmt.Errorf("failed to parse the date header: %s", err)
	}
	ghErr.Date = date

	return ghErr
}

func (ghe GitHubError) Error() string {
	return ghe.innerErr.Error()
}

func (ghe GitHubError) Unwrap() error {
	return ghe.innerErr
}
