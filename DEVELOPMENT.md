# Development

## Prerequisites

### Required

* golang, version >= 1.13
* docker, version >= 19
* task (https://taskfile.dev/), version >= 2.6

### Optional

* [gotestsum](https://github.com/gotestyourself/gotestsum), for more human-friendly test output. If found in `$PATH`, it will be used in place of `go test`.

## Using Task (replacement of make)

Have a look at https://taskfile.dev/, then

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

The end-to-end tests (tests that interact with GitHub) are disabled by default because they require the following out of band setup.

The reason why we require environment variables (as opposed to using a configuration file) to pass test configuration is twofold:

* To enable any contributor to run their own tests without having to edit any file.
* To securely store secrets!

### Secure handling of the GitHub OAuth token

We use the [envchain] tool, that stores secrets in the OS secure store (Keychain for macOS, D-Bus secret service (gnome-keyring) for Linux), associated to a _namespace_. It then makes available all secrets associated to the given _namespace_ as environment variables.

Add the GitHub OAuth token:

```console
envchain --set cogito COGITO_TEST_OAUTH_TOKEN
```

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

```console
envchain cogito env | grep COGITO_
```

should show the following environment variables, with the correct values:

```text
COGITO_TEST_OAUTH_TOKEN
COGITO_TEST_REPO_OWNER
COGITO_TEST_REPO_NAME
COGITO_TEST_COMMIT_SHA
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

Build the Docker image and run the tests:

```console
task build
```

Build and push the Docker image:

```console
task publish
```

If present, the TAG environment variable with be used to tag the Docker image, for example

```console
env TAG=1.2.3 task publish
```

## Suggestions for quick iterations during development

These suggestions apply to the development of any Concourse resource.

After the local tests are passing, the quickest way to test in a pipeline the freshly pushed version of the Docker image is to use the `fly check-resource-type` command. It is faster and less resource-hungry than using a short `check_interval` setting in the pipeline.

For example, assuming that the test pipeline is called `cogito-test`, that the resource in the pipeline is called `cogito` and that there is a job called `Autocat` (all this is true by using the sample pipeline [pipelines/cogito.yml](./pipelines/cogito.yml)), you can do:

```console
task publish &&
fly -t devs check-resource-type -r cogito-test/cogito &&
sleep 5 &&
fly -t devs trigger-job -j cogito-test/Autocat -w
```

On each `put` and `get` step, the cogito resource will print its version, git commit SHA and build date to help validate which version a given build is using:

```text
This is the Cogito GitHub status resource. Tag: latest, commit: 91f64c0, date: 2019-10-09
```

## Contributing

Contributions following the minimalistic spirit of this library are welcome.

Please, before opening a PR, open a ticket to discuss your use case. This allows to better understand the _why_ of a new feature and not to waste your time (and mine) developing a feature that for some reason doesn't fit well with the spirit of the library or could be implemented differently.

I care about code quality, readability and tests, so please follow the current style and provide adequate test coverage. In case of doubts about how to tackle testing something, feel free to ask.

## License

This code is licensed according to the MIT license (see file [LICENSE](./LICENSE)).
