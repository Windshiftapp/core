package template

import "testing"

func TestSubstitute(t *testing.T) {
	cases := []struct {
		name     string
		template string
		vars     map[string]string
		want     string
	}{
		{
			name:     "empty template",
			template: "",
			vars:     map[string]string{"a": "1"},
			want:     "",
		},
		{
			name:     "single var",
			template: "Hello {{name}}",
			vars:     map[string]string{"name": "World"},
			want:     "Hello World",
		},
		{
			name:     "dotted key",
			template: "{{type.name}} from {{requester.email}}",
			vars:     map[string]string{"type.name": "Bug", "requester.email": "x@y"},
			want:     "Bug from x@y",
		},
		{
			name:     "missing var stays literal",
			template: "{{a}} {{b}}",
			vars:     map[string]string{"a": "1"},
			want:     "1 {{b}}",
		},
		{
			name:     "whitespace inside braces is trimmed",
			template: "{{ name }}",
			vars:     map[string]string{"name": "ok"},
			want:     "ok",
		},
		{
			name:     "no placeholders",
			template: "static title",
			vars:     map[string]string{"a": "1"},
			want:     "static title",
		},
		{
			name:     "nil vars",
			template: "{{a}}",
			vars:     nil,
			want:     "{{a}}",
		},
		{
			name:     "empty value substitutes empty",
			template: "[{{a}}]",
			vars:     map[string]string{"a": ""},
			want:     "[]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Substitute(tc.template, tc.vars)
			if got != tc.want {
				t.Errorf("Substitute(%q, %v) = %q, want %q", tc.template, tc.vars, got, tc.want)
			}
		})
	}
}
