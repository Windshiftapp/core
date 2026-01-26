package main

import (
	"testing"
	"time"
)

func TestParseRelativeDate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFrom  bool // just check if from is set
		wantTo    bool // just check if to is set
		wantErr   bool
		checkDays int // expected days difference from today (approximate)
	}{
		{
			name:      "today",
			input:     "today",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 0,
		},
		{
			name:      "week",
			input:     "week",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 7,
		},
		{
			name:      "month",
			input:     "month",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 30, // approximate
		},
		{
			name:      "year",
			input:     "year",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 365,
		},
		{
			name:      "-7d format",
			input:     "-7d",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 7,
		},
		{
			name:      "-30d format",
			input:     "-30d",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 30,
		},
		{
			name:      "case insensitive",
			input:     "TODAY",
			wantFrom:  true,
			wantTo:    true,
			checkDays: 0,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "invalid -Nd format",
			input:   "-abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, err := parseRelativeDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRelativeDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.wantFrom && from == "" {
				t.Errorf("parseRelativeDate() from is empty, want non-empty")
			}
			if tt.wantTo && to == "" {
				t.Errorf("parseRelativeDate() to is empty, want non-empty")
			}
			if from != "" {
				// Verify the date is valid
				_, err := time.Parse("2006-01-02", from)
				if err != nil {
					t.Errorf("parseRelativeDate() from date %q is not valid: %v", from, err)
				}
			}
			if to != "" {
				// Verify the date is valid
				_, err := time.Parse("2006-01-02", to)
				if err != nil {
					t.Errorf("parseRelativeDate() to date %q is not valid: %v", to, err)
				}
			}
		})
	}
}

func TestIsNegatedFilter(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"~done", true},
		{"~todo", true},
		{"done", false},
		{"todo", false},
		{"", false},
		{"~~done", true}, // still starts with ~
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isNegatedFilter(tt.input); got != tt.want {
				t.Errorf("isNegatedFilter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripNegation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"~done", "done"},
		{"~todo", "todo"},
		{"done", "done"},
		{"todo", "todo"},
		{"", ""},
		{"~~done", "~done"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripNegation(tt.input); got != tt.want {
				t.Errorf("stripNegation(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDateFormats(t *testing.T) {
	// Test that today returns today's date
	from, to, err := parseRelativeDate("today")
	if err != nil {
		t.Fatalf("parseRelativeDate(today) error: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	if from != today {
		t.Errorf("parseRelativeDate(today) from = %v, want %v", from, today)
	}
	if to != tomorrow {
		t.Errorf("parseRelativeDate(today) to = %v, want %v", to, tomorrow)
	}

	// Test -7d returns date 7 days ago
	from, _, err = parseRelativeDate("-7d")
	if err != nil {
		t.Fatalf("parseRelativeDate(-7d) error: %v", err)
	}

	expected := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	if from != expected {
		t.Errorf("parseRelativeDate(-7d) from = %v, want %v", from, expected)
	}
}

func TestParseDateUpperCase(t *testing.T) {
	// Test uppercase -7D works
	from, _, err := parseRelativeDate("-7D")
	if err != nil {
		t.Fatalf("parseRelativeDate(-7D) error: %v", err)
	}
	if from == "" {
		t.Error("parseRelativeDate(-7D) from is empty, want non-empty")
	}

	// Verify it's case insensitive
	fromLower, _, _ := parseRelativeDate("-7d")
	if from != fromLower {
		t.Errorf("parseRelativeDate() case insensitive mismatch: -7D=%v, -7d=%v", from, fromLower)
	}
}
