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
- `secretVariables` lists names only. Values are stored in the operating system
  keychain under `awake/<workspace id>/<environment name>/<variable name>` and
  are never written to disk.
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
| `protocol` | string | `http` (default), `sse` or `websocket` |
| `parameters` | list | Query parameters |
| `headers` | list | Request headers |
| `authentication` | object | Authentication settings or `inherit` |
| `body` | object | Request body |
| `documentation` | string | Free-form notes |

### Streaming requests

`protocol` selects how Awake talks to the server. It is omitted for ordinary
HTTP requests.

```yaml
name: Deployment events
method: GET
url: "{{baseUrl}}/events"
protocol: sse
```

- `sse` keeps a GET connection open and consumes the `text/event-stream`
  response. Each dispatched event becomes one entry in the event log.
- `websocket` opens a bidirectional connection. The request URL may use
  `ws://` or `wss://`; an `http://` or `https://` URL is converted. A non-empty
  `body.content` is sent as the first text message.
- Streams are live sessions. Their events are kept in memory only and are
  never written to the workspace or the history database.

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

OAuth2 stores client configuration in the YAML but keeps tokens out of the
workspace: they stay in memory for the session. Supported grants:
authorization code with PKCE, client credentials.

```yaml
authentication:
  type: oauth2
  grantType: authorizationCode
  authorizationUrl: "{{identityUrl}}/authorize"
  tokenUrl: "{{identityUrl}}/token"
  clientId: "{{clientId}}"
  clientSecret: "{{clientSecret}}"
  scopes: [read, write]
  redirectUrl: http://127.0.0.1:53682/callback
  audience: https://api.example.com
```

| Field | Description |
| --- | --- |
| `grantType` | `authorizationCode` or `clientCredentials` |
| `authorizationUrl` | Browser page of the authorization code flow |
| `tokenUrl` | Token endpoint |
| `clientId` | Client identifier |
| `clientSecret` | Secret value or `{{secretVariable}}` reference |
| `scopes` | Requested scopes |
| `redirectUrl` | Loopback callback; an empty value picks a free port |
| `audience` | Optional audience parameter |

The authorization code flow opens the default browser and listens on a
loopback address for the callback. Its access token stays in memory for the
session and is refreshed with the refresh token when one is returned. Tokens
are never written to the workspace or to request history.

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
