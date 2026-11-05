# Conventions

This document is the reference for anyone contributing to Awake. It covers
repository layout, naming, error handling, testing and commit rules.

## Repository layout

Code is split **vertically by feature**. A feature is a self-contained slice
that owns its model, its persistence and the service exposed to the frontend.

```
src/
  app/                  bootstrap, dependency wiring, menus, typed events
  domain/               pure entities shared by several features
  features/
    workspace/
      model.go
      repository.go
      service.go
      service_test.go
    requests/
    environments/
    history/
    httpclient/
    authentication/
    secrets/
    git/
    search/
    updates/
  shared/
    database/
    yamlcodec/
    workspacepath/
    pagination/
    eventbus/
    keychain/
    logging/
    failures/
```

Forbidden:

- horizontal packages such as `models/`, `services/`, `handlers/`, `utils/`;
- imports between two features' internals;
- business logic in `src/app`, which only wires dependencies.

Allowed:

- a feature importing `src/domain` and `src/shared`;
- a feature consuming another feature through an interface it declares itself
  (ports and adapters), with the implementation wired in `src/app`;
- cross-feature notifications through `shared/eventbus`.

### Domain

`src/domain` holds the entities several features must agree on, such as
`Request`, `Collection`, `Parameter` and `Header`. It contains no input or
output: reading and writing files stays in features, and low-level helpers stay
in `src/shared`. A domain type carries its defaults and validation so every
feature applies the same rules.

### Frontend layout

```
frontend/src/app/
  core/                 bindings, configuration, theme, keyboard shortcuts
  shared/               presentational components without business state
  features/
    request-editor/
      request-editor.ts
      request-editor.html
      request-editor.store.ts
      request-editor.routes.ts
```

Feature areas are lazy loaded. State lives in signal stores scoped to the
feature. Shared state is exposed by a dedicated store in `core`.

## Naming

Write full words. Abbreviations are banned except canonical initialisms that
Go and TypeScript tooling expects.

Allowed initialisms: `ID`, `URL`, `HTTP`, `JSON`, `YAML`, `SQL`, `API`, `SSE`,
`MCP`, `OAuth2`.

Banned examples and replacements:

| Banned | Use instead |
| --- | --- |
| `req`, `res`, `resp` | `request`, `response` |
| `env`, `envs` | `environment`, `environments` |
| `cfg`, `conf` | `configuration` |
| `mgr`, `svc`, `repo` | `manager`, `service`, `repository` |
| `db` | `database` |
| `msg` | `message` |
| `btn`, `dlg` | `button`, `dialog` |
| `ctx` | `requestContext`, `operationContext` |
| `i`, `j` | `index`, `position` |

Rule of thumb: if the shortened name is not an official initialism, spell it
out.

### Files and symbols

- Go files use lower snake case: `request_repository.go`.
- Angular files follow the v20+ style guide and omit the `.component` suffix:
  `request-editor.ts`, `request-editor.html`.
- Exported Go identifiers carry a doc comment starting with the identifier.
- TypeScript types use `PascalCase`, values `camelCase`, constants
  `UPPER_SNAKE_CASE` when truly global to a module.

## Readability

No magic values in expressions. Extract named constants and configuration
fields, and name intermediate results.

```go
const (
    maximumInlineResponseSize = 2 * 1024 * 1024
    responseChunkSize         = 64 * 1024
)
```

```ts
const MAXIMUM_INLINE_RESPONSE_SIZE = 2 * 1024 * 1024;
```

Configuration values shared by several features belong in the application
configuration, not in duplicated constants. Keep functions short enough that
the name of the function and the names of its locals explain the code without
extra comments.

## Error handling

- Go: wrap errors with context and `%w`, return typed failures from
  `shared/failures` when the frontend must react to a specific cause.
- Never swallow an error silently.
- TypeScript: services surface failures as rejected promises with a typed
  payload; components render a message, not a raw stack trace.

## Testing

- Tests live next to the code under test.
- `yamlcodec`, `interop`, `git` and conflict resolution require table-driven
  tests with golden files.
- Performance-sensitive paths have benchmarks seeded with a large workspace.
- Every bug fix ships with a regression test.

## Commits

Commits follow Conventional Commits, in English, imperative mood, at most 72
characters for the summary.

```
type(scope): summary
```

Types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`, `build`,
`ci`.

Scopes follow features: `workspace`, `collections`, `requests`, `environments`,
`history`, `httpclient`, `authentication`, `secrets`, `git`, `search`,
`updates`, `yamlcodec`, `database`, `ui`, `deps`.

Rules:

- one commit expresses one reversible intent;
- generated scaffolding is committed separately from manual changes;
- tests ship with the code they cover;
- no `wip` commits, no mixed refactor and feature work;
- generated frontend bindings are committed; build outputs are not.

## Performance rules

- Never send a full workspace tree or a full response body through the
  frontend bridge when a paginated or chunked call is possible.
- Lists displayed by the interface are virtualized.
- Search and history queries use keyset pagination.
- SQLite runs in WAL mode; write operations are batched.
- A workspace index must always be rebuildable from files alone.
