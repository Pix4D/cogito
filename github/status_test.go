package github_test

import (
	"testing"

	"github.com/Pix4D/cogito/github"
)

// We skip this test by default (more about end-to-end tests in README).
func TestGitHubStatusE2E(t *testing.T) {
	cfg := github.SkipTestIfNoEnvVars(t)

	context := "cogito/test"
	status := github.NewStatus(github.API, cfg.Token, cfg.Owner, cfg.Repo, context)
	target_url := "https://cogito.invalid/builds/job/42"
	desc := "This is the description"
	state := "pending"

	err := status.Add(cfg.Sha, state, target_url, desc)

	if err != nil {
		t.Fatalf("wanted: no error, got: %v.", err)
	}
}
