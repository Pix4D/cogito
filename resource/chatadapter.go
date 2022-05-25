package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/Pix4D/cogito/googlechat"
	oc "github.com/cloudboss/ofcourse/ofcourse"
)

// sendToChat sends a message to the chat sink if the chat feature is enabled and the
// state is configured to do so.
func sendToChat(
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
	gitRef string,
) error {
	state, _ := params["state"].(string)
	pipeline := env.Get("BUILD_PIPELINE_NAME")
	job := env.Get("BUILD_JOB_NAME")
	atc := env.Get("ATC_EXTERNAL_URL")
	team := env.Get("BUILD_TEAM_NAME")
	buildN := env.Get("BUILD_NAME")
	instanceVars := env.Get("BUILD_PIPELINE_INSTANCE_VARS")
	buildURL := concourseBuildURL(atc, team, pipeline, job, buildN, instanceVars)

	webhook, ok := source["gchat_webhook"].(string)
	if !ok || webhook == "" {
		log.Debugf("not sending to chat; reason: feature not enabled")
		return nil
	}

	if !shouldSendToChat(state) {
		log.Debugf("not sending to chat; reason: state %s not in enabled states", state)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := gChatMessage(ctx, webhook, gitRef, pipeline, job, state, buildURL)
	if err != nil {
		return err
	}
	log.Infof("Chat state %s for %s/%s posted successfully", state, pipeline, job)

	return nil
}

// shouldSendToChat returns true if the state is configured to do so.
func shouldSendToChat(state string) bool {
	for _, x := range statesToNotifyChat {
		if state == x {
			return true
		}
	}
	return false
}

// GChatMessage sends a one-off text message to webhook hookURL, containing information
// about a Concourse job build status. The thread Key is pipeline + git commit hash.
// Note that the Google Chat API encodes the secret in the webhook itself.
// Parameter pipeline will be used as thread key.
func gChatMessage(
	ctx context.Context,
	hookURL string,
	gitRef string,
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

	threadKey := fmt.Sprintf("%s %s", pipeline, gitRef)

	if len(gitRef) > 10 {
		gitRef = gitRef[0:10]
	}

	// Unfortunately the font is proportional and doesn't support tabs,
	// so we cannot align in columns.
	text := fmt.Sprintf(`%s
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

	return googlechat.TextMessage(ctx, hookURL, threadKey, text)
}
