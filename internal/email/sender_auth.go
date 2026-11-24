package email

import (
	"strings"

	"golang.org/x/net/publicsuffix"

	"windshift/internal/models"
)

// Sender authentication evaluation is diagnostic only (WI-2365): intake never
// holds or drops mail on the SPF/DKIM/DMARC verdict, and the claimed From is
// never an identity. The result is recorded on the tracking row for the admin
// diagnostics view. Authorization to append to a private conversation comes
// from a recipient-bound outbound Message-ID instead.

// ResolveTrustedAuthServIDs returns the receiver hosts whose
// Authentication-Results and Received-SPF headers are trusted for a mailbox.
// These are the mailbox-specific receiver boundary: the final delivery agent
// prepends its own result above its Received header, so a result is trusted
// only when it sits above a Received hop from one of these hosts.
//
// Operators can list the hosts explicitly. Otherwise they are derived from the
// provider and the configured mail host. An empty result means no boundary can
// be established; strict mode then holds everything rather than trusting an
// arbitrary header.
func ResolveTrustedAuthServIDs(config *models.ChannelConfig) []string {
	if config == nil {
		return nil
	}
	var trusted []string
	trusted = append(trusted, config.EmailTrustedAuthServIDs...)

	switch strings.ToLower(strings.TrimSpace(config.EmailOAuthProviderType)) {
	case models.EmailProviderTypeGoogle:
		trusted = append(trusted, "mx.google.com")
	case models.EmailProviderTypeMicrosoft:
		trusted = append(trusted, "mx.microsoft.com", "outbound.protection.outlook.com")
	}

	imapHost := strings.ToLower(strings.TrimSpace(config.IMAPHost))
	switch {
	case strings.Contains(imapHost, "gmail"):
		trusted = append(trusted, "mx.google.com")
	case strings.Contains(imapHost, "outlook"), strings.Contains(imapHost, "office365"):
		trusted = append(trusted, "mx.microsoft.com", "outbound.protection.outlook.com")
	}
	if imapHost != "" {
		trusted = append(trusted, imapHost)
		if org := organizationalDomain(imapHost); org != "" {
			trusted = append(trusted, org)
		}
	}

	// The monitored address's domain is the last resort: a self-hosted mailbox
	// is usually served by a host in the same organizational domain.
	if address := firstNonEmpty(config.EmailOAuthMailbox, config.EmailOAuthEmail, config.IMAPUsername); address != "" {
		if domain := emailDomain(address); domain != "" {
			trusted = append(trusted, domain)
			if org := organizationalDomain(domain); org != "" {
				trusted = append(trusted, org)
			}
		}
	}
	return normalizeTrustedHosts(trusted)
}

// SenderAuthMethod identifies the mechanism a result came from.
type SenderAuthMethod string

const (
	SenderAuthMethodSPF   SenderAuthMethod = "spf"
	SenderAuthMethodDKIM  SenderAuthMethod = "dkim"
	SenderAuthMethodDMARC SenderAuthMethod = "dmarc"
)

// SenderAuthResult is the outcome of evaluating a message's authentication
// headers against its From domain.
type SenderAuthResult struct {
	// Verified is true when a passing, aligned result was found.
	Verified bool
	// Method is the mechanism that produced the result.
	Method SenderAuthMethod
	// Outcome is the raw result token ("pass", "fail", "softfail", "none", ...).
	Outcome string
	// Detail is a short, operator-facing explanation of the decision.
	Detail string
}

// Summary returns the compact, persisted form of the result, e.g. "dmarc=pass".
func (r SenderAuthResult) Summary() string {
	if r.Method == "" {
		return "none"
	}
	if r.Outcome == "" {
		return string(r.Method)
	}
	return string(r.Method) + "=" + r.Outcome
}

// EvaluateSenderAuthentication inspects the receiving MTA's
// Authentication-Results (falling back to Received-SPF) and reports whether
// the From domain is authenticated by an aligned SPF, DKIM, or DMARC pass.
//
// Only results that appear above a trusted receiver hop are considered, and an
// Authentication-Results header is trusted only when its authserv-id is one of
// trustedReceivers. Both conditions are required: the position rejects a
// result injected below the receiving MTA's Received header, and the
// authserv-id rejects an upstream MTA's result that happens to sit above the
// boundary. When no trusted receiver hop exists the message is treated as
// unauthenticated.
func EvaluateSenderAuthentication(message *ParsedEmail, trustedReceivers []string) SenderAuthResult {
	if message == nil {
		return SenderAuthResult{Outcome: "none", Detail: "no message"}
	}
	fromDomain := emailDomain(message.From.Address)
	if fromDomain == "" {
		return SenderAuthResult{Outcome: "none", Detail: "sender address has no domain"}
	}

	boundary := trustedReceiverBoundary(message, trustedReceivers)
	if boundary < 0 {
		return SenderAuthResult{
			Outcome: "none",
			Detail:  "no trusted receiver boundary for " + fromDomain,
		}
	}

	var fallback *SenderAuthResult
	for _, raw := range headerValuesAbove(message, "Authentication-Results", boundary) {
		result, verified := evaluateAuthenticationResults(raw, fromDomain, trustedReceivers)
		if verified {
			return result
		}
		if fallback == nil {
			remembered := result
			fallback = &remembered
		}
	}
	for _, raw := range headerValuesAbove(message, "Received-SPF", boundary) {
		result, verified := evaluateReceivedSPF(raw, fromDomain)
		if verified {
			return result
		}
		if fallback == nil {
			remembered := result
			fallback = &remembered
		}
	}
	if fallback != nil {
		return *fallback
	}
	return SenderAuthResult{
		Outcome: "none",
		Detail:  "no SPF, DKIM, or DMARC result for " + fromDomain,
	}
}

// authMethod is one method=result clause from an Authentication-Results header.
type authMethod struct {
	name   string
	result string
	props  map[string]string
}

// evaluateAuthenticationResults parses one Authentication-Results value and
// reports whether it carries an aligned pass. The bool is true only for a
// verified result; the returned result describes the outcome either way.
func evaluateAuthenticationResults(value, fromDomain string, trustedReceivers []string) (SenderAuthResult, bool) {
	authservID, methods := parseAuthenticationResults(value)
	if len(methods) == 0 {
		return SenderAuthResult{Outcome: "none", Detail: "no SPF, DKIM, or DMARC clause"}, false
	}
	if !authServIDTrusted(authservID, trustedReceivers) {
		return SenderAuthResult{
			Outcome: "untrusted",
			Detail:  "result from untrusted authentication server " + authservID,
		}, false
	}

	// DMARC first: a pass already encodes alignment between the From domain and
	// a passing SPF or DKIM signature. When the receiver records header.from,
	// still confirm it matches the actual From domain.
	for _, method := range methods {
		if method.name != "dmarc" || method.result != "pass" {
			continue
		}
		if claimed := emailDomain(method.props["header.from"]); claimed != "" && !strings.EqualFold(claimed, fromDomain) {
			continue
		}
		return SenderAuthResult{
			Verified: true,
			Method:   SenderAuthMethodDMARC,
			Outcome:  "pass",
			Detail:   "dmarc=pass aligned with " + fromDomain,
		}, true
	}

	// DKIM: the signing domain must share the From domain's organizational
	// domain under DMARC relaxed alignment.
	for _, method := range methods {
		if method.name != "dkim" || method.result != "pass" {
			continue
		}
		domain := dkimSigningDomain(method.props)
		if domain != "" && organizationalDomainsAligned(fromDomain, domain) {
			return SenderAuthResult{
				Verified: true,
				Method:   SenderAuthMethodDKIM,
				Outcome:  "pass",
				Detail:   "dkim=pass signed by " + domain,
			}, true
		}
	}

	// SPF: the envelope (MAIL FROM) domain must share the From domain's
	// organizational domain under DMARC relaxed alignment.
	for _, method := range methods {
		if method.name != "spf" || method.result != "pass" {
			continue
		}
		domain := emailDomain(method.props["smtp.mailfrom"])
		if domain != "" && organizationalDomainsAligned(fromDomain, domain) {
			return SenderAuthResult{
				Verified: true,
				Method:   SenderAuthMethodSPF,
				Outcome:  "pass",
				Detail:   "spf=pass for " + domain,
			}, true
		}
	}

	return summarizeAuthenticationResults(methods), false
}

// parseAuthenticationResults returns the authserv-id and the method clauses of
// an Authentication-Results value. Parsing respects RFC 8601: clauses are
// separated by semicolons outside quoted strings and comments, and a method is
// accepted only as the first token of a clause, so a quoted diagnostic or
// property value can never be read as a verdict.
func parseAuthenticationResults(value string) (string, []authMethod) {
	clauses := splitAuthResultsClauses(value)
	if len(clauses) == 0 {
		return "", nil
	}
	authservID := strings.ToLower(firstToken(clauses[0]))
	var out []authMethod
	for _, clause := range clauses[1:] {
		tokens := tokenizeAuthClause(clause)
		if len(tokens) == 0 {
			continue
		}
		name, rawValue, ok := strings.Cut(tokens[0], "=")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "spf" && name != "dkim" && name != "dmarc" {
			continue
		}
		method := authMethod{
			name:   name,
			result: strings.ToLower(trimQuoted(rawValue)),
			props:  map[string]string{},
		}
		for _, token := range tokens[1:] {
			prop, propValue, ok := strings.Cut(token, "=")
			if !ok {
				continue
			}
			method.props[strings.ToLower(strings.TrimSpace(prop))] = trimQuoted(propValue)
		}
		out = append(out, method)
	}
	return authservID, out
}

// splitAuthResultsClauses splits on semicolons that are outside quoted strings
// and parenthesized comments.
func splitAuthResultsClauses(value string) []string {
	var clauses []string
	var current strings.Builder
	depth := 0
	inQuote := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case inQuote:
			current.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(value):
				i++
				current.WriteByte(value[i])
			case c == '"':
				inQuote = false
			}
		case depth > 0:
			current.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(value):
				i++
				current.WriteByte(value[i])
			case c == '(':
				depth++
			case c == ')':
				depth--
			}
		case c == '"':
			inQuote = true
			current.WriteByte(c)
		case c == '(':
			depth++
			current.WriteByte(c)
		case c == ';':
			clauses = append(clauses, current.String())
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	clauses = append(clauses, current.String())
	return clauses
}

// tokenizeAuthClause splits a clause into whitespace-separated tokens,
// preserving quoted strings and skipping parenthesized comments.
func tokenizeAuthClause(clause string) []string {
	var tokens []string
	var current strings.Builder
	depth := 0
	inQuote := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(clause); i++ {
		c := clause[i]
		switch {
		case inQuote:
			current.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(clause):
				i++
				current.WriteByte(clause[i])
			case c == '"':
				inQuote = false
			}
		case depth > 0:
			switch {
			case c == '\\' && i+1 < len(clause):
				i++
			case c == '(':
				depth++
			case c == ')':
				depth--
			}
		case c == '"':
			inQuote = true
			current.WriteByte(c)
		case c == '(':
			flush()
			depth++
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			flush()
		default:
			current.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// summarizeAuthenticationResults picks the most relevant observed outcome for
// operator display when nothing verified. DMARC is preferred, then DKIM, then
// SPF, because it best explains why a message was held.
func summarizeAuthenticationResults(methods []authMethod) SenderAuthResult {
	for _, name := range []string{"dmarc", "dkim", "spf"} {
		for _, method := range methods {
			if method.name == name {
				return SenderAuthResult{
					Method:  SenderAuthMethod(name),
					Outcome: method.result,
					Detail:  name + "=" + method.result,
				}
			}
		}
	}
	return SenderAuthResult{Outcome: "none", Detail: "no SPF, DKIM, or DMARC result"}
}

// evaluateReceivedSPF parses a Received-SPF value. It only contributes an SPF
// outcome: the header carries no DKIM or DMARC information. A pass without an
// envelope-from was evaluated against HELO, which authenticates the sending
// host rather than the envelope sender, so it cannot establish the From domain.
func evaluateReceivedSPF(value, fromDomain string) (SenderAuthResult, bool) {
	cleaned := stripAuthenticationComments(value)
	fields := strings.Fields(cleaned)
	if len(fields) == 0 {
		return SenderAuthResult{Method: SenderAuthMethodSPF, Outcome: "none", Detail: "empty Received-SPF"}, false
	}
	result := strings.ToLower(strings.TrimSuffix(fields[0], ":"))
	envelopeFrom := trimQuoted(receivedSPFProperty(cleaned, "envelope-from"))
	if envelopeFrom == "" {
		return SenderAuthResult{
			Method:  SenderAuthMethodSPF,
			Outcome: result,
			Detail:  "received-spf=" + result + " has no envelope-from (HELO identity is not the sender)",
		}, false
	}
	if result != "pass" {
		return SenderAuthResult{
			Method:  SenderAuthMethodSPF,
			Outcome: result,
			Detail:  "received-spf=" + result,
		}, false
	}
	domain := emailDomain(envelopeFrom)
	if domain != "" && organizationalDomainsAligned(fromDomain, domain) {
		return SenderAuthResult{
			Verified: true,
			Method:   SenderAuthMethodSPF,
			Outcome:  "pass",
			Detail:   "received-spf=pass for " + domain,
		}, true
	}
	return SenderAuthResult{
		Method:  SenderAuthMethodSPF,
		Outcome: "pass",
		Detail:  "received-spf=pass but not aligned with " + fromDomain,
	}, false
}

// stripAuthenticationComments removes parenthesized comments, which may
// contain spaces, semicolons, and tokens that look like method properties.
func stripAuthenticationComments(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	depth := 0
	inQuote := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case inQuote:
			b.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(value):
				i++
				b.WriteByte(value[i])
			case c == '"':
				inQuote = false
			}
		case depth > 0:
			switch {
			case c == '\\' && i+1 < len(value):
				i++
			case c == '(':
				depth++
			case c == ')':
				depth--
			}
		case c == '"':
			inQuote = true
			b.WriteByte(c)
		case c == '(':
			depth++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// receivedSPFProperty returns a named property from a Received-SPF value.
func receivedSPFProperty(value, key string) string {
	for _, clause := range strings.Split(value, ";") {
		name, rawValue, ok := strings.Cut(clause, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(rawValue)
		}
	}
	return ""
}

// dkimSigningDomain returns the DKIM signing domain from header.d, falling
// back to the AUID in header.i.
func dkimSigningDomain(props map[string]string) string {
	if domain := emailDomain(props["header.d"]); domain != "" {
		return domain
	}
	return emailDomain(props["header.i"])
}

// emailDomain extracts the lowercased domain from an address, a bare domain,
// or a DMARC header.from value. It returns "" for a null or malformed value.
func emailDomain(address string) string {
	address = strings.TrimSpace(address)
	address = strings.Trim(address, "<>\"")
	if address == "" {
		return ""
	}
	if open := strings.LastIndex(address, "<"); open >= 0 {
		if closeIdx := strings.Index(address[open:], ">"); closeIdx > 0 {
			address = address[open+1 : open+closeIdx]
		}
	}
	if at := strings.LastIndex(address, "@"); at >= 0 {
		address = address[at+1:]
	}
	address = strings.Trim(strings.TrimSpace(address), ".")
	return strings.ToLower(address)
}

// organizationalDomain returns the registrable domain of a host using the
// public suffix list, falling back to the host itself for IP literals, single
// labels, and unknown suffixes.
func organizationalDomain(host string) string {
	host = normalizeHost(host)
	if host == "" {
		return ""
	}
	if org, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return org
	}
	return host
}

// organizationalDomainsAligned reports whether two domains share an
// organizational domain, which is DMARC relaxed alignment.
func organizationalDomainsAligned(fromDomain, authenticatedDomain string) bool {
	fromOrg := organizationalDomain(fromDomain)
	authOrg := organizationalDomain(authenticatedDomain)
	return fromOrg != "" && fromOrg == authOrg
}

// trustedReceiverBoundary returns the position of the first Received header
// whose from/by host is a trusted receiver, or -1 when none exists. A result
// is trusted only when it appears before this position.
func trustedReceiverBoundary(message *ParsedEmail, trustedReceivers []string) int {
	if len(message.HeaderOrder) == 0 || len(trustedReceivers) == 0 {
		return -1
	}
	received := headerValues(message, "Received")
	seen := 0
	for i, key := range message.HeaderOrder {
		if !strings.EqualFold(key, "Received") {
			continue
		}
		if seen < len(received) && receivedFromTrustedHost(received[seen], trustedReceivers) {
			return i
		}
		seen++
	}
	return -1
}

// headerValuesAbove returns the values of a header that appear before the
// boundary position in the recorded header order.
func headerValuesAbove(message *ParsedEmail, name string, boundary int) []string {
	values := headerValues(message, name)
	if len(values) == 0 {
		return nil
	}
	var out []string
	seen := 0
	for i, key := range message.HeaderOrder {
		if i >= boundary {
			break
		}
		if strings.EqualFold(key, name) {
			if seen < len(values) {
				out = append(out, values[seen])
			}
			seen++
		}
	}
	return out
}

// receivedFromTrustedHost reports whether a Received header's from/by host is a
// trusted receiver.
func receivedFromTrustedHost(value string, trustedReceivers []string) bool {
	for _, host := range receivedHosts(value) {
		for _, trusted := range trustedReceivers {
			if hostMatches(host, trusted) {
				return true
			}
		}
	}
	return false
}

// receivedHosts extracts the from/by hosts from a Received header.
func receivedHosts(value string) []string {
	fields := strings.Fields(value)
	var hosts []string
	for i, field := range fields {
		switch strings.ToLower(strings.TrimSuffix(field, ":")) {
		case "from", "by":
			if i+1 >= len(fields) {
				continue
			}
			if host := normalizeHost(fields[i+1]); host != "" {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

// authServIDTrusted reports whether an Authentication-Results authserv-id is a
// trusted receiver.
func authServIDTrusted(authservID string, trustedReceivers []string) bool {
	authservID = normalizeHost(authservID)
	if authservID == "" {
		return false
	}
	for _, trusted := range trustedReceivers {
		if hostMatches(authservID, trusted) {
			return true
		}
	}
	return false
}

// hostMatches reports whether host equals trusted or is a subdomain of it.
func hostMatches(host, trusted string) bool {
	host = normalizeHost(host)
	trusted = normalizeHost(trusted)
	if host == "" || trusted == "" {
		return false
	}
	return host == trusted || strings.HasSuffix(host, "."+trusted)
}

// normalizeHost lowercases a host and strips brackets, quotes, and a trailing
// dot.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]()<>\"")
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

// normalizeTrustedHosts lowercases and de-duplicates a trusted host list.
func normalizeTrustedHosts(hosts []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, host := range hosts {
		host = normalizeHost(host)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	return out
}

// headerValues returns the values of a header, looked up case-insensitively
// because RawHeaders may come from either IMAP or raw RFC 5322 parsing.
func headerValues(message *ParsedEmail, name string) []string {
	if message == nil {
		return nil
	}
	for key, values := range message.RawHeaders {
		if strings.EqualFold(key, name) {
			return values
		}
	}
	return nil
}

// firstToken returns the first whitespace-delimited token of a string.
func firstToken(value string) string {
	for _, field := range strings.Fields(value) {
		return field
	}
	return ""
}

// trimQuoted removes surrounding double quotes and whitespace from a value.
func trimQuoted(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"`)
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
