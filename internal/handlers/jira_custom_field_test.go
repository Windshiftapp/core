package handlers

import (
	"reflect"
	"testing"

	"windshift/internal/jira"
)

// TestExtractCustomFieldValue pins the current user/users behavior and
// characterizes the Phase 0 "non-user types are silently dropped" gates. The
// gate cases (10-13) are expected to flip when Phase 1.1 implements B01/B07.
func TestExtractCustomFieldValue(t *testing.T) {
	t.Parallel()

	userMap := map[string]int{
		"acct-known":   42,
		"acct-known-2": 99,
		"acct-known-3": 7,
	}

	mkFields := func(jiraID string, value interface{}) *jira.JiraIssueFields {
		f := &jira.JiraIssueFields{
			CustomFields: map[string]interface{}{jiraID: value},
		}
		return f
	}

	cases := []struct {
		name    string
		mapping CustomFieldMapping
		fields  *jira.JiraIssueFields
		wantOK  bool
		want    any
	}{
		// --- behavior-preserving: user ---
		{
			name:    "single user known accountID",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  mkFields("cf_1", map[string]interface{}{"accountId": "acct-known"}),
			wantOK:  true,
			want:    42,
		},
		{
			name:    "single user unknown accountID skipped",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  mkFields("cf_1", map[string]interface{}{"accountId": "acct-missing"}),
			wantOK:  false,
		},
		{
			name:    "single user value not an object",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  mkFields("cf_1", "not-an-object"),
			wantOK:  false,
		},
		{
			name:    "single user value nil",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  mkFields("cf_1", nil),
			wantOK:  false,
		},
		{
			name:    "skip action overrides everything",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user", Action: "skip"},
			fields:  mkFields("cf_1", map[string]interface{}{"accountId": "acct-known"}),
			wantOK:  false,
		},

		// --- behavior-preserving: users ---
		{
			name:    "multi-user two of three IDs known",
			mapping: CustomFieldMapping{JiraID: "cf_2", WindshiftType: "users"},
			fields: mkFields("cf_2", []interface{}{
				map[string]interface{}{"accountId": "acct-known"},
				map[string]interface{}{"accountId": "acct-missing"},
				map[string]interface{}{"accountId": "acct-known-2"},
			}),
			wantOK: true,
			want:   []int{42, 99},
		},
		{
			name:    "multi-user zero IDs known returns ok=false",
			mapping: CustomFieldMapping{JiraID: "cf_2", WindshiftType: "users"},
			fields: mkFields("cf_2", []interface{}{
				map[string]interface{}{"accountId": "acct-missing"},
				map[string]interface{}{"accountId": "acct-also-missing"},
			}),
			wantOK: false,
		},
		{
			name:    "multi-user value not an array",
			mapping: CustomFieldMapping{JiraID: "cf_2", WindshiftType: "users"},
			fields:  mkFields("cf_2", map[string]interface{}{"accountId": "acct-known"}),
			wantOK:  false,
		},

		// --- common ---
		{
			name:    "field key missing from CustomFields",
			mapping: CustomFieldMapping{JiraID: "cf_absent", WindshiftType: "user"},
			fields:  mkFields("cf_other", map[string]interface{}{"accountId": "acct-known"}),
			wantOK:  false,
		},
		{
			name:    "nil fields",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  nil,
			wantOK:  false,
		},
		{
			name:    "nil CustomFields map",
			mapping: CustomFieldMapping{JiraID: "cf_1", WindshiftType: "user"},
			fields:  &jira.JiraIssueFields{},
			wantOK:  false,
		},

		// --- Phase 0 gates: remove these cases when Phase 1.1 lands ---
		// B01/B07: importer currently drops every non-user/users WindshiftType.
		{
			name:    "B01/B07 gate: text type dropped",
			mapping: CustomFieldMapping{JiraID: "cf_text", WindshiftType: "text"},
			fields:  mkFields("cf_text", "hello"),
			wantOK:  false,
		},
		{
			name:    "B01/B07 gate: number type dropped",
			mapping: CustomFieldMapping{JiraID: "cf_num", WindshiftType: "number"},
			fields:  mkFields("cf_num", 5.0),
			wantOK:  false,
		},
		{
			name:    "B01/B07 gate: select type dropped",
			mapping: CustomFieldMapping{JiraID: "cf_sel", WindshiftType: "select"},
			fields:  mkFields("cf_sel", map[string]interface{}{"value": "High"}),
			wantOK:  false,
		},
		{
			name:    "B01/B07 gate: multiselect type dropped",
			mapping: CustomFieldMapping{JiraID: "cf_msel", WindshiftType: "multiselect"},
			fields: mkFields("cf_msel", []interface{}{
				map[string]interface{}{"value": "A"},
				map[string]interface{}{"value": "B"},
			}),
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractCustomFieldValue(tc.mapping, tc.fields, userMap)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v (value=%v)", ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("value: got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}
