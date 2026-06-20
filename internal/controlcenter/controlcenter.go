// Package controlcenter serves the mikrotik-mcp tools over HTTPS (the MCP
// Streamable HTTP transport) so a single central instance can be reached by
// remote clients and drive multiple RouterOS devices via saved profiles.
package controlcenter

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// Config controls the HTTPS control-center listener.
type Config struct {
	Addr     string // listen address, e.g. ":8443"
	TLSCert  string // path to PEM certificate; empty => generate a self-signed cert
	TLSKey   string // path to PEM private key
	Token    string // required bearer token for the /mcp endpoint
	CertHost string // SAN host/IP for the generated self-signed cert
}

// Serve starts the control-center HTTPS server and blocks until the process is
// interrupted (SIGINT/SIGTERM) or the server fails.
func Serve(cfg Config, mcpSrv *server.MCPServer) error {
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("a bearer token is required for control-center mode " +
			"(set --token or MIKROTIK_MCP_TOKEN); refusing to expose device control unauthenticated")
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8443"
	}

	tlsCfg, certNote, err := buildTLS(cfg)
	if err != nil {
		return err
	}

	streamable := server.NewStreamableHTTPServer(mcpSrv)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/mcp", bearerAuth(cfg.Token, streamable))

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		// Cert/key are supplied via TLSConfig, so pass empty paths here.
		if err := httpSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	fmt.Fprintf(os.Stderr, "mikrotik-mcp control-center listening on https://%s/mcp\n", cfg.Addr)
	fmt.Fprintf(os.Stderr, "  auth: Authorization: Bearer <token>\n")
	fmt.Fprintf(os.Stderr, "  health: https://%s/healthz (no auth)\n", cfg.Addr)
	if certNote != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", certNote)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("control-center serve: %w", err)
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "mikrotik-mcp control-center shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(ctx)
	}
}

// bearerAuth wraps a handler, requiring a matching "Authorization: Bearer
// <token>" header. The comparison is constant-time.
func bearerAuth(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildTLS returns a TLS config from the supplied cert/key files, or generates
// an ephemeral self-signed certificate when none are provided. The returned
// note describes which path was taken (for operator logging).
func buildTLS(cfg Config) (*tls.Config, string, error) {
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return nil, "", errors.New("both --tls-cert and --tls-key must be provided together")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, "", fmt.Errorf("load TLS keypair: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
			"tls: using provided certificate " + cfg.TLSCert, nil
	}

	host := cfg.CertHost
	if host == "" {
		host = "localhost"
	}
	cert, err := selfSignedCert(host)
	if err != nil {
		return nil, "", err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		"tls: using EPHEMERAL self-signed certificate for " + host +
			" (clients must skip verification or pin it; supply --tls-cert/--tls-key for production)", nil
}
