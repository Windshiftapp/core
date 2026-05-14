package database

import "testing"

func TestConvertPlaceholders_BarePlaceholders(t *testing.T) {
	got := ConvertPlaceholders("SELECT * FROM items WHERE id = ? AND name = ?")
	want := "SELECT * FROM items WHERE id = $1 AND name = $2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_NoPlaceholders(t *testing.T) {
	got := ConvertPlaceholders("SELECT * FROM items")
	want := "SELECT * FROM items"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_PreservesJSONBOperators(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "?| array key check",
			in:   "SELECT * FROM items WHERE data ?| ARRAY['k'] AND id = ?",
			want: "SELECT * FROM items WHERE data ?| ARRAY['k'] AND id = $1",
		},
		{
			name: "?& array key check",
			in:   "SELECT * FROM items WHERE data ?& ARRAY['k1','k2'] AND id = ?",
			want: "SELECT * FROM items WHERE data ?& ARRAY['k1','k2'] AND id = $1",
		},
		{
			name: "?? single key check",
			in:   "SELECT * FROM items WHERE data ?? 'key' AND id = ?",
			want: "SELECT * FROM items WHERE data ?? 'key' AND id = $1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ConvertPlaceholders(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConvertPlaceholders_SkipsInsideStringLiterals(t *testing.T) {
	got := ConvertPlaceholders("SELECT '?' AS literal, name FROM items WHERE id = ?")
	want := "SELECT '?' AS literal, name FROM items WHERE id = $1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_SkipsInsideQuotedIdentifiers(t *testing.T) {
	// Double-quoted identifiers are uncommon for ? but the walker should
	// still skip them so an oddly-named column doesn't shift placeholder
	// numbering.
	got := ConvertPlaceholders(`SELECT "weird?col", name FROM items WHERE id = ?`)
	want := `SELECT "weird?col", name FROM items WHERE id = $1`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_SkipsInsideLineComments(t *testing.T) {
	got := ConvertPlaceholders("SELECT * -- ?bogus\nFROM items WHERE id = ?")
	want := "SELECT * -- ?bogus\nFROM items WHERE id = $1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_SkipsInsideBlockComments(t *testing.T) {
	got := ConvertPlaceholders("SELECT /* ? ? ? */ name FROM items WHERE id = ?")
	want := "SELECT /* ? ? ? */ name FROM items WHERE id = $1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConvertPlaceholders_EscapedSingleQuoteInLiteral(t *testing.T) {
	// '' is the SQL escape for a single quote inside a literal. The walker
	// must not exit the string state on the inner ''.
	got := ConvertPlaceholders("SELECT 'O''Brien?', x FROM t WHERE id = ?")
	want := "SELECT 'O''Brien?', x FROM t WHERE id = $1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
