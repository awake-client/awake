# File format

Awake stores a workspace as plain YAML files inside a git repository. Files are
the source of truth; the SQLite database is an index that can be rebuilt from
them at any time.

## Workspace layout

```
awake.yaml                      workspace configuration, tracked by git
AGENT.md                        instructions for coding agents, tracked by git
environments/
  development.yaml              shared variables, tracked by git
  development.local.yaml        local overrides, git ignored
collections/
  platform/
    collection.yaml             collection metadata
    environments/
      development.yaml          refines the global environment of that name
      development.local.yaml    local overrides, git ignored
    users/
      folder.yaml               folder display name
      list-users.yaml           one file per request
local/                          private collections, same layout, git ignored
.awake/                         index and cache, git ignored
```

The application generates the `.gitignore` entries for `*.local.yaml`,
`local/` and `.awake/` when a workspace is created.

`AGENT.md` describes the workspace conventions to command line coding agents.
The Assistant writes a prefilled copy when the file is missing and asks the
agent to read it before changing the workspace. The file is tracked by git, so
the instructions travel with the workspace.

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

### Collection environments

A collection can declare its own environments in `<collection>/environments/`,
with the same format. A collection environment refines the global environment
of the same name: its variables win, and the variables it does not define are
inherited from the global one. A collection may also declare an environment
that has no global counterpart. The environment picker offers the global
environments and those of the collection of the open request (or of the
collection whose settings are open), never those of another collection.
Selecting an environment selects it at both levels. Secret values of a
collection environment are stored under `<collection path>:<environment name>`
so they never mix with global ones.

## Variables resolution order

From lowest to highest precedence:

1. workspace defaults (`awake.yaml`);
2. collection variables (`collection.yaml`);
3. global environment, then its local overrides and secrets;
4. collection environment of the same name, then its local overrides and
   secrets;
5. values set by the pre-request script.

### Dynamic variables

Placeholders starting with `$` are generated each time a request is sent, and
each occurrence receives its own value:

| Placeholder | Value |
| --- | --- |
| `{{$uuid}}` | random UUID v4 |
| `{{$timestamp}}` | Unix time in seconds |
| `{{$timestampMs}}` | Unix time in milliseconds |
| `{{$isoTimestamp}}` | current time in ISO 8601, UTC |
| `{{$date}}` | current date, `YYYY-MM-DD` |
| `{{$randomInt}}` | integer between 0 and 1000 |
| `{{$randomHex}}` | 16 hexadecimal characters |

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
| `scripts` | object | JavaScript hooks |
| `documentation` | string | Free-form notes |

### Scripts

A request may attach two JavaScript hooks. They run inside the application
with goja, synchronously, and are interrupted after five seconds.

```yaml
scripts:
  preRequest: |
    request.headers["X-Request-Id"] = "abc-123";
    awake.log("sending", request.method, request.url);
  postResponse: |
    awake.test("successful", response.status === 200);
    awake.environment.set("lastUserId", response.json().id);
```

| Field | Description |
| --- | --- |
| `preRequest` | Runs before the request is built. May mutate `request`. |
| `postResponse` | Runs after the response preview is captured. |

The `request` object exposes `method`, `url`, `headers`, `query` and `body`.
Mutating it changes the outgoing request: setting a header or a query
parameter adds it, deleting one removes it. The `response` object exposes
`status`, `statusText`, `headers`, `body`, `encoding` and `duration`, and
`response.json()` parses the body. Repeated headers are joined with `, `, so
read cookies from `response.cookies` (name to value) or
`response.cookieHeader` (`a=1; b=2`, ready for a `Cookie` request header)
rather than from `Set-Cookie`.

The `awake` helper provides:

| Function | Description |
| --- | --- |
| `awake.log(...values)` | Appends a line to the script logs |
| `awake.test(name, value)` | Records an assertion; `value` may be a boolean or a function |
| `awake.getVariable(name)` | Reads a resolved variable |
| `awake.setVariable(name, value)` | Sets a variable for the current request only |
| `awake.environment.get(name)` | Reads a resolved variable |
| `awake.environment.set(name, value)` | Saves a variable in the selected environment |
| `awake.environment.unset(name)` | Removes a variable saved in the selected environment |
| `awake.environments.get(name)` | Opens another environment by file or display name, with its own `get`, `set` and `unset` |

`console.log`, `info`, `debug`, `warn` and `error` also write to the script
logs.

```js
awake.environments.get("Production").set("cookies", response.cookieHeader);
```

`awake.setVariable` only applies to that execution. `awake.environment.set`
also saves the value for later requests, in the most specific environment of
the selected name: the collection's one when it declares it, otherwise the
global one. The value goes to the git ignored local overrides file, so tokens
never reach the repository, or to the keychain when the name is a declared
secret. Values that are not strings are stored as JSON. Without a selected
environment, nothing is saved and the response reports it.
`awake.environments.get(name)` follows the same rules for the environment it
opens, looking in the request's collection and in the workspace; its changes
are saved but do not affect the running request. A failing script does not abort the request; its error is
reported with the response. Post-response scripts see the response preview
(first 256 KiB), so assertions on a large body only cover that preview.

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

### Path parameters

A URL segment written `/:name` is a path parameter. Its value comes from
`pathParameters`; it may hold variables, is escaped for the path, so `a/b`
stays one segment, and a missing value stops the request instead of sending
`:name` as is. The colon must follow a slash, so `https://` and `:8080` are
not parameters.

```yaml
url: "{{baseUrl}}/articles/:articleId/publish"
pathParameters:
  - name: articleId
    value: "{{articleId}}"
```

Local overrides of the parameters cover the path parameters too.

### Body types

`none`, `json`, `text`, `xml`, `form-urlencoded`, `multipart`, `binary`.

```yaml
body:
  type: json
  content: |
    {"name": "Ada"}
```

Form bodies, `form-urlencoded` and `multipart`, list their fields instead of
a `content`. Each enabled field is encoded after its variables are resolved,
so a value holding `&` or `=` stays one field. A form written as text by an
earlier version, `a=1&b=2` or one `name: value` per line, is read into fields.

```yaml
body:
  type: form-urlencoded
  fields:
    - name: title
      value: "{{title}}"
      enabled: true
    - name: draft
      value: "yes"
      enabled: false
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

## Editor schemas

Awake writes a JSON schema for each kind of file to `.vscode/awake/` and
associates them in `.vscode/settings.json` under `yaml.schemas`, when a
workspace is created or opened. Editors using the YAML language server, such
as VS Code and Cursor with the YAML extension, then validate and complete the
files by hand. These associations take precedence over the public SchemaStore
catalog, whose unrelated `**/collections/*/*.yaml` pattern would otherwise
claim requests. Files are rewritten only when they change; a settings file with
comments is left untouched, and its associations must then be added by hand.

## Private requests and local overlays

- A private collection lives in `local/` with the same layout as
  `collections/`. It is ignored by git and only visible on the local machine.
- Local environment files override shared values without changing tracked
  files.
- A request may have local overrides in `<name>.local.yaml`, next to it and
  git ignored. Each section present replaces the shared one when the request
  is sent, even when empty; an absent section keeps the shared one. Moving or
  deleting the request carries the overrides with it.

```yaml
# collections/platform/users/list.local.yaml
parameters:
  - name: page
    value: "2"
    enabled: true
body:
  type: json
  content: '{"debug": true}'
```

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
