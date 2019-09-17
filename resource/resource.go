// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.

package resource

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/hlog"

	oc "github.com/cloudboss/ofcourse/ofcourse"
)

// Baked in at build time with the linker. See the Taskfile and the Dockerfile.
var buildinfo = "unknown"

var (
	dummyVersion = oc.Version{"ref": "dummy"}

	mandatoryParams = map[string]struct{}{
		"input-repo": struct{}{},
		"state":      struct{}{},
	}

	validStates = map[string]struct{}{
		"error":   struct{}{},
		"failure": struct{}{},
		"pending": struct{}{},
		"success": struct{}{},
	}

	mandatorySources = map[string]struct{}{
		"owner":        struct{}{},
		"repo":         struct{}{},
		"access_token": struct{}{},
	}

	optionalSources = map[string]struct{}{
		"log_level": struct{}{},
		"log_url":   struct{}{},
	}
)

type missingSourceError struct {
	S string
}

func (e *missingSourceError) Error() string {
	return fmt.Sprintf("missing required source key %q", e.S)
}

type unknownSourceError struct {
	Param string
}

func (e *unknownSourceError) Error() string {
	return fmt.Sprintf("unknown source %q", e.Param)
}

type missingParamError struct {
	S string
}

func (e *missingParamError) Error() string {
	return fmt.Sprintf("missing parameter %q", e.S)
}

type invalidParamError struct {
	Param string
	Value string
}

func (e *invalidParamError) Error() string {
	return fmt.Sprintf("invalid parameter %q: %q", e.Param, e.Value)
}

type unknownParamError struct {
	Param string
}

func (e *unknownParamError) Error() string {
	return fmt.Sprintf("unknown parameter %q", e.Param)
}

// BuildInfo returns human-readable build information (tag, git commit, date, ...).
// This is useful to understand in the Concourse UI and logs which resource it is, since log
// output in Concourse doesn't mention  the name of the resource (or task) generating it.
func BuildInfo() string {
	return "This is the Cogito GitHub status resource. " + buildinfo

}

// Resource satisfies the ofcourse.Resource interface.
type Resource struct{}

// Check satisfies ofcourse.Resource.Check(), corresponding to the /opt/resource/check command.
func (r *Resource) Check(
	source oc.Source,
	version oc.Version,
	env oc.Environment,
	log *oc.Logger,
) ([]oc.Version, error) {
	// Note about logging:
	// For `check` we cannot use ofcourse.Logger due to the fact that the Concourse web UI or
	// `fly check-resource` do NOT show anything printed to stderr unless the `check` executable
	// itself exited with a non-zero status code :-(

	// Optional. If it doesn't exist or is not a string, we will not log.
	logURL, _ := source["log_url"].(string)
	hlog.Infof(logURL, BuildInfo())
	hlog.Debugf(logURL, "check: starting")

	// To make Concourse happy it is enough to return always the same version (but not an
	// empty version!) Since it is not clear if it makes sense to return a "real" version for
	// this resource, we keep it simple.
	versions := []oc.Version{dummyVersion}
	return versions, nil
}

// In satisfies ofcourse.Resource.In(), corresponding to the /opt/resource/in command.
func (r *Resource) In(
	outputDirectory string,
	source oc.Source,
	params oc.Params,
	version oc.Version,
	env oc.Environment,
	log *oc.Logger,
) (oc.Version, oc.Metadata, error) {
	log.Infof(BuildInfo())
	log.Debugf("in: starting.")

	// Since it is not clear if it makes sense to return a "real" version for this
	// resource, we keep it simple and return the same version we have been called with.
	return version, oc.Metadata{}, nil
}

// Out satisfies ofcourse.Resource.Out(), corresponding to the /opt/resource/out command.
func (r *Resource) Out(
	inputDirectory string,
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
) (oc.Version, oc.Metadata, error) {
	log.Infof(BuildInfo())
	log.Debugf("out: starting.")

	if err := outValidateSources(source); err != nil {
		return nil, nil, err
	}

	if err := outValidateParams(params); err != nil {
		return nil, nil, err
	}
	repodir, _ := params["input-repo"].(string)
	state, _ := params["state"].(string)

	// All the resource `inputs:` are below inputDirectory (which is an absolute path).

	fpath := filepath.Join(inputDirectory, repodir, ".git/ref")
	data, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading git ref file %w", err)
	}
	ref, tag, err := parseGitRef(string(data))
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("parsed ref %q", ref)
	log.Debugf("parsed tag %q", tag)

	// Finally, post the status to GitHub.
	token, _ := source["access_token"].(string)
	owner, _ := source["owner"].(string)
	repo, _ := source["repo"].(string)
	pipeline := env.Get("BUILD_PIPELINE_NAME")
	job := env.Get("BUILD_JOB_NAME")
	context := pipeline + "/" + job
	status := github.NewStatus(github.API, token, owner, repo, context)

	atc := env.Get("ATC_EXTERNAL_URL")
	team := env.Get("BUILD_TEAM_NAME")
	buildN := env.Get("BUILD_NAME")
	// https://ci.example.com/teams/developers/pipelines/cognito/jobs/hello/builds/25
	target_url := fmt.Sprintf("%s/teams/%s/pipelines/%s/jobs/%s/builds/%s",
		atc, team, pipeline, job, buildN)
	description := "Build " + buildN
	log.Debugf("Posting state %v, owner %v, repo: %v, ref %v, context %v, target_url %v", state, owner, repo, ref, context, target_url)
	err = status.Add(ref, state, target_url, description)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("State posted successfully")

	metadata := oc.Metadata{}
	metadata = append(metadata, oc.NameVal{Name: "state", Value: state})

	return dummyVersion, metadata, nil
}

func outValidateSources(source oc.Source) error {
	// Any missing source?
	for wantS := range mandatorySources {
		if _, ok := source[wantS].(string); !ok {
			return &missingSourceError{wantS}
		}
	}

	// Any unknown source?
	for s := range source {
		_, ok1 := mandatorySources[s]
		_, ok2 := optionalSources[s]
		if !ok1 && !ok2 {
			return &unknownSourceError{s}
		}
	}

	return nil
}

func outValidateParams(params oc.Params) error {
	// Any missing parameter?
	for wantP := range mandatoryParams {
		if _, ok := params[wantP].(string); !ok {
			return &missingParamError{wantP}
		}
	}

	// Any invalid parameter?
	state, _ := params["state"].(string)
	if _, ok := validStates[state]; !ok {
		return &invalidParamError{"state", state}
	}

	// Any unknown parameter?
	for p := range params {
		if _, ok := mandatoryParams[p]; !ok {
			return &unknownParamError{p}
		}
	}

	return nil
}

// Parse the contents of the file ".git/ref" (created by the concourse git resource) and return
// the ref and the tag (if present).
// Normally that file contains only the ref, but it will contain also the tag when the git
// resource is using tag_filter.
func parseGitRef(in string) (string, string, error) {
	if len(in) == 0 {
		return "", "", fmt.Errorf("parseGitRef: empty input")
	}
	tokens := strings.Split(in, "\n")
	ref := tokens[0]
	tag := ""
	if len(tokens) > 1 {
		tag = tokens[1]
	}
	return ref, tag, nil
}
