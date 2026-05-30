package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/mikrotik-mcp/internal/tools"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mikrotik-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv := server.NewMCPServer(
		"mikrotik-mcp",
		version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
		server.WithInstructions(
			"mikrotik-mcp manages MikroTik RouterOS devices via the binary API.\n"+
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
