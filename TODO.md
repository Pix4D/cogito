TODO:

the tag_filter bug

[ ] search for commit_sha template key (used in ref.temmplate)
[ ] change tests
[ ] change implementation
[ ] remove parseGitRef()
[ ] remove TestParseGitRef()
[ ] mhhh this requires changing all the tests that use the testdata :-(
[ ] maybe it is better to change setup() to take a generic template map as parameter
    the template keys that need to be passed are:
    {{.repo_url}}
    {{.commit_sha}}
    {{.branch_name}}

[X] remove testdata/a-repo/dot.git/ref.template
[X] add dot.git/HEAD   (fixed file name)
        cat .git/HEAD
        ref: refs/heads/fix-git-tag-filter
        ^^^^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^^^
           fixed             branch_name
[X]   add dot.git/refs/heads/fix-git-tag-filter (variable file name :-()
          ^^^^^^^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^^^
           fixed dirs          branch-name
        cat .git/refs/heads/fix-git-tag-filter
        5f0462e38635bfdb3aef7cf2c20d8a7997f02c87
        ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
            template key commit_sha

[X] How do I create the variable file name at CopyDir() time ???
rm -r ~/tmp/foo/* ; task copydir && ./bin/copydir resource/testdata/a-repo ~/tmp/foo --dot --template branch_name=the-branch repo_url=https://github.com/wow/splash.git commit_sha=123  && tree -a ~/tmp/foo

---

[ ]	FIXME all statements of the form		if gotErr != tc.wantErr
    are wrong; they should be if !errors.Is()

[ ]	CopyDir: FIXME longstanding bug: we apply template processing always, also if the file doesn't have the .template suffix!
] refactor helper.CopyDir() to be more composable in the transformers.

[ ] MOST IMPORTANT parametric is fine, but I would like a way to use the testdata also when not running the e2e tests. Is it possible? Let's see:
    [ ] key names should be the same as the documented environment variables, but lowercase to hint that a potential transformation is happening:
    COGITO_TEST_REPO_OWNER -> {{.cogito_test_repo_owner}}
    COGITO_TEST_REPO_NAME  -> {{.cogito_test_repo_name}}
    [ ] I should have stub default values for these keys, for example
    {{.cogito_test_repo_owner}} <- default_repo_owner
    {{.cogito_test_repo_name}}  <- default_repo_name
    these stub values are used when the corresponding env variables are not set; this implies
    that the tests are using mocks instead than e2e
    [ ] fix the fact that I am using url = {{.repo_url}} instead of the individual variables,
        because this makes it possible to use only 1 file template to handle both SSH and HTTPS

    [ ] to completely enable mock I also need to restructure / split the tests
    [ ] i need to carefully ask the question what am I testing? Am I testing cogito Out or am I testing GitHub status API? Or am I testing the full integration?
    [ ] still need a simple, not error-prone way to make the difference between e2e and not. Maybe I could reconsider making it more explicit by putting the e2e in a separate test executable, maybe with compilation guards? I don't know

[ ] use text/template, not html/template!
[ ] open ticket on github.com/alexflint/go-arg, cannot use positional arguments after a slice, and conventional `--` to stop parsing is not respected?

[ ] Add fake version for github tests, using httptest.NewServer as in hmsg
[ ] Add fake version for resource tests, using httptest.NewServer as in hmsg
[ ] Find a way to run also the E2E tests in the Docker build
[ ] How do I test this thing ???? How do I pass the fake server to Out() ??? Need to wrap in two levels or to make the code read an env var :-( or to always run the real test (no fake). I can pass the server via the "source" map. But fro security (avoid exfiltrating the token) I don't accept the server, I accept a flag like test_api_server boolean. If set, the api server will be hardcoded to "localhost" ?
[ ] CopyDir() the renamers can be replaced by a func (string) string

[ ] cogito.yml sample pipeline: is there a trick to add a dependency between the two jobs without having to use S3 or equivalent ? Maybe a resource like semver, but without any external storage / external dependency ???
[ ] When I switched from "repo" to "input-repo" I got a bug because I didn't change all instances of the "repo" key. Two possibilities:
    1. change the strings to constants everywhere
    2. return error if an expected key doesn't exist.
[ ] Taskfile: A clean target would be useful for removing the built docker images.

[ ] prepare to open source it :-)
    [ ] in README explain that there is no public docker image OR take a decision if we want to provide it. It would be way better.
    [ ] completely paramterize the pipeline, to be used also outside pix4d
    [ ] add screenshots to the README to explain what is the context, the target_url and the description.

[ ] move the testhelper to its own repo
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
[ ] is there a newline or not in the gitref, when a tag is present?
[ ] would it make sense to add error logging to sentry ?

[ ] rename package resource to package cogito !!!
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
