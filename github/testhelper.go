package github

import (
	"os"
	"testing"
)

// TestCfgE2E is a test configuration to run E2E tests.
type TestCfgE2E struct {
	Token string
	Owner string
	Repo  string
	SHA   string
}

// SkipTestIfNoEnvVars is used to decide wether to run an end-to-end test or not.
// The decision is based on the presence or absence of environment variables detailed
// in the README file.
// Requiring the testing.T parameter is done on purpose to combat the temptation to use this
// function in production :-)
func SkipTestIfNoEnvVars(t *testing.T) TestCfgE2E {
	token := os.Getenv("COGITO_TEST_OAUTH_TOKEN")
	owner := os.Getenv("COGITO_TEST_REPO_OWNER")
	repo := os.Getenv("COGITO_TEST_REPO_NAME")
	SHA := os.Getenv("COGITO_TEST_COMMIT_SHA")

	// If none of the environment variables are set, we skip the test.
	if len(token) == 0 && len(owner) == 0 && len(repo) == 0 && len(SHA) == 0 {
		t.Skip("Skipping end-to-end test. See CONTRIBUTING for how to enable.")
	}
	// If some of the environment variables are set and some not, we fail the test.
	if len(token) == 0 || len(owner) == 0 || len(repo) == 0 || len(SHA) == 0 {
		t.Fatal("Some end-to-end env vars are set and some not. See CONTRIBUTING for how to fix.")
	}

	return TestCfgE2E{token, owner, repo, SHA}
}
