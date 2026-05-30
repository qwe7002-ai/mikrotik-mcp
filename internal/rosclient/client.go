// Package rosclient wraps the go-routeros library with helpers for one-shot
// API calls used by the MCP tools.
package rosclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-routeros/routeros/v3"
)

// Target is the connection target for a RouterOS device.
type Target struct {
	Host          string
	Port          int
	User          string
	Password      string
	UseTLS        bool
	TLSSkipVerify bool
	Timeout       time.Duration
}

// Address returns host:port, defaulting port based on UseTLS.
func (t Target) Address() string {
	port := t.Port
	if port == 0 {
		if t.UseTLS {
			port = 8729
		} else {
			port = 8728
		}
	}
	return net.JoinHostPort(t.Host, strconv.Itoa(port))
}

// Dial opens a connection (TLS or plain) and logs in.
func Dial(ctx context.Context, t Target) (*routeros.Client, error) {
	if t.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	if t.User == "" {
		return nil, fmt.Errorf("missing user")
	}
	addr := t.Address()
	if t.Timeout <= 0 {
		t.Timeout = 15 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	var (
		c   *routeros.Client
		err error
	)
	if t.UseTLS {
		cfg := &tls.Config{InsecureSkipVerify: t.TLSSkipVerify} //nolint:gosec // skip is opt-in
		c, err = routeros.DialTLSContext(dialCtx, addr, t.User, t.Password, cfg)
	} else {
		c, err = routeros.DialContext(dialCtx, addr, t.User, t.Password)
	}
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return c, nil
}
