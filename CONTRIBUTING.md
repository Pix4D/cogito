# Contributing and developing

Contributions following the minimalist spirit of this project are welcome.

**Please, before opening a PR, open a ticket to discuss your use case**.
This allows to better understand the _why_ of a new feature and not to waste your time (and ours) developing a feature that for some reason doesn't fit well with the spirit of the project or could be implemented differently.
This is in the spirit of [Talk, then code](https://dave.cheney.net/2019/02/18/talk-then-code).

We care about code quality, readability and tests, so please follow the current style and provide adequate test coverage.
In case of doubts about how to tackle testing something, feel free to ask.

# Development Prerequisites

## Required

* Go, version >= 1.16
* Docker, version >= 20
* [Task], version >= 3.7

## Optional

* [gopass] to securely store secrets for integration tests.
* [gotestsum], for more human-friendly test output. If found in `$PATH`, it will be used in place of `go test`.

# Using Task (replacement of make)

Have a look at the [task documentation][Task], then run:

```
$ task --list
```

# Running the default tests

The Taskfile includes a `test:unit` target, and tests are also run inside the Docker build.

Run the default tests:

```
$ task test:unit
```

# Integration tests

There are two types of integration tests:

* tests against GitHub
* tests the Docker image as resource inside Concourse

# Integration tests against GitHub

The integration tests (tests that interact with GitHub) are disabled by default because they require some out of band setup, explained below.

We require environment variables (as opposed to using a configuration file) to pass test configuration. The reason is twofold:

* To enable any contributor to run their own tests without having to edit any file.
* To securely store secrets!

The following sections contain first instructions for the setup, then instructions on how to run the tests.

## Default test repository

* https://github.com/Pix4D/cogito-test-read-write

## Secure handling of the GitHub OAuth token

We use the [gopass] tool, that stores secrets in the file system using GPG. We then make the secrets available as environment variables in [Taskfile.yml](Taskfile.yml).

Add the GitHub OAuth token:

```console
$ gopass insert cogito/test_oauth_token
```

## Secure handling of the DockerHub token

**NOTE**: You need to follow this section only if you want to fork this repository; if you only want to provide a PR you don't strictly need this part (although in this case you will have to push by hand the docker image to use in your tests, so maybe it is time well spent anyway :-).

Do not use your DockerHub password, instead create a dedicated access token, see documentation at [dockerhub access tokens](https://docs.docker.com/docker-hub/access-tokens/). This allows to:

1. Reduce exposure (principle of least privilege), since a token has less capabilities than an account password.
2. Enable auditing of token usage.
3. Enable token revocation.

Unfortunately it is not possible to limit the scope of a token to a given image repository: a token has access to all repositories of an account. Nonetheless, it still makes sense to use a separate token per image repository, since it enables better auditing.

Login to your account and go to Settings | Security. Create a token, give it a name such as `CI Project Cogito` and securely back it up in your OS key store.

From an API point of view, the token can be used with `docker login` as if it was a password.

Add the GitHub configuration:

```console
$ gopass insert cogito/docker_username
$ gopass insert cogito/docker_org
$ gopass insert cogito/docker_token
```

## Prepare the test repository

1. In your GitHub account, create a test repository, say for example `cogito-test`.
2. In the test repository, push at least one commit, on any branch you fancy. Take note of the 40 digits commit SHA (the API wants the full SHA).

## Add test repository information as environment variables

```console
$ gopass insert cogito/test_repo_owner # Your GitHub user or organization
$ gopass insert cogito/test_repo_name  # The repo name (without the trailing .git)
$ gopass insert cogito/test_commit_sha
```

### Verify your setup

```console
$ gopass ls cogito
```

should show:

```text
cogito/
├── docker_org
├── docker_token
├── docker_username
├── test_commit_sha
├── test_oauth_token
├── test_repo_name
└── test_repo_owner
```

## Running the integration tests

We are finally ready to run also the integration tests:

```
$ task test-integration
```

The integration tests have the following logic:

* If none of the environment variables are set, we skip the test.
* If all of the environment variables are set, we run the test.
* If some of the environment variables are set and some not, we fail the test. We do this on purpose to signal to the user that the environment variables are misconfigured.

## Running a specific end-to-end test

Use the `test:env` task target, that runs a shell with available all the secrets needed for the integration tests.

Run all the subtests of a table-driven test:

```
$ task test:env -- go test ./github -count=1 -run 'TestUnderstandGitHubStatusFailures'
```

Run an individual subtest of a table-driven test:

```
$ task test:env -- go test ./github -count=1 -run 'TestUnderstandGitHubStatusFailures/non_existing_SHA:_Unprocessable_Entity'
```

# Building and publishing the image

The Taskfile includes targets for building and publishing the docker image.

## All-in-one, using the same script as CI

**WARNING**: If you are working on a commit that has a tag, using the CI script will also have an effect on the published Docker image tag. Double-check what you are doing.

FIXME: with the move from envchain to gopass, I need to think how to fix this.

```console
$ envchain cogito ci/travis.sh
```

## Step-by-step

Simply have a look at the contents of `ci/travis.sh` and run each step there manually.

Run the tests

```console
$ task test
```

Build the Docker image

```console
$ task docker-build
```

Run simple smoke test of the image

```console
$ task docker-smoke
```

Push the Docker image. This will always generate a Docker image with a tag corresponding to the branch name. If the tip of the branch has a git tag (for example `v1.2.3`), this will also generate a Docker image with that tag (for example `1.2.3`).

```console
$ task docker-push
```

# Integration tests: test the Docker image as resource inside Concourse

Have a look at the sample pipeline in [pipelines/cogito.yml](pipelines/cogito.yml).

You can use my other project [concourse-in-a-box](https://github.com/marco-m/concourse-in-a-box), an all-in-one Concourse CI/CD system based on Docker Compose, with Minio S3-compatible storage and HashiCorp Vault secret manager, to easily test the cogito image.

See also the next section.

# Suggestions for quick iterations during development

These suggestions apply to the development of any Concourse resource.

After the local tests are passing, the quickest way to test in a pipeline the freshly pushed version of the Docker image is to use the `fly check-resource-type` command. It is faster and less resource-hungry than using a short `check_interval` setting in the pipeline.

For example, assuming that the test pipeline is called `cogito-test`, that the resource in the pipeline is called `cogito` and that there is a job called `autocat` (all this is true by using the sample pipeline [pipelines/cogito.yml](./pipelines/cogito.yml)), you can do:

```
$ fly -t cogito login --concourse-url=http://localhost:8080 --open-browser
```

```
$ fly -t cogito set-pipeline -p cogito-test -c pipelines/cogito.yml \
  -y github-owner=(gopass show cogito/test_repo_owner) \
  -y repo-name=(gopass show cogito/test_repo_name) \
  -y oauth-personal-access-token=(gopass show cogito/test_oauth_token) \
  -y tag=(git branch --show-current) \
  -y branch=stable
```

```
$ task docker-build docker-push &&
  fly -t cogito check-resource-type -r cogito-test/cogito &&
  sleep 5 &&
  fly -t cogito trigger-job -j cogito-test/autocat -w
```

On each `put` and `get` step, the cogito resource will print its version, git commit SHA and build date to help validate which version a given build is using:

```text
This is the Cogito GitHub status resource. Tag: latest, commit: 91f64c0, date: 2019-10-09
```

## Testing instanced vars

(Instanced vars)[https://concourse-ci.org/instanced-pipelines.html] is a feature introduced in Concourse 7.4 to group together pipelines generated from the same pipeline configuration file.

With reference to the sample pipeline in [pipelines/cogito.yml](pipelines/cogito.yml), you can use the `((branch))` variable as an instanced var:

Pipeline instance 1:

```
$ fly -t cogito set-pipeline -p cogito-test -c pipelines/cogito.yml \
  -y github-owner=(gopass show cogito/test_repo_owner) \
  -y repo-name=(gopass show cogito/test_repo_name) \
  -y oauth-personal-access-token=(gopass show cogito/test_oauth_token) \
  -y tag=(git branch --show-current) \
  --instance-var branch=stable
```

Pipeline instance 2:

```
$ fly -t cogito set-pipeline -p cogito-test -c pipelines/cogito.yml \
  -y github-owner=(gopass show cogito/test_repo_owner) \
  -y repo-name=(gopass show cogito/test_repo_name) \
  -y oauth-personal-access-token=(gopass show cogito/test_oauth_token) \
  -y tag=(git branch --show-current) \
  --instance-var branch=another-branch
```

## refreshing the resource image when using instanced vars

```
$ task docker-build docker-push &&
    fly -t cogito check-resource-type -r cogito-test/branch:stable/cogito &&
    fly -t cogito check-resource-type -r cogito-test/branch:another-branch/cogito
```

# License

This code is licensed according to the MIT license (see file [LICENSE](./LICENSE)).

[Task]: https://taskfile.dev/
[gotestsum]: https://github.com/gotestyourself/gotestsum
[gopass]: https://github.com/gopasspw/gopass
