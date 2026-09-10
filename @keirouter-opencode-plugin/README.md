# @keirouter/opencode-plugin

Minimal OpenCode plugin for KeiRouter.

It only fetches `GET /v1/models`, caches the result in memory, and exposes the
models to OpenCode. It does not call combo, auto-combo, pricing, or MCP endpoints.

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [
    ["@keirouter/opencode-plugin", { "baseURL": "http://127.0.0.1:20180" }]
  ]
}
```

For a local KeiRouter Docker container, run the repository installer instead:

```sh
./build-opencode-plugin.sh
```

It installs the global local plugin, securely signs in to the local dashboard,
creates an OpenCode-specific gateway key when one is not already valid, and
saves it in OpenCode's `auth.json`. It prompts for the dashboard password; set
`KEIROUTER_DASHBOARD_PASSWORD` when running non-interactively. The installer
only accepts a loopback HTTP `KEIROUTER_URL` (default
`http://127.0.0.1:20180`) so it never sends that password to a remote host.

Options:

- `providerId` default `keirouter`
- `displayName` default `KeiRouter`
- `baseURL` default `http://127.0.0.1:20180`
- `modelCacheTtl` default `300000`
