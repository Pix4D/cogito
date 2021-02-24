// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.
package resource

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/hlog"
	"github.com/sasbury/mini"

	oc "github.com/cloudboss/ofcourse/ofcourse"
)

// Baked in at build time with the linker. See the Taskfile and the Dockerfile.
var buildinfo = "unknown"

var (
	errKeyNotFound = errors.New("key not found")
	errWrongRemote = errors.New("wrong git remote")
	errInvalidURL  = errors.New("invalid git URL")
	errInvalidHead = errors.New("invalid HEAD format")
)

var (
	dummyVersion = oc.Version{"ref": "dummy"}

	outMandatoryParams = map[string]struct{}{
		"state": {},
	}

	outOptionalParams = map[string]struct{}{
		"context": {},
	}

	outValidStates = map[string]struct{}{
		"error":   {},
		"failure": {},
		"pending": {},
		"success": {},
	}

	mandatorySources = map[string]struct{}{
		"owner":        {},
		"repo":         {},
		"access_token": {},
	}

	optionalSources = map[string]struct{}{
		"log_level":      {},
		"log_url":        {},
		"context_prefix": {},
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
// output in Concourse doesn't mention the name of the resource (or task) generating it.
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
	hlog.Debugf(logURL, "check: started")
	defer hlog.Debugf(logURL, "check: finished")

	if err := validateSources(source); err != nil {
		return nil, err
	}

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
	log.Debugf("in: started")
	defer log.Debugf("in: finished")

	log.Debugf("in: params:\n%s", stringify1(params))
	log.Debugf("in: env:\n%s", stringify2(env.GetAll()))

	if err := validateSources(source); err != nil {
		return nil, nil, err
	}

	// Since it is not clear if it makes sense to return a "real" version for this
	// resource, we keep it simple and return the same version we have been called with, ensuring
	// we never return a nul version.
	if len(version) == 0 {
		version = dummyVersion
	}
	return version, oc.Metadata{}, nil
}

// Out satisfies ofcourse.Resource.Out(), corresponding to the /opt/resource/out command.
func (r *Resource) Out(
	inputDirectory string, // All the pipeline `inputs:` are below here.
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
) (oc.Version, oc.Metadata, error) {
	log.Infof(BuildInfo())
	log.Debugf("out: started")
	defer log.Debugf("out: finished")

	log.Debugf("out: params:\n%s", stringify1(params))
	log.Debugf("out: env:\n%s", stringify2(env.GetAll()))

	if err := validateSources(source); err != nil {
		return nil, nil, err
	}

	if err := outValidateParams(params); err != nil {
		return nil, nil, err
	}
	state, _ := params["state"].(string)

	owner, _ := source["owner"].(string)
	repo, _ := source["repo"].(string)

	inputDirs, err := collectInputDirs(inputDirectory)
	if err != nil {
		return nil, nil, err
	}
	if len(inputDirs) != 1 {
		err := fmt.Errorf("found %d input dirs: %v. "+
			"Want exactly 1, corresponding to the GitHub repo %s/%s",
			len(inputDirs), inputDirs, owner, repo)
		return nil, nil, err
	}

	repoDir := filepath.Join(inputDirectory, inputDirs[0])
	if err := checkRepoDir(repoDir, owner, repo); err != nil {
		return nil, nil, err
	}

	ref, err := GitCommit(repoDir)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("out: parsed ref %q", ref)

	// Finally, post the status to GitHub.
	token, _ := source["access_token"].(string)
	pipeline := env.Get("BUILD_PIPELINE_NAME")
	job := env.Get("BUILD_JOB_NAME")

	context := job // default
	if v, ok := params["context"].(string); ok {
		context = v
	}

	if prefix, ok := source["context_prefix"].(string); ok {
		context = fmt.Sprintf("%s/%s", prefix, context)
	}

	status := github.NewStatus(github.API, token, owner, repo, context)

	atc := env.Get("ATC_EXTERNAL_URL")
	team := env.Get("BUILD_TEAM_NAME")
	buildN := env.Get("BUILD_NAME")
	instanceVars := env.Get("BUILD_PIPELINE_INSTANCE_VARS")

	// https://ci.example.com/teams/main/pipelines/cogito/jobs/hello/builds/25
	targetURL := fmt.Sprintf("%s/teams/%s/pipelines/%s/jobs/%s/builds/%s",
		atc, team, pipeline, job, buildN)

	if instanceVars != "" {
		targetURL = fmt.Sprintf("%s?vars=%s", targetURL, url.QueryEscape(instanceVars))
	}

	description := "Build " + buildN
	log.Debugf(`out: posting:
  state: %v
  owner: %v
  repo: %v
  ref: %v
  context: %v
  targetURL: %v`,
		state, owner, repo, ref, context, targetURL)
	err = status.Add(ref, state, targetURL, description)
	if err != nil {
		return nil, nil, err
	}
	log.Infof("out: GitHub state %s for ref %s posted successfully", state, ref[0:9])

	metadata := oc.Metadata{}
	metadata = append(metadata, oc.NameVal{Name: "state", Value: state})

	return dummyVersion, metadata, nil
}

func stringify1(xs map[string]interface{}) string {
	var bld strings.Builder
	for k, v := range xs {
		fmt.Fprintf(&bld, "  %s: %v\n", k, v)
	}
	return bld.String()
}

func stringify2(xs map[string]string) string {
	var bld strings.Builder
	for k, v := range xs {
		fmt.Fprintf(&bld, "  %s: %v\n", k, v)
	}
	return bld.String()
}

func validateSources(source oc.Source) error {
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
	for wantP := range outMandatoryParams {
		if _, ok := params[wantP].(string); !ok {
			return &missingParamError{wantP}
		}
	}

	// Any invalid parameter?
	state, _ := params["state"].(string)
	if _, ok := outValidStates[state]; !ok {
		return &invalidParamError{"state", state}
	}

	// Any unknown parameter?
	for p := range params {
		_, ok1 := outMandatoryParams[p]
		_, ok2 := outOptionalParams[p]
		if !ok1 && !ok2 {
			return &unknownParamError{p}
		}
	}

	return nil
}

// Return a list of all directories below dir (non-recursive).
func collectInputDirs(dir string) ([]string, error) {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("collecting directories in %v: %w", dir, err)
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// Check if dir is a git repository, of the form owner/repo, with an origin accessed
// over HTTP, HTTPS or SSH.
func checkRepoDir(dir, owner, repo string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("parsing .git/config: abspath: %w", err)
	}
	cfg, err := mini.LoadConfiguration(filepath.Join(dir, ".git/config"))
	if err != nil {
		return fmt.Errorf("parsing .git/config: %w", err)
	}

	// .git/config contains a section like:
	//
	// [remote "origin"]
	//     url = git@github.com:Pix4D/cogito.git
	//     fetch = +refs/heads/*:refs/remotes/origin/*
	//
	const section = `remote "origin"`
	const key = "url"
	remote := cfg.StringFromSection(section, key, "")
	if remote == "" {
		return fmt.Errorf(".git/config: key '%s/%s': %w", section, key, errKeyNotFound)
	}
	gu, err := parseGitPseudoURL(remote)
	if err != nil {
		return fmt.Errorf(".git/config: remote: %w", err)
	}
	left := []string{"github.com", owner, repo}
	right := []string{gu.Host, gu.Owner, gu.Repo}
	for i, l := range left {
		r := right[i]
		if !strings.EqualFold(l, r) {
			return fmt.Errorf("resource source configuration and git repository are incompatible.\nGit remote: %q\n"+
				"Resource config: host: github.com, owner: %q, repo: %q. %w", remote, owner, repo, errWrongRemote,
			)
		}
	}
	return nil
}

type gitURL struct {
	Scheme string // "ssh", "https" or "http"
	Host   string
	Owner  string
	Repo   string
}

// Three types of pseudo URLs:
//     git@github.com:Pix4D/cogito.git
// https://github.com/Pix4D/cogito.git
//  http://github.com/Pix4D/cogito.git
func parseGitPseudoURL(URL string) (gitURL, error) {
	var path string
	gu := gitURL{}
	if strings.HasPrefix(URL, "git@") {
		gu.Scheme = "ssh"
		path = URL[4:]
		if strings.Count(path, ":") != 1 {
			return gitURL{}, fmt.Errorf("url: %v: %w", URL, errInvalidURL)
		}
		path = strings.Replace(path, ":", "/", 1)
	} else if strings.HasPrefix(URL, "https://") {
		gu.Scheme = "https"
		path = URL[8:]
	} else if strings.HasPrefix(URL, "http://") {
		gu.Scheme = "http"
		path = URL[7:]
	} else {
		return gitURL{}, fmt.Errorf("url: %v: %w", URL, errInvalidURL)
	}
	// github.com/Pix4D/cogito.git
	tokens := strings.Split(path, "/")
	if len(tokens) != 3 {
		return gitURL{}, fmt.Errorf("path: %v: %w", path, errInvalidURL)
	}
	gu.Host = tokens[0]
	gu.Owner = tokens[1]
	gu.Repo = strings.TrimSuffix(tokens[2], ".git")
	return gu, nil
}

// GitCommit looks into a git repository and extracts the commit SHA of the HEAD.
func GitCommit(repoPath string) (string, error) {
	dotGitPath := filepath.Join(repoPath, ".git")

	headPath := filepath.Join(dotGitPath, "HEAD")
	headBuf, err := ioutil.ReadFile(headPath)
	if err != nil {
		return "", fmt.Errorf("reading HEAD: %w", err)
	}

	// The HEAD file can have two completely different contents:
	// 1. if a branch checkout: "ref: refs/heads/BRANCH_NAME"
	// 2. if a detached head : the commit SHA
	// A detached head with Concourse happens in two cases:
	// 1. if the git resource has a `tag_filter:`
	// 2. if the git resource has a `version:`

	head := strings.TrimSuffix(string(headBuf), "\n")
	tokens := strings.Fields(head)
	var sha string
	switch len(tokens) {
	case 1:
		// detached head
		sha = head
	case 2:
		// branch checkout
		shaRelPath := tokens[1]
		shaPath := filepath.Join(dotGitPath, shaRelPath)
		shaBuf, err := ioutil.ReadFile(shaPath)
		if err != nil {
			return "", fmt.Errorf("reading SHA file: %w", err)
		}
		sha = strings.TrimSuffix(string(shaBuf), "\n")
	default:
		return "", errInvalidHead
	}

	// Minimal validation that the file contents look like a 40-digit SHA.
	const shaLen = 40
	if len(sha) != shaLen {
		return "", fmt.Errorf("SHA: %v: got len of %v; want %v", sha, len(sha), shaLen)
	}

	return sha, nil
}
