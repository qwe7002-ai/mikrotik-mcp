# mikrotik-mcp

An [MCP](https://modelcontextprotocol.io) server for managing MikroTik RouterOS
devices through the binary API, plus a small terminal UI (TUI) for storing your
login details so they never have to be typed into the conversation.

## Build

```sh
go build -o mikrotik-mcp ./cmd/mikrotik-mcp
```

## Commands

```
mikrotik-mcp [serve]   Run the MCP server over stdio (default)
mikrotik-mcp tui       Manage saved connection profiles interactively
mikrotik-mcp version   Print version
mikrotik-mcp help      Show usage
```

## Login profiles (TUI)

Instead of passing `host` / `user` / `password` on every tool call, save a named
**profile** once and reference it by name. Launch the TUI:

```sh
mikrotik-mcp tui
```

Keys:

| Screen | Keys |
| ------ | ---- |
| List   | `↑/↓` move · `a` add · `e`/`enter` edit · `d` delete · `q` quit |
| Form   | `tab`/`↑↓` move between fields · `space` toggle TLS checkboxes · `enter` next/save · `ctrl+s` save · `esc` cancel |

Profiles are stored as JSON at `<os config dir>/mikrotik-mcp/profiles.json`
(e.g. `~/.config/mikrotik-mcp/profiles.json` on Linux). The file is written with
`0600` permissions because it contains credentials. Override the location with
the `MIKROTIK_MCP_CONFIG` environment variable.

> **Security:** the password is stored in plaintext on disk (protected by file
> permissions). Treat the config file like an SSH private key.

## Using profiles from the MCP tools

Every device tool accepts a `profile` argument. When set, the saved
host/user/password and TLS settings are used; any inline field still overrides
the matching profile field.

- `mikrotik_profiles` — list saved profile names and metadata (never passwords).
- `mikrotik_print`, `mikrotik_command`, `mikrotik_add`, … — pass
  `profile: "<name>"` instead of inline credentials.

Example (conceptual tool call):

```json
{ "tool": "mikrotik_print", "args": { "profile": "home-router", "path": "interface" } }
```

## Safety

Disruptive commands (`/system/reboot`, `/system/shutdown`,
`/system/reset-configuration`) are blocked by policy. Call `mikrotik_help` from
the MCP client for the full usage and security guide.
