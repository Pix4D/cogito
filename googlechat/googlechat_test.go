package googlechat_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/Pix4D/cogito/googlechat"
)

func TestTextMessageIntegration(t *testing.T) {
	gchatUrl := os.Getenv("COGITO_TEST_GCHAT_HOOK")
	if len(gchatUrl) == 0 {
		t.Skip("Skipping integration test. See CONTRIBUTING for how to enable.")
	}
	threadKey := "banana"
	ts := time.Now().Format("2006-01-02 15:04:05 MST")
	text := fmt.Sprintf("%s message oink! 🐷 sent to thread %s", ts, threadKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := googlechat.TextMessage(ctx, gchatUrl, threadKey, text)

	if err != nil {
		t.Fatalf("have: %s; want: <no error>", err)
	}
}

func TestRedactURL(t *testing.T) {
	hook := "https://chat.googleapis.com/v1/spaces/SSS/messages?key=KKK&token=TTT"
	want := "https://chat.googleapis.com/v1/spaces/SSS/messages?REDACTED"
	theURL, err := url.Parse(hook)
	if err != nil {
		t.Fatal(err)
	}

	have := googlechat.RedactURL(theURL).String()

	if have != want {
		t.Fatalf("\nhave: %s\nwant: %s", have, want)
	}
}

func TestRedactString(t *testing.T) {
	hook := "https://chat.googleapis.com/v1/spaces/SSS/messages?key=KKK&token=TTT"
	want := "https://chat.googleapis.com/v1/spaces/SSS/messages?REDACTED"

	have := googlechat.RedactURLString(hook)

	if have != want {
		t.Fatalf("\nhave: %s\nwant: %s", have, want)
	}
}
