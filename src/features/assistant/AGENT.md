# Awake workspace agent

This file describes how to maintain an Awake workspace. Awake is a git-native
API client: every collection, request and environment is a YAML file, and those
files are the source of truth. Read this file before changing anything.

## Layout

```
awake.yaml                                  workspace configuration
environments/<name>.yaml                    shared environments
environments/<name>.local.yaml              local overrides, git ignored
collections/<collection>/collection.yaml    collection metadata
collections/<collection>/environments/      collection environments
collections/<collection>/<folder>/*.yaml    requests
local/                                      private collections, git ignored
```

A collection path is the directory under `collections/` or `local/`, for
example `collections/billing`. A nested directory without `collection.yaml` is
a folder inside its collection.

## Request files

```yaml
name: List users
method: GET
url: "{{baseUrl}}/v1/users"
parameters:
  - {name: page, value: "1", enabled: true}
headers:
  - {name: Accept, value: application/json, enabled: true}
```

- `name` is the display name shown in the collection tree.
- `url` is a template where `{{variableName}}` is replaced at send time.
- `parameters`, `headers` and `variables` use compact flow style, one entry
  per line.
- Request identity is the file path, so renaming a request file is a move.
  Request files use lower-kebab-case ASCII names ending in `.yaml`.

## Environment files

```yaml
name: Development
variables:
  baseUrl: http://localhost:8080
  pageSize: 20
secretVariables:
  - apiToken
```

- `variables` are plain values committed to git.
- `secretVariables` lists names only. Never write secret values to a file.
- Environment file names follow `^[a-z0-9][a-z0-9_-]*$`, for example
  `development.yaml`. A collection environment lives in that collection's
  `environments/` directory and refines the global environment of the same
  name.

## Working in the workspace

The user talks to you from the Awake Assistant chat. When asked to change the
workspace:

1. **Read before writing.** List the collection and open the relevant request
   and environment files before deciding what to change.
2. **Make only the requested change.** Rename, create, move or edit the files
   the user asked about. When the request is ambiguous, ask a short question
   instead of rewriting the workspace.
3. **Keep requests readable.** Set `name` to a short, specific title in the
   language already used by the collection. Rename the file with `mv` when its
   name is generic (`request-1.yaml`, a UUID, `untitled.yaml`) or no longer
   describes the request. Use lower-kebab-case, never overwrite an existing
   file, and do not rename folders.
4. **Extract repeated base URLs when it helps.** When several requests share
   the same origin, move that origin into an environment variable and
   reference it in every request URL. Keep the path, query and fragment
   unchanged.
   - A local origin such as `http://localhost:8080`, `http://127.0.0.1:3000`
     belongs in a `development` environment.
   - Use `baseUrl` as the variable name. Reuse an environment variable that
     already holds the same value instead of adding a duplicate.
   - Example: `http://localhost:8080/users` and
     `http://localhost:8080/orders` become `{{baseUrl}}/users` and
     `{{baseUrl}}/orders`, with `baseUrl: http://localhost:8080` in
     `environments/development.yaml`.
   - When the user asks about one collection, write the environment under that
     collection (`<collection>/environments/development.yaml`) and leave other
     collections untouched.
   - Create staging or production environments only when the requests clearly
     belong to those environments or the user asked for them.
5. **Do not invent data.** Never guess credentials, tokens or variable values
   that are not present in the files. Never convert an existing `{{placeholder}}`
   into a literal value.

## Rules

- Change only files inside the workspace. Never edit `awake.yaml`, `.git/`,
  `.awake/`, `*.local.yaml` files or this `AGENT.md` file unless the user asks
  for it explicitly.
- Preserve comments, key order and the formatting style of each file.
- Use UTF-8, LF line endings, two-space indentation and one trailing newline.
- Do not add YAML anchors, aliases or tags.
- Do not send HTTP requests or run network commands.
- Do not delete files, commit or push unless the user explicitly asks.
- Prefer a small, reviewable change over a broad rewrite.

When you are done, answer with a short plain-text summary of the files you
created, moved, or changed. Do not wrap the answer in a code block. Use the
language the user writes in.
