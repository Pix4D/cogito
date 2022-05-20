package resource

import (
	"fmt"
	"net/url"
	"path"

	"github.com/Pix4D/cogito/github"
	oc "github.com/cloudboss/ofcourse/ofcourse"
)

func postGitHubCommitStatus(
	ghAPI string,
	gitRef string,
	pipeline string,
	job string,
	buildN string,
	state string,
	buildURL string,
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
) error {
	// Prepare API parameter "context".
	context := job // default
	if v, ok := params["context"].(string); ok {
		context = v
	}
	if prefix, ok := source["context_prefix"].(string); ok {
		context = fmt.Sprintf("%s/%s", prefix, context)
	}

	token, _ := source["access_token"].(string)
	owner, _ := source["owner"].(string)
	repo, _ := source["repo"].(string)

	commitStatus := github.NewCommitStatus(ghAPI, token, owner, repo, context)

	description := "Build " + buildN

	log.Debugf(`posting to GH Commit status:
  state: %v
  owner: %v
  repo: %v
  ref: %v
  context: %v
  targetURL: %v
  description: %v`,
		state, owner, repo, gitRef, context, buildURL, description)

	return commitStatus.Add(gitRef, state, buildURL, description)
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
