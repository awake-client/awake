# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Git-native workspaces: collections, requests and environments stored as plain
  YAML files with reviewable diffs and ordinary git history.
- HTTP, SSE and WebSocket requests with parameters, headers, bodies, request
  scripts, and basic, bearer, API key and OAuth 2 authentication.
- Shared environments with local overrides; secret values are stored in the
  operating system keychain.
- Coding-agent integration for Claude Code and OpenCode.
- Import and export for Bruno, OpenCollection, OpenAPI and Postman.
- Virtualized collection tree, SQLite index, search, command palette, request
  history, and light and dark themes.
- Hiding a collection from the tree, with a menu to display the hidden ones again.
- Folder level configuration: a settings page per folder like the collections,
  with its own variables and authentication.
- Requests automatically inherit the authentication of their closest folder,
  then of their collection, even when their file omits the `authentication`
  section; the editor shows the inherited credentials and where they come from.
- In-app update checker backed by GitHub Releases.
- macOS and Linux installers, plus a Windows NSIS installer and portable archive.

[Unreleased]: https://github.com/awake-client/awake/commits/main
