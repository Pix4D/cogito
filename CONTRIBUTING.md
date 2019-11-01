# Contributing and developing

## Contributing

Contributions following the minimalist spirit of this project are welcome.

Please, before opening a PR, open a ticket to discuss your use case. This allows to better understand the _why_ of a new feature and not to waste your time (and mine) developing a feature that for some reason doesn't fit well with the spirit of the library or could be implemented differently. This is in the spirit of [Talk, then code](https://dave.cheney.net/2019/02/18/talk-then-code).

I care about code quality, readability and tests, so please follow the current style and provide adequate test coverage. In case of doubts about how to tackle testing something, feel free to ask.

## Development Prerequisites

### Required

* golang, version >= 1.13
* docker, version >= 19
* [Task], version >= 2.7

### Optional

* [envchain] to securely store secrets for end-to-end tests.
* [gotestsum], for more human-friendly test output. If found in `$PATH`, it will be used in place of `go test`.

## Using Task (replacement of make)

Have a look at [Task], then

```console
task --list
```

## Running the default tests

The Taskfile includes a `test` target, and tests are also run inside the Docker build.

Run the default tests:

```console
task test
```

## The end-to-end tests

The end-to-end tests (tests that interact with GitHub) are disabled by default because they require some out of band setup, explained below.

We require environment variables (as opposed to using a configuration file) to pass test configuration. The reason is twofold:

* To enable any contributor to run their own tests without having to edit any file.
* To securely store secrets!

### Default test repositories

* https://github.com/Pix4D/cogito-test-read-write
* https://github.com/Pix4D/cogito-test-read-only

### Secure handling of the GitHub OAuth token

We use the [envchain] tool, that stores secrets in the OS secure store (Keychain for macOS, D-Bus secret service (gnome-keyring) for Linux), associated to a _namespace_. It then makes available all secrets associated to the given _namespace_ as environment variables.

Add the GitHub OAuth token:

```console
envchain --set cogito COGITO_TEST_OAUTH_TOKEN
```

### Secure handling of the DockerHub token

**NOTE**: You need to follow this section only if you want to fork this repository; if you only want to provide a PR you don't strictly need this part (although in this case you will have to push by hand the docker image to use in your tests, so maybe it is time well spent anyway :-).

Do now use your DockerHub password, instead create a dedicated access token, see documentation at [dockerhub access tokens](https://docs.docker.com/docker-hub/access-tokens/). This allows to:

1. Reduce exposure (principle of least privilege), since a token has less capabilities than an account password.
2. Enable auditing of token usage.
3. Enable token revocation.

Unfortunately it is not possible to limit the scope of a token to a given image repository: a token has access to all repositories of an account. Nonetheless, it still makes sense to use a separate token per image repository, since it enables better auditing.

Login to your account and go to Settings | Security. Create a token, give it a name such as `Travis Project Cogito` and securely back it up in your OS key store.

From an API point of view, the token can be used with `docker login` as if it was a password.

Add the GitHub configuration:

```text
# This is used for docker login
$ envchain --set cogito DOCKER_USERNAME
# This is used to tag the image; in case of doubt use the same value of DOCKER_USERNAME
$ envchain --set cogito DOCKER_ORG
$ envchain --set cogito DOCKER_TOKEN
```

See next section (Travis secrets setup) for how to configure this secret with Travis.

### Travis secrets setup

Please read the reference documentation [travis encryption-keys] before continuing.

The main idea is to store the secrets in the source repository (the repository containing the `.travis.yml` file), using the encrypted environment variables feature of Travis.

Note that this feature, for security reasons, does NOT make secure environment variables available to PRs coming from a forked source repository.

The [travis encryption-keys] documentation contains also pointers to the `travis` CLI. For macOS, `brew install travis` just works.

Do not follow the documentation example (`travis encrypt SOMEVAR="secretvalue"`) because it would leave the secrets in the shell history. Instead, run the tool in interactive mode with the `-i` flag:

```console
$ cd the-repo
$ travis encrypt --add -i
Detected repository as marco-m/travis-go-dockerhub, is this correct? |yes|
Reading from stdin, press Ctrl+D when done
DOCKER_TOKEN="YOUR_TOKEN"  <= this is a real secret
THE_SECRET="42"            <= this shows how to pass additional secrets; see the tests
```

Add the output string to the `env` dictionary of the `.travis.yml` file.

### Prepare the test repository

1. In your GitHub account, create a test repository, say for example `cogito-test`.
2. In the test repository, push at least one commit, on any branch you fancy. Take note of the 40 digits commit SHA (the API wants the full SHA).

### Add test repository information as environment variables

```console
envchain --set cogito COGITO_TEST_REPO_OWNER   # Your GitHub user or organization
envchain --set cogito COGITO_TEST_REPO_NAME    # The repo name (without the trailing .git)
envchain --set cogito COGITO_TEST_COMMIT_SHA
```

#### Verify your setup

```text
envchain cogito env | grep COGITO_
```

should show the following environment variables, with the correct values:

```text
COGITO_TEST_OAUTH_TOKEN
COGITO_TEST_REPO_OWNER
COGITO_TEST_REPO_NAME
COGITO_TEST_COMMIT_SHA
```

And

```text
envchain cogito env | grep DOCKER_
```

should show the following environment variables, with the correct values:

```text
DOCKER_TOKEN
DOCKER_USERNAME
DOCKER_ORG
```

### Optional read-only repository

There are some failure modes that are testable only with an additional repository, for which the user that issues the OAuth token must have read-only access to it.

To run the corresponding tests, you need to export the following environment variables:

```text
COGITO_TEST_READ_ONLY_REPO_NAME
COGITO_TEST_READ_ONLY_COMMIT_SHA
```

### Running the end-to-end tests

We are finally ready to run also the end-to-end tests:

```console
envchain cogito task test
```

The end-to-end tests have the following logic:

* If none of the environment variables are set, we skip the test.
* If all of the environment variables are set, we run the test.
* If some of the environment variables are set and some not, we fail the test. We do this on purpose to signal to the user that the environment variables are misconfigured.

### Making the environment variables available to your editor

If you want to run the tests from within your editor test runner, it is enough to use `envchain` to start the editor:

```console
envchain cogito $EDITOR
```

## Building and publishing the image

The Taskfile includes targets for building and publishing the docker image.

### All-in-one, using the same script as CI

**WARNING**: If you are working on a commit that has a tag, using the CI script will also have an effect on the published Docker image tag. Double-check what you are doing.

```console
$ ci/travis.sh
```

### Step-by-step

Simply have a look at the contents of `ci/travis.sh` and run each step there manually.

Run the tests

```text
envchain cogito task test
```

Build the Docker image

```text
envchain cogito task docker-build
```

Run simple smoke test of the image

```text
envchain cogito task docker-smoke
```

Push the Docker image. This will always generate a Docker image with a tag corresponding to the branch name. If the tip of the branch has a git tag (for example `v1.2.3`), this will also generate a Docker image with that tag (for example `1.2.3`).

```text
envchain cogito task docker-psuh
```

## Suggestions for quick iterations during development

These suggestions apply to the development of any Concourse resource.

After the local tests are passing, the quickest way to test in a pipeline the freshly pushed version of the Docker image is to use the `fly check-resource-type` command. It is faster and less resource-hungry than using a short `check_interval` setting in the pipeline.

For example, assuming that the test pipeline is called `cogito-test`, that the resource in the pipeline is called `cogito` and that there is a job called `autocat` (all this is true by using the sample pipeline [pipelines/cogito.yml](./pipelines/cogito.yml)), you can do:

```text
envchain cogito task docker-build &&
envchain cogito task docker-push &&
fly -t devs check-resource-type -r cogito-test/cogito &&
sleep 5 &&
fly -t devs trigger-job -j cogito-test/autocat -w
```

On each `put` and `get` step, the cogito resource will print its version, git commit SHA and build date to help validate which version a given build is using:

```text
This is the Cogito GitHub status resource. Tag: latest, commit: 91f64c0, date: 2019-10-09
```

## License

This code is licensed according to the MIT license (see file [LICENSE](./LICENSE)).

[Task]: https://taskfile.dev/
[gotestsum]: https://github.com/gotestyourself/gotestsum
[envchain]: https://github.com/sorah/envchain
[travis encryption-keys]: https://docs.travis-ci.com/user/encryption-keys/
