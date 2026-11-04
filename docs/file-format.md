# File format

Awake stores a workspace as plain YAML files inside a git repository. Files are
the source of truth; the SQLite database is an index that can be rebuilt from
them at any time.

## Workspace layout

```
awake.yaml                      workspace configuration, tracked by git
environments/
  development.yaml              shared variables, tracked by git
  development.local.yaml        local overrides, git ignored
collections/
  platform/
    collection.yaml             collection metadata
    users/
      list-users.yaml           one file per request
local/                          private requests, git ignored
.awake/                         index and cache, git ignored
```

The application generates the `.gitignore` entries for `*.local.yaml`,
`local/` and `.awake/` when a workspace is created.

## Formatting rules

These rules exist to keep git diffs readable and mergeable.

- UTF-8, LF line endings, two-space indentation, one trailing newline.
- No YAML anchors, aliases or tags; they hide content from diffs.
- Canonical key order: `version`, `id`, `name`, then the rest in declaration
  order.
- Entry lists (headers, parameters, variables) use compact flow style, one
  entry per line, so a change touches a single line.
- Comments are preserved on save. Rewriting a file without changing its
  content must produce an empty git diff.

## Workspace configuration

`awake.yaml`:

```yaml
version: 1
id: workspace_01JABCDEF
name: Platform API
mode: team
defaultEnvironment: development
variables:
  apiVersion: v1
git:
  remote: origin
  branch: main
  automaticPull: true
  automaticPush: true
  synchronizationInterval: 5m
```

- `id` is generated once and never changes. It keys the local history database
  so history survives a move of the workspace directory.
- `mode` is `team` or `personal`. A personal workspace may omit the `git`
  section and stay local.
- `variables` are workspace defaults available to every request.

## Environments

`environments/development.yaml`:

```yaml
name: Development
variables:
  baseUrl: https://api.dev.example.com
  pageSize: 20
secretVariables:
  - apiToken
```

- `variables` are plain values committed to git.
- `secretVariables` lists names only. Values are stored in the macOS Keychain
  under `awake/<workspace id>/<environment name>/<variable name>` and are never
  written to disk.
- A missing secret is reported when a request needs it, and the interface
  prompts for the value.

`environments/development.local.yaml` has the same shape and is git ignored. It
overrides the shared environment on the local machine.

## Variables resolution order

From lowest to highest precedence:

1. workspace defaults (`awake.yaml`);
2. shared environment;
3. local environment overrides;
4. collection variables;
5. request variables;
6. values provided at send time.

Secret values are resolved lazily when a request is sent. Referencing an
undefined variable is an error, not an empty string.

## Collections

`collections/platform/collection.yaml`:

```yaml
name: Platform
variables:
  apiVersion: v1
authentication:
  type: inherit
documentation: |
  Requests for the platform API.
```

The directory tree defines nesting. A collection directory without a
`collection.yaml` is valid and inherits the parent metadata.

## Requests

`collections/platform/users/list-users.yaml`:

```yaml
name: List users
method: GET
url: "{{baseUrl}}/{{apiVersion}}/users"
parameters:
  - {name: page, value: "1", enabled: true}
  - {name: query, value: "", enabled: false}
headers:
  - {name: Accept, value: application/json, enabled: true}
authentication:
  type: inherit
body:
  type: none
documentation: |
  Returns a paginated list of users.
```

### Request fields

| Field | Type | Description |
| --- | --- | --- |
| `name` | string | Display name |
| `method` | string | HTTP method |
| `url` | string | URL template with variables |
| `parameters` | list | Query parameters |
| `headers` | list | Request headers |
| `authentication` | object | Authentication settings or `inherit` |
| `body` | object | Request body |
| `documentation` | string | Free-form notes |

### Body types

`none`, `json`, `text`, `xml`, `form-urlencoded`, `multipart`, `binary`.

```yaml
body:
  type: json
  content: |
    {"name": "Ada"}
```

Binary bodies reference a file relative to the request file. Files above the
configured inline limit are never embedded in YAML.

### Authentication types

`inherit`, `none`, `basic`, `bearer`, `apiKey`, `oauth2`.

```yaml
authentication:
  type: apiKey
  key: X-Api-Key
  value: "{{apiToken}}"
  location: header
```

OAuth2 stores client configuration in the YAML but keeps tokens in the
Keychain. Supported grants: authorization code with PKCE, client credentials.

## Private requests and local overlays

- A request marked private moves to `local/`, mirroring its collection path. It
  is ignored by git and only visible on the local machine.
- Marking a request as shared moves it back into `collections/` and stages it
  for the next commit.
- Local environment files override shared values without changing tracked
  files.

## Merge rules

Identity is the file path. Renaming a request is a move, not an edit.

When a merge involves both sides:

- maps (variables, workspace configuration) merge per key;
- request lists merge per entry, keyed by `name`; a removed entry stays removed
  unless the other side modified it;
- the same key modified with different values is a conflict and requires an
  explicit choice;
- added entries from both sides are combined, preserving each side's order;
- documentation fields merge as text and fall back to a three-way text merge.

Conflicts are presented per file with base, local and incoming versions, and
can be resolved field by field or by keeping one side entirely.

## Versioning

`version` is an integer. The application refuses to open a workspace with a
newer version than it supports, and migrates older versions on open after
creating a backup commit.
