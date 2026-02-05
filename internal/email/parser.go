package email

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"regexp"
	"strings"
	"time"

	goMessage "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// Parser handles parsing of email messages
type Parser struct {
	maxAttachmentSize int64
}

// NewParser creates a new email parser
func NewParser() *Parser {
	return &Parser{
		maxAttachmentSize: 10 * 1024 * 1024, // 10MB default
	}
}

// SetMaxAttachmentSize sets the maximum attachment size to process
func (p *Parser) SetMaxAttachmentSize(size int64) {
	p.maxAttachmentSize = size
}

// Parse parses a fetched IMAP message into a ParsedEmail struct
func (p *Parser) Parse(msg *FetchedMessage) (*ParsedEmail, error) {
	parsed := &ParsedEmail{
		UID:        msg.UID,
		RawHeaders: make(map[string][]string),
	}

	// Parse envelope data from IMAP
	if msg.Envelope != nil {
		parsed.Subject = msg.Envelope.Subject
		parsed.MessageID = msg.Envelope.MessageID
		// InReplyTo is []string in go-imap/v2, take first if present
		if len(msg.Envelope.InReplyTo) > 0 {
			parsed.InReplyTo = msg.Envelope.InReplyTo[0]
		}
		parsed.Date = msg.Envelope.Date

		// Parse From address
		if len(msg.Envelope.From) > 0 {
			from := msg.Envelope.From[0]
			parsed.From = EmailAddress{
				Name:    from.Name,
				Address: fmt.Sprintf("%s@%s", from.Mailbox, from.Host),
			}
		}

		// Parse To addresses
		for _, to := range msg.Envelope.To {
			parsed.To = append(parsed.To, EmailAddress{
				Name:    to.Name,
				Address: fmt.Sprintf("%s@%s", to.Mailbox, to.Host),
			})
		}
	}

	// Parse headers for References (not in envelope)
	if len(msg.Header) > 0 {
		headers, err := mail.ReadMessage(bytes.NewReader(append(msg.Header, '\r', '\n')))
		if err == nil {
			// Get References header for threading
			refs := headers.Header.Get("References")
			if refs != "" {
				parsed.References = parseReferences(refs)
			}

			// Store all headers
			for key, values := range headers.Header {
				parsed.RawHeaders[key] = values
			}
		}
	}

	// Parse body
	if len(msg.Body) > 0 || len(msg.Header) > 0 {
		fullMessage := append(msg.Header, msg.Body...)
		err := p.parseBody(bytes.NewReader(fullMessage), parsed)
		if err != nil {
			slog.Warn("failed to parse email body", "error", err, "message_id", parsed.MessageID)
			// Fall back to raw body as plain text
			parsed.PlainBody = string(msg.Body)
		}
	}

	return parsed, nil
}

// parseBody parses the email body, extracting text content and attachments
func (p *Parser) parseBody(r io.Reader, parsed *ParsedEmail) error {
	entity, err := goMessage.Read(r)
	if err != nil {
		return fmt.Errorf("failed to read message entity: %w", err)
	}

	return p.walkEntity(entity, parsed)
}

// walkEntity recursively walks through MIME parts
func (p *Parser) walkEntity(entity *goMessage.Entity, parsed *ParsedEmail) error {
	mediaType, params, err := entity.Header.ContentType()
	if err != nil {
		mediaType = "text/plain"
	}

	switch {
	case strings.HasPrefix(mediaType, "multipart/"):
		return p.walkMultipart(entity, params, parsed)

	case mediaType == "text/plain":
		body, err := io.ReadAll(entity.Body)
		if err != nil {
			return err
		}
		if parsed.PlainBody == "" {
			parsed.PlainBody = string(body)
		}

	case mediaType == "text/html":
		body, err := io.ReadAll(entity.Body)
		if err != nil {
			return err
		}
		if parsed.HTMLBody == "" {
			parsed.HTMLBody = string(body)
		}

	default:
		// Potential attachment
		return p.handleAttachment(entity, mediaType, parsed)
	}

	return nil
}

// walkMultipart processes multipart message parts
func (p *Parser) walkMultipart(entity *goMessage.Entity, params map[string]string, parsed *ParsedEmail) error {
	mr := multipart.NewReader(entity.Body, params["boundary"])

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read multipart: %w", err)
		}

		partEntity, err := goMessage.Read(part)
		if err != nil {
			slog.Warn("failed to read part entity", "error", err)
			continue
		}

		if err := p.walkEntity(partEntity, parsed); err != nil {
			slog.Warn("failed to walk part entity", "error", err)
		}
	}

	return nil
}

// handleAttachment processes an attachment
func (p *Parser) handleAttachment(entity *goMessage.Entity, mediaType string, parsed *ParsedEmail) error {
	// Get filename from Content-Disposition or Content-Type
	filename := ""

	disposition, params, _ := entity.Header.ContentDisposition()
	if disposition == "attachment" || disposition == "inline" {
		filename = params["filename"]
	}

	if filename == "" {
		_, typeParams, _ := entity.Header.ContentType()
		filename = typeParams["name"]
	}

	if filename == "" {
		// Skip parts without filename (likely inline content)
		return nil
	}

	// Decode filename if encoded
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(filename); err == nil {
		filename = decoded
	}

	// Read attachment data (with size limit)
	data, err := io.ReadAll(io.LimitReader(entity.Body, p.maxAttachmentSize+1))
	if err != nil {
		return fmt.Errorf("failed to read attachment: %w", err)
	}

	if int64(len(data)) > p.maxAttachmentSize {
		slog.Warn("attachment exceeds size limit", "filename", filename, "max_size", p.maxAttachmentSize)
		return nil
	}

	parsed.Attachments = append(parsed.Attachments, Attachment{
		Filename:    filename,
		ContentType: mediaType,
		Size:        int64(len(data)),
		Data:        data,
	})

	return nil
}

// parseReferences parses the References header into individual message IDs
func parseReferences(refs string) []string {
	// References header contains space-separated message IDs
	var result []string
	refs = strings.TrimSpace(refs)

	// Split on whitespace and newlines
	parts := strings.Fields(refs)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			result = append(result, part)
		}
	}

	return result
}

// ExtractReplyContent removes quoted content from email body
func ExtractReplyContent(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inQuote := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip lines starting with ">"
		if strings.HasPrefix(trimmed, ">") {
			inQuote = true
			continue
		}

		// Skip "On <date> <person> wrote:" lines
		if isQuoteHeader(trimmed) {
			inQuote = true
			continue
		}

		// Skip signature delimiter and everything after
		if trimmed == "--" || trimmed == "-- " {
			break
		}

		// Skip common footer patterns
		if isFooterLine(trimmed) {
			continue
		}

		if !inQuote {
			result = append(result, line)
		}

		// Reset quote state on blank line
		if trimmed == "" {
			inQuote = false
		}
	}

	// Clean up result
	text := strings.Join(result, "\n")
	text = strings.TrimSpace(text)

	// Remove multiple consecutive blank lines
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return text
}

// isQuoteHeader checks if a line is a quote attribution
var quoteHeaderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^On .+ wrote:$`),
	regexp.MustCompile(`^On .+, .+ wrote:$`),
	regexp.MustCompile(`^.+ wrote:$`),
	regexp.MustCompile(`^-{3,} Original Message -{3,}$`),
	regexp.MustCompile(`^-{3,} Forwarded Message -{3,}$`),
	regexp.MustCompile(`^From: .+$`),
	regexp.MustCompile(`^Sent: .+$`),
	regexp.MustCompile(`^To: .+$`),
	regexp.MustCompile(`^Subject: .+$`),
}

func isQuoteHeader(line string) bool {
	for _, pattern := range quoteHeaderPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// isFooterLine checks for common email footer lines
var footerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^Sent from my .+$`),
	regexp.MustCompile(`^Get Outlook for .+$`),
	regexp.MustCompile(`^Sent from Mail for .+$`),
}

func isFooterLine(line string) bool {
	for _, pattern := range footerPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// signOffPatterns matches common email sign-off lines
var signOffPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(best|kind|warm|warmest)?\s*regards?,?\s*$`),
	regexp.MustCompile(`(?i)^thanks?,?\s*$`),
	regexp.MustCompile(`(?i)^thank\s+you,?\s*$`),
	regexp.MustCompile(`(?i)^cheers,?\s*$`),
	regexp.MustCompile(`(?i)^sincerely,?\s*$`),
	regexp.MustCompile(`(?i)^(all\s+the\s+)?best,?\s*$`),
}

// contactInfoPattern matches lines containing contact information
var contactInfoPattern = regexp.MustCompile(`[@]|(\+?\d[\d\s\-()]{7,})|www\.|https?://|[|]`)

// StripSignature removes business email signatures from plain text.
// It scans from the bottom of the message, looking for explicit delimiters (-- ),
// footer patterns (Sent from my iPhone), and sign-off heuristics (Best regards,).
// Errs on the side of keeping content — false negatives are preferred over false positives.
func StripSignature(body string) string {
	if body == "" {
		return ""
	}

	lines := strings.Split(body, "\n")

	// Layer 1: Explicit delimiter (-- or "-- ")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "--" || trimmed == "-- " || strings.TrimSpace(line) == "--" {
			// Check the actual trimmed content
			t := strings.TrimSpace(line)
			if t == "--" || t == "-- " {
				result := strings.TrimRight(strings.Join(lines[:i], "\n"), " \t\r\n")
				return result
			}
		}
	}

	// Layer 2: Footer patterns (Sent from my iPhone, etc.)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFooterLine(trimmed) {
			result := strings.TrimRight(strings.Join(lines[:i], "\n"), " \t\r\n")
			return result
		}
	}

	// Layer 3: Sign-off heuristic (conservative)
	// Only look in the last ~15 lines
	totalLines := len(lines)
	searchStart := 0
	if totalLines > 15 {
		searchStart = totalLines - 15
	}

	for i := searchStart; i < totalLines; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isSignOff(trimmed) {
			// Validate: what follows should be short (≤10 lines) and/or contain contact info
			remaining := lines[i+1:]
			if validateSignatureBlock(remaining) {
				result := strings.TrimRight(strings.Join(lines[:i], "\n"), " \t\r\n")
				return result
			}
		}
	}

	return body
}

// isSignOff checks if a line matches a sign-off pattern
func isSignOff(line string) bool {
	for _, pattern := range signOffPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// validateSignatureBlock checks that the content after a sign-off looks like a signature
// (short block and/or contains contact info patterns)
func validateSignatureBlock(lines []string) bool {
	// Count non-empty lines
	nonEmpty := 0
	hasContactInfo := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmpty++
			if contactInfoPattern.MatchString(trimmed) {
				hasContactInfo = true
			}
		}
	}

	// Accept if the block is short (≤10 non-empty lines)
	if nonEmpty <= 10 {
		return true
	}

	// Accept if it contains contact info even if slightly longer
	if hasContactInfo && nonEmpty <= 15 {
		return true
	}

	return false
}

// StripHTML removes HTML tags from a string (for HTML-only emails)
func StripHTML(html string) string {
	// Remove script and style elements (using separate patterns since RE2 doesn't support backreferences)
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = scriptRe.ReplaceAllString(html, "")
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = styleRe.ReplaceAllString(html, "")

	// Replace common block elements with newlines
	blockElements := []string{"</p>", "</div>", "</tr>", "</li>", "<br>", "<br/>", "<br />"}
	for _, elem := range blockElements {
		html = strings.ReplaceAll(html, elem, "\n")
	}

	// Remove all remaining HTML tags
	tagRe := regexp.MustCompile(`<[^>]*>`)
	text := tagRe.ReplaceAllString(html, "")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

// GetBodyText returns the best text representation of the email body
func (e *ParsedEmail) GetBodyText() string {
	if e.PlainBody != "" {
		return e.PlainBody
	}
	if e.HTMLBody != "" {
		return StripHTML(e.HTMLBody)
	}
	return ""
}

// GetSubjectForItem returns a cleaned subject for use as item title
func (e *ParsedEmail) GetSubjectForItem() string {
	subject := e.Subject

	// Remove Re: and Fwd: prefixes
	prefixes := []string{"Re:", "RE:", "Fwd:", "FWD:", "Fw:", "FW:"}
	for _, prefix := range prefixes {
		for strings.HasPrefix(subject, prefix) {
			subject = strings.TrimPrefix(subject, prefix)
			subject = strings.TrimSpace(subject)
		}
	}

	if subject == "" {
		subject = "(No Subject)"
	}

	return subject
}

// IsReply checks if this email is a reply to another email
func (e *ParsedEmail) IsReply() bool {
	return e.InReplyTo != "" || len(e.References) > 0
}

// GetThreadIDs returns message IDs that could reference the original thread
func (e *ParsedEmail) GetThreadIDs() []string {
	var ids []string

	// In-Reply-To takes priority
	if e.InReplyTo != "" {
		ids = append(ids, e.InReplyTo)
	}

	// Then References (in reverse order, most recent first)
	for i := len(e.References) - 1; i >= 0; i-- {
		ref := e.References[i]
		// Avoid duplicates
		found := false
		for _, id := range ids {
			if id == ref {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, ref)
		}
	}

	return ids
}

// FormatDate formats the email date for display
func (e *ParsedEmail) FormatDate() string {
	if e.Date.IsZero() {
		return ""
	}
	return e.Date.Format(time.RFC1123)
}
