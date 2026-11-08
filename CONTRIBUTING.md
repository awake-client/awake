# Contributing to Awake

Thanks for taking the time to contribute. Awake is licensed under
[GPL-3.0-or-later](LICENSE); by submitting a change you agree to license it
under the same terms.

Please read the [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26 or newer
- [Node.js](https://nodejs.org/) 22 or newer (24 is used in CI)
- The [Wails v3 CLI](https://v3.wails.io):

  ```sh
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
  ```

Linux builds also need GTK4 and WebKitGTK 6.0 at runtime (Ubuntu 24.04+,
Debian 13+ or equivalent).

## Set up the repository

```sh
git clone https://github.com/awake-client/awake.git
cd awake
wails3 task install:frontend:deps
```

## Develop

```sh
wails3 task dev     # run with frontend hot reload
wails3 task build   # build for the host platform
wails3 task package # create the platform package
```

## Test and lint

```sh
wails3 task test    # Go and Angular test suites
wails3 task lint    # golangci-lint over the Go sources
wails3 task format  # gofmt
```

Every bug fix should ship with a regression test. The full test suite must pass
before a pull request is reviewed.

## Conventions

Read [docs/conventions.md](docs/conventions.md) before writing code. It is the
reference for repository layout, naming, error handling, testing and commit
rules.

## Commits

Commits follow [Conventional Commits](https://www.conventionalcommits.org/), in
English, imperative mood, with a summary of at most 72 characters:

```
type(scope): summary
```

Types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`, `build`,
`ci`. Scopes follow features: `workspace`, `collections`, `requests`,
`environments`, `history`, `httpclient`, `authentication`, `secrets`, `git`,
`search`, `updates`, `yamlcodec`, `database`, `ui`, `deps`.

Keep one reversible intent per commit. Generated scaffolding is committed
separately from manual changes, generated frontend bindings are committed, and
build outputs are not.

## Pull requests

- Keep the change focused and describe the behaviour it changes.
- Link the issue it fixes, if any.
- Add or update tests, and make sure CI is green.
- Update the documentation and [CHANGELOG.md](CHANGELOG.md) when relevant.

## Reporting bugs

Open an [issue](https://github.com/awake-client/awake/issues) with the Awake
version, the operating system, the steps to reproduce, and any relevant log
output. For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead
of opening a public issue.
