# mikrotik-mcp

![build](https://github.com/qwe7002-ai/mikrotik-mcp/actions/workflows/build.yml/badge.svg)

An [MCP](https://modelcontextprotocol.io) server for managing MikroTik RouterOS
devices through the binary API, plus a small terminal UI (TUI) for storing your
login details so they never have to be typed into the conversation.

## Install

Using [Claude Code](https://claude.com/claude-code), the bundled
**`install-mikrotik-mcp`** skill walks you through the whole setup — building the
binary, registering it as an MCP server, saving a login profile via the TUI, and
verifying the connection. Just ask Claude to "install mikrotik-mcp". A second
skill, **`use-mikrotik-mcp`**, teaches the assistant how to operate the device
through the tools (profile-first connections, RouterOS API syntax, safe rule
editing) and loads automatically when it manages a RouterOS device.

Manual build:

```sh
go build -o mikrotik-mcp ./cmd/mikrotik-mcp
```

## Commands

```
mikrotik-mcp [serve]          Run the MCP server over stdio (default)
mikrotik-mcp control-center   Serve the MCP over HTTPS for multiple clients/devices
mikrotik-mcp tui              Manage saved connection profiles interactively
mikrotik-mcp version          Print version
mikrotik-mcp help             Show usage
```

## Control center (HTTPS)

`serve` mode talks to a single client over stdio. **Control-center mode** instead
serves the same tools over **HTTPS** (the MCP Streamable HTTP transport) so a
central instance can be reached by remote clients and drive **multiple RouterOS
devices** through the saved profiles.

```sh
export MIKROTIK_MCP_TOKEN="$(openssl rand -hex 24)"   # required bearer token
mikrotik-mcp control-center --addr :8443 --cert-host mcp.example.com
```

- Endpoint: `https://<host>:8443/mcp` — every request must send
  `Authorization: Bearer <token>`.
- Health probe: `https://<host>:8443/healthz` (no auth).
- TLS: pass `--tls-cert` and `--tls-key` for a real certificate. If omitted, an
  **ephemeral self-signed** cert is generated for `--cert-host` (clients must
  skip verification or pin it — fine behind a reverse proxy / for testing).

| Flag | Env | Default |
| ---- | --- | ------- |
| `--addr` | `MIKROTIK_MCP_ADDR` | `:8443` |
| `--token` | `MIKROTIK_MCP_TOKEN` | *(required)* |
| `--tls-cert` | `MIKROTIK_MCP_TLS_CERT` | self-signed |
| `--tls-key` | `MIKROTIK_MCP_TLS_KEY` | self-signed |
| `--cert-host` | `MIKROTIK_MCP_CERT_HOST` | `localhost` |

Configure it as a remote MCP server in your client (URL + bearer token). Select
the target device per call with the `profile` argument, or operate on the whole
fleet at once with **`mikrotik_multi_command`** (runs one command across many
profiles concurrently).

> Control-center mode exposes device control over the network. Always set a
> strong token, terminate TLS properly, and restrict who can reach the port.

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
- `mikrotik_test_connection` — dial + log in and return identity/version/uptime
  to verify a profile or inline credentials work (makes no changes).
- `mikrotik_move` — reorder an item in an ordered list (firewall/NAT/queue).
- `mikrotik_multi_command` — run one command across many profiles at once
  (control-center fan-out); pass `profiles: ["a","b"]` or `["*"]` for all.
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
