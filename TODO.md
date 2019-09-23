TODO:

[ ] tag our first release! 0.1.0
[ ] When we start using this resource, pin it in all pipelines. Do not use latest because it is still in flux!
[ ] bring back better parametric e2e testing!
[ ] cogito.yml sample pipeline: is there a trick to add a dependency between the two jobs without having to use S3 or equivalent ? Maybe a resource like semver, but without any external storage / external dependency ???
[ ] README when referring to other sections, use anchors.
[ ] When I switched from "repo" to "input-repo" I got a bug because I didn't change all instances of the "repo" key. Two possibilities:
    1. change the strings to constants everywhere
    2. return error if an expected key doesn't exist.
[ ] Taskfile: A clean target would be useful for removing the built docker images.

[ ] prepare to open source it :-)
    [ ] add "Contributing" section; follow the hmsg README.
    [ ] in README explain that there is no public docker image OR take a decision if we want to provide it. It would be way better.
    [ ] completely paramterize the pipeline, to be used also outside pix4d
    [ ] add screenshots to the README to explain what is the context, the target_url and the description.

[ ] reduce docker image size
    [ ] do we gain anything from deleting some of the packages:
        apk del libc-utils alpine-baselayout alpine-keys busybox apk-tools
    [ ] why among the dependecies, ofcourse wants yaml ? The resource doesn't need it, it gets a JSON object. That yaml is problematic because it seems to be the one that requires gcc. If I can get rid of it, I can maybe reach a smaller image ?
    [ ] is there a way to use the busybox image (smaller) and bring on the certificates? Maybe I can use alpine to build, install the certs with apk, then copy the certs directory over to a busybox? Maybe, since this resource speaks only to github, I can even copy over only the cert of the CA of github ?

[ ] better docker experience
    [ ] adding the go ldflags to the dockerfile as I did is wrong; now I rebuild way too often because docker detects that variables such as build time or commit hash have changed and decides to reinstall the packages!!! Fix this.

[ ] replace all our usages of the other resources with this one
[ ] probably I can replace reflect.TypeOf(err) with the new errors.Is()
[ ] remove or update hmsg
[ ] Add fake version for github tests, using httptest.NewServer as in hmsg
[ ] Add fake version for resource tests, using httptest.NewServer as in hmsg
[ ] Find a way to run also the E2E tests in the Docker build
[ ] How do I test this thing ???? How do I pass the fake server to Out() ??? Need to wrap in two levels or to make the code read an env var :-( or to always run the real test (no fake). I can pass the server via the "source" map. But fro security (avoid exfiltrating the token) I don't accept the server, I accept a flag like test_api_server boolean. If set, the api server will be hardcoded to "localhost" ?
[ ] is there a newline or not in the gitref, when a tag is present?

[ ] rename package resource to package cogito !!!
[ ] move packages below pkg/
[ ] package github: provide custom user agent (required by GH)
[ ]	How to parse .git/ref (created by the git resource) when it contains also a tag?
    .git/ref: Version reference detected and checked out. It will usually contain the commit SHA
    ref, but also the detected tag name when using tag_filter.

[ ] investigate if this is a bug in path.Join() and open ticket if yes
	  // it adds a 3rd slash "/": Post https:///api.github.c ...
	  // API: POST /repos/:owner/:repo/statuses/:sha
    // try also with and without the beginning / for "repos"
	  url := path.Join(s.server, "repos", s.owner, s.repo, "statuses", sha)

[ ] package resource: add more tests for TestIn
[ ] package resource: add more tests for TestCheck

[ ] package resource: is there something cleaner than this "struct{}{}" thing ?
	mandatorySources = map[string]struct{}{
		"owner_repo":   struct{}{},
		"access_token": struct{}{},

[ ] package resource: TestPut:
	find a way to test missing repo dir due to `input:` pipeline misconfiguration
[ ] package resource: TestPut:
	find a way to test mismatch between input: and repo:

[ ] package github: is it possible to return information about current rate limiting, to aid
    in throubleshooting?
[ ] package github: is it possible to detect abuse rate limiting and report it, to help throubleshooting? On the other hand, this is already visible in the error message ...

[ ] extract the userid from the commit :-D and make it available optionally
[ ] add test TestGitHubStatusFake
  Use the http.testing API
  func TestGitHubStatusFake(t *testing.T) {
  	fakeAPI := "http://localhost:8888"
  	repoStatus := github.NewStatus(fakeAPI, ...)
  }

[ ] add to TestGitHubStatusE2E(t *testing.T)
  Query the API to validate that the status has been added! But to do this, I need a unique text in the description, maybe I can just store the timestamp or generate an UUID and keep it in memory?

[ ] Currently we validate that state is one of the valid values in the resource itself.
  Decide what do to among the following:
  - leave it there
  - move it to the github package
  - remove it completely, since GitHub will validate in any case
  Rationale:
  - since the final validation is done anycase in GitHub, what is the point of adding more code to have in any case a partial validation?
  - not validating allows to stay open: if tomorrow github adds another valid state, the resoulce will still work and support the new state withouh requiring a change (yes, not very probable, but still the reasoning make sense, no?)

[] A defect of go test, which wants to be too terse by default. What it should do in my opinion is yes to stay terse, but to print a summary line at the end with the number of skipped tests, so one can spot unexpected things. To actually see the skip message, you need to run the tests with the -v flag (verbose), but then you get a lot of stuff.
   - is there a way to show the number of skipped tests in a summary line at the end ?
   - Maybe I could add test-verbose to the Taskfile ?
   - maybe I could use the https://github.com/gotestyourself/gotestsum ?
