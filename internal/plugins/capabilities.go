//go:build !noplugins

package plugins

import (
	"fmt"
	"sort"
	"strings"

	extism "github.com/extism/go-sdk"
)

// Host-function permissions. A plugin may call a host function only when its
// manifest declares the matching permission; everything else is denied by
// default. The names form a closed catalog: an unknown name fails the plugin
// load, so a typo cannot silently leave a host function unavailable.
//
// The `log` host function is always available and needs no permission.
const (
	PermissionProcessExec   = "process.exec"
	PermissionNetworkEgress = "network.egress"
	PermissionMailSend      = "mail.send"
	PermissionStorageKV     = "storage.kv"
	PermissionCommentsWrite = "comments.write"
	PermissionSCMWrite      = "scm.write"
)

// hostFunctionPermissions maps each registered host function to the permission
// a plugin must declare to call it. TestHostFunctionPermissionMapping fails when
// a registered host function is missing from this table, so a new host function
// cannot ship unguarded.
var hostFunctionPermissions = map[string]string{
	"cli_exec":             PermissionProcessExec,
	"http_fetch":           PermissionNetworkEgress,
	"smtp_send":            PermissionMailSend,
	"kv_get":               PermissionStorageKV,
	"kv_set":               PermissionStorageKV,
	"kv_delete":            PermissionStorageKV,
	"create_comment":       PermissionCommentsWrite,
	"scm_create_branch":    PermissionSCMWrite,
	"scm_create_item_link": PermissionSCMWrite,
}

// ungatedHostFunctions are always available and require no permission.
var ungatedHostFunctions = map[string]struct{}{
	"log": {},
}

// knownPermissions is the closed catalog of valid manifest permissions.
var knownPermissions = func() map[string]struct{} {
	known := make(map[string]struct{}, len(hostFunctionPermissions))
	for _, permission := range hostFunctionPermissions {
		known[permission] = struct{}{}
	}
	return known
}()

// normalizePermissions trims, deduplicates, and validates a manifest's declared
// permissions. An unknown name is an error so a typo fails the load instead of
// silently leaving the host function denied.
func normalizePermissions(declared []string) ([]string, error) {
	seen := make(map[string]struct{}, len(declared))
	for _, raw := range declared {
		permission := strings.TrimSpace(raw)
		if permission == "" {
			continue
		}
		if _, ok := knownPermissions[permission]; !ok {
			return nil, fmt.Errorf("unknown permission %q", permission)
		}
		seen[permission] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(seen))
	for permission := range seen {
		normalized = append(normalized, permission)
	}
	sort.Strings(normalized)
	return normalized, nil
}

// deniedHostFunctions lists the gated host functions a plugin cannot call with
// the given effective permissions, in a stable order. The load path logs this so
// an operator sees exactly which host functions a plugin lost access to.
func deniedHostFunctions(effective []string) []string {
	granted := make(map[string]struct{}, len(effective))
	for _, permission := range effective {
		granted[permission] = struct{}{}
	}
	var denied []string
	for hostFunction, permission := range hostFunctionPermissions {
		if _, ok := granted[permission]; !ok {
			denied = append(denied, hostFunction)
		}
	}
	sort.Strings(denied)
	return denied
}

// permissionDeniedMessage is the response body a host function returns when the
// calling plugin did not declare its permission.
func permissionDeniedMessage(permission string) string {
	return fmt.Sprintf("host function denied: plugin does not declare permission %q", permission)
}

// validateHostFunctionCatalog verifies that every registered host function is
// either explicitly ungated or carries a permission mapping. NewManager runs it
// against its host function set so a new host function cannot ship unguarded.
func validateHostFunctionCatalog(hostFunctions []extism.HostFunction) error {
	for _, hostFunction := range hostFunctions {
		if _, ungated := ungatedHostFunctions[hostFunction.Name]; ungated {
			continue
		}
		if _, mapped := hostFunctionPermissions[hostFunction.Name]; !mapped {
			return fmt.Errorf("host function %q has no permission mapping", hostFunction.Name)
		}
	}
	return nil
}
