package controlcenter

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestServeRequiresToken(t *testing.T) {
	err := Serve(Config{Addr: ":0"}, server.NewMCPServer("t", "0"))
	if err == nil {
		t.Fatal("expected error when token is empty")
	}
}

func TestBearerAuth(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := bearerAuth("s3cret", next)

	cases := []struct {
		name   string
		header string
		want   int
		passed bool
	}{
		{"no header", "", http.StatusUnauthorized, false},
		{"wrong token", "Bearer nope", http.StatusUnauthorized, false},
		{"missing scheme", "s3cret", http.StatusUnauthorized, false},
		{"correct", "Bearer s3cret", http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if called != tc.passed {
				t.Fatalf("next called = %v, want %v", called, tc.passed)
			}
		})
	}
}

func TestSelfSignedCert(t *testing.T) {
	for _, host := range []string{"localhost", "192.168.88.1"} {
		cert, err := selfSignedCert(host)
		if err != nil {
			t.Fatalf("selfSignedCert(%q): %v", host, err)
		}
		if len(cert.Certificate) == 0 {
			t.Fatalf("selfSignedCert(%q): empty certificate", host)
		}
	}
}

func TestBuildTLSSelfSigned(t *testing.T) {
	cfg, note, err := buildTLS(Config{CertHost: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) == 0 || cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected tls config: %+v", cfg)
	}
	if note == "" {
		t.Fatal("expected a descriptive note")
	}
}

func TestBuildTLSRequiresBothCertAndKey(t *testing.T) {
	if _, _, err := buildTLS(Config{TLSCert: "only-cert.pem"}); err == nil {
		t.Fatal("expected error when only cert is provided")
	}
}
