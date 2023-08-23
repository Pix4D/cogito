package github

import (
	"net/http"
	"slices"
	"time"

	"github.com/Pix4D/cogito/retry"
)

// FIXME is this the best place??? Not really...
type HttpResponse struct {
	statusCode         int
	body               string
	oauthInfo          string
	rateLimitRemaining int
	rateLimitReset     time.Time
	date               time.Time
}

// FIXME is this the best place???
var retryableStatusCodes = []int{
	http.StatusInternalServerError, // 500
	http.StatusBadGateway,          // 502
	http.StatusServiceUnavailable,  // 503
	http.StatusGatewayTimeout,      // 504
}

// FIXME is this the best place???
func Classifier(err error, userCtx any) retry.Action {
	if err != nil {
		return retry.HardFail
	}

	res := userCtx.(*HttpResponse)

	// Do we have a retryable HTTP status code ?
	if slices.Contains(retryableStatusCodes, res.statusCode) {
		return retry.SoftFail
	}

	// Are we rate limited ?
	if RateLimited(res) {
		return retry.SoftFail
	}

	// FIXME maybe consider the whole 2xx range instead ???
	if res.statusCode == http.StatusCreated || res.statusCode == http.StatusOK {
		return retry.Success
	}

	return retry.HardFail
}

func ExponentialBackoff(first bool, previous, limit time.Duration, userCtx any,
) time.Duration {
	res := userCtx.(*HttpResponse)

	// Optimization: Are we rate limited ? This allows to immediately terminate the retry
	// loop if it would take too long.
	if RateLimited(res) {
		// Calculate the sleep time based solely on the server clock. This is unaffected
		// by the inevitable clock drift between server and client.
		sleepTime := res.rateLimitReset.Sub(res.date)
		// Be robust to possible races in the GitHub backend. This avoids failing too early.
		// FIXME IS THIS STILL A PROBLEM???
		if sleepTime < 0 {
			sleepTime = 0
		}
		return sleepTime
	}

	// Normal path, no optimizations.
	if first {
		return previous
	}
	next := 2 * previous
	return min(next, limit)
}

// Are we rate limited ?
// If the request exceeds the rate limit, the response will have status 403 Forbidden
// and the x-ratelimit-remaining header will be 0
// https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
func RateLimited(res *HttpResponse) bool {
	return res.statusCode == http.StatusForbidden && res.rateLimitRemaining == 0
}
