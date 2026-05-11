package utils

import (
	"strings"
	"testing"
)

// Round-trip: characters that bluemonday entity-encodes inside text nodes
// (', ", <, >, &) must be decoded back to their plain-text form so the value
// stored in the database is not "Jamie&#39;s" but "Jamie's". Output-context
// escaping happens at the renderer.
func TestStripHTMLTags_DecodesEntities(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Jamie's task", "Jamie's task"},
		{`He said "hello"`, `He said "hello"`},
		{"a & b", "a & b"},
		{"<script>alert(1)</script>safe", "safe"},
		{"plain text", "plain text"},
		{"", ""},
	}
	for _, c := range cases {
		got := StripHTMLTags(c.in)
		if got != c.want {
			t.Errorf("StripHTMLTags(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeCommentContent_DecodesEntities(t *testing.T) {
	in := "Don't ship a [bad](javascript:alert(1)) link"
	got := SanitizeCommentContent(in)
	if strings.Contains(got, "&#39;") {
		t.Errorf("SanitizeCommentContent kept entity-encoded apostrophe: %q", got)
	}
	if !strings.Contains(got, "Don't") {
		t.Errorf("SanitizeCommentContent dropped the apostrophe: %q", got)
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("SanitizeCommentContent did not neutralize dangerous URL: %q", got)
	}
}

func TestSanitizeDescription_DecodesEntities_KeepsBR(t *testing.T) {
	in := "Line 1<br />Jamie's text<br />with \"quotes\""
	got := SanitizeDescription(in)
	if strings.Contains(got, "&#39;") || strings.Contains(got, "&#34;") {
		t.Errorf("SanitizeDescription left entity-encoded chars: %q", got)
	}
	if !strings.Contains(got, "<br />") {
		t.Errorf("SanitizeDescription dropped <br /> tags: %q", got)
	}
	if !strings.Contains(got, `Jamie's`) || !strings.Contains(got, `"quotes"`) {
		t.Errorf("SanitizeDescription mangled the round-trip: %q", got)
	}
}

func TestSanitizeDescription_StripsDangerousMarkdownURLs(t *testing.T) {
	in := "Click [here](javascript:alert(1)) please"
	got := SanitizeDescription(in)
	if strings.Contains(got, "javascript:") {
		t.Errorf("SanitizeDescription left a javascript: URL: %q", got)
	}
}

func TestSanitizeTitle_RegressionFree(t *testing.T) {
	if got := SanitizeTitle("Jamie's task"); got != "Jamie's task" {
		t.Errorf("SanitizeTitle round-trip broken: %q", got)
	}
}
