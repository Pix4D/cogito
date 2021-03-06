package github

import (
	"os"
	"testing"
)

// TestCfg is a test configuration.
type TestCfg struct {
	Token string
	Owner string
	Repo  string
	SHA   string
}

// FakeTestCfg is a fake test configuration that can be used in some tests that need
// configuration but don't really use any external service.
var FakeTestCfg = TestCfg{
	Token: "fakeToken",
	Owner: "fakeOwner",
	Repo:  "fakeRepo",
	SHA:   "0123456789012345678901234567890123456789",
}

// SkipTestIfNoEnvVars is used to decide wether to run an end-to-end test or not.
// The decision is based on the presence or absence of environment variables detailed
// in the README file.
// Requiring the testing.T parameter is done on purpose to combat the temptation to use this
// function in production :-)
func SkipTestIfNoEnvVars(t *testing.T) TestCfg {
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

	return TestCfg{token, owner, repo, SHA}
}
