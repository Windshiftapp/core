//go:build test

package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MockHTTPRequest creates a mock HTTP request for testing handlers
func MockHTTPRequest(method, url string, body interface{}) (*http.Request, error) {
	var reqBody *bytes.Buffer
	
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer([]byte{})
	}
	
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// ResponseRecorder wraps httptest.ResponseRecorder with additional helper methods
type ResponseRecorder struct {
	*httptest.ResponseRecorder
	t *testing.T
}

// NewResponseRecorder creates a new ResponseRecorder
func NewResponseRecorder(t *testing.T) *ResponseRecorder {
	return &ResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		t:                t,
	}
}

// AssertStatusCode verifies the HTTP status code
func (r *ResponseRecorder) AssertStatusCode(expected int) *ResponseRecorder {
	if r.Code != expected {
		r.t.Fatalf("Expected status code %d, got %d. Body: %s", expected, r.Code, r.Body.String())
	}
	return r
}

// AssertContentType verifies the Content-Type header
func (r *ResponseRecorder) AssertContentType(expected string) *ResponseRecorder {
	contentType := r.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, expected) {
		r.t.Fatalf("Expected Content-Type %s, got %s", expected, contentType)
	}
	return r
}

// AssertJSONResponse verifies the response is valid JSON and unmarshals it
func (r *ResponseRecorder) AssertJSONResponse(target interface{}) *ResponseRecorder {
	r.AssertContentType("application/json")
	
	if err := json.Unmarshal(r.Body.Bytes(), target); err != nil {
		r.t.Fatalf("Failed to unmarshal JSON response: %v. Body: %s", err, r.Body.String())
	}
	
	return r
}

// AssertBodyContains verifies the response body contains the expected string
func (r *ResponseRecorder) AssertBodyContains(expected string) *ResponseRecorder {
	body := r.Body.String()
	if !strings.Contains(body, expected) {
		r.t.Fatalf("Expected response body to contain '%s', but got: %s", expected, body)
	}
	return r
}

// AssertBodyEquals verifies the response body equals the expected string
func (r *ResponseRecorder) AssertBodyEquals(expected string) *ResponseRecorder {
	body := r.Body.String()
	if body != expected {
		r.t.Fatalf("Expected response body '%s', but got: %s", expected, body)
	}
	return r
}

// AssertHeaderExists verifies a header exists
func (r *ResponseRecorder) AssertHeaderExists(header string) *ResponseRecorder {
	if r.Header().Get(header) == "" {
		r.t.Fatalf("Expected header '%s' to exist", header)
	}
	return r
}

// AssertHeaderEquals verifies a header has the expected value
func (r *ResponseRecorder) AssertHeaderEquals(header, expected string) *ResponseRecorder {
	actual := r.Header().Get(header)
	if actual != expected {
		r.t.Fatalf("Expected header '%s' to be '%s', got '%s'", header, expected, actual)
	}
	return r
}

// GetJSONField extracts a field from JSON response
func (r *ResponseRecorder) GetJSONField(fieldPath string) interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		r.t.Fatalf("Failed to unmarshal JSON for field extraction: %v", err)
	}
	
	// Simple field path extraction (supports only top-level fields for now)
	return result[fieldPath]
}

// TestHandler represents a handler function for testing
type TestHandler func(w http.ResponseWriter, r *http.Request)

// ExecuteRequest executes a request against a handler and returns a ResponseRecorder
func ExecuteRequest(t *testing.T, handler TestHandler, req *http.Request) *ResponseRecorder {
	rr := NewResponseRecorder(t)
	handler(rr, req)
	return rr
}

// CreateJSONRequest creates a JSON request with the given method, URL, and body
func CreateJSONRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	req, err := MockHTTPRequest(method, url, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	return req
}

// AssertValidationError checks if the response contains a validation error
func AssertValidationError(t *testing.T, rr *ResponseRecorder, expectedMessage string) {
	rr.AssertStatusCode(http.StatusBadRequest)
	rr.AssertBodyContains(expectedMessage)
}

// AssertInternalServerError checks if the response is an internal server error
func AssertInternalServerError(t *testing.T, rr *ResponseRecorder) {
	rr.AssertStatusCode(http.StatusInternalServerError)
}

// AssertSuccessResponse checks if the response indicates success
func AssertSuccessResponse(t *testing.T, rr *ResponseRecorder) {
	if rr.Code < 200 || rr.Code >= 300 {
		t.Fatalf("Expected success status code (2xx), got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

// TestData contains commonly used test data structures
type TestData struct {
	ValidUser struct {
		Email     string
		Username  string
		FirstName string
		LastName  string
		Password  string
	}
	
	InvalidUser struct {
		Email     string
		Username  string
		FirstName string
		LastName  string
		Password  string
	}
	
	ValidModuleSettings struct {
		TimeTrackingEnabled  bool
		TestManagementEnabled bool
	}
}

// GetTestData returns a TestData instance with predefined test data
func GetTestData() TestData {
	data := TestData{}
	
	data.ValidUser.Email = "admin@example.com"
	data.ValidUser.Username = "admin"
	data.ValidUser.FirstName = "Admin"
	data.ValidUser.LastName = "User"
	data.ValidUser.Password = "password123"
	
	data.InvalidUser.Email = "invalid-email"
	data.InvalidUser.Username = ""
	data.InvalidUser.FirstName = ""
	data.InvalidUser.LastName = ""
	data.InvalidUser.Password = "123" // Too short
	
	data.ValidModuleSettings.TimeTrackingEnabled = true
	data.ValidModuleSettings.TestManagementEnabled = false
	
	return data
}

// JSONEqual compares two JSON strings for equality (ignoring formatting)
func JSONEqual(t *testing.T, expected, actual string) {
	var expectedMap, actualMap interface{}
	
	if err := json.Unmarshal([]byte(expected), &expectedMap); err != nil {
		t.Fatalf("Failed to unmarshal expected JSON: %v", err)
	}
	
	if err := json.Unmarshal([]byte(actual), &actualMap); err != nil {
		t.Fatalf("Failed to unmarshal actual JSON: %v", err)
	}
	
	expectedBytes, _ := json.Marshal(expectedMap)
	actualBytes, _ := json.Marshal(actualMap)
	
	if string(expectedBytes) != string(actualBytes) {
		t.Fatalf("JSON not equal.\nExpected: %s\nActual: %s", expectedBytes, actualBytes)
	}
}

// IntToString converts an integer to string for URL path construction
func IntToString(i int) string {
	return fmt.Sprintf("%d", i)
}