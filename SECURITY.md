# Security policy

## Supported versions

Awake is in beta. Security fixes are provided for the latest release only.

| Version | Supported |
| --- | --- |
| latest release | yes |
| older releases | no |

## Reporting a vulnerability

Please do not open a public issue for a security problem. Report it privately
through GitHub's [private vulnerability reporting][advisories]:

1. Open the [Security tab][security] of the repository.
2. Click **Report a vulnerability**.
3. Describe the issue with enough detail to reproduce it.

Include, when possible:

- the Awake version and the operating system;
- the affected area (secrets, HTTP client, import/export, git integration,
  request scripts, updater, or the desktop shell);
- step-by-step reproduction and the expected versus observed behaviour;
- any proof of concept, logs or screenshots.

You will get an acknowledgement as soon as possible, and a fix or mitigation
plan once the report has been triaged. Please give us reasonable time to ship a
fix before disclosing the issue publicly.

## Scope

Areas of particular interest:

- handling of environment secrets and the operating system keychain;
- the HTTP, SSE and WebSocket clients;
- importers and exporters (Bruno, OpenCollection, OpenAPI, Postman);
- request scripts and the coding-agent integration;
- the update checker and the release download path;
- the macOS, Linux and Windows packaging and installers.

[advisories]: https://github.com/awake-client/awake/security/advisories/new
[security]: https://github.com/awake-client/awake/security
