package cogito

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/retry"
)

const (
	// retryUpTo is the total maximum duration of the retries.
	retryUpTo = 15 * time.Minute

	// retryFirstDelay is duration of the first backoff.
	retryFirstDelay = 2 * time.Second

	// retryBackoffLimit is the upper bound duration of a backoff.
	// That is, with an exponential backoff and a retryFirstDelay = 2s, the sequence will be:
	// 2s 4s 8s 16s 32s 60s ... 60s, until reaching a cumulative delay of retryUpTo.
	retryBackoffLimit = 1 * time.Minute
)

// GitHubCommitStatusSink is an implementation of [Sinker] for the Cogito resource.
type GitHubCommitStatusSink struct {
	Log     *slog.Logger
	GitRef  string
	Request PutRequest
}

// Send sets the build status via the GitHub Commit status API endpoint.
func (sink GitHubCommitStatusSink) Send() error {
	sink.Log.Debug("send: started")
	defer sink.Log.Debug("send: finished")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	httpClient := &http.Client{}

	ghState := ghAdaptState(sink.Request.Params.State)
	buildURL := concourseBuildURL(sink.Request.Env)
	context := ghMakeContext(sink.Request)

	target := &github.Target{
		Client: httpClient,
		Server: github.ApiRoot(sink.Request.Source.GhHostname),
		Retry: retry.Retry{
			FirstDelay:   retryFirstDelay,
			BackoffLimit: retryBackoffLimit,
			UpTo:         retryUpTo,
			Log:          sink.Log,
		},
	}
	commitStatus := github.NewCommitStatus(target, sink.Request.Source.AccessToken,
		sink.Request.Source.Owner, sink.Request.Source.Repo, context, sink.Log)
	description := "Build " + sink.Request.Env.BuildName

	sink.Log.Debug("posting to GitHub Commit Status API",
		"state", ghState, "owner", sink.Request.Source.Owner,
		"repo", sink.Request.Source.Repo, "git-ref", sink.GitRef,
		"context", context, "buildURL", buildURL, "description", description)
	if sink.Request.Source.OmitTargetURL {
		buildURL = ""
	}
	if err := commitStatus.Add(ctx, sink.GitRef, ghState, buildURL, description); err != nil {
		return err
	}
	sink.Log.Info("commit status posted successfully",
		"state", ghState, "git-ref", sink.GitRef[0:9])

	return nil
}

// The states allowed by cogito are more than the states allowed by the GitHub Commit
// status API. Adapt accordingly.
func ghAdaptState(state BuildState) string {
	if state == StateAbort {
		return string(StateError)
	}
	return string(state)
}

// ghMakeContext returns the "context" parameter of the GitHub Commit Status API, based
// on the fields of request.
func ghMakeContext(request PutRequest) string {
	var context string
	if request.Source.ContextPrefix != "" {
		context = request.Source.ContextPrefix + "/"
	}
	if request.Params.Context != "" {
		context += request.Params.Context
	} else {
		context += request.Env.BuildJobName
	}
	return context
}
