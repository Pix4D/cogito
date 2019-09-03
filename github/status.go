// Package github implements the GitHub status API, following the GitHub REST API v3.
//
// See the README file for additional information, caveats about GitHub API and imposed limits,
// and reference to official documentation.

package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"time"
)

const API = "https://api.github.com"

type status struct {
	server  string
	token   string
	owner   string
	repo    string
	context string
}

// NewStatus returns a status object associated to a specific GitHub owner and repo.
// Parameter token is the personal OAuth token for owner. It only needs the repo:status scope.
// Parameter context is what created the status, for example "JOBNAME", or "PIPELINENAME/JOBNAME".
// See also:
// * https://developer.github.com/v3/repos/statuses/
// * README file
func NewStatus(server, token, owner, repo, context string) status {
	return status{server, token, owner, repo, context}
}

// Add adds state to the given sha, decorating it with target_url and optional description.
// Parameter sha is the 40 hexadecimal digit sha associated to the commit to decorate.
// Parameter state is one of error, failure, pending, success.
// Parameter target_url (optional) points to the specific process (for example, a CI build)
// that generated this state.
// Parameter description (optional) gives more information about the status.
// The returned error contains plenty of diagnostic information to throubleshoot.
func (s status) Add(sha, state, target_url, description string) error {
	// API: POST /repos/:owner/:repo/statuses/:sha
	url := s.server + path.Join("/repos", s.owner, s.repo, "statuses", sha)

	// Field names must be uppercase (that is, exported) for the JSON encoder to consider
	// them, but the GitHub API wants lowercase names, so we use the optional `json:...`
	// tag to override the case.
	data := struct {
		State       string `json:"state"`
		Target_url  string `json:"target_url"`
		Description string `json:"description"`
		Context     string `json:"context"`
	}{state, target_url, description, s.context}

	json, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON encode: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(json))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Authorization", "token "+s.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// By default there is no timeout, so the call could hang forever.
	client := &http.Client{Timeout: time.Second * 30}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http client Do: %w", err)
	}
	defer resp.Body.Close()

	// If the HTTP response status is not what the API returns for success, return an
	// error. Read also the body to provide some more diagnostic to the caller.
	if resp.StatusCode != http.StatusCreated {
		data, err := ioutil.ReadAll(resp.Body)
		// If there is an error while reading the body, the best we can do is to pass what
		// we got.
		msg := fmt.Sprintf("unexpected status (%d).\nDetails: %s", resp.StatusCode, string(data))
		if err != nil {
			// Note that we do not wrap the error in this case, because it would be misleading.
			return fmt.Errorf("%s\nAdditional error while reading the body: %s.", msg, err)
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}
