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
	)
	tools.Register(srv)

	log.SetOutput(os.Stderr)
	log.Printf("mikrotik-mcp %s starting", version)
	if err := server.ServeStdio(srv); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
