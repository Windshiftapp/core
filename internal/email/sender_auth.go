package email

import "strings"

// SenderAuthPolicy controls whether the From address of inbound mail must be
// backed by a passing, aligned SPF, DKIM, or DMARC result before intake trusts
// it. The From header is entirely attacker-controlled; without this check
// anyone who can deliver a message can claim to be a known customer and either
// join a private thread or create a request in their name (WI-2362).
type SenderAuthPolicy string

const (
	// SenderAuthStrict holds mail whose From domain is not authenticated. It is
	// the default for any channel or intake that does not configure a policy,
	// so a missing or misspelled value can never silently disable the check.
	SenderAuthStrict SenderAuthPolicy = "strict"
	// SenderAuthOff trusts the From header the way intake did before sender
	// authentication existed. Intended for mailbox providers that do not add
	// Authentication-Results / Received-SPF headers.
	SenderAuthOff SenderAuthPolicy = "off"
)

// ResolveSenderAuthPolicy returns the effective policy. Anything other than an
// explicit "off" resolves to strict.
func ResolveSenderAuthPolicy(value string) SenderAuthPolicy {
	if strings.EqualFold(strings.TrimSpace(value), string(SenderAuthOff)) {
		return SenderAuthOff
	}
	return SenderAuthStrict
}

// IsValidSenderAuthPolicy reports whether value is a recognized policy or the
// empty default sentinel.
func IsValidSenderAuthPolicy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(SenderAuthStrict), string(SenderAuthOff):
		return true
	default:
		return false
	}
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
// Only headers that precede the first Received header are trusted: the final
// receiving MTA prepends its own Authentication-Results above the Received
// header it adds, so an Authentication-Results injected by the sender sits
// below a Received header and is ignored. When the message was not parsed from
// raw bytes (HeaderOrder is empty) the topmost header is used.
func EvaluateSenderAuthentication(message *ParsedEmail) SenderAuthResult {
	if message == nil {
		return SenderAuthResult{Outcome: "none", Detail: "no message"}
	}
	fromDomain := emailDomain(message.From.Address)
	if fromDomain == "" {
		return SenderAuthResult{Outcome: "none", Detail: "sender address has no domain"}
	}

	var fallback *SenderAuthResult
	for _, raw := range trustedHeaderValues(message, "Authentication-Results") {
		result, verified := evaluateAuthenticationResults(raw, fromDomain)
		if verified {
			return result
		}
		if fallback == nil {
			remembered := result
			fallback = &remembered
		}
	}
	for _, raw := range trustedHeaderValues(message, "Received-SPF") {
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
func evaluateAuthenticationResults(value, fromDomain string) (SenderAuthResult, bool) {
	methods := parseAuthenticationResults(value)

	// DMARC first: a pass already encodes alignment between the From domain and
	// a passing SPF or DKIM signature. When the receiver records header.from,
	// still confirm it matches the From domain.
	for _, method := range methods {
		if method.name != "dmarc" || method.result != "pass" {
			continue
		}
		if claimed := emailDomain(method.props["header.from"]); claimed != "" && !domainsAligned(fromDomain, claimed) {
			continue
		}
		return SenderAuthResult{
			Verified: true,
			Method:   SenderAuthMethodDMARC,
			Outcome:  "pass",
			Detail:   "dmarc=pass aligned with " + fromDomain,
		}, true
	}

	// DKIM: the signing domain must align with the From domain.
	for _, method := range methods {
		if method.name != "dkim" || method.result != "pass" {
			continue
		}
		domain := dkimSigningDomain(method.props)
		if domain != "" && domainsAligned(fromDomain, domain) {
			return SenderAuthResult{
				Verified: true,
				Method:   SenderAuthMethodDKIM,
				Outcome:  "pass",
				Detail:   "dkim=pass signed by " + domain,
			}, true
		}
	}

	// SPF: the envelope (MAIL FROM) domain must align with the From domain.
	for _, method := range methods {
		if method.name != "spf" || method.result != "pass" {
			continue
		}
		domain := emailDomain(method.props["smtp.mailfrom"])
		if domain != "" && domainsAligned(fromDomain, domain) {
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

// parseAuthenticationResults tokenizes an Authentication-Results value into
// method clauses. The first semicolon-delimited segment is the authserv-id and
// is skipped. Parenthesized comments are removed first so a comment can never
// masquerade as a property.
func parseAuthenticationResults(value string) []authMethod {
	clauses := strings.Split(stripAuthenticationComments(value), ";")
	var out []authMethod
	for i, clause := range clauses {
		if i == 0 {
			continue // authserv-id
		}
		var current *authMethod
		for _, field := range strings.Fields(clause) {
			name, rawValue, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			name = strings.ToLower(strings.TrimSpace(name))
			rawValue = strings.Trim(strings.TrimSpace(rawValue), `"`)
			switch name {
			case "spf", "dkim", "dmarc":
				out = append(out, authMethod{
					name:   name,
					result: strings.ToLower(rawValue),
					props:  map[string]string{},
				})
				current = &out[len(out)-1]
			default:
				if current != nil && name != "" {
					current.props[name] = rawValue
				}
			}
		}
	}
	return out
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
// outcome: the header carries no DKIM or DMARC information.
func evaluateReceivedSPF(value, fromDomain string) (SenderAuthResult, bool) {
	cleaned := stripAuthenticationComments(value)
	fields := strings.Fields(cleaned)
	if len(fields) == 0 {
		return SenderAuthResult{Method: SenderAuthMethodSPF, Outcome: "none", Detail: "empty Received-SPF"}, false
	}
	result := strings.ToLower(strings.TrimSuffix(fields[0], ":"))
	if result != "pass" {
		return SenderAuthResult{
			Method:  SenderAuthMethodSPF,
			Outcome: result,
			Detail:  "received-spf=" + result,
		}, false
	}
	domain := emailDomain(receivedSPFProperty(cleaned, "envelope-from"))
	if domain != "" && domainsAligned(fromDomain, domain) {
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
	for _, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
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
			return strings.Trim(strings.TrimSpace(rawValue), `"`)
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

// domainsAligned reports whether the authenticated domain aligns with the From
// domain under DMARC relaxed alignment: identical, or one is a parent of the
// other on a label boundary.
func domainsAligned(fromDomain, authenticatedDomain string) bool {
	fromDomain = strings.ToLower(strings.TrimSpace(fromDomain))
	authenticatedDomain = strings.ToLower(strings.TrimSpace(authenticatedDomain))
	if fromDomain == "" || authenticatedDomain == "" {
		return false
	}
	if fromDomain == authenticatedDomain {
		return true
	}
	return strings.HasSuffix(fromDomain, "."+authenticatedDomain) ||
		strings.HasSuffix(authenticatedDomain, "."+fromDomain)
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

// trustedHeaderValues returns the values of name that appear before the first
// Received header. When the message has no recorded header order (constructed
// in tests or by an older parser) every value is returned.
func trustedHeaderValues(message *ParsedEmail, name string) []string {
	values := headerValues(message, name)
	if len(values) == 0 || len(message.HeaderOrder) == 0 {
		return values
	}
	var trusted []string
	seen := 0
	for _, key := range message.HeaderOrder {
		if strings.EqualFold(key, "Received") {
			return trusted
		}
		if strings.EqualFold(key, name) {
			if seen < len(values) {
				trusted = append(trusted, values[seen])
			}
			seen++
		}
	}
	return trusted
}
