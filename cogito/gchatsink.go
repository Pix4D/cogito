package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/Pix4D/cogito/googlechat"
	"github.com/hashicorp/go-hclog"
)

// GoogleChatSink is an implementation of [Sinker] for the Cogito resource.
type GoogleChatSink struct {
	Log     hclog.Logger
	GitRef  string
	Request PutRequest
}

// Send sends a message to Google Chat if the configuration matches.
func (sink GoogleChatSink) Send() error {
	sink.Log.Debug("send: started")
	defer sink.Log.Debug("send: finished")

	if sink.Request.Source.GChatWebHook == "" {
		sink.Log.Debug("not sending to chat",
			"reason", "feature not enabled")
		return nil
	}

	state := sink.Request.Params.State
	if !shouldSendToChat(state) {
		sink.Log.Debug("not sending to chat",
			"reason", "state not in enabled states", "state", state)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline := sink.Request.Env.BuildPipelineName
	job := sink.Request.Env.BuildJobName
	buildURL := concourseBuildURL(sink.Request.Env)

	threadKey := fmt.Sprintf("%s %s", pipeline, sink.GitRef)
	text := gChatFormatText(sink.GitRef, pipeline, job, state, buildURL)

	if err := googlechat.TextMessage(ctx, sink.Request.Source.GChatWebHook, threadKey,
		text); err != nil {
		return fmt.Errorf("GoogleChatSink: %s", err)
	}

	sink.Log.Info("state posted successfully to chat",
		"state", state, "pipeline", pipeline, "job", job, "buildURL", buildURL)
	return nil
}

// shouldSendToChat returns true if the state is configured to do so.
func shouldSendToChat(state BuildState) bool {
	// States that will trigger a chat notification by default.
	statesToNotifyChat := []BuildState{StateAbort, StateError, StateFailure}

	for _, x := range statesToNotifyChat {
		if state == x {
			return true
		}
	}
	return false
}

// gChatFormatText returns a plain text message to be sent to Google Chat.
func gChatFormatText(gitRef string, pipeline string, job string, state BuildState,
	buildURL string,
) string {
	ts := time.Now().Format("2006-01-02 15:04:05 MST")
	gitRef = fmt.Sprintf("%.10s", gitRef)

	var icon string
	switch state {
	case StateAbort:
		icon = "🟤"
	case StateError:
		icon = "🟠"
	case StateFailure:
		icon = "🔴"
	case StatePending:
		icon = "🟡"
	case StateSuccess:
		icon = "🟢"
	default:
		icon = "❓"
	}

	// Unfortunately the font is proportional and doesn't support tabs,
	// so we cannot align in columns.
	return fmt.Sprintf(`%s
*pipeline* %s
*job* %s
*commit* %s
*state* %s %s
*build* %s`,
		ts,
		pipeline,
		job,
		gitRef,
		icon, state,
		buildURL)
}
