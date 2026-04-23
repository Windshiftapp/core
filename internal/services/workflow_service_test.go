package services

import "testing"

func TestSlugifyStatusName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"In Review", "in-review"},
		{"IN REVIEW", "in-review"},
		{"in-review", "in-review"},
		{"in--review", "in-review"},
		{"  in review  ", "in-review"},
		{"start review / ready", "start-review-ready"},
		{"#close", "close"},
		{"", ""},
		{"   ", ""},
		{"Done!", "done"},
		{"v1.0", "v1-0"},
	}
	for _, c := range cases {
		if got := slugifyStatusName(c.in); got != c.want {
			t.Errorf("slugifyStatusName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
