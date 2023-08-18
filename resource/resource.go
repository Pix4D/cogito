// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.
package resource

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Pix4D/cogito/github"
	"github.com/sasbury/mini"

	oc "github.com/cloudboss/ofcourse/ofcourse"
)

// Baked in at build time with the linker. See the Taskfile and the Dockerfile.
var buildinfo = "unknown"

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

	mandatorySourceKeys = map[string]struct{}{
		"owner":        {},
		"repo":         {},
		"access_token": {},
	}

	optionalSourceKeys = map[string]struct{}{
		"log_level":      {},
		"log_url":        {},
		"context_prefix": {},
		"github_host":    {},
	}
)

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
	log.Infof(BuildInfo())
	log.Debugf("check: started")
	defer log.Debugf("check: finished")

	if err := validateSource(source); err != nil {
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

	if err := validateSource(source); err != nil {
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

	if err := validateSource(source); err != nil {
		return nil, nil, err
	}

	if err := validateOutParams(params); err != nil {
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

	githubHost := github.HOST // default
	if hostname, ok := source["github_host"].(string); ok {
		githubHost = hostname
	}

	repoDir := filepath.Join(inputDirectory, inputDirs[0])
	if err := checkRepoDir(repoDir, owner, repo, githubHost); err != nil {
		return nil, nil, err
	}

	ref, err := GitGetCommit(repoDir)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("out: parsed ref %q", ref)

	//
	// Finally, post the status to GitHub.
	//
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

	apiServer := fmt.Sprintf("https://api.%s", githubHost)

	status := github.NewStatus(apiServer, token, owner, repo, context)

	atc := env.Get("ATC_EXTERNAL_URL")
	team := env.Get("BUILD_TEAM_NAME")
	buildN := env.Get("BUILD_NAME")
	instanceVars := env.Get("BUILD_PIPELINE_INSTANCE_VARS")
	targetURL := ghTargetURL(atc, team, pipeline, job, buildN, instanceVars)
	description := "Build " + buildN
	log.Debugf(`out: posting:
  state: %v
  owner: %v
  repo: %v
  ref: %v
  context: %v
  targetURL: %v
  description: %v`,
		state, owner, repo, ref, context, targetURL, description)

	err = status.Add(ref, state, targetURL, description)
	if err != nil {
		return nil, nil, err
	}
	log.Infof("out: GitHub state %s for ref %s posted successfully", state, ref[0:9])

	metadata := oc.Metadata{}
	metadata = append(metadata, oc.NameVal{Name: "state", Value: state})

	return dummyVersion, metadata, nil
}

// ghTargetURL builds an URL suitable to be used as the target_url parameter for the
// Github commit status API.
func ghTargetURL(atc, team, pipeline, job, buildN, instanceVars string) string {
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/hello/builds/25

	targetURL := atc +
		path.Join("/teams", team, "pipelines", pipeline, "jobs", job, "builds", buildN)
	// targetURL := fmt.Sprintf("%s/teams/%s/pipelines/%s/jobs/%s/builds/%s",
	// 	atc, team, pipeline, job, buildN)

	// BUILD_PIPELINE_INSTANCE_VARS: {"branch":"stable"}
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/autocat/builds/3?vars=%7B%22branch%22%3A%22stable%22%7D
	if instanceVars != "" {
		targetURL = fmt.Sprintf("%s?vars=%s", targetURL, url.QueryEscape(instanceVars))
	}

	return targetURL
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

func validateSource(source oc.Source) error {
	// Any missing source key?
	missing := make([]string, 0, len(mandatorySourceKeys))
	for key := range mandatorySourceKeys {
		if _, ok := source[key].(string); !ok {
			missing = append(missing, key)
		}
	}

	// Any unknown source key?
	unknown := make([]string, 0, len(source))
	for key := range source {
		_, ok1 := mandatorySourceKeys[key]
		_, ok2 := optionalSourceKeys[key]
		if !ok1 && !ok2 {
			unknown = append(unknown, key)
		}
	}

	errMsg := make([]string, 0, 2)
	if len(missing) > 0 {
		sort.Strings(missing)
		errMsg = append(errMsg, fmt.Sprintf("missing source keys: %s", missing))
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		errMsg = append(errMsg, fmt.Sprintf("unknown source keys: %s", unknown))
	}
	if len(errMsg) > 0 {
		return errors.New(strings.Join(errMsg, "; "))
	}

	return nil
}

func validateOutParams(params oc.Params) error {
	// Any missing parameter?
	for wantParam := range outMandatoryParams {
		if _, ok := params[wantParam].(string); !ok {
			return fmt.Errorf("missing put parameter '%s'", wantParam)
		}
	}

	// Any invalid parameter?
	state, _ := params["state"].(string)
	if _, ok := outValidStates[state]; !ok {
		return fmt.Errorf("invalid put parameter 'state: %s'", state)
	}

	// Any unknown parameter?
	for param := range params {
		_, ok1 := outMandatoryParams[param]
		_, ok2 := outOptionalParams[param]
		if !ok1 && !ok2 {
			return fmt.Errorf("unknown put parameter '%s'", param)
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

// checkRepoDir validates whether DIR, assumed to be received as input of a put step,
// contains a git repository usable with the Cogito source configuration:
// - DIR is indeed a git repository.
// - The repo configuration contains a "remote origin" section.
// - The remote origin url can be parsed following the Github conventions.
// - The result of the parse matches OWNER and REPO.
func checkRepoDir(dir, owner, repo, githubHost string) error {
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
	url := cfg.StringFromSection(section, key, "")
	if url == "" {
		return fmt.Errorf(".git/config: key [%s]/%s: not found", section, key)
	}
	gu, err := parseGitPseudoURL(url)
	if err != nil {
		return fmt.Errorf(".git/config: remote: %w", err)
	}

	left := []string{githubHost, owner, repo}
	right := []string{gu.URL.Host, gu.Owner, gu.Repo}
	for i, l := range left {
		r := right[i]
		if !strings.EqualFold(l, r) {
			return fmt.Errorf(`the received git repository is incompatible with the Cogito configuration.

Git repository configuration (received as 'inputs:' in this PUT step):
      url: %s
    owner: %s
     repo: %s

Cogito SOURCE configuration:
    owner: %s
     repo: %s`,
				url, gu.Owner, gu.Repo,
				owner, repo)
		}
	}
	return nil
}

type gitURL struct {
	URL   *url.URL
	Owner string
	Repo  string
}

// parseGitPseudoURL attempts to parse rawURL as a git remote URL compatible with the
// Github naming conventions.
//
// It supports the following types of git pseudo URLs:
// - ssh:   git@github.com:Pix4D/cogito.git; will be rewritten to the valid URL
//          ssh://git@github.com/Pix4D/cogito.git
// - https: https://github.com/Pix4D/cogito.git
// - http:  http://github.com/Pix4D/cogito.git
func parseGitPseudoURL(rawURL string) (gitURL, error) {
	workURL := rawURL
	// If ssh pseudo URL, we need to massage the rawURL ourselves :-(
	if strings.HasPrefix(workURL, "git@") {
		if strings.Count(workURL, ":") != 1 {
			return gitURL{}, fmt.Errorf("invalid git SSH URL %s: want exactly one ':'", rawURL)
		}
		// Make the URL a real URL, ready to be parsed. For example:
		// git@github.com:Pix4D/cogito.git -> ssh://git@github.com/Pix4D/cogito.git
		workURL = "ssh://" + strings.Replace(workURL, ":", "/", 1)
	}

	anyUrl, err := url.Parse(workURL)
	if err != nil {
		return gitURL{}, err
	}

	scheme := anyUrl.Scheme
	if scheme == "" {
		return gitURL{}, fmt.Errorf("invalid git URL %s: missing scheme", rawURL)
	}
	if scheme != "ssh" && scheme != "http" && scheme != "https" {
		return gitURL{}, fmt.Errorf("invalid git URL %s: invalid scheme: %s", rawURL, scheme)
	}

	// Further parse the path component of the URL to see if it complies with the Github
	// naming conventions.
	// Example of compliant path: github.com/Pix4D/cogito.git
	tokens := strings.Split(anyUrl.Path, "/")
	if have, want := len(tokens), 3; have != want {
		return gitURL{},
			fmt.Errorf("invalid git URL: path: want: %d components; have: %d %s",
				want, have, tokens)
	}

	// All OK. Fill our gitURL struct
	gu := gitURL{
		URL:   anyUrl,
		Owner: tokens[1],
		Repo:  strings.TrimSuffix(tokens[2], ".git"),
	}
	return gu, nil
}

// GitGetCommit looks into a git repository and extracts the commit SHA of the HEAD.
func GitGetCommit(repoPath string) (string, error) {
	dotGitPath := filepath.Join(repoPath, ".git")

	headPath := filepath.Join(dotGitPath, "HEAD")
	headBuf, err := os.ReadFile(headPath)
	if err != nil {
		return "", fmt.Errorf("git commit: read HEAD: %w", err)
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
		shaBuf, err := os.ReadFile(shaPath)
		if err != nil {
			return "", fmt.Errorf("git commit: branch checkout: read SHA file: %w", err)
		}
		sha = strings.TrimSuffix(string(shaBuf), "\n")
	default:
		return "", fmt.Errorf("git commit: invalid HEAD format: %q", head)
	}

	// Minimal validation that the file contents look like a 40-digit SHA.
	const shaLen = 40
	if len(sha) != shaLen {
		return "", fmt.Errorf("git commit: SHA %s: have len of %d; want %d", sha, len(sha), shaLen)
	}

	return sha, nil
}
