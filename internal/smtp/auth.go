package smtp

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTP AUTH mechanisms selectable per channel. "auto" (also the empty value,
// so channels saved before the setting existed keep working) picks the first
// mechanism from authAutoPreference that the server advertises in EHLO.
const (
	AuthMethodAuto    = "auto"
	AuthMethodPlain   = "plain"
	AuthMethodLogin   = "login"
	AuthMethodCRAMMD5 = "cram-md5"
	AuthMethodNone    = "none"
)

// authAutoPreference is the order "auto" tries mechanisms in. PLAIN first
// preserves the historical behavior for servers that advertise it.
var authAutoPreference = []string{AuthMethodPlain, AuthMethodLogin, AuthMethodCRAMMD5}

// errAuthRequiresTLS mirrors net/smtp's own guard in PlainAuth: credentials
// are never sent on an unencrypted connection, whatever mechanism is chosen.
var errAuthRequiresTLS = errors.New("SMTP authentication requires an encrypted connection")

// normalizeAuthMethod lowercases and trims, mapping "" to AuthMethodAuto.
func normalizeAuthMethod(method string) string {
	m := strings.ToLower(strings.TrimSpace(method))
	if m == "" {
		return AuthMethodAuto
	}
	return m
}

// AuthMethodAllowed reports whether method is a supported SMTP AUTH setting.
func AuthMethodAllowed(method string) bool {
	switch normalizeAuthMethod(method) {
	case AuthMethodAuto, AuthMethodPlain, AuthMethodLogin, AuthMethodCRAMMD5, AuthMethodNone:
		return true
	default:
		return false
	}
}

// newSMTPAuth builds the smtp.Auth for the configured mechanism. It returns
// nil when no authentication should be attempted.
func newSMTPAuth(method, username, password, host string) (smtp.Auth, error) {
	method = normalizeAuthMethod(method)
	if !AuthMethodAllowed(method) {
		return nil, fmt.Errorf("SMTP auth method %q not allowed; use \"auto\", \"plain\", \"login\", \"cram-md5\", or \"none\"", method)
	}
	if method == AuthMethodNone || username == "" || password == "" {
		return nil, nil
	}
	return &selectingAuth{method: method, username: username, password: password, host: host}, nil
}

// selectingAuth resolves the concrete mechanism once the server's EHLO
// capabilities are known, so "auto" can negotiate and explicit choices can
// fail with a clear message when the server doesn't offer them.
type selectingAuth struct {
	method   string
	username string
	password string
	host     string
	active   smtp.Auth
}

func (a *selectingAuth) Start(server *smtp.ServerInfo) (proto string, toServer []byte, err error) {
	if !server.TLS {
		return "", nil, errAuthRequiresTLS
	}
	method := a.method
	if method == AuthMethodAuto {
		method = pickAuthMethod(server.Auth)
	} else if len(server.Auth) > 0 && !serverAdvertises(server.Auth, method) {
		return "", nil, fmt.Errorf("SMTP server does not support AUTH %s (advertised: %s)",
			strings.ToUpper(method), strings.Join(server.Auth, " "))
	}

	switch method {
	case AuthMethodPlain:
		a.active = smtp.PlainAuth("", a.username, a.password, a.host)
	case AuthMethodLogin:
		a.active = &loginAuth{username: a.username, password: a.password}
	case AuthMethodCRAMMD5:
		a.active = smtp.CRAMMD5Auth(a.username, a.password)
	default:
		return "", nil, fmt.Errorf("SMTP auth method %q not allowed", method)
	}
	return a.active.Start(server)
}

func (a *selectingAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if a.active == nil {
		return nil, errors.New("SMTP auth not started")
	}
	return a.active.Next(fromServer, more)
}

// pickAuthMethod returns the first preferred mechanism the server advertises.
// Servers that omit the AUTH extension get PLAIN, matching the old behavior.
func pickAuthMethod(advertised []string) string {
	for _, m := range authAutoPreference {
		if serverAdvertises(advertised, m) {
			return m
		}
	}
	return AuthMethodPlain
}

func serverAdvertises(advertised []string, method string) bool {
	for _, a := range advertised {
		if strings.EqualFold(a, method) {
			return true
		}
	}
	return false
}

// loginAuth implements the non-standard but widely deployed AUTH LOGIN
// mechanism (Exchange/Office 365, many hosting providers). net/smtp has
// already base64-decoded the server prompts by the time Next sees them.
type loginAuth struct {
	username string
	password string
	step     int
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (proto string, toServer []byte, err error) {
	if !server.TLS {
		return "", nil, errAuthRequiresTLS
	}
	a.step = 0
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	a.step++
	switch {
	case strings.HasPrefix(prompt, "user"):
		return []byte(a.username), nil
	case strings.HasPrefix(prompt, "pass"):
		return []byte(a.password), nil
	// Some servers send empty or non-English prompts; fall back to order.
	case a.step == 1:
		return []byte(a.username), nil
	case a.step == 2:
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected AUTH LOGIN challenge %q", fromServer)
	}
}
