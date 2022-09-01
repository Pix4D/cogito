# Cogito Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.8.1] - UNRELEASED

### Added

- Google Chat: allow selecting which states trigger a notification to the chat, via configuration `source.chat_notify_on_states`. Default (as before this tunable): [`abort`, `error`, `failure`].
- Google Chat: enrich cogito logging with human-readable information about the chat space.
- Google Chat: allow to specify a different chat space/webhook during put (see `params.gchat_webhook`).
- Google Chat: allow to specify a custom chat message, overriding the default build summary (see `params.chat_message`).
- Google Chat: when passing a custom message or a custom message file, choose whether to append to it the default build summary or not (see `params.chat_message_append`).
- Google Chat: allow to specify a file containing a custom chat message (see `params.chat_message_file`).

## [v0.8.0] - 2022-08-11

### Changed

- Redesign of the whole cogito code, based on the removal of the ofcourse package. This simplifies the code and allows to use more of the Go type system (JSON is parsed into typed Go structs, instead of string maps): many validations are done directly by the JSON parser.
  - The behaviour should be unchanged.
  - Some error messages when parsing the pipeline configuration (source and params) are slightly different, but should cover the same area.
- Increase test coverage, simplify tests.
- Replace logging library. Log amount and content should be the same; some formatting differences.
- OCI image size (compressed, reported by DockerHub) is 30% smaller, from 7.5 MB to 5.09 MB.

## [v0.7.1] - 2022-07-04

### Added

- Support state `abort`. If the state is `abort`, we map it to `error` when using the GitHub Commit status API, and we leave it as-is when sending a chat notification. See the example `on_abort` hook in the README and section [Build states mapping](README.md#build-states-mapping).
- More debug logging: see clearly which features are enabled (GitHub Commit status, Google Chat notification) and the reason for skipping the sending of a chat message.

## [v0.7.0] - 2022-05-24

### Added

- Integration with Google Chat (optional). This allows to reduce the verbosity of a Concourse pipeline and especially to reduce the number of resource containers in a Concourse deployment, thus reducing load. Notification is sent only on states `abort`, `error` and `failure`.

### Somehow breaking

- Rename `github.NewStatus` to `github.NewCommitStatus`. This impacts only Go source code using the github package; it does NOT impact the cogito Concourse resource in any way.

### Changed

- Update to Go 1.18.
- Code cleanups.

## [v0.6.2] - 2022-01-24

### Added

- Support the case where the git remote URL contains basic auth information (this doesn't happen with the Concourse git resource, but can happen with a PR resource, see #46 for a discussion).
  Thanks @Mike-Dax for the initial implementation (#46).

### Changed

- Extensive code and tests refactoring.

### Fixed

- Return more user-friendly errors from the github/status API.
- Return more user-friendly errors when validating the Cogito and git configuration.

## [v0.6.1] - 2022-01-06

### Breaking

- Since Concourse 7.x _finally_ prints logging output also for the `check` step, we removed support for logging the output of the check step to Google Chat: now all output is printed to the web UI and to `fly check-rersource`.
  The `log_url` attribute for the source configuration is **DEPRECATED**, will be removed in a future release and is a no-op.

### Changed

- CI: move from Travis to GitHub Actions.
- tests: new Task targets `test:unit` (replaces target `test`) and `test:integration` (replaces target `test-integration`).
- build: cleaner Taskfile.

### Fixed

- Return standard error description in addition to the error code from the github/status API.

## [v0.6.0] - 2021-08-26

### Added

- Documentation: more details about integration tests and explain how to test instanced pipelines.
- Support Concourse 7.4 [instanced pipelines](https://concourse-ci.org/instanced-pipelines.html): the "details" URL in the GitHub status commit will link to the correct instanced pipeline.
  Thanks @lsjostro for the initial implementation (#26).

### Changed

- Allow `http` schema in the URI of the git resource (in addition to `https` and `ssh`). This change is transparent to the user and allows to use a HTTP-only git proxy of GitHub.
  Thanks @lsjostro for the initial implementation (#27).
- Better log messages for Info and Debug level.
- Renamed e2e tests to integration tests.

## [v0.5.1] - 2021-07-06

### Changed

- give better error message to the user when the resource is misconfigured

### Fixed

- remove flawed logic attempting to find the real cause when receiving an opaque GitHub API error. Instead, we now print to the user all the possible reasons.

## [v0.5.0] - 2021-03-09

### Changed

- tests: where it makes sense, remove the need to perform integration tests. For the `resource` package, total coverage stayed the same, but the non-integration coverage went from 53% to 89%.

### Added

- Optional source configuration `context_prefix`, that will be prepended to the context. See [Effects on GitHub](README.md#effects-on-github) of the README.
  Thanks @tgolsson for the discussion (#24) and for the initial implementation (#25). Thanks @eitah for a similar discussion (#23).
- Optional put step parameter `context`, that will override the default current job name. See [Effects on GitHub](README.md#effects-on-github) of the README.
  Thanks @eitah for the discussion (#23).

## [v0.4.0] - 2021-03-05

### Fixed

- Now the output correctly identifies the Docker tag:
  ```
  This is the Cogito GitHub status resource.
  Tag: update-tooling, commit: 5dbf3c4296, build date: 2021-03-04
  ```

### Changed

- Go 1.16
- Task > 3
- Moved from `envchain` to `gopass` to store secrets for integration tests (see file `CONTRIBUTING.md`).
- Documentation enhancements.

### Added

- document how Cogito uses the GitHub API.

## [v0.3.0] - 2019-11-14

### Changed

- Cogito is now open source.
- Docker images available at https://hub.docker.com/r/pix4d/cogito
- The cogito.yml sample pipeline is fully parametric.
- Renamed DEVELOPMENT file to CONTRIBUTING.

### Added

- Build and release via Travis.
- Extensive documentation.

## [v0.2.1] - 2019-10-22

### Fixed

- Always return a non-null version also for a get step. This is unlikely to have caused any problem, but better safer than sorry.

## [v0.2.0] - 2019-10-16

### Fixed

- If the git resource has a tag_filter, cogito parsing of the commit SHA is broken.
- If the git repo is in detached HEAD state, cogito parsing of the commit SHA is broken.
- The context of Cogito notifications make it impossible to use GitHub branch protection rules.

### Changed

- integration tests are now fully parametric, so that any contributor can run their integration tests.
- Split contributing and developing information from README into their own DEVELOPMENT (then CONTRIBUTING) file.
- Simplify testdata

### Added

- Support for gotestsum, see DEVELOPMENT (then CONTRIBUTING) file.
- Provide better GH API error diagnostics via heuristics to workaround GH API bug (see commit comments for 2c812c6 for details).

## [v0.1.0] - 2019-09-23

### Added

- First version, deployed in production at Pix4D.



[v0.1.0]: https://github.com/Pix4D/cogito/releases/tag/v0.1.0
[v0.2.0]: https://github.com/Pix4D/cogito/releases/tag/v0.2.0
[v0.2.1]: https://github.com/Pix4D/cogito/releases/tag/v0.2.1
[v0.3.0]: https://github.com/Pix4D/cogito/releases/tag/v0.3.0
[v0.4.0]: https://github.com/Pix4D/cogito/releases/tag/v0.4.0
[v0.5.0]: https://github.com/Pix4D/cogito/releases/tag/v0.5.0
[v0.5.1]: https://github.com/Pix4D/cogito/releases/tag/v0.5.1
[v0.6.0]: https://github.com/Pix4D/cogito/releases/tag/v0.6.0
[v0.6.1]: https://github.com/Pix4D/cogito/releases/tag/v0.6.1
[v0.6.2]: https://github.com/Pix4D/cogito/releases/tag/v0.6.2
[v0.7.0]: https://github.com/Pix4D/cogito/releases/tag/v0.7.0
[v0.7.1]: https://github.com/Pix4D/cogito/releases/tag/v0.7.1
[v0.8.0]: https://github.com/Pix4D/cogito/releases/tag/v0.8.0
