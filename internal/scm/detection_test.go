package scm

import (
	"testing"
)

func TestParseSmartCommitActions(t *testing.T) {
	d := NewItemKeyDetector()

	type want struct {
		key     string
		command string
		payload string
	}
	cases := []struct {
		name   string
		text   string
		prefix string
		want   []want
	}{
		{
			name: "single key single transition",
			text: "PROJ-1 #in-review",
			want: []want{{"PROJ-1", "in-review", ""}},
		},
		{
			name: "comment captures rest of line",
			text: "PROJ-1 #comment shipped it",
			want: []want{{"PROJ-1", "comment", "shipped it"}},
		},
		{
			name: "multi command single key",
			text: "PROJ-1 #in-review #comment finished first pass",
			want: []want{
				{"PROJ-1", "in-review", ""},
				{"PROJ-1", "comment", "finished first pass"},
			},
		},
		{
			name: "multi key multi command cross product",
			text: "PROJ-1 PROJ-2 #close #comment done",
			want: []want{
				{"PROJ-1", "close", ""},
				{"PROJ-1", "comment", "done"},
				{"PROJ-2", "close", ""},
				{"PROJ-2", "comment", "done"},
			},
		},
		{
			name:   "prefix restriction excludes other workspaces",
			text:   "PROJ-1 OTHER-9 #close",
			prefix: "PROJ",
			want:   []want{{"PROJ-1", "close", ""}},
		},
		{
			name: "comment stops at next command",
			text: "PROJ-1 #comment first part #close",
			want: []want{
				{"PROJ-1", "comment", "first part"},
				{"PROJ-1", "close", ""},
			},
		},
		{
			name: "ignores hash inside URL",
			text: "PROJ-1 see https://example.com/#section for details",
			want: nil,
		},
		{
			name: "no key on line means no actions",
			text: "random commit message #comment nothing",
			want: nil,
		},
		{
			name: "multi-line: each line independent",
			text: "PROJ-1 #close\nPROJ-2 #comment thanks",
			want: []want{
				{"PROJ-1", "close", ""},
				{"PROJ-2", "comment", "thanks"},
			},
		},
		{
			name: "transition args discarded",
			text: "PROJ-1 #in-review skip this",
			want: []want{{"PROJ-1", "in-review", ""}},
		},
		{
			name: "key after hash is ignored",
			text: "#close PROJ-1",
			want: nil,
		},
		{
			name: "keys-only line produces nothing",
			text: "PROJ-1 PROJ-2 fix bug",
			want: nil,
		},
		{
			name: "CRLF line endings",
			text: "PROJ-1 #close\r\nPROJ-2 #comment done",
			want: []want{
				{"PROJ-1", "close", ""},
				{"PROJ-2", "comment", "done"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := d.ParseSmartCommitActions(tc.text, tc.prefix)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d actions, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				if got[i].Key.Key != w.key {
					t.Errorf("action[%d].Key = %q, want %q", i, got[i].Key.Key, w.key)
				}
				if got[i].Command != w.command {
					t.Errorf("action[%d].Command = %q, want %q", i, got[i].Command, w.command)
				}
				if got[i].Payload != w.payload {
					t.Errorf("action[%d].Payload = %q, want %q", i, got[i].Payload, w.payload)
				}
			}
		})
	}
}
