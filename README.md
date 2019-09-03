# cogito

Cogito (**Co**ncourse **git** status res**o**urce) is a Concourse resource to update GitHub Status during a build.

Written in Go, it has the following characteristics:

- As lightweight as possible (Docker Alpine image).
- Extensive test suite.
- No assumptions on the git repository (for example, doesn't assume that the default branch is `main` or that branch `main` even exists).
- Configurable logging for the three steps (check, in, out) to help throubleshooting.
- Boilerplate code generated with https://github.com/cloudboss/ofcourse

## Example

See also `pipelines/cogito.yml` for a bigger example and for how to use YAML anchors to reduce as much as possible YAML verbosity.

```yaml
resource_types:
- name: cogito
  type: registry-image
  check_every: 24h
  source:
    repository: ((your-docker-registry))/cogito

resources:
- name: gh-status
  type: cogito
  check_every: 24h
  source:
    owner: ((your-github-user-or-organization))
    repo: ((your-repo-name))
    access_token: ((your-OAuth-personal-access-token))

- name: the-repo
  type: git
  source:
    uri: https://github.com/((your-github-user-or-organization))/((your-repo-name))
    branch: ((branch))

jobs:
  - name: Autocat
    on_success:
      put: gh-status
      inputs: [the-repo] # Useful optimization!
      params:
        state: success
        repo: the-repo
    on_failure:
      put: gh-status
      inputs: [the-repo] # Useful optimization!
      params:
        state: failure
        repo: the-repo
    on_error:
      put: gh-status
      inputs: [the-repo] # Useful optimization!
      params:
        state: error
        repo: the-repo
    plan:
      - get: the-repo
        trigger: true
      - put: gh-status
        inputs: [the-repo] # Useful optimization!
        params:
          state: pending
          repo: the-repo
      - task: maybe-fail
        config:
          platform: linux
          image_resource:
            type: registry-image
            source: { repository: alpine }
          run:
            path: /bin/sh
            args:
              - -c
              - |
                set -o errexit
                echo "Hello world!"
```

## Source Configuration

### Required

- `owner`: The GitHub user or organization.
- `repo`: The GitHub repository name.
- `access-token`: The OAuth access token. See section "GitHub OAuth token" below.

### Optional

- `log_level`: The log level (one of `debug`, `info`, `warn`, `error`, `silent`). Default `info`.
- `log_url`. A Google Hangout Chat webhook. Useful to obtain logging for the `check` step.

## The `check` step

It is currently a no-op.

## The `in` (corresponding to `get`) step

It is currenty a no-op.

## The `out` (corresponding to `put`) step

Sets or updates the GitHub status for a given commit, following the [GitHub status API].

### Parameters

#### Required

- `repo`: The `input:` corresponding to the repository for which you want to set the state (see the example above).
- `state`: The state to be set. One of `error`, `failure`, `pending`, `success`.

## GitHub OAuth token

Follow the instructions at [GitHub personal access token] to create a personal access token.

Give to it the absolute minimum permissions to get the job done. This resource only needs the `repo:status` scope, as explained at [GitHub status API].

NOTE: The token is security-sensitive. Treat it as you would treat a password. Do not encode it in the pipeline YAML and do not store it in a YAML file. Use one of the Concourse-supported credentials managers, see [Concourse credential managers].

See also the section below `The end-to-end tests` for how to securely store the token to run the end-to-end tests.

## Caveat: GitHub rate limiting

From [GitHub API v3]

> Rate limiting
>
> For API requests using Basic Authentication or OAuth, you can make up to 5000 requests
> per hour. All OAuth applications authorized by a user share the same quota of 5000
> requests per hour when they authenticate with different tokens owned by the same user.
>
> For unauthenticated requests, the rate limit allows for up to 60 requests per hour.
> Unauthenticated requests are associated with the originating IP address, and not the
> user making requests.

In case of rate limiting, the error message in the output of the `put` step will mention it.

## Development

### Prerequisites

* golang, version >= 1.13
* docker, version >= 19
* task (https://taskfile.dev/), version >= 2.6

### Using Task (replacement of make)

Have a look at https://taskfile.dev/, then

```
task --list
```

### Running the default tests

The Taskfile includes a `test` target, and tests are also run inside the Docker build.

Run the default tests:

```console
task test
```

### The end-to-end (e2e) tests

The end-to-end tests (tests that interact with GitHub) are disabled by default because they require the following out of band setup.

The reason why we require enviroment variables (as opposed to using a configuration file) to pass test configuration is twofold:

- To enable any contributor to run his/her own tests without having to edit any file.
- To securely store secrets!

#### Secure handling of the GitHub OAuth token

We use the [envchain] tool, that stores secrets in the OS secure store (Keychain for macOS, D-Bus secret service (gnome-keyring) for Linux), associated to a _namespace_. It then makes available all secrets associated to the given _namespace_ as environment variables.

Add the GitHub OAuth token:

```console
envchain --set cogito COGITO_TEST_OAUTH_TOKEN
```

#### Prepare the test repository

1. In your GitHub account, create a test repository, say for example `cogito-test`.
2. In the test repository, push at least one commit, on any branch you fancy. Take note of the 40 digits commit SHA (the API wants the full SHA).

#### Add test repository information as environment variables

```console
envchain --set cogito COGITO_TEST_REPO_OWNER   # Your GitHub user or organization
envchain --set cogito COGITO_TEST_REPO_NAME
envchain --set cogito COGITO_TEST_COMMIT_SHA
```

#### Verify your setup

```console
envchain cogito env | grep COGITO_
```

should show the following environment variables, with the correct values:

```
COGITO_TEST_OAUTH_TOKEN
COGITO_TEST_REPO_OWNER
COGITO_TEST_REPO_NAME
COGITO_TEST_COMMIT_SHA
```

#### Running the end-to-end tests

We are finally ready to run also the end-to-end tests:

```console
envchain cogito task test
```

The end-to-end tests have the following logic:

- If none of the environment variables are set, we skip the test.
- If all of the environment variables are set, we run the test.
- If some of the environment variables are set and some not, we fail the test. We do this on purpose to signal to the user that the environment variables are misconfigured.
	
#### Making the environment variables available to your editor

If you want to run the tests from within your editor test runner, it is enough to use `envchain` to start the editor. For example:

```console
envchain cogito code
```

#### Caveat: GitHub API limits on the number of statuses per commit

From [GitHub status API], there is a limit of 1000 statuses per sha and context within a
repository. Attempts to create more than 1000 statuses will result in a validation error.

If this happens, just create another commit and update the `COGITO_TEST_COMMIT_SHA` environment variable.

### Building and publishing the image

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

[GitHub status API]: https://developer.github.com/v3/repos/statuses/
[GitHub API v3]: https://developer.github.com/v3/
[GitHub personal access token]: https://help.github.com/en/articles/creating-a-personal-access-token-for-the-command-line

[Concourse credential managers]: https://concourse-ci.org/creds.html.

[envchain]: https://github.com/sorah/envchain
