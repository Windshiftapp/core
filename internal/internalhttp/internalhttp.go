// Package internalhttp builds the loopback URL and HTTP client Windshift uses
// to reach its own internal listener — the built-in healthcheck and the SSH
// TUI. That traffic stays on 127.0.0.1 (never the public reverse proxy),
// selects https when the listener terminates TLS directly, and pins the
// configured certificate so a certificate issued for the public hostname still
// verifies without disabling certificate verification globally.
package internalhttp

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// BaseURL returns the loopback base URL for the internal listener: the given
// port, the normalized context path, and the https scheme when direct TLS is
// configured.
func BaseURL(port, contextPath string, directTLS bool) string {
	if strings.TrimSpace(port) == "" {
		port = "8080"
	}
	scheme := "http"
	if directTLS {
		scheme = "https"
	}
	return scheme + "://127.0.0.1:" + port + NormalizeContextPath(contextPath)
}

// NormalizeContextPath returns the context path with a leading slash and no
// trailing slash, or "" for the root.
func NormalizeContextPath(contextPath string) string {
	p := strings.TrimSpace(contextPath)
	if p == "" || p == "/" {
		return ""
	}
	return "/" + strings.Trim(p, "/")
}

// Client returns the HTTP client for the internal listener. Without a
// certificate it is a plain client. With direct TLS it pins the certificate at
// certPath: the loopback connection presents the server's own certificate,
// which may be issued for the public hostname, so the client verifies that the
// presented leaf exactly matches the configured certificate instead of relying
// on hostname verification.
func Client(certPath string, timeout time.Duration) (*http.Client, error) {
	if strings.TrimSpace(certPath) == "" {
		return &http.Client{Timeout: timeout}, nil
	}
	leaf, err := loadLeafCertificate(certPath)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				// Hostname verification is replaced by exact certificate
				// pinning in VerifyConnection: the loopback listener may present
				// a certificate for the public hostname. VerifyConnection runs on
				// every connection, including resumed sessions.
				InsecureSkipVerify: true, //nolint:gosec // pinned in VerifyConnection
				VerifyConnection: func(cs tls.ConnectionState) error {
					if len(cs.PeerCertificates) == 0 {
						return errors.New("internal TLS: server presented no certificate")
					}
					if !cs.PeerCertificates[0].Equal(leaf) {
						return errors.New("internal TLS: presented certificate does not match the configured certificate")
					}
					return nil
				},
			},
		},
	}, nil
}

func loadLeafCertificate(certPath string) (*x509.Certificate, error) {
	pemBytes, err := os.ReadFile(certPath) //nolint:gosec // operator-configured TLS certificate path
	if err != nil {
		return nil, fmt.Errorf("read internal TLS certificate: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("internal TLS certificate %q has no PEM certificate block", certPath)
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse internal TLS certificate: %w", err)
	}
	return leaf, nil
}
