# Awake

Awake is a git-native API client for macOS. Collections, requests and
environments are plain YAML files stored in a workspace directory, so
collaboration happens through git instead of a proprietary cloud.

> Status: early development. The repository currently contains the project
> scaffolding and the foundations described below.

## Why

Existing API clients either hide data behind a cloud service, store everything
in a single opaque file, or make git collaboration an afterthought. Awake
treats files as the source of truth and git as the collaboration layer, while
keeping the application fast on large workspaces.

## Stack

| Layer | Technology |
| --- | --- |
| Desktop runtime | [Wails](https://wails.io) v3 |
| Backend | Go |
| Frontend | Angular 22, standalone components, signals, zoneless |
| Local index and history | SQLite |
| File format | YAML, one file per request |

## Architecture

The guiding rule is that **files are the source of truth**. SQLite is a
disposable index that can be rebuilt from disk at any time. It stores the
collection index, search data, request history and UI state, never the only
copy of a workspace.

```
awake/
  main.go                     Wails entry point
  src/
    app/                      application bootstrap and dependency wiring
    features/                 vertical slices, one package per feature
    shared/                   cross-cutting primitives without business logic
  frontend/
    src/app/
      core/                   bindings, configuration, theme
      shared/                 presentational components
      features/               lazy loaded feature areas
  docs/                       architecture and conventions
  build/                      packaging assets
```

Each feature owns its model, its storage access and the service exposed to the
frontend. Features communicate through events or through interfaces declared
by the consumer, never through direct imports of another feature's internals.
See [docs/conventions.md](docs/conventions.md) for the full rules.

## Workspace layout

A workspace is a regular git repository:

```
awake.yaml                  workspace configuration
environments/dev.yaml       shared variables
environments/dev.local.yaml local overrides, git ignored
collections/plateforme/     collections and requests
local/                      private requests, git ignored
.awake/                     disposable index and cache, git ignored
```

Secret values never appear in these files. Only their names are stored, with
the values kept in the macOS Keychain.

## Development

Requirements:

- Go 1.25 or newer
- Node.js 22 or newer
- Wails v3 CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`
- Git

```sh
task dev          # run the application with hot reload
task test         # run the Go test suite
task lint         # run the Go and TypeScript linters
task build        # build the macOS application bundle
```

## Documentation

- [Architecture and conventions](docs/conventions.md)

## License

Awake is free software released under the GNU General Public License,
version 3 or any later version. See [LICENSE](LICENSE).
