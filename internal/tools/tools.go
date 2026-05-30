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

	"github.com/qwe7002/mikrotik-mcp/internal/rosclient"
)

// Register attaches all mikrotik-mcp tools to the MCP server.
func Register(s *server.MCPServer) {
	h := &handlers{}

	commonTarget := []mcp.ToolOption{
		mcp.WithString("host", mcp.Required(), mcp.Description("RouterOS device hostname or IP.")),
		mcp.WithString("user", mcp.Required(), mcp.Description("API username.")),
		mcp.WithString("password", mcp.Description("API password (omit if empty).")),
		mcp.WithNumber("port", mcp.Description("API port. Default 8728 (8729 for TLS).")),
		mcp.WithBoolean("use_tls", mcp.Description("Use api-ssl. Default false.")),
		mcp.WithBoolean("tls_skip_verify", mcp.Description("Skip TLS cert verification. Default false.")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Per-call timeout. Default 30.")),
	}

	with := func(extra ...mcp.ToolOption) []mcp.ToolOption {
		return append(append([]mcp.ToolOption{}, commonTarget...), extra...)
	}

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

func target(args map[string]any) (rosclient.Target, error) {
	host, ok := argString(args, "host")
	if !ok || host == "" {
		return rosclient.Target{}, errors.New("missing 'host'")
	}
	user, ok := argString(args, "user")
	if !ok || user == "" {
		return rosclient.Target{}, errors.New("missing 'user'")
	}
	pwd, _ := argString(args, "password")
	return rosclient.Target{
		Host:          host,
		User:          user,
		Password:      pwd,
		Port:          argInt(args, "port", 0),
		UseTLS:        argBool(args, "use_tls", false),
		TLSSkipVerify: argBool(args, "tls_skip_verify", false),
		Timeout:       time.Duration(argInt(args, "timeout_seconds", 30)) * time.Second,
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
	c, err := rosclient.Dial(ctx, t)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.RunArgsContext(ctx, sentence)
}

// --- handlers --------------------------------------------------------------

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
