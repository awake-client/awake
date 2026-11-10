# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Environment controlled authentication: the reserved `authType` and
  `grantType` variables choose how a request authenticates, so one request can
  switch between none, basic, bearer, API key and OAuth 2, and between OAuth 2
  grants, across environments.

### Fixed

- The environment editor stays open when the page behind it is clicked; only
  its close button or the Escape key closes it.

## [0.3.0] - 2026-11-10

### Added

- Git-native workspaces: collections, requests and environments stored as plain
  YAML files with reviewable diffs and ordinary git history.
- HTTP, SSE and WebSocket requests with parameters, headers, bodies, request
  scripts, and basic, bearer, API key and OAuth 2 authentication.
- Shared environments with local overrides; secret values are stored in the
  operating system keychain.
- Private environments: an environment can live in a git ignored local file
  instead of a shared one, with editor, picker and secrets handling.
- Coding-agent integration for Claude Code and OpenCode.
- Import and export for Bruno, OpenCollection, OpenAPI and Postman.
- Virtualized collection tree, SQLite index, search, command palette, request
  history, and light and dark themes.
- Hiding a collection from the tree, with a menu to display the hidden ones again.
- Folder level configuration: a settings page per folder like the collections,
  reachable from the folder menu, with its own variables and authentication.
- Requests automatically inherit the authentication of their closest folder,
  then of their collection, even when their file omits the `authentication`
  section; the editor shows the inherited credentials and where they come from.
- In-app update checker backed by GitHub Releases.
- macOS and Linux installers, plus a Windows NSIS installer and portable archive.

### Fixed

- Folder settings open for folders created outside Awake, such as imported ones
  without a `folder.yaml`: they start empty, named after their directory, and
  saving writes the metadata file.
- Importing an OpenCollection whose OAuth 2 password flow nests its client and
  resource owner credentials now keeps them, so Keycloak-style collections no
  longer fail with `domain: invalid collection`.
- Requests inside a folder keep the environments of their collection: the
  picker offers them and the title bar keeps the selected one, instead of
  showing "No environment". Scripts writing environment variables find the
  collection environment from a request in one of its folders.

[Unreleased]: https://github.com/awake-client/awake/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/awake-client/awake/compare/v0.2.3...v0.3.0
