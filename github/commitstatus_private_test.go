package github

import (
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

const maxSleepTime = 15 * time.Minute

var serverDate = time.Date(2001, time.April, 30, 13, 0, 0, 0, time.UTC)

func TestCheckForRetrySuccess(t *testing.T) {
	type testCase struct {
		name       string
		res        httpResponse
		waitTime   time.Duration
		jitter     time.Duration
		wantSleep  time.Duration
		wantReason string
	}

	run := func(t *testing.T, tc testCase) {
		sleep, reason, err := checkForRetry(tc.res, tc.waitTime, maxSleepTime, tc.jitter)

		assert.NilError(t, err)
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

// BUG: sleeptime + jitter might cause a failure; test sleepTime > maxSleepTime should
// be done before?

// We saw this happening in production. Since we didn't have debug logging, we cannot
// be sure of the cause, so we show two possible causes in the test cases.
func TestCheckForRetryNegativeSleepTime(t *testing.T) {
	type testCase struct {
		name    string
		res     httpResponse
		jitter  time.Duration
		wantErr string
	}

	run := func(t *testing.T, tc testCase) {
		// Not in the code path, no effect.
		waitTime := 0 * time.Second

		_, _, err := checkForRetry(tc.res, waitTime, maxSleepTime, tc.jitter)

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name: "server date slightly after rateLimitReset, too small jitter",
			// Server date slightly after rateLimitReset.
			// This can be explained by a benign race in the backend.
			res: httpResponse{
				statusCode:     http.StatusForbidden,
				date:           serverDate,
				rateLimitReset: serverDate.Add(-2 * time.Second),
			},
			// Too small jitter.
			jitter:  1 * time.Second,
			wantErr: "unexpected: negative sleep time: -1s",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}
