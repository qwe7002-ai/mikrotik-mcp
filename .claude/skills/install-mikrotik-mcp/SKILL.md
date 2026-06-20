---
name: install-mikrotik-mcp
description: Install and set up the mikrotik-mcp server from this repository — build the Go binary, register it as an MCP server in Claude Code, and store MikroTik login credentials via the TUI. Use when the user wants to install, set up, configure, onboard, or "get started with" mikrotik-mcp, or to add a MikroTik/RouterOS device connection.
---

# Install mikrotik-mcp

Guide the user through installing this repository's MCP server end to end:
build → register with Claude Code → save login profile → verify. Run the steps
in order. Stop and report if a step fails; do not skip ahead.

## 0. Prerequisites

- Go **1.24+** (`go version`). The required version is in `go.mod`.
- The user's MikroTik device must have the **RouterOS API service enabled**
  (`/ip/service`: `api` on 8728, or `api-ssl` on 8729 for TLS). Mention this if a
  later connection test fails with a dial timeout.

If Go is missing, point the user to https://go.dev/dl/ and stop.

## 1. Build the binary

From the repository root:

```sh
go build -o mikrotik-mcp ./cmd/mikrotik-mcp
```

Verify it runs:

```sh
./mikrotik-mcp version
```

Capture the **absolute path** to the built binary (`realpath ./mikrotik-mcp` or
`pwd`) — the MCP registration in the next step needs it.

> Optional: to make it available system-wide, copy it onto the user's PATH
> (e.g. `install -m 0755 mikrotik-mcp ~/.local/bin/`) and use that path below.

## 2. Register as an MCP server in Claude Code

The server speaks MCP over **stdio** (the default `serve` subcommand). Register
it with the Claude Code CLI, using the absolute path from step 1:

```sh
claude mcp add mikrotik-mcp -- /ABSOLUTE/PATH/TO/mikrotik-mcp
```

- Add `--scope user` to make it available in all projects, or `--scope project`
  to share it with the team via `.mcp.json`. Default scope is `local`.
- If the `claude` CLI is unavailable, register it manually by adding this to the
  appropriate MCP config (`.mcp.json` in the project, or the user/global Claude
  config):

  ```json
  {
    "mcpServers": {
      "mikrotik-mcp": {
        "command": "/ABSOLUTE/PATH/TO/mikrotik-mcp",
        "args": ["serve"]
      }
    }
  }
  ```

Confirm registration with `claude mcp list` (the server should appear and report
as connected). The user may need to restart their Claude Code session for the
tools to load.

## 3. Save login credentials (TUI)

Credentials should NOT be typed into the conversation. Have the user run the
interactive TUI themselves in their terminal:

```sh
./mikrotik-mcp tui
```

In the TUI: press `a` to add a profile, fill in **Name / Host / User /
Password** (and `space` to toggle **Use TLS** for api-ssl), then `ctrl+s` to
save. Profiles are written to `~/.config/mikrotik-mcp/profiles.json` with `0600`
permissions (override the path with `MIKROTIK_MCP_CONFIG`).

**Security — do this yourself, not for the user:**
- Do not ask the user to paste their password into the chat. Direct them to
  enter it in the TUI, which masks the input.
- Never echo, log, summarize, or commit the password. Treat the profiles file
  like an SSH private key.
- Recommend `Use TLS` for any non-loopback device so credentials are not sent in
  plaintext.

## 4. Verify the connection

Once the server is registered and a profile is saved, verify from within Claude
Code using the MCP tools (no credentials in the prompt):

1. Call **`mikrotik_profiles`** — the saved profile name should be listed.
2. Call **`mikrotik_test_connection`** with `profile: "<name>"` — a healthy
   result returns `connected: true` plus the device identity, RouterOS version,
   board name, and uptime.

If `connected` is `false`, report the `error` field and check, in order: the API
service is enabled on the device, the host/port are reachable, TLS settings
match the service (8728 plain vs 8729 api-ssl), and the username/password.

## 5. Done

Summarize for the user: binary path, that the MCP server is registered, the
profile name(s) saved, and that they can now drive the device by passing
`profile: "<name>"` to the `mikrotik_*` tools. Point them at `mikrotik_help` for
the full tool reference. Remind them that reboot/shutdown/reset-configuration are
blocked by policy.
