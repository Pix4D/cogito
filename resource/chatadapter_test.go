package resource

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestChatAdapterMock(t *testing.T) {
}

func TestChatAdapterIntegration(t *testing.T) {
	gchatUrl := os.Getenv("COGITO_TEST_GCHAT_HOOK")
	if len(gchatUrl) == 0 {
		t.Skip("Skipping integration test. See CONTRIBUTING for how to enable.")
	}
	gitRef := "32e4b4f91b"
	job := "peel"

	// We send multiple messages to better see the UI in GChat and run the subtests
	// in parallel for speed.
	testCases := []struct {
		pipeline string
		state    string
	}{
		{
			pipeline: "coconut",
			state:    abortState,
		},
		{
			pipeline: "coconut",
			state:    errorState,
		},
		{
			pipeline: "coconut",
			state:    failureState,
		},
		{
			pipeline: "coconut",
			state:    pendingState,
		},
		{
			pipeline: "coconut",
			state:    successState,
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable, needed for t.Parallel to work correctly.
		t.Run(tc.pipeline, func(t *testing.T) {
			t.Parallel()
			buildURL := "https://cogito.invalid/teams/TEAM/pipelines/PIPELINE/jobs/JOB/builds/42"
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := gChatMessage(ctx, gchatUrl, gitRef, tc.pipeline, job, tc.state,
				buildURL)

			if err != nil {
				t.Fatalf("have: %s; want: <no error>", err)
			}
		})
	}
}
