# MCP server

Awake includes a [Model Context Protocol](https://modelcontextprotocol.io)
server, so coding agents such as Claude Code can list, read, send, create and
update the requests of a workspace.

The server is local only. The agent starts it as a subprocess and talks to it
over stdin and stdout; no port is opened. It runs whether or not the Awake
window is open.

## Register it

The Assistant page shows the exact command for the open workspace. It has this
shape:

```sh
claude mcp add awake -- /Applications/Awake.app/Contents/MacOS/awake mcp --workspace ~/my-workspace
```

```
awake mcp [--workspace <directory>] [--read-only]
```

| Flag | Meaning |
| --- | --- |
| `--workspace` | Workspace directory. By default, the closest parent of the current directory that holds `awake.yaml`. |
| `--read-only` | Leaves out `create_request`, `update_request`, `create_folder` and `create_collection`. Requests can still be sent. |

## Turn it off

The Assistant page has an **Allow agents to use this workspace** switch. It is
on by default and saved in `.awake/mcp.yaml`, which git ignores, so each person
decides for their own computer:

```yaml
disabled: true
```

When it is off, `awake mcp` refuses to start for that workspace, and a server
already running answers every tool call with an error until it is turned back
on. No restart of the agent is needed either way.

## Conventions

- Paths are relative to the workspace and use `/`. A collection is
  `collections/<name>`, or `local/<name>` when private. A folder adds
  `/<folder>`, and a request is a `.yaml` file inside them.
- `environment` is an environment file name without extension, such as
  `staging`. When omitted, the `defaultEnvironment` of `awake.yaml` applies.
  An empty string sends without environment.
- Requests use the fields of their [YAML file](file-format.md), not Go names.
  Unknown fields are rejected.
- Errors come back as tool errors (`isError: true`) with a message saying what
  to do, such as which tool to call instead.

## Tools

### Read

| Tool | Input | Output |
| --- | --- | --- |
| `list_requests` | `path?`, `query?`, `limit?` (200) | `entries: [{path, name, kind, method}]`, `truncated`. `kind` is `collection`, `folder` or `request`. Without `query`, the tree under `path` is listed breadth first; with `query`, requests whose name, path or method hold every word. |
| `get_request` | `path` | `path`, `request` (file fields), `inheritedAuthentication?: {type, from}` when the request inherits it |
| `list_environments` | — | `environments: [{fileName, name, scope, private}]`, `default` |
| `get_variables` | `requestPath`, `environment?` | `environment`, `variables: [{name, value, source}]` |
| `list_history` | `requestPath?`, `limit?` (20) | `entries: [{id, requestPath, method, url, statusCode, durationMs, environment, executedAt}]` |
| `get_history_entry` | `id` | the entry and its `exchange`: request and response headers and bodies (64 KiB at most each) |

### Send

| Tool | Input | Output |
| --- | --- | --- |
| `send_request` | `path`, `environment?`, `maxBodyBytes?` (65536, at most 262144) | `environment`, `statusCode`, `status`, `headers`, `contentType`, `body` (text, or `null` when binary), `bodyTruncated`, `sizeBytes`, `durationMs`, `scripts: {preRequest?, postResponse?}` |

A request is sent exactly as with the Send button: variables, inherited
authentication, `authType` and `grantType` environment variables, OAuth 2
tokens and scripts apply, and the exchange is added to the history. The
authorization code grant needs a browser: authorize the request in Awake first.

### Write

| Tool | Input | Behavior |
| --- | --- | --- |
| `create_request` | `path`, `request` | Fails when the file exists. Missing folders are created. Returns the saved request, defaults included. |
| `update_request` | `path`, `request` | Replaces the whole request: read it with `get_request`, change it, send it back. Fails when the file does not exist. |
| `create_folder` | `path`, `name?` | Creates a folder in an existing collection or folder. |
| `create_collection` | `name`, `private?` | Creates `collections/<slug>`, or `local/<slug>` when private. |

Deleting and moving are not available: git and the Awake interface remain the
way to undo or reorganize.

## Resources

| URI | Content |
| --- | --- |
| `awake://guide` | The workspace conventions and file formats, as written to `AGENT.md`. |

## What stays private

- Secret values never leave Awake. `get_variables` shows `•••` for secrets and
  for variables whose name suggests a credential, such as `password`, `token`
  or `apiKey`.
- The headers and URL sent are not returned by `send_request`; the history
  keeps them with credentials masked.
- Values stored by scripts are reported by name only.
