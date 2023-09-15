// Adapters and helpers to use the [cogito/retry] package for the GitHub API.

package github

import (
	"errors"
	"net/http"
	"time"

	"github.com/Pix4D/cogito/retry"
)

// Classifier implements [retry.ClassifierFunc] for GitHub.
func Classifier(err error) retry.Action {
	if err == nil {
		return retry.Success
	}

	var ghErr GitHubError
	if errors.As(err, &ghErr) {
		if TransientError(ghErr.StatusCode) {
			return retry.SoftFail
		}
		if RateLimited(ghErr) {
			return retry.SoftFail
		}
		return retry.HardFail
	}

	return retry.HardFail
}

// Backoff implements [retry.BackoffFunc] for GitHub.
func Backoff(first bool, previous, limit time.Duration, err error) time.Duration {
	// Optimization: Are we rate limited?
	// This allows to immediately terminate the retry loop if it would take too
	// long, instead of keeping retrying and discovering at the end that we are
	// still rate limited.
	var ghErr GitHubError
	if errors.As(err, &ghErr) {
		if RateLimited(ghErr) {
			// Calculate the delay based solely on the server clock. This is
			// unaffected by the inevitable clock drift between server and client.
			delay := ghErr.RateLimitReset.Sub(ghErr.Date)

			// We observed in production both a zero and a negative delay from
			// GitHub. This can be due to race conditions in the GitHub backend
			// or to other causes. Since this is a retry mechanism, we want to
			// be resilient, so we optimize only if we are 100% sure.
			if delay > 0 {
				return delay
			}
		}
	}

	// We are here for two different reasons:
	// 1. we are not rate limited (normal case)
	// 2. we are rate limited but the calculated delay is either zero or negative
	//    (see beginning of this function).
	return retry.ExponentialBackoff(first, previous, limit, nil)
}

// RateLimited returns true if the http.Response in err reports being rate limited.
// See https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
func RateLimited(err GitHubError) bool {
	return err.StatusCode == http.StatusForbidden && err.RateLimitRemaining == 0
}

// TransientError returns true if the http.Response in err has a status code
// that can be retried.
func TransientError(statusCode int) bool {
	switch statusCode {
	case
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}
