package resource

import (
	"fmt"
	"net/url"
	"path"
)

// ghTargetURL builds an URL suitable to be used as the target_url parameter for the
// Github commit status API.
func ghTargetURL(atc, team, pipeline, job, buildN, instanceVars string) string {
	// Example:
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/hello/builds/25
	targetURL := atc +
		path.Join("/teams", team, "pipelines", pipeline, "jobs", job, "builds", buildN)

	// Example:
	// BUILD_PIPELINE_INSTANCE_VARS: {"branch":"stable"}
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/autocat/builds/3?vars=%7B%22branch%22%3A%22stable%22%7D
	if instanceVars != "" {
		targetURL = fmt.Sprintf("%s?vars=%s", targetURL, url.QueryEscape(instanceVars))
	}

	return targetURL
}
