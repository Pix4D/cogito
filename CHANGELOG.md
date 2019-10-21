# Cogito Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]


### Changed

- Cogito is now open source.
- The cogito.yml sample pipeline is fully parametric.
- Renamed DEVELOPMENT file to CONTRIBUTING.

## [v0.2.1] - 2019-10-22

### Fixed

- Always return a non-null version also for a get step. This is unlikely to have caused any problem, but better safer than sorry.

## [v0.2.0] - 2019-10-16

### Fixed

- If the git resource has a tag_filter, cogito parsing of the commit SHA is broken.
- If the git repo is in detached HEAD state, cogito parsing of the commit SHA is broken.
- The context of Cogito notifications make it impossible to use GitHub branch protection rules.

### Changed

- e2e tests are now fully parametric, so that any contributor can run their e2e tests.
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
