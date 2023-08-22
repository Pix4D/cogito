package github

import (
	"errors"
	"net/http"
	"time"

	"github.com/Pix4D/cogito/retry"
)

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

func Backoff(first bool, previous, limit time.Duration, err error) time.Duration {
	// Optimization: Are we rate limited?
	// This allows to immediately terminate the retry loop if it would take too long.
	var ghErr GitHubError
	if errors.As(err, &ghErr) {
		if RateLimited(ghErr) && !ghErr.RateLimitReset.IsZero() {
			// Calculate the delay based solely on the server clock. This is unaffected
			// by the inevitable clock drift between server and client.
			delay := ghErr.RateLimitReset.Sub(ghErr.Date)
			// Be robust to possible races in the GitHub backend. This avoids failing too early.
			// FIXME IS THIS STILL A PROBLEM???
			if delay < 0 {
				delay = 0
			}
			return delay
		}
	}

	// We are here for two different reasons:
	// 1. we are not rate limited (normal case)
	// 2. we are rate limited but ghErr.RateLimitReset has the zero value for
	//    time.Time (1970-01-01). This is a bug, but at least we ensure that the
	//    backoff sooner or later will terminate.
	return retry.ExponentialBackoff(first, previous, limit, nil)
}

// Are we rate limited ?
// If the request exceeds the rate limit, the response will have status 403 Forbidden
// and the x-ratelimit-remaining header will be 0
// https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
func RateLimited(err GitHubError) bool {
	return err.StatusCode == http.StatusForbidden && err.RateLimitRemaining == 0
}

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
