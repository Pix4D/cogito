package cogito

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/sasbury/mini"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/sets"
)

// ProdPutter is an implementation of a [Putter] for the Cogito resource.
// Use [NewPutter] to create an instance.
type ProdPutter struct {
	Request  PutRequest
	InputDir string
	// Cogito specific fields.
	log    hclog.Logger
	gitRef string
}

// NewPutter returns a Cogito ProdPutter.
func NewPutter(log hclog.Logger) *ProdPutter {
	return &ProdPutter{
		log: log,
	}
}

func (putter *ProdPutter) LoadConfiguration(input []byte, args []string) error {
	putter.log = putter.log.Named("put")
	putter.log.Debug("started")
	defer putter.log.Debug("finished")

	request, err := NewPutRequest(input)
	if err != nil {
		return err
	}
	putter.Request = request
	putter.log.Debug("parsed put request",
		"source", putter.Request.Source,
		"params", putter.Request.Params,
		"environment", putter.Request.Env,
		"args", args)

	sourceSinks := putter.Request.Source.Sinks
	putParamsSinks := putter.Request.Params.Sinks

	// Validate optional sinks configuration.
	_, err = MergeAndValidateSinks(sourceSinks, putParamsSinks)
	if err != nil {
		return fmt.Errorf("put: arguments: unsupported sink(s): %w", err)
	}

	// args[0] contains the path to a directory containing all the "put inputs".
	if len(args) == 0 {
		return fmt.Errorf("put: concourse resource protocol violation: missing input directory")
	}
	putter.InputDir = args[0]
	putter.log.Debug("", "input-directory", putter.InputDir)
	buildState := putter.Request.Params.State
	putter.log.Debug("", "state", buildState)

	return nil
}

func (putter *ProdPutter) ProcessInputDir() error {
	// putter.InputDir, corresponding to key "put:inputs:", may contain 0, 1 or 2 dirs.
	// If it contains zero, Cogito addresses only a supported chat system (custom sinks configured).
	// If it contains one, it could be the git repo or the directory containing the chat message:
	// in the first case we support autodiscovery by not requiring to name it, we know
	// that it should be the git repo.
	// If on the other hand it contains two, one should be the git repo (still nameless)
	// and the other should be the directory containing the chat_message_file, which is
	// named by the first element of the path in "chat_message_file".
	// This allows (although clumsily) to distinguish which is which.
	// This complexity has historical reasons to preserve backwards compatibility
	// (the nameless git repo).
	//
	// Somehow independent is the reason why we enforce the count of directories to be
	// max 2: this is to avoid the default Concourse behavior of streaming _all_ the
	// volumes "just in case".

	params := putter.Request.Params
	source := putter.Request.Source

	// Get wanted sinks (already validated in LoadConfiguration()).
	sinks, _ := MergeAndValidateSinks(source.Sinks, params.Sinks)

	var msgDir string

	collected, err := collectInputDirs(putter.InputDir)
	if err != nil {
		return err
	}

	inputDirs := sets.From(collected...)

	if params.ChatMessageFile != "" {
		msgDir, _ = path.Split(params.ChatMessageFile)
		msgDir = strings.TrimSuffix(msgDir, "/")
		if msgDir == "" {
			return fmt.Errorf("chat_message_file: wrong format: have: %s, want: path of the form: <dir>/<file>",
				params.ChatMessageFile)
		}

		found := inputDirs.Remove(msgDir)
		if !found {
			return fmt.Errorf("put:inputs: directory for chat_message_file not found: have: %v, chat_message_file: %s",
				collected, params.ChatMessageFile)
		}
	}

	switch inputDirs.Size() {
	case 0:
		// If the size is 0 after removing the directory containing the chat message
		// and Cogito should update the commit status, return an error.
		if sinks.Contains("github") {
			return fmt.Errorf(
				"put:inputs: missing directory for GitHub repo: have: %v, GitHub: %s/%s",
				inputDirs, source.Owner, source.Repo)
		}
		putter.log.Debug("", "inputDirs", inputDirs, "msgDir", msgDir)
	case 1:
		repoDir := filepath.Join(putter.InputDir, inputDirs.OrderedList()[0])
		putter.log.Debug("", "inputDirs", inputDirs, "repoDir", repoDir, "msgDir", msgDir)
		if err := checkGitRepoDir(repoDir, source.GhHostname, source.Owner, source.Repo); err != nil {
			return err
		}
		putter.gitRef, err = getGitCommit(repoDir)
		if err != nil {
			return err
		}
		putter.log.Debug("", "git-ref", putter.gitRef)
	default:
		// If the size exceeds 1, too many directories are passed to Cogito.
		return fmt.Errorf(
			"put:inputs: want only directory for GitHub repo: have: %v, GitHub: %s/%s",
			inputDirs, source.Owner, source.Repo)
	}

	return nil
}

func (putter *ProdPutter) Sinks() []Sinker {
	supportedSinkers := map[string]Sinker{
		"github": GitHubCommitStatusSink{
			Log:     putter.log.Named("ghCommitStatus"),
			GitRef:  putter.gitRef,
			Request: putter.Request,
		},
		"gchat": GoogleChatSink{
			Log: putter.log.Named("gChat"),
			// TODO putter.InputDir itself should be of type fs.FS.
			InputDir: os.DirFS(putter.InputDir),
			GitRef:   putter.gitRef,
			Request:  putter.Request,
		},
	}
	source := putter.Request.Source.Sinks
	params := putter.Request.Params.Sinks
	sinks, _ := MergeAndValidateSinks(source, params)

	sinkers := make([]Sinker, 0, sinks.Size())
	for _, s := range sinks.OrderedList() {
		sinkers = append(sinkers, supportedSinkers[s])
	}

	return sinkers
}

func (putter *ProdPutter) Output(out io.Writer) error {
	// Following the protocol for put, we return the version and metadata.
	// For Cogito, the metadata contains the Concourse build state.
	output := Output{
		Version:  DummyVersion,
		Metadata: []Metadata{{Name: KeyState, Value: string(putter.Request.Params.State)}},
	}
	enc := json.NewEncoder(out)
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	putter.log.Debug("success", "output.version", output.Version,
		"output.metadata", output.Metadata)

	return nil
}

// MergeAndValidateSinks returns an error if the user set an unsupported sink in source or put.params.
// If validation passes, it return the list of sinks to address:
// - return sinks in put.params if found.
// - return sinks in source if found.
// - return all supported sinks.
func MergeAndValidateSinks(sourceSinks []string, paramsSinks []string) (*sets.Set[string], error) {
	sinks := sets.From([]string{"github", "gchat"}...)
	supportedSinks := sinks
	if len(sourceSinks) > 0 {
		sinks = sets.From(sourceSinks...)
	}
	if len(paramsSinks) > 0 {
		sinks = sets.From(paramsSinks...)
	}

	difference := sinks.Difference(supportedSinks)
	if difference.Size() > 0 {
		return nil, fmt.Errorf("%s", difference)
	}
	return sinks, nil
}

// collectInputDirs returns a list of all directories below dir (non-recursive).
func collectInputDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("collecting directories in %v: %w", dir, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// checkGitRepoDir validates whether DIR, assumed to be received as input of a put step,
// contains a git repository usable with the Cogito source configuration:
// - DIR is indeed a git repository.
// - The repo configuration contains a "remote origin" section.
// - The remote origin url can be parsed following the GitHub conventions.
// - The result of the parse matches OWNER and REPO.
func checkGitRepoDir(dir, hostname, owner, repo string) error {
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
	gitUrl := cfg.StringFromSection(section, key, "")
	if gitUrl == "" {
		return fmt.Errorf(".git/config: key [%s]/%s: not found", section, key)
	}
	gu, err := github.ParseGitPseudoURL(gitUrl)
	if err != nil {
		return fmt.Errorf(".git/config: remote: %w", err)
	}
	left := []string{hostname, owner, repo}
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
    hostname: %s
    owner: %s
    repo: %s`,
				gitUrl, gu.Owner, gu.Repo,
				hostname, owner, repo)
		}
	}
	return nil
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

	return sha, nil
}

// concourseBuildURL builds a URL pointing to a specific build of a job in a pipeline.
func concourseBuildURL(env Environment) string {
	// Example:
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/hello/builds/25
	buildURL := env.AtcExternalUrl + path.Join(
		"/teams", env.BuildTeamName,
		"pipelines", env.BuildPipelineName,
		"jobs", env.BuildJobName,
		"builds", env.BuildName)

	// Example:
	// BUILD_PIPELINE_INSTANCE_VARS: {"branch":"stable"}
	// https://ci.example.com/teams/main/pipelines/cogito/jobs/autocat/builds/3?vars=%7B%22branch%22%3A%22stable%22%7D
	if env.BuildPipelineInstanceVars != "" {
		//convert the JSON instance vars into URL parameters
		var instanceVars map[string]interface{}
		if err := json.Unmarshal([]byte(env.BuildPipelineInstanceVars), &instanceVars); err != nil {
			panic(err)
		}
		params := url.Values{}
		for key, value := range instanceVars {
			valueStr := fmt.Sprintf("\"%v\"", value)
			params.Add("vars."+key, valueStr)
		}
		encodedString := params.Encode()
		//concourse wants spaces not + if you have them in a variable
		paramsString := strings.ReplaceAll(encodedString, "+", " ")
		buildURL += fmt.Sprintf("?%s", paramsString)
	}

	return buildURL
}
