package resource

import (
	"fmt"
	"net/url"
	"path"

	"github.com/Pix4D/cogito/github"
	oc "github.com/cloudboss/ofcourse/ofcourse"
)

func gitHubCommitStatus(
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
	gitRef string,
	ghAPI string,
) error {
	state, _ := params[stateKey].(string)
	state = ghAdaptState(state)
	pipeline := env.Get("BUILD_PIPELINE_NAME")
	job := env.Get("BUILD_JOB_NAME")
	atc := env.Get("ATC_EXTERNAL_URL")
	team := env.Get("BUILD_TEAM_NAME")
	buildN := env.Get("BUILD_NAME")
	instanceVars := env.Get("BUILD_PIPELINE_INSTANCE_VARS")
	buildURL := concourseBuildURL(atc, team, pipeline, job, buildN, instanceVars)

	description := "Build " + buildN
	token, _ := source[accessTokenKey].(string)
	owner, _ := source[ownerKey].(string)
	repo, _ := source[repoKey].(string)

	// Prepare API parameter "context".
	context := job // default
	if v, ok := params[contextKey].(string); ok {
		context = v
	}
	if prefix, ok := source[contextPrefixKey].(string); ok {
		context = fmt.Sprintf("%s/%s", prefix, context)
	}

	commitStatus := github.NewCommitStatus(ghAPI, token, owner, repo, context)

	log.Debugf(`posting to GH Commit status:
  state: %v
  owner: %v
  repo: %v
  ref: %v
  context: %v
  buildURL: %v
  description: %v`,
		state, owner, repo, gitRef, context, buildURL, description)

	if err := commitStatus.Add(gitRef, state, buildURL, description); err != nil {
		return err
	}
	log.Infof("GitHub commit status %s for ref %s posted successfully", state,
		gitRef[0:9])

	return nil
}

// The states allowed by cogito are more than the states allowed by the GitHub Commit
// status API. Adapt accordingly.
func ghAdaptState(state string) string {
	if state == abortState {
		return errorState
	}
	return state
}

// concourseBuildURL builds an URL pointing to a specific build of a job in a pipeline.
func concourseBuildURL(atc, team, pipeline, job, buildN, instanceVars string) string {
	// Example:
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/hello/builds/25
	buildURL := atc +
		path.Join("/teams", team, "pipelines", pipeline, "jobs", job, "builds", buildN)

	// Example:
	// BUILD_PIPELINE_INSTANCE_VARS: {"branch":"stable"}
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/autocat/builds/3?vars=%7B%22branch%22%3A%22stable%22%7D
	if instanceVars != "" {
		buildURL = fmt.Sprintf("%s?vars=%s", buildURL, url.QueryEscape(instanceVars))
	}

	return buildURL
}
