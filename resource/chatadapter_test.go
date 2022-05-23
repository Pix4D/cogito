package resource_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Pix4D/cogito/resource"
)

func TestChatAdapterMock(t *testing.T) {
}

func TestChatAdapterIntegration(t *testing.T) {
	gchatUrl := os.Getenv("COGITO_TEST_GCHAT_HOOK")
	if len(gchatUrl) == 0 {
		t.Skip("Skipping integration test. See CONTRIBUTING for how to enable.")
	}
	gitRef := "cafefade"
	job := "peel"

	// We send multiple messages to better see the UI in GChat and run the subtests
	// in parallel for speed.
	testCases := []struct {
		pipeline string
		state    string
	}{
		{
			pipeline: "avocado",
			state:    "success",
		},
		{
			pipeline: "coconut",
			state:    "failure",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable, needed for t.Parallel to work correctly.
		t.Run(tc.pipeline, func(t *testing.T) {
			t.Parallel()
			buildURL := "https://ci.example.com/invalid"
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := resource.GChatMessage(ctx, gchatUrl, gitRef, tc.pipeline, job, tc.state,
				buildURL)

			if err != nil {
				t.Fatalf("have: %s; want: <no error>", err)
			}
		})
	}
}
