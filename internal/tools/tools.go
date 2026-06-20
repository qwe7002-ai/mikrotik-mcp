// Package tools wires RouterOS API operations into MCP tool handlers.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-routeros/routeros/v3"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
	"github.com/qwe7002/mikrotik-mcp/internal/rosclient"
)

// Register attaches all mikrotik-mcp tools to the MCP server.
func Register(s *server.MCPServer) {
	h := &handlers{}

	s.AddTool(mcp.NewTool("mikrotik_help",
		mcp.WithDescription("Show usage guide for mikrotik-mcp: available tools, RouterOS API word syntax, common examples, and the list of blocked commands. Call this first if you are unsure how to drive the device."),
		mcp.WithString("topic", mcp.Description("Optional topic filter: 'security', 'tools', 'syntax', 'examples', 'blocked', or 'all' (default).")),
	), h.help)

	s.AddTool(mcp.NewTool("mikrotik_profiles",
		mcp.WithDescription("List the names of saved connection profiles (configured by the user via `mikrotik-mcp tui`). Returns only non-sensitive metadata (name, host, user, port, tls) — never passwords. Pass a returned name as the 'profile' argument to other tools to connect without inline credentials."),
	), h.profiles)

	commonTarget := []mcp.ToolOption{
		mcp.WithString("profile", mcp.Description("Name of a saved connection profile (configured by the user via `mikrotik-mcp tui`). When set, its host/user/password and TLS settings are used so credentials stay out of the conversation. Inline fields below override matching profile fields. Use mikrotik_profiles to list available profile names.")),
		mcp.WithString("host", mcp.Description("RouterOS device hostname or IP. Required unless 'profile' supplies it. SENSITIVE — do NOT echo, log, or transmit to any third-party/cloud service; use only for this local API call.")),
		mcp.WithString("user", mcp.Description("API username. Required unless 'profile' supplies it. SENSITIVE — credential material; do NOT log, summarize, or send to any cloud/third-party service.")),
		mcp.WithString("password", mcp.Description("API password (omit if empty or provided by 'profile'). SECRET — never echo back to the user, never include in summaries, never write to files, never send to any cloud or external tool. Treat as write-only.")),
		mcp.WithNumber("port", mcp.Description("API port. Default 8728 (8729 for TLS).")),
		mcp.WithBoolean("use_tls", mcp.Description("Use api-ssl. Default false. Strongly recommended for any non-loopback host so credentials are not sent in plaintext.")),
		mcp.WithBoolean("tls_skip_verify", mcp.Description("Skip TLS cert verification. Default false.")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Per-call timeout. Default 30.")),
	}

	with := func(extra ...mcp.ToolOption) []mcp.ToolOption {
		return append(append([]mcp.ToolOption{}, commonTarget...), extra...)
	}

	s.AddTool(mcp.NewTool("mikrotik_test_connection",
		with(
			mcp.WithDescription("Verify that the connection works: dial the device, log in, and read non-destructive identity/resource info. Use this to confirm a saved 'profile' or inline credentials are valid before running other commands. Returns the device identity, RouterOS version, board name, and uptime. Performs no changes."),
		)...,
	), h.testConnection)

	s.AddTool(mcp.NewTool("mikrotik_move",
		with(
			mcp.WithDescription("Reorder an item within an ordered list (e.g. ip/firewall/filter, ip/firewall/nat, ip/firewall/mangle, queue/simple). Moves the item with the given .id to sit just before 'destination'; omit 'destination' to move it to the end. Order is significant for firewall/NAT rules."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Menu path of the ordered list, e.g. 'ip/firewall/filter'.")),
			mcp.WithString("id", mcp.Required(), mcp.Description("The .id of the item to move (e.g. '*3').")),
			mcp.WithString("destination", mcp.Description("Optional .id to move the item before. Omit to move the item to the end of the list.")),
		)...,
	), h.move)

	s.AddTool(mcp.NewTool("mikrotik_command",
		with(
			mcp.WithDescription("Run a raw RouterOS API sentence. Provide the command path and an optional list of API words (e.g. =name=value, ?disabled=true, =.proplist=name,address). Returns parsed reply rows plus the !done sentence."),
			mcp.WithString("command", mcp.Required(), mcp.Description("API command path, e.g. /interface/print, /ip/address/add, /system/reboot.")),
			mcp.WithArray("words", mcp.Description("Additional API words. Each entry is a raw API word like '=name=ether1', '?disabled=true', or '=.proplist=name,address'. Equivalent to extra arguments passed to client.Run().")),
		)...,
	), h.command)

	s.AddTool(mcp.NewTool("mikrotik_print",
		with(
			mcp.WithDescription("Convenience for '/<path>/print'. Optional 'where' map adds ?key=value filters; 'proplist' limits returned properties."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Menu path WITHOUT trailing /print, e.g. 'interface', 'ip/address', 'system/resource'.")),
			mcp.WithObject("where", mcp.Description("Optional filter as a JSON object of {key:value}. Each entry becomes a ?key=value query word.")),
			mcp.WithArray("proplist", mcp.Description("Optional list of property names to include in the response (=.proplist=...).")),
		)...,
	), h.print)

	s.AddTool(mcp.NewTool("mikrotik_add",
		with(
			mcp.WithDescription("Add a new item under a menu path. 'props' is a JSON object of {key:value} converted to =key=value words. Returns the new .id."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Menu path, e.g. 'ip/address', 'ip/firewall/filter'.")),
			mcp.WithObject("props", mcp.Required(), mcp.Description("Properties for the new item as a JSON object.")),
		)...,
	), h.add)

	s.AddTool(mcp.NewTool("mikrotik_set",
		with(
			mcp.WithDescription("Set properties on an existing item identified by .id (or numbers/name where the path supports it)."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Menu path, e.g. 'ip/address'.")),
			mcp.WithString("id", mcp.Required(), mcp.Description("Item identifier (.id value like '*1', or 'numbers'/'name' where supported).")),
			mcp.WithObject("props", mcp.Required(), mcp.Description("Properties to set as a JSON object.")),
		)...,
	), h.set)

	s.AddTool(mcp.NewTool("mikrotik_remove",
		with(
			mcp.WithDescription("Remove an item identified by .id under a menu path."),
			mcp.WithString("path", mcp.Required(), mcp.Description("Menu path, e.g. 'ip/address'.")),
			mcp.WithString("id", mcp.Required(), mcp.Description("Item .id to remove.")),
		)...,
	), h.remove)

	s.AddTool(mcp.NewTool("mikrotik_enable",
		with(
			mcp.WithDescription("Enable an item by .id under a menu path."),
			mcp.WithString("path", mcp.Required()),
			mcp.WithString("id", mcp.Required()),
		)...,
	), h.enable)

	s.AddTool(mcp.NewTool("mikrotik_disable",
		with(
			mcp.WithDescription("Disable an item by .id under a menu path."),
			mcp.WithString("path", mcp.Required()),
			mcp.WithString("id", mcp.Required()),
		)...,
	), h.disable)
}

type handlers struct{}

// --- helpers ---------------------------------------------------------------

func argMap(req mcp.CallToolRequest) map[string]any {
	if req.Params.Arguments == nil {
		return map[string]any{}
	}
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func argString(args map[string]any, k string) (string, bool) {
	v, ok := args[k]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func argInt(args map[string]any, k string, def int) int {
	v, ok := args[k]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return def
		}
		return i
	}
	return def
}

func argBool(args map[string]any, k string, def bool) bool {
	v, ok := args[k]
	if !ok {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func argStringSlice(args map[string]any, k string) []string {
	v, ok := args[k]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return s
	}
	return nil
}

func argObject(args map[string]any, k string) map[string]any {
	v, ok := args[k]
	if !ok {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		// RouterOS API expects yes/no for boolean fields.
		if x {
			return "yes"
		}
		return "no"
	case float64:
		// preserve integer-looking values without trailing .0
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, anyToString(e))
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(x, ",")
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// target builds a connection target from the tool arguments. If a 'profile'
// name is given, the saved profile provides the defaults; any inline argument
// (host, user, password, port, use_tls, tls_skip_verify, timeout_seconds)
// overrides the matching profile field.
func target(args map[string]any) (rosclient.Target, error) {
	var base config.Profile
	if name, ok := argString(args, "profile"); ok && strings.TrimSpace(name) != "" {
		cfg, err := config.Load()
		if err != nil {
			return rosclient.Target{}, fmt.Errorf("load profiles: %w", err)
		}
		p, found := cfg.Get(strings.TrimSpace(name))
		if !found {
			return rosclient.Target{}, fmt.Errorf("unknown profile %q (use mikrotik_profiles to list saved profiles)", name)
		}
		base = p
	}

	host := base.Host
	if v, ok := argString(args, "host"); ok && v != "" {
		host = v
	}
	if host == "" {
		return rosclient.Target{}, errors.New("missing 'host' (provide it inline or via a saved 'profile')")
	}

	user := base.User
	if v, ok := argString(args, "user"); ok && v != "" {
		user = v
	}
	if user == "" {
		return rosclient.Target{}, errors.New("missing 'user' (provide it inline or via a saved 'profile')")
	}

	pwd := base.Password
	if v, ok := argString(args, "password"); ok {
		pwd = v
	}

	timeout := base.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}

	return rosclient.Target{
		Host:          host,
		User:          user,
		Password:      pwd,
		Port:          argInt(args, "port", base.Port),
		UseTLS:        argBool(args, "use_tls", base.UseTLS),
		TLSSkipVerify: argBool(args, "tls_skip_verify", base.TLSSkipVerify),
		Timeout:       time.Duration(argInt(args, "timeout_seconds", timeout)) * time.Second,
	}, nil
}

func errorResult(format string, a ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf(format, a...))
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult("encode result: %v", err)
	}
	return mcp.NewToolResultText(string(b))
}

// normalizePath strips surrounding '/' then re-prefixes a single '/'.
func normalizePath(p string) string {
	return "/" + strings.Trim(p, "/")
}

// blockedCommands lists API command words that this MCP refuses to send.
// Disruptive lifecycle operations stay off-limits to the LLM to prevent
// accidental device outages or factory wipes.
var blockedCommands = []string{
	"/system/reboot",
	"/system/shutdown",
	"/system/reset-configuration",
}

// checkBlocked returns an error if the (already-normalized) command path
// matches a blocked operation. Matches the leaf command exactly and also
// any sub-path under it (e.g. /system/reset-configuration/anything).
func checkBlocked(cmd string) error {
	c := strings.ToLower(strings.TrimRight(cmd, "/"))
	for _, b := range blockedCommands {
		if c == b || strings.HasPrefix(c, b+"/") {
			return fmt.Errorf("command %q is disabled by mikrotik-mcp policy (reboot/shutdown/reset-configuration are blocked)", cmd)
		}
	}
	return nil
}

func formatReply(r *routeros.Reply) map[string]any {
	rows := make([]map[string]string, 0, len(r.Re))
	for _, s := range r.Re {
		// Copy Map so callers see a flat key/value snapshot.
		m := make(map[string]string, len(s.Map))
		for k, v := range s.Map {
			m[k] = v
		}
		rows = append(rows, m)
	}
	out := map[string]any{
		"rows":  rows,
		"count": len(rows),
	}
	if r.Done != nil && len(r.Done.Map) > 0 {
		out["done"] = r.Done.Map
	}
	return out
}

// run dials the device, executes a single command, and returns the reply.
func runOnce(ctx context.Context, t rosclient.Target, sentence []string) (*routeros.Reply, error) {
	if len(sentence) == 0 {
		return nil, fmt.Errorf("empty sentence")
	}
	if err := checkBlocked(sentence[0]); err != nil {
		return nil, err
	}
	c, err := rosclient.Dial(ctx, t)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.RunArgsContext(ctx, sentence)
}

// --- handlers --------------------------------------------------------------

func (h *handlers) help(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topic := strings.ToLower(strings.TrimSpace(func() string {
		s, _ := argString(argMap(req), "topic")
		return s
	}()))
	if topic == "" {
		topic = "all"
	}

	sections := map[string]string{
		"security": `SECURITY — credentials are sensitive:
  * host / user / password identify and unlock a network device. Treat them
    like SSH keys or API tokens.
  * DO NOT echo passwords back to the user, include them in chat summaries,
    write them to files, paste them into PR descriptions, commit messages,
    bug reports, or any artifact that may be uploaded.
  * DO NOT forward credentials to any cloud service, external API, web
    search, code-execution sandbox, or other MCP server. They are scoped to
    THIS local mikrotik-mcp call only.
  * Prefer use_tls=true for any non-loopback host so the password is not
    sent in plaintext over the wire.
  * If the user asks you to "remember" or "save" the password, refuse and
    suggest they store it in their own secrets manager instead.`,

		"tools": `Available tools. Connection args are shared: either pass a saved
'profile' name OR inline host/user/password/port/use_tls/tls_skip_verify/timeout_seconds
(inline fields override the profile). Users manage profiles with ` + "`mikrotik-mcp tui`" + `.
  mikrotik_help     - this guide
  mikrotik_profiles - list saved connection profile names (no passwords)
  mikrotik_test_connection - dial + login, return identity/version (no changes)
  mikrotik_command  - raw API: command path + words[] (=k=v, ?k=v, =.proplist=...)
  mikrotik_print    - <path>/print with optional where{} filter and proplist[]
  mikrotik_add      - <path>/add with props{}; returns new .id in "id"
  mikrotik_set      - <path>/set =.id=<id> + props{}
  mikrotik_remove   - <path>/remove =.id=<id>
  mikrotik_enable   - <path>/enable =.id=<id>
  mikrotik_disable  - <path>/disable =.id=<id>
  mikrotik_move     - <path>/move =numbers=<id> [=destination=<id>] (reorder rules)`,

		"syntax": `RouterOS API word syntax (used in mikrotik_command "words"):
  =key=value   set a property (e.g. =name=ether1, =address=192.168.1.1/24)
  ?key=value   query/filter on print (e.g. ?disabled=true)
  =.proplist=a,b,c   limit returned properties on print
  =.id=*1      target an existing item by id
JSON value coercion in props/where:
  bool true/false -> "yes"/"no" (RouterOS expects yes/no)
  array ["a","b"] -> "a,b" (comma-joined)
  numbers passed through verbatim`,

		"examples": `Examples (omitting host/user/password for brevity):
  mikrotik_print  path="system/resource"
  mikrotik_print  path="interface"  proplist=["name","type","running"]
  mikrotik_print  path="ip/address" where={"interface":"ether1"}
  mikrotik_add    path="ip/address" props={"address":"192.168.88.1/24","interface":"ether1"}
  mikrotik_set    path="ip/address" id="*1" props={"comment":"lan"}
  mikrotik_disable path="ip/firewall/filter" id="*3"
  mikrotik_move   path="ip/firewall/filter" id="*5" destination="*1"
  mikrotik_test_connection profile="home-router"
  mikrotik_remove path="ip/address" id="*1"
  mikrotik_command command="/ip/route/print" words=["?dst-address=0.0.0.0/0"]
  mikrotik_command command="/interface/ethernet/monitor" words=["=numbers=ether1","=once="]`,

		"blocked": "Blocked commands (will return a policy error):\n  " +
			strings.Join(blockedCommands, "\n  ") +
			"\nThese cover the literal command and any sub-path (e.g. /system/reboot/...).",
	}

	order := []string{"security", "tools", "syntax", "examples", "blocked"}
	var body strings.Builder
	body.WriteString("mikrotik-mcp help\n=================\n\n")
	if topic == "all" {
		for _, k := range order {
			body.WriteString("## " + k + "\n" + sections[k] + "\n\n")
		}
	} else if s, ok := sections[topic]; ok {
		body.WriteString("## " + topic + "\n" + s + "\n")
	} else {
		return errorResult("unknown topic %q (use one of: security, tools, syntax, examples, blocked, all)", topic), nil
	}
	return mcp.NewToolResultText(body.String()), nil
}

// profiles lists saved connection profiles without exposing passwords.
func (h *handlers) profiles(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return errorResult("load profiles: %v", err), nil
	}
	type entry struct {
		Name        string `json:"name"`
		Host        string `json:"host"`
		User        string `json:"user"`
		Port        int    `json:"port,omitempty"`
		UseTLS      bool   `json:"use_tls"`
		HasPassword bool   `json:"has_password"`
	}
	list := make([]entry, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		list = append(list, entry{
			Name:        p.Name,
			Host:        p.Host,
			User:        p.User,
			Port:        p.Port,
			UseTLS:      p.UseTLS,
			HasPassword: p.Password != "",
		})
	}
	path, _ := config.Path()
	return jsonResult(map[string]any{
		"profiles": list,
		"count":    len(list),
		"path":     path,
		"hint":     "Pass a 'name' value as the 'profile' argument to other tools. Manage profiles with `mikrotik-mcp tui`.",
	}), nil
}

// testConnection dials and logs in, then reads identity/resource info to prove
// the credentials work. It makes no changes to the device.
func (h *handlers) testConnection(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	t, err := target(argMap(req))
	if err != nil {
		return errorResult("%v", err), nil
	}
	c, err := rosclient.Dial(ctx, t)
	if err != nil {
		return jsonResult(map[string]any{
			"connected": false,
			"address":   t.Address(),
			"error":     err.Error(),
		}), nil
	}
	defer c.Close()

	info := map[string]any{
		"connected": true,
		"address":   t.Address(),
		"use_tls":   t.UseTLS,
	}
	// Best-effort enrichment; a login that succeeds is already proof of life.
	if r, err := c.RunArgsContext(ctx, []string{"/system/identity/print"}); err == nil && len(r.Re) > 0 {
		if name, ok := r.Re[0].Map["name"]; ok {
			info["identity"] = name
		}
	}
	if r, err := c.RunArgsContext(ctx, []string{"/system/resource/print"}); err == nil && len(r.Re) > 0 {
		m := r.Re[0].Map
		for _, k := range []string{"version", "board-name", "uptime", "architecture-name"} {
			if v, ok := m[k]; ok {
				info[strings.ReplaceAll(k, "-", "_")] = v
			}
		}
	}
	return jsonResult(info), nil
}

// move reorders an item within an ordered list using RouterOS /move.
func (h *handlers) move(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := argMap(req)
	t, err := target(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	p, ok := argString(args, "path")
	if !ok || p == "" {
		return errorResult("missing 'path'"), nil
	}
	id, ok := argString(args, "id")
	if !ok || id == "" {
		return errorResult("missing 'id'"), nil
	}
	sentence := []string{normalizePath(p) + "/move", "=numbers=" + id}
	if dest, ok := argString(args, "destination"); ok && strings.TrimSpace(dest) != "" {
		sentence = append(sentence, "=destination="+dest)
	}
	r, err := runOnce(ctx, t, sentence)
	if err != nil {
		return errorResult("api: %v", err), nil
	}
	return jsonResult(formatReply(r)), nil
}

func (h *handlers) command(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := argMap(req)
	t, err := target(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	cmd, ok := argString(args, "command")
	if !ok || cmd == "" {
		return errorResult("missing 'command'"), nil
	}
	sentence := []string{normalizePath(cmd)}
	sentence = append(sentence, argStringSlice(args, "words")...)

	r, err := runOnce(ctx, t, sentence)
	if err != nil {
		return errorResult("api: %v", err), nil
	}
	return jsonResult(formatReply(r)), nil
}

func (h *handlers) print(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := argMap(req)
	t, err := target(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	p, ok := argString(args, "path")
	if !ok || p == "" {
		return errorResult("missing 'path'"), nil
	}
	sentence := []string{normalizePath(p) + "/print"}
	if pl := argStringSlice(args, "proplist"); len(pl) > 0 {
		sentence = append(sentence, "=.proplist="+strings.Join(pl, ","))
	}
	if where := argObject(args, "where"); where != nil {
		for k, v := range where {
			sentence = append(sentence, "?"+k+"="+anyToString(v))
		}
	}

	r, err := runOnce(ctx, t, sentence)
	if err != nil {
		return errorResult("api: %v", err), nil
	}
	return jsonResult(formatReply(r)), nil
}

func (h *handlers) add(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := argMap(req)
	t, err := target(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	p, ok := argString(args, "path")
	if !ok || p == "" {
		return errorResult("missing 'path'"), nil
	}
	props := argObject(args, "props")
	if len(props) == 0 {
		return errorResult("missing or empty 'props'"), nil
	}
	sentence := []string{normalizePath(p) + "/add"}
	for k, v := range props {
		sentence = append(sentence, "="+k+"="+anyToString(v))
	}
	r, err := runOnce(ctx, t, sentence)
	if err != nil {
		return errorResult("api: %v", err), nil
	}
	out := formatReply(r)
	if r.Done != nil {
		if id, ok := r.Done.Map["ret"]; ok {
			out["id"] = id
		}
	}
	return jsonResult(out), nil
}

func (h *handlers) mutate(ctx context.Context, args map[string]any, suffix string, includeProps bool) (*mcp.CallToolResult, error) {
	t, err := target(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	p, ok := argString(args, "path")
	if !ok || p == "" {
		return errorResult("missing 'path'"), nil
	}
	id, ok := argString(args, "id")
	if !ok || id == "" {
		return errorResult("missing 'id'"), nil
	}
	sentence := []string{normalizePath(p) + suffix, "=.id=" + id}
	if includeProps {
		props := argObject(args, "props")
		if len(props) == 0 {
			return errorResult("missing or empty 'props'"), nil
		}
		for k, v := range props {
			sentence = append(sentence, "="+k+"="+anyToString(v))
		}
	}
	r, err := runOnce(ctx, t, sentence)
	if err != nil {
		return errorResult("api: %v", err), nil
	}
	return jsonResult(formatReply(r)), nil
}

func (h *handlers) set(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.mutate(ctx, argMap(req), "/set", true)
}

func (h *handlers) remove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.mutate(ctx, argMap(req), "/remove", false)
}

func (h *handlers) enable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.mutate(ctx, argMap(req), "/enable", false)
}

func (h *handlers) disable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return h.mutate(ctx, argMap(req), "/disable", false)
}
