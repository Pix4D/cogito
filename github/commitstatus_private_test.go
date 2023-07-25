package github

import (
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCheckForRetrySuccess(t *testing.T) {
	type testCase struct {
		name       string
		res        httpResponse
		waitTime   time.Duration
		jitter     time.Duration
		wantSleep  time.Duration
		wantReason string
	}

	const maxSleepTime = 15 * time.Minute
	var serverDate = time.Date(2001, time.April, 30, 13, 0, 0, 0, time.UTC)

	run := func(t *testing.T, tc testCase) {
		sleep, reason := checkForRetry(tc.res, tc.waitTime, maxSleepTime, tc.jitter)

		assert.Equal(t, sleep, tc.wantSleep)
		assert.Equal(t, reason, tc.wantReason)
	}

	testCases := []testCase{
		{
			name:      "status OK: sleep==0",
			res:       httpResponse{statusCode: http.StatusOK},
			wantSleep: 0 * time.Second,
		},
		{
			name:      "non retryable status code: sleep==0",
			res:       httpResponse{statusCode: http.StatusTeapot},
			wantSleep: 0 * time.Second,
		},
		{
			name:       "retryable status code: sleep==waitTime",
			res:        httpResponse{statusCode: http.StatusInternalServerError},
			waitTime:   42 * time.Second,
			wantSleep:  42 * time.Second,
			wantReason: "Internal Server Error",
		},
		{
			name: "rate limited, would sleep too long: sleep==0",
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(30 * time.Minute),
			},
			wantSleep: 0 * time.Second,
		},
		{
			name: "rate limited, would sleep a bit, adding also the jitter",
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(5 * time.Minute),
			},
			jitter:     8 * time.Second,
			wantSleep:  5*time.Minute + 8*time.Second,
			wantReason: "rate limited",
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
			jitter: 0 * time.Second,
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
			wantSleep:  1 * time.Second,
			wantReason: "rate limited",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}
