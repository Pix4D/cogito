package cogito

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/sasbury/mini"
)

// Put implements the "put" step (the "out" executable).
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-out:
//
// The out script is passed a path to the directory containing the build's full set of
// sources as command line argument $1, and is given on stdin the configured params and
// the resource's source configuration.
//
// The script must emit the resulting version of the resource.
//
// Additionally, the script may emit metadata as a list of key-value pairs. This data is
// intended for public consumption and will make it upstream, intended to be shown on the
// build's page.
func Put(log hclog.Logger, in io.Reader, out io.Writer, args []string) error {
	var pi PutInput
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pi); err != nil {
		return fmt.Errorf("put: parsing JSON from stdin: %s", err)
	}
	pi.Env.Fill()

	if err := pi.Source.ValidateLog(); err != nil {
		return fmt.Errorf("put: %s", err)
	}
	log = log.Named("put")
	log.SetLevel(hclog.LevelFromString(pi.Source.LogLevel))

	log.Debug("started",
		"source", pi.Source,
		"params", pi.Params,
		"environment", pi.Env,
		"args", args)

	if err := pi.Source.Validate(); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	// args[0] contains the path to a directory containing all the "put inputs".
	if len(args) == 0 {
		return fmt.Errorf("put: arguments: missing input directory")
	}
	inputDir := args[0]
	log.Debug("", "input-directory", inputDir)

	buildState := pi.Params.State
	if err := buildState.Validate(); err != nil {
		return fmt.Errorf("put: params: %s", err)
	}
	log.Debug("", "state", buildState)

	gitHash, err := processInputDir(inputDir, pi.Source.Owner, pi.Source.Repo)
	if err != nil {
		return fmt.Errorf("put: processing the input dir: %s", err)
	}
	log.Debug("", "git-commit", gitHash)

	// Following the protocol for put, we return the version and metadata.
	// For Cogito, the metadata contains the Concourse build state.
	output := Output{
		Version:  DummyVersion,
		Metadata: []Metadata{{Name: KeyState, Value: string(buildState)}},
	}
	enc := json.NewEncoder(out)
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	log.Debug("success", "output", output)
	return nil
}

// processInputDir checks whether inputDir, containing the "put inputs", conforms to
// what we expect and returns the git commit hash of the repo passed as put input.
func processInputDir(inputDir string, owner string, repo string) (string, error) {
	inputDirs, err := collectInputDirs(inputDir)
	if err != nil {
		return "", err
	}
	if len(inputDirs) != 1 {
		return "", fmt.Errorf(
			"found %d input dirs: %v. Want exactly 1, corresponding to the GitHub repo %s/%s",
			len(inputDirs), inputDirs, owner, repo)
	}

	// Since we require inputDir to contain only one directory, we assume that this
	// directory is the git repo.
	repoDir := filepath.Join(inputDir, inputDirs[0])
	if err := checkGitRepoDir(repoDir, owner, repo); err != nil {
		return "", err
	}

	gitHash, err := getGitCommit(repoDir)
	if err != nil {
		return "", err
	}

	return gitHash, nil
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
