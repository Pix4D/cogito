package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/Pix4D/cogito/googlechat"
)

// GChatMessage sends a one-off text message to webhook hookURL, containing information
// about a Concourse job build status.
// Note that the Google Chat API encodes the secret in the webhook itself.
// Parameter pipeline will be used as thread key.
func GChatMessage(
	ctx context.Context,
	hookURL string,
	pipeline string,
	job string,
	state string,
	buildURL string,
) error {
	ts := time.Now().Format("2006-01-02 15:04:05 MST")

	var icon string
	switch state {
	case "pending":
		icon = "🟡"
	case "success":
		icon = "🟢"
	case "failure":
		icon = "🔴"
	case "error":
		icon = "🟠"
	default:
		icon = "❓"
	}

	// Unfortunately the font is proportional and doesn't support tabs,
	// so we cannot align in columns.
	text := fmt.Sprintf(`%s
*pipeline* %s
*job* %s
*state* %s %s
%s`,
		ts,
		pipeline,
		job,
		icon, state,
		buildURL)

	return googlechat.TextMessage(ctx, hookURL, pipeline, text)
}
