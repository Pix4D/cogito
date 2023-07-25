package github

import (
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCheckForRetrySuccess(t *testing.T) {
	type testCase struct {
		name         string
		res          httpResponse
		waitTime     time.Duration
		maxSleepTime time.Duration
		jitter       time.Duration
		wantSleep    time.Duration
		wantReason   string
	}

	run := func(t *testing.T, tc testCase) {
		sleep, reason, err := checkForRetry(tc.res, tc.waitTime, tc.maxSleepTime, tc.jitter)

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
				statusCode: http.StatusForbidden,
				date:       time.Date(2001, time.April, 30, 13, 0, 0, 0, time.UTC),
				// 1h after
				rateLimitReset: time.Date(2001, time.April, 30, 14, 0, 0, 0, time.UTC),
			},
			maxSleepTime: 10 * time.Second,
			wantSleep:    0 * time.Second,
		},
		{
			name: "rate limited, would sleep a bit, adding also the jitter",
			res: httpResponse{
				statusCode: http.StatusForbidden,
				date:       time.Date(2001, time.April, 30, 13, 0, 0, 0, time.UTC),
				// 1h after
				rateLimitReset: time.Date(2001, time.April, 30, 14, 0, 0, 0, time.UTC),
			},
			maxSleepTime: 2 * time.Hour,
			jitter:       8 * time.Second,
			wantSleep:    1*time.Hour + 8*time.Second,
			wantReason:   "rate limited",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

// BUG: sleeptime + jitter might cause a failure; test sleepTime > maxSleepTime should
// be done before?
