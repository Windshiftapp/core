package handlers

import (
	"net/http"
	"strings"
)

const contextPathHeader = "X-Windshift-Context-Path"

func requestContextPrefix(r *http.Request) string {
	prefix := r.Header.Get(contextPathHeader)
	if prefix == "" || prefix == "/" || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#\\") || strings.Contains(prefix, "//") || strings.Contains(prefix, "..") {
		return ""
	}
	return strings.TrimSuffix(prefix, "/")
}

// rewriteAttachmentURLs replaces internal attachment URLs in markdown/HTML with
// a surface-specific download prefix. Content may carry the deployment context
// prefix (a client already translated the URL) or a bare root-relative path,
// so both forms are rewritten. The context-prefixed form is replaced first so
// the bare replacement cannot leave a duplicated prefix.
func rewriteAttachmentURLs(content, contextPrefix, targetPrefix string) string {
	if contextPrefix != "" {
		content = strings.ReplaceAll(content, contextPrefix+"/api/attachments/", targetPrefix)
	}
	return strings.ReplaceAll(content, "/api/attachments/", targetPrefix)
}
