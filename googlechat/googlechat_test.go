package googlechat_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/Pix4D/cogito/googlechat"
	"github.com/Pix4D/cogito/testhelp"
)

func TestTextMessageIntegration(t *testing.T) {
	log := testhelp.MakeTestLog()
	gchatUrl := os.Getenv("COGITO_TEST_GCHAT_HOOK")
	if len(gchatUrl) == 0 {
		t.Skip("Skipping integration test. See CONTRIBUTING for how to enable.")
	}
	ts := time.Now().Format("2006-01-02 15:04:05 MST")
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	threadKey := "banana-" + user
	text := fmt.Sprintf("%s message oink! 🐷 sent to thread %s by user %s",
		ts, threadKey, user)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reply, err := googlechat.TextMessage(ctx, log, gchatUrl, threadKey, text)

	assert.NilError(t, err)
	assert.Assert(t, cmp.Contains(reply.Text, text))
}

func TestRedactURL(t *testing.T) {
	hook := "https://chat.googleapis.com/v1/spaces/SSS/messages?key=KKK&token=TTT"
	want := "https://chat.googleapis.com/v1/spaces/SSS/messages?REDACTED"
	theURL, err := url.Parse(hook)
	assert.NilError(t, err)

	have := googlechat.RedactURL(theURL).String()

	assert.Equal(t, have, want)
}

func TestRedactString(t *testing.T) {
	hook := "https://chat.googleapis.com/v1/spaces/SSS/messages?key=KKK&token=TTT"
	want := "https://chat.googleapis.com/v1/spaces/SSS/messages?REDACTED"

	have := googlechat.RedactURLString(hook)

	assert.Equal(t, have, want)
}
