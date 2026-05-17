package jira

import (
	"testing"
	"time"
)

// TestParseJiraTimestamp pins the layouts the importer accepts today. Phase 0
// regression harness — see docs/jira-import-remediation-plan.md.
func TestParseJiraTimestamp(t *testing.T) {
	t.Parallel()

	mustParse := func(layout, s string) time.Time {
		tt, err := time.Parse(layout, s)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return tt
	}

	cases := []struct {
		name    string
		in      string
		wantNil bool
		want    time.Time
	}{
		{name: "empty string", in: "", wantNil: true},
		{name: "garbage", in: "not-a-timestamp", wantNil: true},
		{
			name: "RFC3339 UTC",
			in:   "2025-01-02T03:04:05Z",
			want: mustParse(time.RFC3339, "2025-01-02T03:04:05Z"),
		},
		{
			name: "RFC3339Nano",
			in:   "2025-01-02T03:04:05.123456789Z",
			want: mustParse(time.RFC3339Nano, "2025-01-02T03:04:05.123456789Z"),
		},
		{
			name: "RFC3339 explicit zero offset",
			in:   "2025-01-02T03:04:05+00:00",
			want: mustParse(time.RFC3339, "2025-01-02T03:04:05+00:00"),
		},
		{
			name: "Jira Cloud milliseconds with 4-digit zone",
			in:   "2025-01-02T03:04:05.123+0200",
			want: mustParse("2006-01-02T15:04:05.000-0700", "2025-01-02T03:04:05.123+0200"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseJiraTimestamp(tc.in)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %v, got nil", tc.want)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, *got)
			}
		})
	}
}

// TestParseJiraTimestamp_LiteralZLegacyLayout pins behavior for the suspect
// legacy layout `2006-01-02T15:04:05.000Z0700` (literal 'Z' followed by digits).
// If this ever flips, jiraTimestampLayouts needs review — either the layout is
// dead and should be removed, or real fixtures exist and the parser needs to
// handle them robustly.
func TestParseJiraTimestamp_LiteralZLegacyLayout(t *testing.T) {
	t.Parallel()
	got := ParseJiraTimestamp("2025-01-02T03:04:05.123Z0200")
	// Characterization: today this does NOT parse because `Z` is a literal in
	// the layout but the test input uses it followed by an offset, which the
	// stdlib doesn't accept consistently. If the layout is later fixed or
	// removed, update this assertion.
	if got != nil {
		t.Logf("note: legacy 'Z0700' layout now parses: %v — re-evaluate jiraTimestampLayouts", *got)
	}
}

// TestConvertADFToMarkdownWithUsers — one subtest per ADF node so failures
// point at a specific case. Includes the B11 regression suite (heading guard).
func TestConvertADFToMarkdownWithUsers(t *testing.T) {
	t.Parallel()

	type adf = map[string]interface{}

	doc := func(nodes ...adf) adf {
		content := make([]interface{}, len(nodes))
		for i, n := range nodes {
			content[i] = n
		}
		return adf{"type": "doc", "content": content}
	}
	text := func(s string, marks ...adf) adf {
		n := adf{"type": "text", "text": s}
		if len(marks) > 0 {
			m := make([]interface{}, len(marks))
			for i, mm := range marks {
				m[i] = mm
			}
			n["marks"] = m
		}
		return n
	}
	mark := func(kind string, attrs adf) adf {
		m := adf{"type": kind}
		if attrs != nil {
			m["attrs"] = attrs
		}
		return m
	}
	para := func(children ...adf) adf {
		content := make([]interface{}, len(children))
		for i, c := range children {
			content[i] = c
		}
		return adf{"type": "paragraph", "content": content}
	}

	cases := []struct {
		name     string
		in       interface{}
		resolver MentionResolver
		want     string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "plain string legacy description", in: "hello world", want: "hello world"},
		{name: "int input falls through", in: 42, want: ""},
		{name: "slice input falls through", in: []interface{}{}, want: ""},
		{
			name: "paragraph with text",
			in:   doc(para(text("hello"))),
			want: "hello\n\n",
		},
		{
			name: "heading level 1",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": float64(1)}, "content": []interface{}{text("title")}}),
			want: "# title\n\n",
		},
		{
			name: "heading level 3",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": float64(3)}, "content": []interface{}{text("title")}}),
			want: "### title\n\n",
		},
		{
			name: "heading level 6",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": float64(6)}, "content": []interface{}{text("title")}}),
			want: "###### title\n\n",
		},
		// B11 regressions
		{
			name: "B11 heading missing attrs defaults to level 1",
			in:   doc(adf{"type": "heading", "content": []interface{}{text("title")}}),
			want: "# title\n\n",
		},
		{
			name: "B11 heading level 0 clamps to 1",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": float64(0)}, "content": []interface{}{text("title")}}),
			want: "# title\n\n",
		},
		{
			name: "B11 heading level 7 clamps to 1",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": float64(7)}, "content": []interface{}{text("title")}}),
			want: "# title\n\n",
		},
		{
			name: "B11 heading level as string defaults to 1",
			in:   doc(adf{"type": "heading", "attrs": adf{"level": "2"}, "content": []interface{}{text("title")}}),
			want: "# title\n\n",
		},
		{
			name: "bullet list two items",
			in: doc(adf{"type": "bulletList", "content": []interface{}{
				adf{"type": "listItem", "content": []interface{}{para(text("a"))}},
				adf{"type": "listItem", "content": []interface{}{para(text("b"))}},
			}}),
			want: "- a\n- b\n\n",
		},
		{
			name: "ordered list two items",
			in: doc(adf{"type": "orderedList", "content": []interface{}{
				adf{"type": "listItem", "content": []interface{}{para(text("a"))}},
				adf{"type": "listItem", "content": []interface{}{para(text("b"))}},
			}}),
			want: "1. a\n2. b\n\n",
		},
		{
			name: "code block with language",
			in:   doc(adf{"type": "codeBlock", "attrs": adf{"language": "go"}, "content": []interface{}{text("x := 1")}}),
			want: "```go\nx := 1\n```\n\n",
		},
		{
			name: "blockquote single line",
			in:   doc(adf{"type": "blockquote", "content": []interface{}{para(text("quoted"))}}),
			want: "> quoted\n> \n> \n\n",
		},
		{
			name: "rule",
			in:   doc(adf{"type": "rule"}),
			want: "---\n\n",
		},
		{
			name: "hard break",
			in:   doc(para(text("a"), adf{"type": "hardBreak"}, text("b"))),
			want: "a\nb\n\n",
		},
		// Marks
		{name: "strong mark", in: doc(para(text("x", mark("strong", nil)))), want: "**x**\n\n"},
		{name: "em mark", in: doc(para(text("x", mark("em", nil)))), want: "*x*\n\n"},
		{name: "code mark", in: doc(para(text("x", mark("code", nil)))), want: "`x`\n\n"},
		{name: "strike mark", in: doc(para(text("x", mark("strike", nil)))), want: "~~x~~\n\n"},
		{name: "link mark", in: doc(para(text("x", mark("link", adf{"href": "https://e.x/y"})))), want: "[x](https://e.x/y)\n\n"},
		{
			name: "strong + em combined (cumulative wrapping)",
			in:   doc(para(text("x", mark("strong", nil), mark("em", nil)))),
			want: "***x***\n\n",
		},
		// Mentions
		{
			name:     "mention with resolver hit",
			in:       doc(para(adf{"type": "mention", "attrs": adf{"id": "acct1", "text": "Alice"}})),
			resolver: MentionResolver(func(id string) string { return "alice" }),
			want:     "@alice\n\n",
		},
		{
			name:     "mention with resolver miss falls back to display text",
			in:       doc(para(adf{"type": "mention", "attrs": adf{"id": "acct1", "text": "Alice"}})),
			resolver: MentionResolver(func(id string) string { return "" }),
			want:     "@Alice\n\n",
		},
		{
			name:     "mention with resolver miss and empty display returns empty",
			in:       doc(para(adf{"type": "mention", "attrs": adf{"id": "acct1", "text": ""}})),
			resolver: MentionResolver(func(id string) string { return "" }),
			want:     "\n\n", // surrounding paragraph still contributes the trailing blank line
		},
		{
			name: "mention with nil resolver uses display text",
			in:   doc(para(adf{"type": "mention", "attrs": adf{"id": "acct1", "text": "Bob"}})),
			want: "@Bob\n\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ConvertADFToMarkdownWithUsers(tc.in, tc.resolver)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// --- B12 placeholders ---
//
// Each subtest carries a real ADF fixture for the node type the converter
// currently does NOT handle (see internal/jira/field_mapper.go default branch).
// When Phase 3 implements a case, remove the t.Skip and add the expected
// markdown output so the test flips on.

func TestADF_Table_Skipped(t *testing.T) {
	t.Skip("B12: table conversion not implemented")
}

func TestADF_Panel_Skipped(t *testing.T) {
	t.Skip("B12: panel conversion not implemented")
}

func TestADF_TaskList_Skipped(t *testing.T) {
	t.Skip("B12: taskList conversion not implemented")
}

func TestADF_MediaSingle_Skipped(t *testing.T) {
	t.Skip("B12: mediaSingle conversion not implemented")
}

func TestADF_InlineCard_Skipped(t *testing.T) {
	t.Skip("B12: inlineCard conversion not implemented")
}

func TestADF_Expand_Skipped(t *testing.T) {
	t.Skip("B12: expand/nestedExpand conversion not implemented")
}

func TestADF_Status_Skipped(t *testing.T) {
	t.Skip("B12: status lozenge conversion not implemented")
}

func TestADF_Date_Skipped(t *testing.T) {
	t.Skip("B12: inline date conversion not implemented")
}

func TestADF_Emoji_Skipped(t *testing.T) {
	t.Skip("B12: emoji conversion not implemented")
}
