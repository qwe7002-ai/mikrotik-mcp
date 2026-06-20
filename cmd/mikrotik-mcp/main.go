package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
	"github.com/qwe7002/mikrotik-mcp/internal/tools"
	"github.com/qwe7002/mikrotik-mcp/internal/tui"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "mikrotik-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "", "serve":
		return serve()
	case "tui", "config", "login":
		return tui.Run()
	case "version", "-v", "--version":
		fmt.Println("mikrotik-mcp " + version)
		return nil
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage(w *os.File) {
	p, _ := config.Path()
	fmt.Fprintf(w, `mikrotik-mcp %s — MCP server for MikroTik RouterOS

Usage:
  mikrotik-mcp [serve]   Run the MCP server over stdio (default)
  mikrotik-mcp tui       Manage saved connection profiles interactively
  mikrotik-mcp version   Print version
  mikrotik-mcp help      Show this help

Profiles are stored at:
  %s

Saved profiles can be used by the MCP tools via the "profile" argument
instead of passing host/user/password on every call.
`, version, p)
}

func serve() error {
	srv := server.NewMCPServer(
		"mikrotik-mcp",
		version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
		server.WithInstructions(
			"mikrotik-mcp manages MikroTik RouterOS devices via the binary API.\n"+
				"CREDENTIALS: Connection details can be supplied inline (host/user/password) or, "+
				"preferably, via a saved profile name (the 'profile' argument) configured by the user "+
				"with `mikrotik-mcp tui`. Using a profile keeps the password out of the conversation.\n"+
				"SECURITY: host/user/password passed to these tools are SENSITIVE credentials. "+
				"NEVER echo the password back to the user, include it in summaries, write it to files, "+
				"or forward it to any cloud service, external API, web search, or other MCP server. "+
				"Use them only as inputs to mikrotik-mcp tool calls in this local session. "+
				"Prefer use_tls=true on non-loopback hosts so credentials are not sent in plaintext. "+
				"Disruptive commands (/system/reboot, /system/shutdown, /system/reset-configuration) are blocked by policy. "+
				"Call mikrotik_help for full usage and security guidance.",
		),
	)
	tools.Register(srv)

	log.SetOutput(os.Stderr)
	log.Printf("mikrotik-mcp %s starting", version)
	if err := server.ServeStdio(srv); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
