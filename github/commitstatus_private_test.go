package github

import (
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCheckForRetry(t *testing.T) {
	type testCase struct {
		name       string
		res        httpResponse
		waitTime   time.Duration
		jitter     time.Duration
		wantRetry  bool
		wantSleep  time.Duration
		wantReason string
	}

	const maxSleepTime = 15 * time.Minute
	var serverDate = time.Date(2001, time.April, 30, 13, 0, 0, 0, time.UTC)

	run := func(t *testing.T, tc testCase) {
		retry, sleep, reason := checkForRetry(
			tc.res, tc.waitTime, maxSleepTime, tc.jitter)

		assert.Equal(t, retry, tc.wantRetry)
		assert.Equal(t, sleep, tc.wantSleep)
		assert.Equal(t, reason, tc.wantReason)
	}

	testCases := []testCase{
		{
			name:       "status OK: do not retry",
			res:        httpResponse{statusCode: http.StatusOK},
			wantRetry:  false,
			wantReason: "no retryable reasons",
		},
		{
			name:       "non retryable status code: do not retry",
			res:        httpResponse{statusCode: http.StatusTeapot},
			wantReason: "no retryable reasons",
			wantRetry:  false,
		},
		{
			name:       "retryable status code: retry, sleep==waitTime",
			res:        httpResponse{statusCode: http.StatusInternalServerError},
			waitTime:   42 * time.Second,
			wantRetry:  true,
			wantSleep:  42 * time.Second,
			wantReason: "Internal Server Error",
		},
		{
			name: "rate limited, would sleep too long: do not retry",
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(30 * time.Minute),
			},
			wantReason: "rate limited, sleepTime > maxSleepTime, should not retry",
			wantRetry:  false,
		},
		{
			name: "rate limited, do retry, sleep adding also the jitter",
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(5 * time.Minute),
			},
			jitter:     8 * time.Second,
			wantRetry:  true,
			wantSleep:  5*time.Minute + 8*time.Second,
			wantReason: "rate limited, should retry",
		},
		{
			// Fix https://github.com/Pix4D/cogito/issues/124
			name: "same server date and rateLimitReset, zero jitter, repro of Pix4D/cogito#124",
			// Same server date and rateLimitReset.
			// This can be explained by a benign race in the backend.
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate,
			},
			// Since we set jitter from rand.Intn, which can return 0, jitter can be 0.
			jitter:     0 * time.Second,
			wantReason: "rate limited, should retry",
			wantRetry:  true,
		},
		{
			name: "robust against server date after rateLimitReset",
			// Server date slightly after rateLimitReset.
			// This can be explained by a benign race in the backend.
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(-2 * time.Second),
			},
			jitter:     1 * time.Second,
			wantRetry:  true,
			wantSleep:  1 * time.Second,
			wantReason: "rate limited, should retry",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}
