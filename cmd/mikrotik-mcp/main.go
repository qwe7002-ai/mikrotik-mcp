package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/server"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
	"github.com/qwe7002/mikrotik-mcp/internal/controlcenter"
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
	case "control-center", "cc", "serve-http":
		return controlCenter(args[1:])
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
  mikrotik-mcp [serve]          Run the MCP server over stdio (default)
  mikrotik-mcp control-center   Serve the MCP over HTTPS for multiple clients/devices
  mikrotik-mcp tui              Manage saved connection profiles interactively
  mikrotik-mcp version          Print version
  mikrotik-mcp help             Show this help

control-center flags (env in parentheses):
  --addr        listen address (MIKROTIK_MCP_ADDR), default :8443
  --token       required bearer token (MIKROTIK_MCP_TOKEN)
  --tls-cert    PEM certificate path (MIKROTIK_MCP_TLS_CERT); self-signed if unset
  --tls-key     PEM private key path (MIKROTIK_MCP_TLS_KEY)
  --cert-host   SAN host/IP for the self-signed cert (MIKROTIK_MCP_CERT_HOST)

Profiles are stored at:
  %s

Saved profiles can be used by the MCP tools via the "profile" argument
instead of passing host/user/password on every call.
`, version, p)
}

// newMCPServer builds the MCP server with all tools and instructions attached.
func newMCPServer() *server.MCPServer {
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
				"CONTROL CENTER: When run with `mikrotik-mcp control-center`, the same tools are served "+
				"over HTTPS so one instance can drive multiple RouterOS devices; select the target per "+
				"call with the 'profile' argument, or fan out across devices with mikrotik_multi_command.\n"+
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
	return srv
}

func serve() error {
	srv := newMCPServer()
	log.SetOutput(os.Stderr)
	log.Printf("mikrotik-mcp %s starting (stdio)", version)
	if err := server.ServeStdio(srv); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func controlCenter(args []string) error {
	fs := flag.NewFlagSet("control-center", flag.ContinueOnError)
	addr := fs.String("addr", envOr("MIKROTIK_MCP_ADDR", ":8443"), "listen address")
	token := fs.String("token", os.Getenv("MIKROTIK_MCP_TOKEN"), "bearer token (required)")
	cert := fs.String("tls-cert", os.Getenv("MIKROTIK_MCP_TLS_CERT"), "PEM certificate path")
	key := fs.String("tls-key", os.Getenv("MIKROTIK_MCP_TLS_KEY"), "PEM private key path")
	certHost := fs.String("cert-host", envOr("MIKROTIK_MCP_CERT_HOST", "localhost"), "SAN host/IP for self-signed cert")
	if err := fs.Parse(args); err != nil {
		return err
	}

	log.SetOutput(os.Stderr)
	log.Printf("mikrotik-mcp %s starting (control-center)", version)
	return controlcenter.Serve(controlcenter.Config{
		Addr:     *addr,
		TLSCert:  strings.TrimSpace(*cert),
		TLSKey:   strings.TrimSpace(*key),
		Token:    *token,
		CertHost: *certHost,
	}, newMCPServer())
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
