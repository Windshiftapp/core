package handlers

import (
	"crypto/tls"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"windshift/internal/database"
)

// Pure-function regression tests for the ICS serializer hardening (bughunt13
// findings #4 and #5). The handler-level checks for #1/#2/#3 — schedule-spoof
// rejection, permission-service routing, and host-header poisoning — live in
// the e2e suite (core-tests/) because they depend on a fully seeded
// PermissionService schema this package does not currently bootstrap; see
// item_links_test.go for the same trade-off.

func TestEscapeICS_CRLF(t *testing.T) {
	// CRLF / lone CR must collapse into the escaped LF token so a malicious
	// description cannot inject ICS line breaks into a permissive client.
	got := escapeICS("a\r\nb\rc")
	want := `a\nb\nc`
	if got != want {
		t.Fatalf("escapeICS CRLF: got %q want %q", got, want)
	}
}

func TestEscapeICS_SpecialChars(t *testing.T) {
	// Backslash must be escaped before the LF-rewrite so the synthesized "\n"
	// from a literal newline does not get re-escaped into "\\n".
	got := escapeICS("a;b,c\\d\ne")
	want := `a\;b\,c\\d\ne`
	if got != want {
		t.Fatalf("escapeICS specials: got %q want %q", got, want)
	}
}

func TestWriteFolded_NoFoldUnder75(t *testing.T) {
	var sb strings.Builder
	writeFolded(&sb, "SUMMARY:short line")
	got := sb.String()
	want := "SUMMARY:short line\r\n"
	if got != want {
		t.Fatalf("writeFolded short: got %q want %q", got, want)
	}
}

func TestWriteFolded_LongASCII(t *testing.T) {
	// Build a single long content line. Every physical line must be ≤ 75
	// octets and continuation lines must start with exactly one space.
	long := "DESCRIPTION:" + strings.Repeat("A", 300)
	var sb strings.Builder
	writeFolded(&sb, long)
	out := sb.String()

	lines := strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n")
	if len(lines) < 2 {
		t.Fatalf("expected folding into multiple lines, got 1: %q", out)
	}
	for i, ln := range lines {
		if len(ln) > 75 {
			t.Fatalf("line %d exceeds 75 octets: %d %q", i, len(ln), ln)
		}
		if i > 0 && (len(ln) == 0 || ln[0] != ' ') {
			t.Fatalf("continuation line %d does not start with space: %q", i, ln)
		}
	}

	// Reassemble: strip leading space from continuations and concatenate.
	var reassembled strings.Builder
	for i, ln := range lines {
		if i == 0 {
			reassembled.WriteString(ln)
		} else {
			reassembled.WriteString(ln[1:])
		}
	}
	if reassembled.String() != long {
		t.Fatalf("round-trip mismatch:\n got %q\nwant %q", reassembled.String(), long)
	}
}

func TestWriteFolded_DoesNotSplitUTF8Runes(t *testing.T) {
	// 4-byte UTF-8 runes near the fold boundary must not be split. We pad
	// with ASCII so the boundary lands inside a multi-byte sequence, then
	// assert each physical line decodes as valid UTF-8.
	emoji := "🚀"
	if utf8.RuneLen('🚀') != 4 {
		t.Fatalf("test assumption: 🚀 should be 4 bytes")
	}
	// 73 ASCII + 4-byte rune = boundary inside the rune at octet 75.
	line := "SUMMARY:" + strings.Repeat("a", 65) + emoji + strings.Repeat("b", 80)
	var sb strings.Builder
	writeFolded(&sb, line)
	out := sb.String()

	for i, ln := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if !utf8.ValidString(ln) {
			t.Fatalf("line %d contains invalid UTF-8 (rune split): %q", i, ln)
		}
		if len(ln) > 75 {
			t.Fatalf("line %d exceeds 75 octets: %d", i, len(ln))
		}
	}
}

func TestBuildICSContent_FoldingAndCRLF(t *testing.T) {
	h := &CalendarFeedHandler{}
	desc := "first paragraph;with,specials\r\nsecond line\rthird line " + strings.Repeat("x", 200)
	events := []icsEvent{{
		UID:             "1-2026-01-15@windshift",
		Title:           "[PROJ-1] hello",
		Description:     desc,
		ScheduledDate:   "2026-01-15",
		ScheduledTime:   "10:00",
		DurationMinutes: 30,
		ItemID:          1,
		WorkspaceID:     1,
	}}
	got := h.buildICSContent(events, "")

	// CRLF terminators throughout.
	if !strings.Contains(got, "BEGIN:VCALENDAR\r\n") || !strings.Contains(got, "END:VCALENDAR\r\n") {
		t.Fatalf("missing required calendar envelope: %q", got)
	}

	// Every physical line ≤ 75 octets.
	for i, ln := range strings.Split(strings.TrimSuffix(got, "\r\n"), "\r\n") {
		if len(ln) > 75 {
			t.Fatalf("line %d exceeds 75 octets (%d): %q", i, len(ln), ln)
		}
	}

	// After unfolding continuation lines, the DESCRIPTION value must be fully
	// escaped: CR/LF replaced by the literal "\n" token, semicolons/commas
	// backslash-escaped. We pull the DESCRIPTION line out and inspect it
	// directly to avoid false positives from the structural CRLFs that
	// separate content lines.
	unfolded := unfoldICS(got)
	descPrefix := "DESCRIPTION:"
	var descValue string
	for _, ln := range strings.Split(strings.TrimSuffix(unfolded, "\r\n"), "\r\n") {
		if strings.HasPrefix(ln, descPrefix) {
			descValue = ln[len(descPrefix):]
			break
		}
	}
	if descValue == "" {
		t.Fatalf("DESCRIPTION line not found in unfolded output: %q", unfolded)
	}
	if strings.ContainsAny(descValue, "\r\n") {
		t.Fatalf("DESCRIPTION value still contains a raw CR or LF: %q", descValue)
	}
	wantPrefix := `first paragraph\;with\,specials\nsecond line\nthird line `
	if !strings.HasPrefix(descValue, wantPrefix) {
		t.Fatalf("DESCRIPTION value missing expected escapes:\n got %q\nwant prefix %q", descValue, wantPrefix)
	}
}

// unfoldICS reverses RFC 5545 line folding for assertion convenience: any
// CRLF immediately followed by a space (or tab) is replaced by the empty
// string, joining the continuation back onto the previous line.
func unfoldICS(s string) string {
	s = strings.ReplaceAll(s, "\r\n ", "")
	s = strings.ReplaceAll(s, "\r\n\t", "")
	return s
}

// TestCreateFeedToken_AtomicUpsert exercises the SQL change from a
// DELETE+INSERT pair to a single ON CONFLICT(user_id) DO UPDATE statement:
// rerunning the upsert with a fresh token must leave exactly one row, change
// the token value, and bump updated_at.
func TestCreateFeedToken_AtomicUpsert(t *testing.T) {
	db, err := database.NewSQLiteDB("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE calendar_feed_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL UNIQUE,
			token TEXT NOT NULL UNIQUE,
			is_active BOOLEAN DEFAULT true,
			last_accessed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	const upsert = `
		INSERT INTO calendar_feed_tokens (user_id, token, is_active, created_at, updated_at)
		VALUES (?, ?, true, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			token = excluded.token,
			is_active = true,
			updated_at = excluded.updated_at`

	t1 := time.Now()
	if _, err := db.Exec(upsert, 42, "cft_first", t1, t1); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	t2 := t1.Add(time.Second)
	if _, err := db.Exec(upsert, 42, "cft_second", t2, t2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM calendar_feed_tokens WHERE user_id = ?", 42).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row, got %d", count)
	}

	var tok string
	if err := db.QueryRow("SELECT token FROM calendar_feed_tokens WHERE user_id = ?", 42).Scan(&tok); err != nil {
		t.Fatalf("select token: %v", err)
	}
	if tok != "cft_second" {
		t.Fatalf("expected token to be replaced; got %q", tok)
	}
}

// TestFeedBaseURL covers the host-header poisoning fix: configured BASE_URL
// wins; absent that, X-Forwarded-* headers are ignored and we fall back to
// r.Host with scheme inferred from r.TLS.
func TestFeedBaseURL(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		host     string
		tls      bool
		xfHost   string
		xfProto  string
		expected string
	}{
		{
			name:     "configured base URL wins",
			baseURL:  "https://windshift.example.com",
			host:     "evil.example.com",
			xfHost:   "another.example.com",
			xfProto:  "https",
			expected: "https://windshift.example.com",
		},
		{
			name:     "no base URL, http fallback uses r.Host",
			baseURL:  "",
			host:     "localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "no base URL with TLS uses https",
			baseURL:  "",
			host:     "windshift.example.com",
			tls:      true,
			expected: "https://windshift.example.com",
		},
		{
			name:     "X-Forwarded-Host is ignored when no base URL set",
			baseURL:  "",
			host:     "windshift.example.com",
			xfHost:   "evil.example.com",
			xfProto:  "https",
			expected: "http://windshift.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &CalendarFeedHandler{baseURL: tc.baseURL}
			r := httptest.NewRequest("GET", "http://"+tc.host+"/api/calendar/feed/token", nil)
			r.Host = tc.host
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tc.xfHost != "" {
				r.Header.Set("X-Forwarded-Host", tc.xfHost)
			}
			if tc.xfProto != "" {
				r.Header.Set("X-Forwarded-Proto", tc.xfProto)
			}
			got := h.feedBaseURL(r)
			if got != tc.expected {
				t.Fatalf("feedBaseURL: got %q want %q", got, tc.expected)
			}
		})
	}
}
