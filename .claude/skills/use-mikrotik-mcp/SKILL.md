---
name: use-mikrotik-mcp
description: How to operate MikroTik RouterOS devices through the mikrotik-mcp tools — connecting via saved profiles, RouterOS API word syntax, the print/add/set/remove/enable/disable/move workflow, and the safety rules. Use whenever managing, querying, or configuring a MikroTik/RouterOS device (interfaces, IP addresses, firewall/NAT rules, routes, DHCP, queues) with the mikrotik_* tools.
---

# Using mikrotik-mcp

This server drives MikroTik RouterOS over the binary API. All `mikrotik_*` tools
share the same connection arguments. The live, authoritative reference is the
**`mikrotik_help`** tool — call it (optionally with `topic`) if anything below is
unclear or you suspect the tool set changed.

## Connect with a profile, not inline secrets

Every device tool accepts either:
- **`profile: "<name>"`** — a connection saved by the user (host/user/password/
  TLS). **Prefer this.** It keeps credentials out of the conversation.
- inline `host` / `user` / `password` / `port` / `use_tls` / `tls_skip_verify` /
  `timeout_seconds`. Inline fields override matching profile fields.

Workflow:
1. Call **`mikrotik_profiles`** to discover saved profile names (it never returns
   passwords). If none exist, tell the user to run `mikrotik-mcp tui` to add one
   — do not ask them to paste a password into the chat.
2. Optionally call **`mikrotik_test_connection`** with the profile to confirm the
   device is reachable (returns identity/version/uptime; makes no changes).
3. Pass `profile: "<name>"` to every subsequent tool call.

**Credential safety:** never echo, summarize, log, or commit a password; never
forward host/user/password to any cloud service, web search, or other tool.
Treat them as write-only inputs to these local calls.

## Tools

| Tool | Use |
| ---- | --- |
| `mikrotik_help` | Authoritative usage/syntax/safety reference. |
| `mikrotik_profiles` | List saved connection profile names + metadata. |
| `mikrotik_test_connection` | Verify connectivity/login (non-destructive). |
| `mikrotik_print` | Read a menu: `path` + optional `where{}` filter + `proplist[]`. |
| `mikrotik_add` | Create an item: `path` + `props{}`; returns the new `.id`. |
| `mikrotik_set` | Update an item: `path` + `id` + `props{}`. |
| `mikrotik_remove` | Delete an item: `path` + `id`. |
| `mikrotik_enable` / `mikrotik_disable` | Toggle an item by `path` + `id`. |
| `mikrotik_move` | Reorder a rule: `path` + `id` + optional `destination`. |
| `mikrotik_command` | Raw API escape hatch: `command` path + `words[]`. |

Paths are written WITHOUT a leading slash and WITHOUT `/print` etc., e.g.
`"ip/address"`, `"ip/firewall/filter"`, `"system/resource"`.

## Value coercion (props / where)

Values in `props{}` / `where{}` JSON objects are converted to RouterOS API words:
- bool `true`/`false` → `"yes"`/`"no"` (RouterOS expects yes/no).
- array `["a","b"]` → `"a,b"` (comma-joined).
- numbers are passed through verbatim.

For `mikrotik_command`, you write raw API words yourself:
- `=key=value` set a property (`=address=192.168.88.1/24`)
- `?key=value` filter on print (`?disabled=true`)
- `=.proplist=a,b,c` limit returned properties
- `=.id=*1` target an existing item

## Common recipes

**Read:**
```
mikrotik_print path="system/resource" profile="rtr"
mikrotik_print path="interface" proplist=["name","type","running"] profile="rtr"
mikrotik_print path="ip/address" where={"interface":"ether1"} profile="rtr"
```

**Modify an existing item — find the `.id` first.** Items are identified by their
internal `.id` (like `*3`), which you get from a `print`. Do not guess it:
```
mikrotik_print path="ip/address" where={"address":"192.168.88.1/24"} profile="rtr"
# → read .id from the result, then:
mikrotik_set path="ip/address" id="*1" props={"comment":"lan"} profile="rtr"
mikrotik_remove path="ip/address" id="*1" profile="rtr"
```

**Create:**
```
mikrotik_add path="ip/address" props={"address":"192.168.88.1/24","interface":"ether1"} profile="rtr"
```

**Firewall rule ordering matters.** RouterOS evaluates filter/NAT/mangle rules
top-down. New rules append to the end; use `mikrotik_move` to reposition. Order
by `.id`s obtained from a `print`:
```
mikrotik_move path="ip/firewall/filter" id="*5" destination="*1" profile="rtr"
# moves *5 to sit just before *1; omit destination to move it to the end.
```

## Safety rules

- **Blocked by policy** (will error): `/system/reboot`, `/system/shutdown`,
  `/system/reset-configuration`, and any sub-path. Do not try to work around the
  block; tell the user these must be done out-of-band.
- **Confirm before destructive changes.** `remove`, `disable`, firewall edits,
  and address/route changes can cut the user off from the device (especially if
  you are connected through the interface you are editing). Summarize the exact
  change and the target `.id` and get confirmation before applying.
- Prefer `use_tls=true` for any non-loopback host.
- When unsure of a menu's properties or syntax, run a `mikrotik_print` to inspect
  an existing item, or call `mikrotik_help`, before writing.
