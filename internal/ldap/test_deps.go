//go:build test

package ldap

// Import gldap test dependencies so they remain in go.mod for
// integration tests that run via the core-tests overlay.
import (
	_ "github.com/jimlambrt/gldap"
	_ "github.com/jimlambrt/gldap/testdirectory"
)
