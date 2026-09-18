package v2

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"windshift/internal/repository"
	"windshift/internal/services"
)

func TestParseCommaSeparatedPositiveInts(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?label_ids=", nil)
		got, err := parseCommaSeparatedPositiveInts(req, "label_ids")
		if err != nil || got != nil {
			t.Fatalf("got %v err %v", got, err)
		}
	})

	t.Run("valid list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?label_ids=1,2,3", nil)
		got, err := parseCommaSeparatedPositiveInts(req, "label_ids")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?label_ids=1,abc", nil)
		_, err := parseCommaSeparatedPositiveInts(req, "label_ids")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
			t.Fatalf("expected bad request, got %v", err)
		}
	})
}

func TestOptionalBoolPtrQuery(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		got, err := optionalBoolPtrQuery(req, "has_item_links")
		if err != nil || got != nil {
			t.Fatalf("got %v err %v", got, err)
		}
	})

	t.Run("true and false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?has_item_links=true", nil)
		got, err := optionalBoolPtrQuery(req, "has_item_links")
		if err != nil || got == nil || !*got {
			t.Fatalf("got %v err %v", got, err)
		}

		req = httptest.NewRequest(http.MethodGet, "/?has_item_links=false", nil)
		got, err = optionalBoolPtrQuery(req, "has_item_links")
		if err != nil || got == nil || *got {
			t.Fatalf("got %v err %v", got, err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?has_item_links=maybe", nil)
		_, err := optionalBoolPtrQuery(req, "has_item_links")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
			t.Fatalf("expected bad request, got %v", err)
		}
	})
}

func TestRequirementErrorMapping(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		err := requirementError(services.ErrRequirementNotFound)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("repository not found", func(t *testing.T) {
		err := requirementError(repository.ErrNotFound)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		err := requirementError(services.ErrRequirementAlreadyExists)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
			t.Fatalf("got %v", err)
		}
	})
}
