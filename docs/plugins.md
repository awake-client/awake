# Plugins

Plugins extend Awake with request hooks, dynamic variables, authentication
types, import and export formats, commands, response viewers and request tabs.
A plugin is a git repository; a marketplace is a git repository that lists
plugins. Both are fetched with the `git` command, so credential helpers and SSH
keys work as for any other repository.

[`examples/plugins/hmac-signer`](../examples/plugins/hmac-signer) uses every
kind of contribution, and [`examples/marketplace`](../examples/marketplace)
lists it.

## Using plugins

Open **Plugins** in the sidebar.

- **Marketplaces** adds a marketplace by the git URL of its repository, lists
  its plugins and installs them. **Install from a repository** takes a plugin
  repository URL and a tag or branch instead.
- Before anything is installed, Awake downloads the exact commit and shows its
  repository, commit, permissions and contributions. Installing pins that
  commit in `awake.yaml`, so the whole team gets the same version through git.
- **Installed** lists the plugins of the workspace and what each one needs on
  this computer.

### Teammates enable plugins themselves

A plugin pulled with `awake.yaml` does not run until each user reviews and
enables it on their own computer: otherwise a change to a shared file could
run any code with their credentials. The approval covers one commit and its
permissions. Updating the plugin, or a new permission, asks again.

Approvals live outside every workspace, in the user configuration directory
(`awake/plugins/trust.yaml`), where git cannot write. **Disable** removes the
approval on this computer and keeps the plugin for the team; **Uninstall**
removes it from `awake.yaml`.

| State | Meaning |
| --- | --- |
| Enabled | Approved on this computer; it runs. |
| Needs approval | Downloaded, waiting for **Review**. |
| Not downloaded | Declared in `awake.yaml`; **Download** fetches the commit to review it. |
| Needs a newer Awake | The manifest asks for a newer `engine`. |
| Invalid | The commit holds no valid plugin, or another plugin than declared. |
| Failed to load | Approved, but its bundle threw or registered handlers its manifest does not declare. |

### Settings

Shared settings are saved in `awake.yaml` when they differ from their default,
so a new default reaches the team on update. Secret settings stay in the
keychain of each computer and never reach the workspace.

### Updates

**Update** looks at the ref the marketplace lists today, or at the tag or
branch the plugin was installed from, and shows the review again when a newer
commit exists.

## Workspace declaration

```yaml
marketplaces:
  - name: team-plugins
    repository: https://github.com/your-team/awake-marketplace.git
plugins:
  - id: hmac-signer
    marketplace: team-plugins
    repository: https://github.com/your-team/awake-hmac-signer.git
    ref: v1.0.0
    commit: 3f2a9c1e0b7d4a5f6e8c9d0a1b2c3d4e5f6a7b8c
    settings:
      header: X-Signature
```

`commit` is the full hash Awake runs. `ref` and `marketplace` only serve to
look for updates. Plugins run in the order of this list.

Repository URLs are `https://`, `ssh://`, `git@host:path` or, for plugins under
development, `file://`.

## Writing a plugin

A plugin repository holds `awake-plugin.yaml` at its root and one JavaScript
bundle:

```
awake-plugin.yaml
dist/plugin.js
ui/viewer.html        optional view pages
```

The bundle runs in Awake's JavaScript runtime, the one request scripts use. It
has no module loader, file system or network of its own: bundle dependencies
into one file, and use the `awake` object for the rest. Symbolic links are not
extracted.

To try a plugin while writing it, commit it in a local repository and install
it from `file:///path/to/repository`. A new commit is picked up with
**Update**.

### Manifest

```yaml
version: 1
id: hmac-signer                # lower case letters, digits and dashes
name: HMAC Signer
description: Signs requests.
pluginVersion: 1.0.0           # semantic version
engine: ">=0.11.0"             # oldest Awake it runs on, optional
main: dist/plugin.js
permissions: [network]
settings:
  - { key: secret, type: secret, label: Signing secret }
  - { key: region, type: select, label: Region, options: [eu, us], default: eu }
contributes:
  hooks: [beforeRequest, afterResponse]
  dynamicVariables: [{ name: $requestId, description: Random identifier }]
  authentication:
    - type: hmac
      label: HMAC
      fields: [{ key: keyId, type: string, label: Key id }]
  importFormats: [{ format: urls, label: URL list, extensions: [.txt] }]
  exportFormats: [{ format: urls, label: URL list, extensions: [.txt] }]
  commands: [{ id: show-settings, title: "HMAC Signer: show settings" }]
  responseViewers:
    - { id: outline, title: Outline, contentTypes: [application/json], view: ui/outline.html }
  requestTabs:
    - { id: signature, title: Signature, view: ui/signature.html }
```

Unknown fields are refused, so a misspelt key fails loudly instead of silently
dropping a contribution. Paths are relative to the repository and may not leave
it.

| Setting type | Control | Stored in |
| --- | --- | --- |
| `string` | text field | `awake.yaml` |
| `boolean` | checkbox, `"true"` or `"false"` | `awake.yaml` |
| `select` | one of `options` | `awake.yaml` |
| `secret` | password field | keychain of each computer |

### Permissions

| Permission | Grants |
| --- | --- |
| `network` | `awake.http.fetch`. Without it, the function does not exist. |
| `secrets` | Hooks see the variables read from the keychain. Without it, they are left out of `context.variables`. |
| `workspace:write` | Reserved for plugins that change workspace files. |
| `clipboard` | Reserved for plugins that write to the clipboard. |

A plugin always reads its own secret settings.

### Registering handlers

The bundle calls `awake.register` once. Every contribution of the manifest
needs a handler, and every handler needs a contribution: a bundle that does
not match its manifest fails to load. What the user approved is therefore all
the plugin does.

```js
awake.register({
  beforeRequest(context) {},
  afterResponse(context) {},
  dynamicVariables: { $requestId: (context) => "…" },
  authentication: { hmac(context) {} },
  importFormats: { urls: (context) => [/* collections */] },
  exportFormats: { urls: (context) => "document" },
  commands: { "show-settings"(context) {} },
  views: { signature: { canonical: (context) => ({}) } },
});
```

Handlers may be `async`, but the runtime has no timer or event loop: a promise
must settle with what is already available. Each call is bounded by five
seconds, not counting the time spent in `awake.http.fetch`. Loading the bundle
is bounded the same way.

Every handler receives a `context` with `settings`, the merged values of the
plugin settings, plus:

| Handler | Context | Effect |
| --- | --- | --- |
| `beforeRequest` | `request`, `variables` | Runs before the pre-request script of each request. Changes to `request.method`, `url`, `headers`, `query` and `body`, and to `variables`, apply to the request. |
| `afterResponse` | `request`, `response`, `variables` | Runs before the post-response script, which sees the variables it sets. |
| `dynamicVariables.$name` | none | Returns the value of `{{$name}}`, computed once per request. |
| `authentication.type` | `request`, `fields` | Runs on the final request just before it is sent. `request` has `method`, `url`, `headers` and `body`; the handler may change `url` and `headers`, not the server. `fields` holds the values the request sets, variables resolved. |
| `importFormats.format` | `fileName`, `content` | Returns collections. |
| `exportFormats.format` | `collections` | Returns the document as a string. |
| `commands.id` | none | Runs from the command palette; its logs are shown there. |
| `views.view.handler` | `payload` | Answers a call of a view page; see [Views](#views). |

`request` and `response` have the shape request scripts use:
`response.status`, `statusText`, `headers`, `body`, `json()`, `cookies`.

A hook that throws is reported in the script logs of the response, prefixed
with the plugin identifier, and the request goes on. An authentication handler
that throws stops the request, since sending it unsigned would hide the
problem.

### The awake object

| Member | Purpose |
| --- | --- |
| `awake.register(handlers)` | Registers the handlers, once. |
| `awake.settings` | The settings, as in `context.settings`. |
| `awake.plugin.id`, `awake.plugin.version` | The manifest identity. |
| `awake.log(...values)`, `console.log/info/warn/error` | Lines shown in the script logs or the command palette. |
| `awake.crypto.sha256(text)` | Hexadecimal SHA-256. |
| `awake.crypto.hmacSha256(key, text, keyEncoding)` | Hexadecimal HMAC-SHA256. `keyEncoding` `"hex"` reads the key as hexadecimal, to chain derived keys. |
| `awake.encoding.base64Encode(text)`, `base64Decode(text)` | Base64 of UTF-8 text. |
| `awake.http.fetch(url, { method, headers, body })` | With `network` only. Synchronous; returns a response like `context.response`. `http` and `https` only, 30 seconds, 10 MB of body. |

### Collections in import and export formats

Formats exchange a list of collections whose requests use the fields of the
request files described in [file-format.md](file-format.md):

```json
[
  {
    "name": "Platform API",
    "documentation": "…",
    "variables": { "baseUrl": "https://api.example.com" },
    "authentication": { "type": "bearer", "token": "{{token}}" },
    "requests": [
      { "name": "List users", "request": { "method": "GET", "url": "{{baseUrl}}/users" } }
    ],
    "folders": [{ "name": "Admin", "requests": [], "folders": [] }],
    "environments": [
      { "name": "Production", "variables": { "baseUrl": "…" }, "secretVariables": ["token"] }
    ]
  }
]
```

On import, unknown fields are refused and a request Awake cannot store is
skipped with a warning. Formats appear in **Import / Export** as
`plugin:<plugin id>/<format>`; an export writes one file with the first
extension of the format.

### Views

Response viewers and request tabs are HTML pages of the plugin repository.
Awake serves them only for an enabled plugin and renders them in a frame
sandboxed without same-origin access, under a policy that forbids any network
request, form or nested frame. A page cannot reach Awake, its storage or the
network: it talks to the interface with `postMessage`.

Keep a page self-contained, with inline scripts and styles. Images may be
`data:` URLs.

1. The page announces itself:
   `window.parent.postMessage({ type: "awake:ready" }, "*")`.
2. Awake answers, and again whenever the data or the theme changes, with
   `{ type: "awake:context", theme: "light" | "dark", kind, data }`.
   - A response viewer receives `status`, `statusText`, `headers`,
     `contentType`, `encoding`, `size`, `body` and `complete`. A binary body
     (`encoding: "base64"`) is the base64 preview Awake keeps; `complete` tells
     whether the whole body is there.
   - A request tab receives `path` and `request`, the request as edited, with
     its variables unresolved. It is read only.
3. The page may call the handlers its plugin registers under
   `views.<view id>`:
   `{ type: "awake:invoke", id: 1, handler: "canonical", payload: {…} }`.
   Awake answers `{ type: "awake:result", id: 1, result }` or
   `{ type: "awake:error", id: 1, message }`. The handler runs in the plugin
   runtime with the approved permissions; the payload and the result are JSON.

A response viewer is offered when the media type of the response matches one
of its `contentTypes`: an exact type such as `application/x-protobuf`, a
`type/*` wildcard, or `*`.

## Publishing

1. Push the plugin repository and tag a release, such as `v1.0.0`. Bump
   `pluginVersion` with it.
2. List it in a marketplace repository, at the root of which lives
   `marketplace.yaml`:

```yaml
version: 1
name: team-plugins             # lower case letters, digits and dashes
description: Plugins maintained by the team.
plugins:
  - id: hmac-signer
    repository: https://github.com/your-team/awake-hmac-signer.git
    ref: v1.0.0                 # tag or branch; empty for the default branch
    description: Signs requests with an HMAC-SHA256.
    tags: [authentication]
```

Workspaces using the marketplace see the new ref after **Refresh catalogs**,
and **Update** offers it.

## Security model

- Nothing runs before the user approves the exact commit and its permissions
  on their computer; git can change `awake.yaml` but not the approvals.
- Each plugin has its own runtime, without modules, file system or network,
  and its calls are bounded in time. Memory is not bounded.
- Commits are extracted from `git archive` into a cache directory that is
  renamed once complete. Paths leaving the repository are refused, symbolic
  links are skipped and a commit may hold at most 10,000 files and 64 MB.
- `git` never runs a command named by an `ext::` URL, and refs that read like
  options are refused.
- View pages are sandboxed and policed as described in [Views](#views), and the
  application refuses runtime calls from sandboxed origins.
