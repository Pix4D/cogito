# cogito

[![Travis Build Status](https://travis-ci.org/Pix4D/cogito.svg?branch=master)](https://travis-ci.org/Pix4D/cogito)

Cogito (**Co**ncourse **git** status res**o**urce) is a [Concourse resource] to update the GitHub status of a commit during a build. The name is a humble homage to [René Descartes].

Written in Go, it has the following characteristics:

- As lightweight as possible (Docker Alpine image).
- Extensive test suite.
- Autodiscovery of configuration parameters.
- No assumptions on the git repository (for example, doesn't assume that the default branch is `main` or that branch `main` even exists).
- Helpful error messages when something goes wrong with the GitHub API.
- Configurable logging for the three steps (check, in, out) to help troubleshooting.
- Boilerplate code generated with [ofcourse](https://github.com/cloudboss/ofcourse)

[Concourse resource]: https://concourse-ci.org/resources.html
[René Descartes]: https://en.wikipedia.org/wiki/Ren%C3%A9_Descartes

## Contributing and Development

This document explains how to use this resource. See [CONTRIBUTING](./CONTRIBUTING.md) for how to build the Docker image, develop, test and contribute to this resource.

## Semver, releases and Docker images

This project follows [Semantic Versioning](https://semver.org/) and has a [CHANGELOG](./CHANGELOG).

**NOTE** Following semver, no backwards compatibility is guaranteed as long as the major version is 0.

Releases are tagged in the git repository with the semver format `vMAJOR.MINOR.PATCH` (note the `v` prefix). The corresponding Docker image has tag `MAJOR.MINOR.PATCH` and is available from [DockerHub](https://hub.docker.com/r/pix4d/cogito).

### Which Docker tag to use?

You can pin the resource to a specific release tag `MAJOR.MINOR.PATCH`.

If you omit the pinning, you will be following the Docker tag `latest`, which for this resource always points to the  latest release, not to the tip of master. This should normally be fine, but can still break in case of change of major version!

## Example

See also [pipelines/cogito.yml](pipelines/cogito.yml) for a bigger example and for how to use YAML anchors to reduce as much as possible YAML verbosity.

```yaml
resource_types:
- name: cogito
  type: registry-image
  check_every: 1h
  source:
    repository: pix4d/cogito

resources:
- name: gh-status
  type: cogito
  check_every: 1h
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
  - name: autocat
    on_success:
      put: gh-status
      inputs: [the-repo]
      params: {state: success}
    on_failure:
      put: gh-status
      inputs: [the-repo]
      params: {state: failure}
    on_error:
      put: gh-status
      inputs: [the-repo]
      params: {state: error}
    plan:
      - get: the-repo
        trigger: true
      - put: gh-status
        inputs: [the-repo]
        params: {state: pending}
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
- `access-token`: The OAuth access token. See section [GitHub OAuth token](#github-oauth-token).

### Optional

- `log_level`: The log level (one of `debug`, `info`, `warn`, `error`, `silent`). Default `info`.
- `log_url`. A Google Hangout Chat webhook. Useful to obtain logging for the `check` step.

### Suggestions

We suggest to set a long interval for `check_interval`, for example 1 hour, as shown in the example above. This helps to reduce the number of check containers in a busy Concourse deployment and, for this resource, has no adverse effects.

## The check step

It is currently a no-op and will always return the same version, `dummy`.

## The get/in step

It is currently a no-op.

## The put/out step

Sets or updates the GitHub status for a given commit, following the [GitHub status API].

### Parameters

#### Required

- `state`: The state to be set. One of `error`, `failure`, `pending`, `success`.

### Note

It requires one and only one ["put inputs"] to be specified, otherwise it will error out. For example:

```yaml
on_success:
  put: gh-status
  # This is the name of the git resource corresponding to the GitHub repo to be updated.
  inputs: [the-repo]
  params: {state: success}
```

As all the other GitHub status resources, it requires as input the git repo passed by the git resource because it will look inside it to gather information such as the commit hash for which to set the status.

It requires only one put input to help you have an efficient pipeline, since if the "put inputs" list is not set explicitly, Concourse will stream all inputs used by the job to this resource, which can have a big performance impact. From the ["put inputs"] documentation:

> inputs: [string]
>
> Optional. If specified, only the listed artifacts will be provided to the container. If not specified, all artifacts will be provided.

To better understand from where `the-repo` comes from, see the full example at the beginning of this document.

["put inputs"]: https://concourse-ci.org/put-step.html#put-step-inputs

## GitHub OAuth token

Follow the instructions at [GitHub personal access token] to create a personal access token.

Give to it the absolute minimum permissions to get the job done. This resource only needs the `repo:status` scope, as explained at [GitHub status API].

NOTE: The token is security-sensitive. Treat it as you would treat a password. Do not encode it in the pipeline YAML and do not store it in a YAML file. Use one of the Concourse-supported credentials managers, see [Concourse credential managers].

See also the section [The end-to-end tests](./CONTRIBUTING.md#the-end-to-end-tests) for how to securely store the token to run the end-to-end tests.

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

## License

This code is licensed according to the MIT license (see file [LICENSE](./LICENSE)).

[GitHub status API]: https://developer.github.com/v3/repos/statuses/
[GitHub API v3]: https://developer.github.com/v3/
[GitHub personal access token]: https://help.github.com/en/articles/creating-a-personal-access-token-for-the-command-line

[Concourse credential managers]: https://concourse-ci.org/creds.html.

[envchain]: https://github.com/sorah/envchain
