package server

import (
	"bytes"
	"html"
	"strconv"
)

// indexBootstrap is the server-side state the SPA shell needs at load time.
// BaseURL is the effective public base URL, including the localhost fallback;
// BaseURLConfigured is false when the operator never set BASE_URL/--base-url.
type indexBootstrap struct {
	ContextPath       string
	BaseURL           string
	BaseURLConfigured bool
}

func prepareIndexHTML(indexHTML []byte, nonce string, boot indexBootstrap) []byte {
	htmlBytes := bytes.Replace(indexHTML, []byte("<script>"), []byte(`<script nonce="`+html.EscapeString(nonce)+`">`), 1)

	baseHref := "/"
	if boot.ContextPath != "" {
		baseHref = boot.ContextPath + "/"
		htmlBytes = prefixRootRelativeHTMLAttrs(htmlBytes, boot.ContextPath)
	}

	bootstrap := []byte(`<base href="` + html.EscapeString(baseHref) + `">` + "\n    " +
		`<script nonce="` + html.EscapeString(nonce) + `">` +
		`window.__WINDSHIFT_CONTEXT_PATH__=` + strconv.Quote(boot.ContextPath) + `;` +
		`window.__WINDSHIFT_BASE_URL__=` + strconv.Quote(boot.BaseURL) + `;` +
		`window.__WINDSHIFT_BASE_URL_CONFIGURED__=` + strconv.FormatBool(boot.BaseURLConfigured) + `;` +
		`</script>`)
	if bytes.Contains(htmlBytes, []byte("<head>")) {
		htmlBytes = bytes.Replace(htmlBytes, []byte("<head>"), append([]byte("<head>\n    "), bootstrap...), 1)
	}
	return htmlBytes
}

func prefixRootRelativeHTMLAttrs(htmlBytes []byte, contextPath string) []byte {
	prefix := []byte(contextPath)
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte(`href="/`), append([]byte(`href="`), append(prefix, '/')...))
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte(`src="/`), append([]byte(`src="`), append(prefix, '/')...))
	return htmlBytes
}
