// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.
package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pix4D/cogito/github"
	oc "github.com/cloudboss/ofcourse/ofcourse"
)

const (
	accessTokenKey  = "access_token"
	gchatWebhookKey = "gchat_webhook"

	contextKey       = "context"
	contextPrefixKey = "context_prefix"
	ownerKey         = "owner"
	repoKey          = "repo"
	stateKey         = "state"
)

var (
	// States that will trigger a chat notification by default.
	statesToNotifyChat = []string{abortState, errorState, failureState}
)

// Resource satisfies the ofcourse.Resource interface.
type Resource struct {
	githubAPI string
}

// New returns a new Resource object using the default GitHub API endpoint.
func New() *Resource {
	return NewWith(github.API)
}

// NewWith returns a new Resource object with a custom Github API endpoint.
//
// Can be used by tests to "mock" with net/http/httptest:
//   ts := httptest.NewServer(...)
//   defer func() {
// 	     ts.Close()
//   }()
//   res := resource.newWith(ts.URL)
func NewWith(githubAPI string) *Resource {
	return &Resource{
		githubAPI: githubAPI,
	}
}

// Out satisfies ofcourse.Resource.Out(), corresponding to the /opt/resource/out command.
func (r *Resource) Out(
	inputDir string, // All the resource "put inputs" are below this directory.
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
) (oc.Version, oc.Metadata, error) {

	// STUFF DELETED

	gitRef, err := getGitCommit(repoDir)
	if err != nil {
		return nil, nil, err
	}
	log.Debugf("out: parsed ref %q", gitRef)

	//
	// Post the status to all sinks and collect the sinkErrors.
	//
	var sinkErrors = map[string]error{}

	//
	// Post the status to GitHub Commit status sink.
	//
	err = gitHubCommitStatus(source, params, env, log, gitRef, r.githubAPI)
	if err != nil {
		sinkErrors["github commit status"] = err
	}
	//
	// Post the status to chat sink.
	//
	err = sendToChat(source, params, env, log, gitRef)
	if err != nil {
		sinkErrors["google chat"] = err
	}

	// We treat all sinks as equal: it is enough for one to fail to cause the put
	// operation to fail.
	if len(sinkErrors) > 0 {
		return nil, nil, fmt.Errorf("out: %s", stringify(sinkErrors))
	}

	state, _ := params[stateKey].(string)
	metadata := oc.Metadata{}
	metadata = append(metadata, oc.NameVal{Name: stateKey, Value: state})

	return dummyVersion, metadata, nil
}

// getGitCommit looks into a git repository and extracts the commit SHA of the HEAD.
func getGitCommit(repoPath string) (string, error) {
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
