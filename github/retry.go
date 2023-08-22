package github

import (
	"net/http"
	"slices"
	"time"

	"github.com/hashicorp/go-hclog"
)

type HttpResponse struct {
	statusCode         int
	body               string
	oauthInfo          string
	rateLimitRemaining int
	rateLimitReset     time.Time
	date               time.Time
}

type Retry struct {
	// MaxAttempts is the maximum number of attempts when retrying an HTTP request to
	// GitHub, no matter the reason (rate limited or transient error).
	MaxAttempts int
	// WaitTransient is the wait time before the next attempt when encountering a
	// transient error from GitHub.
	WaitTransient time.Duration
	// MaxSleepRateLimited is the maximum sleep time (over all attempts) when rate
	// limited from GitHub.
	MaxSleepRateLimited time.Duration
	// Jitter is added to the sleep duration to prevent creating a Thundering Herd.
	Jitter time.Duration

	sleepFn func(d time.Duration) // Overridable by tests.
}

// SetSleepFn overrides time.Sleep, used when retrying an HTTP request, with sleepFn.
// WARNING Use only in tests.
func (re *Retry) SetSleepFn(sleepFn func(d time.Duration)) {
	re.sleepFn = sleepFn
}

// Do retries calling function `work` according to the Retry configuration.
func (re *Retry) Do(
	log hclog.Logger,
	work func() (HttpResponse, error),
) (HttpResponse, error) {
	if re.sleepFn == nil {
		re.sleepFn = time.Sleep
	}

	var response HttpResponse
	for attempt := 1; ; attempt++ {
		var err error
		response, err = work()
		if err != nil {
			return response, err
		}
		if attempt == re.MaxAttempts {
			break
		}
		retry, timeToSleep, reason := checkForRetry(response, re.WaitTransient,
			re.MaxSleepRateLimited, re.Jitter)
		if !retry {
			break
		}
		log.Info("Sleeping for", "duration", timeToSleep, "reason", reason)
		re.sleepFn(timeToSleep)
	}
	return response, nil
}

// checkForRetry determines if the HTTP request should be retried after a sleep.
// If yes, checkForRetry returns true, the sleep duration and a reason.
// If no, checkForRetry returns false and a reason.
//
// To take a decision, use only the boolean value. Do not use the duration nor the reason.
//
// It considers two different reasons for a retry:
//  1. The request encountered a GitHub-specific rate limit.
//     In this case, it considers parameters maxSleepTime and jitter.
//  2. The HTTP status code is in a retryable subset of the 5xx status codes.
//     In this case, it returns the same as the input parameter waitTime.
func checkForRetry(res HttpResponse, waitTime, maxSleepTime, jitter time.Duration,
) (bool, time.Duration, string) {
	retryableStatusCodes := []int{
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	// Are we rate limited ?
	// If the request exceeds the rate limit, the response will have status 403 Forbidden
	// and the x-ratelimit-remaining header will be 0
	// https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
	if res.statusCode == http.StatusForbidden && res.rateLimitRemaining == 0 {
		// Calculate the sleep time based solely on the server clock. This is unaffected
		// by the inevitable clock drift between server and client.
		sleepTime := res.rateLimitReset.Sub(res.date)
		// Be robust to possible races in the GitHub backend. This avoids failing too early.
		if sleepTime < 0 {
			sleepTime = 0
		}
		// Be a good netizen by adding some jitter to the time we sleep.
		sleepTime += jitter

		if sleepTime > maxSleepTime {
			return false, 0, "rate limited, sleepTime > maxSleepTime, should not retry"
		}
		return true, sleepTime, "rate limited, should retry"
	}

	// Do we have a retryable HTTP status code ?
	if slices.Contains(retryableStatusCodes, res.statusCode) {
		return true, waitTime, http.StatusText(res.statusCode)
	}

	// The status code could be 200 OK or any other error we did not process before.
	// In any case, there is nothing to sleep, return 0 and let the caller take a
	// decision.
	return false, 0, "no retryable reasons"
}
