package handlers

import (
	"errors"
	"strings"
	"testing"
)

// Pure-function regression tests for bughunt14 findings #3, #4, and #6:
// quote-aware top-level "and" splitting, the SCIM error classifier helper,
// and strict boolean parsing for the `active` attribute. Handler-level
// coverage for the transactional ReplaceGroup (#2) and the scan-error
// propagation paths (#5) lives in the e2e suite (core-tests/) because the
// surrounding plumbing — PermissionService schema, audit logger — is not
// bootstrapped in this package; see item_links_test.go for the same
// trade-off.

func TestSplitTopLevelAnd_QuotedAnd(t *testing.T) {
	parts := splitTopLevelAnd(`userName eq "Research and Development"`)
	if len(parts) != 1 {
		t.Fatalf("quoted 'and' must not split: got %d parts: %#v", len(parts), parts)
	}
}

func TestSplitTopLevelAnd_MultipleTopLevel(t *testing.T) {
	parts := splitTopLevelAnd(`userName eq "x" and active eq true and displayName co "y"`)
	want := []string{`userName eq "x"`, `active eq true`, `displayName co "y"`}
	if len(parts) != len(want) {
		t.Fatalf("expected %d parts, got %d: %#v", len(want), len(parts), parts)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("part %d: got %q want %q", i, parts[i], want[i])
		}
	}
}

func TestSplitTopLevelAnd_ParenGrouping(t *testing.T) {
	parts := splitTopLevelAnd(`(displayName eq "A and B") and active eq true`)
	want := []string{`(displayName eq "A and B")`, `active eq true`}
	if len(parts) != len(want) {
		t.Fatalf("expected %d parts, got %d: %#v", len(want), len(parts), parts)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("part %d: got %q want %q", i, parts[i], want[i])
		}
	}
}

func TestSplitTopLevelAnd_EscapedQuote(t *testing.T) {
	parts := splitTopLevelAnd(`displayName eq "A\"and\"B"`)
	if len(parts) != 1 {
		t.Fatalf("escaped quotes must not break quote tracking: got %d parts: %#v", len(parts), parts)
	}
}

func TestSplitTopLevelAnd_CaseInsensitive(t *testing.T) {
	parts := splitTopLevelAnd(`userName eq "x" AND active eq true`)
	if len(parts) != 2 {
		t.Fatalf("uppercase AND must split too: got %#v", parts)
	}
}

func TestExtractResourceTypeFilter_QuotedAnd(t *testing.T) {
	rt, rem := ExtractResourceTypeFilter(`meta.resourceType eq "Group" and displayName eq "A and B"`)
	if rt != "Group" {
		t.Fatalf("resourceType: got %q want %q", rt, "Group")
	}
	if rem != `displayName eq "A and B"` {
		t.Fatalf("remaining: got %q want %q", rem, `displayName eq "A and B"`)
	}
}

func TestParseSCIMFilter_BooleanStrict_Reject(t *testing.T) {
	for _, value := range []string{"banana", "1", "0", "yes", "no", ""} {
		_, err := ParseSCIMFilter("active eq "+value, "User")
		if err == nil {
			t.Fatalf("active eq %q must error", value)
		}
		if !strings.Contains(err.Error(), "invalid filter") {
			t.Fatalf("active eq %q: error must contain 'invalid filter', got: %v", value, err)
		}
	}
}

func TestParseSCIMFilter_BooleanStrict_AcceptTrueFalse(t *testing.T) {
	cases := []struct {
		filter   string
		wantArg  bool
		wantWhere string
	}{
		{"active eq true", true, "is_active = ?"},
		{"active eq TRUE", true, "is_active = ?"},
		{"active eq false", false, "is_active = ?"},
		{"active ne false", false, "is_active != ?"},
	}
	for _, tc := range cases {
		got, err := ParseSCIMFilter(tc.filter, "User")
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.filter, err)
		}
		if got.WhereClause != tc.wantWhere {
			t.Fatalf("%q: WhereClause %q want %q", tc.filter, got.WhereClause, tc.wantWhere)
		}
		if len(got.Args) != 1 || got.Args[0] != tc.wantArg {
			t.Fatalf("%q: Args %#v want [%v]", tc.filter, got.Args, tc.wantArg)
		}
	}
}

func TestParseSCIMFilterWithAnd_QuotedAndInValue(t *testing.T) {
	// userName cannot legally contain " and " (LIKE quirks aside), so use
	// displayName which maps to a concatenated column where the value is a
	// pass-through. The point of the test is that the top-level splitter
	// keeps the quoted value intact.
	got, err := ParseSCIMFilterWithAnd(`displayName eq "Research and Development" and active eq true`, "User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two clauses (one per side of the top-level `and`), AND-joined.
	if !strings.Contains(got.WhereClause, " AND ") {
		t.Fatalf("expected AND-joined clauses; got %q", got.WhereClause)
	}
	if len(got.Args) != 2 {
		t.Fatalf("expected 2 args, got %#v", got.Args)
	}
	if got.Args[0] != "Research and Development" {
		t.Fatalf("first arg must be the full quoted value; got %#v", got.Args[0])
	}
	if got.Args[1] != true {
		t.Fatalf("second arg must be the parsed boolean true; got %#v", got.Args[1])
	}
}

func TestIsInvalidFilterErr(t *testing.T) {
	if !isInvalidFilterErr(errors.New("invalid filter: bad token")) {
		t.Fatal("must classify invalid filter prefix")
	}
	if isInvalidFilterErr(errors.New("connection refused")) {
		t.Fatal("must not classify generic DB error")
	}
	if isInvalidFilterErr(nil) {
		t.Fatal("nil must not classify")
	}
}
