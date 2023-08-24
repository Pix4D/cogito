package cogito

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/Pix4D/cogito/googlechat"
)

// GoogleChatSink is an implementation of [Sinker] for the Cogito resource.
type GoogleChatSink struct {
	Log      hclog.Logger
	InputDir fs.FS
	GitRef   string
	Request  PutRequest
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
	if !shouldSendToChat(sink.Request) {
		sink.Log.Debug("not sending to chat",
			"reason", "state not in configured states", "state", state)
		return nil
	}

	text, err := prepareChatMessage(sink.InputDir, sink.Request, sink.GitRef)
	if err != nil {
		return fmt.Errorf("GoogleChatSink: %s", err)
	}

	threadKey := fmt.Sprintf("%s %s", sink.Request.Env.BuildPipelineName, sink.GitRef)
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
func shouldSendToChat(request PutRequest) bool {
	if request.Params.ChatMessage != "" || request.Params.ChatMessageFile != "" {
		return true
	}
	for _, x := range request.Source.ChatNotifyOnStates {
		if request.Params.State == x {
			return true
		}
	}
	return false
}

// prepareChatMessage returns a message ready to be sent to the chat sink.
func prepareChatMessage(inputDir fs.FS, request PutRequest, gitRef string,
) (string, error) {
	params := request.Params

	var parts []string
	if params.ChatMessage != "" {
		parts = append(parts, params.ChatMessage)
	}
	if params.ChatMessageFile != "" {
		contents, err := fs.ReadFile(inputDir, params.ChatMessageFile)
		if err != nil {
			return "", fmt.Errorf("reading chat_message_file: %s", err)
		}
		parts = append(parts, string(contents))
	}

	if len(parts) == 0 || (len(parts) > 0 && params.ChatAppendSummary) {
		parts = append(
			parts,
			gChatBuildSummaryText(gitRef, params.State, request.Source, request.Env))
	}

	return strings.Join(parts, "\n\n"), nil
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

	// Unfortunately the font is proportional and doesn't support tabs,
	// so we cannot align in columns.
	var bld strings.Builder
	fmt.Fprintf(&bld, "%s\n", now)
	fmt.Fprintf(&bld, "*pipeline* %s\n", env.BuildPipelineName)
	fmt.Fprintf(&bld, "*job* %s\n", job)
	fmt.Fprintf(&bld, "*state* %s\n", decorateState(state))
	// An empty gitRef means that cogito has been configured as chat only.
	if gitRef != "" {
		commitUrl := fmt.Sprintf("https://github.com/%s/%s/commit/%s",
			src.Owner, src.Repo, gitRef)
		commit := fmt.Sprintf("<%s|%.10s> (repo: %s/%s)",
			commitUrl, gitRef, src.Owner, src.Repo)
		fmt.Fprintf(&bld, "*commit* %s\n", commit)
	}

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
