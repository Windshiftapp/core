package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/services"
)

type calendarPreferencesStub struct{ patch services.UserPreferencesPatch }

func (s *calendarPreferencesStub) GetSnapshot(int) (services.UserPreferencesSnapshot, error) {
	return services.UserPreferencesSnapshot{Calendar: "auto"}, nil
}
func (s *calendarPreferencesStub) UpdateSnapshot(_ int, patch services.UserPreferencesPatch) (services.UserPreferencesSnapshot, error) {
	s.patch = patch
	if patch.CalendarSet && patch.Calendar != "auto" && patch.Calendar != "gregory" && patch.Calendar != "persian" {
		return services.UserPreferencesSnapshot{}, services.ErrInvalidCalendar
	}
	return services.UserPreferencesSnapshot{Calendar: patch.Calendar}, nil
}
func TestCalendarPreferencePatchContract(t *testing.T) {
	cases := []struct {
		body   string
		set    bool
		value  string
		status int
	}{
		{"{}", false, "", 0},
		{"{\"calendar\":null}", true, "auto", 0},
		{"{\"calendar\":\"persian\"}", true, "persian", 0},
		{"{\"calendar\":\"invalid\"}", true, "invalid", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			var input preferencesPatchRequest
			if err := json.Unmarshal([]byte(tc.body), &input); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPatch, "/users/me/preferences", nil)
			req = req.WithContext(context.WithValue(req.Context(), contextkeys.User, &models.User{ID: 7}))
			stub := &calendarPreferencesStub{}
			_, err := updatePreferences(stub)(req, input)
			if tc.status == 0 && err != nil {
				t.Fatal(err)
			}
			if tc.status != 0 {
				e, ok := err.(*Error)
				if !ok || e.Status != tc.status {
					t.Fatalf("expected status %d, got %v", tc.status, err)
				}
			}
			if stub.patch.CalendarSet != tc.set || stub.patch.Calendar != tc.value {
				t.Fatalf("unexpected patch: %+v", stub.patch)
			}
		})
	}
}
