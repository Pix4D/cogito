package cogito

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Pix4D/cogito/sets"
	"github.com/hashicorp/go-hclog"
	"github.com/sasbury/mini"
)

// ProdPutter is an implementation of a [Putter] for the Cogito resource.
// Use [NewPutter] to create an instance.
type ProdPutter struct {
	Request  PutRequest
	InputDir string
	// Cogito specific fields.
	ghAPI  string
	log    hclog.Logger
	gitRef string
}

// NewPutter returns a Cogito ProdPutter.
func NewPutter(ghAPI string, log hclog.Logger) *ProdPutter {
	return &ProdPutter{
		ghAPI: ghAPI,
		log:   log,
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

	// args[0] contains the path to a directory containing all the "put inputs".
	if len(args) == 0 {
		return fmt.Errorf("put: arguments: missing input directory")
	}
	putter.InputDir = args[0]
	putter.log.Debug("", "input-directory", putter.InputDir)

	buildState := putter.Request.Params.State
	putter.log.Debug("", "state", buildState)

	return nil
}

func (putter *ProdPutter) ProcessInputDir() error {
	// putter.InputDir, corresponding to key "put:inputs:", should contain 0, 1 or 2 dirs.
	// If it contains zero, aka undefined, Cogito will only send to chat.
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

	// If the size is 0 after removing the directory containing the chat message, this will be a chat only put.
	if inputDirs.Size() == 0 {
		putter.log.Debug("No GitHub repositories in inputs, Cogito will only send to chat")
		return nil
	} else if inputDirs.Size() > 1 {
		return fmt.Errorf(
			"put:inputs: want only directory for GitHub repo: have: %v, GitHub: %s/%s",
			inputDirs, source.Owner, source.Repo)
	}

	// The set has one or two elements. if it exists, remove from the set the message
	// directory. The remaining one is the git repo.
	putter.log.Debug("", "inputDirs", inputDirs, "msgDir", msgDir)
	remaining := inputDirs.Difference(sets.From(msgDir))
	repoDir := filepath.Join(putter.InputDir, remaining.OrderedList()[0])

	if err := checkGitRepoDir(repoDir, source.Owner, source.Repo); err != nil {
		return err
	}

	putter.gitRef, err = getGitCommit(repoDir)
	if err != nil {
		return err
	}
	putter.log.Debug("", "git-ref", putter.gitRef)

	return nil
}

func (putter *ProdPutter) Sinks() ([]Sinker, error) {
	var err error
	supportedSinks := map[string]Sinker{
		"github": GitHubCommitStatusSink{
			Log:     putter.log.Named("ghCommitStatus"),
			GhAPI:   putter.ghAPI,
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

	sinksParams := putter.Request.Params.Sinks
	if len(sinksParams) == 0 {
		// No sink specified, we default to github and ghcat for backward compatibility.
		sinksParams = []string{"github", "gchat"}
	}
	sinkers := make([]Sinker, 0, len(sinksParams))
	for _, s := range sinksParams {
		// Check configured sink is in supported list.
		sinker, ok := supportedSinks[s]
		if !ok {
			err = fmt.Errorf("unsupported sink: %s", s)
		} else {
			sinkers = append(sinkers, sinker)
		}
	}

	return sinkers, err
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
func checkGitRepoDir(dir, owner, repo string) error {
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
	gu, err := parseGitPseudoURL(gitUrl)
	if err != nil {
		return fmt.Errorf(".git/config: remote: %w", err)
	}
	left := []string{"github.com", owner, repo}
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
				gitUrl, gu.Owner, gu.Repo,
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

// safeUrlParse wraps [url.Parse] and returns only the error and not the URL to avoid leaking
// passwords of the form http://user:password@example.com
//
// From https://github.com/golang/go/issues/53993
func safeUrlParse(rawURL string) (*url.URL, error) {
	parsedUrl, err := url.Parse(rawURL)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			// url.Parse returns a wrapped error that contains also the URL.
			// Instead, we return only the error.
			return nil, uerr.Err
		}
		return nil, errors.New("invalid URL")
	}
	return parsedUrl, nil
}

// parseGitPseudoURL attempts to parse rawURL as a git remote URL compatible with the
// Github naming conventions.
//
// It supports the following types of git pseudo URLs:
//   - ssh:   			git@github.com:Pix4D/cogito.git; will be rewritten to the valid URL
//     ssh://git@github.com/Pix4D/cogito.git
//   - https: 			https://github.com/Pix4D/cogito.git
//   - https with u:p: 	https//username:password@github.com/Pix4D/cogito.git
//   - http: 			http://github.com/Pix4D/cogito.git
//   - http with u:p: 	http://username:password@github.com/Pix4D/cogito.git
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

	anyUrl, err := safeUrlParse(workURL)
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

	// Further parse the path component of the URL to see if it complies with the GitHub
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
		buildURL += fmt.Sprintf("?vars=%s", url.QueryEscape(env.BuildPipelineInstanceVars))
	}

	return buildURL
}
