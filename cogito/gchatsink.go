package cogito

import (
	"context"
	"fmt"
	"strings"
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

	// If present, params.gchat_webhook overrides source.gchat_webhook.
	webHook := sink.Request.Source.GChatWebHook
	if sink.Request.Params.GChatWebHook != "" {
		webHook = sink.Request.Params.GChatWebHook
		sink.Log.Debug("params.gchat_webhook is overriding source.gchat_webhook")
	}
	if webHook == "" {
		sink.Log.Info("not sending to chat", "reason", "feature not enabled")
		return nil
	}

	state := sink.Request.Params.State
	if !shouldSendToChat(state, sink.Request.Source.NotifyOnStates) {
		sink.Log.Debug("not sending to chat",
			"reason", "state not in enabled states", "state", state)
		return nil
	}

	// Prepare the message.
	threadKey := fmt.Sprintf("%s %s", sink.Request.Env.BuildPipelineName, sink.GitRef)
	text := prepareChatMessage(sink.Request, sink.GitRef)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reply, err := googlechat.TextMessage(ctx, webHook, threadKey, text)
	if err != nil {
		return fmt.Errorf("GoogleChatSink: %s", err)
	}

	sink.Log.Info("state posted successfully to chat",
		"state", state, "space", reply.Space.DisplayName,
		"sender", reply.Sender.DisplayName, "text", text)
	return nil
}

// shouldSendToChat returns true if the state is configured to do so.
func shouldSendToChat(state BuildState, notifyOnStates []BuildState) bool {
	for _, x := range notifyOnStates {
		if state == x {
			return true
		}
	}
	return false
}

// prepareChatMessage returns a message ready to be sent to the chat sink.
func prepareChatMessage(request PutRequest, gitRef string) string {
	var text string
	if request.Params.ChatMessage != "" {
		text = request.Params.ChatMessage
	} else {
		text = gChatBuildSummaryText(gitRef, request.Params.State, request.Source,
			request.Env)
	}

	return text
}

// gChatBuildSummaryText returns a plain text message to be sent to Google Chat.
func gChatBuildSummaryText(gitRef string, state BuildState, src Source, env Environment,
) string {
	now := time.Now().Format("2006-01-02 15:04:05 MST")

	// Google Chat format for links with alternate name:
	// <https://example.com/foo|my link text>
	// GitHub link to commit:
	// https://github.com/Pix4D/cogito/commit/e8c6e2ac0318b5f0baa3f55
	job := fmt.Sprintf("<%s|%s/%s>",
		concourseBuildURL(env), env.BuildJobName, env.BuildName)
	commitUrl := fmt.Sprintf("https://github.com/%s/%s/commit/%s",
		src.Owner, src.Repo, gitRef)
	commit := fmt.Sprintf("<%s|%.10s> (repo: %s/%s)",
		commitUrl, gitRef, src.Owner, src.Repo)

	// Unfortunately the font is proportional and doesn't support tabs,
	// so we cannot align in columns.
	var bld strings.Builder
	fmt.Fprintf(&bld, "%s\n", now)
	fmt.Fprintf(&bld, "*pipeline* %s\n", env.BuildPipelineName)
	fmt.Fprintf(&bld, "*job* %s\n", job)
	fmt.Fprintf(&bld, "*state* %s\n", decorateState(state))
	fmt.Fprintf(&bld, "*commit* %s\n", commit)

	return bld.String()
}

func decorateState(state BuildState) string {
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

	return fmt.Sprintf("%s %s", icon, state)
}
