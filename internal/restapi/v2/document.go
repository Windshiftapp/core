package v2

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type document[T any] struct {
	Data T `json:"data"`
}

type metadataDocument[Data, Meta any] struct {
	Data Data `json:"data"`
	Meta Meta `json:"meta"`
}

type pageDocument[T any] struct {
	Data       []T                `json:"data"`
	Pagination paginationDocument `json:"pagination"`
}

type pageMetadataDocument[Data, Meta any] struct {
	Data       []Data             `json:"data"`
	Pagination paginationDocument `json:"pagination"`
	Meta       Meta               `json:"meta"`
}

type paginationDocument struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

type readOperation[Response any] func(*http.Request) (Response, error)

type metadataOperation[Response, Meta any] func(*http.Request) (Response, Meta, error)

type pageOperation[Response any] func(*http.Request) ([]Response, Pagination, int, error)
type pageMetadataOperation[Response, Meta any] func(*http.Request) ([]Response, Pagination, int, Meta, error)

type jsonOperation[Request, Response any] func(*http.Request, Request) (Response, error)

type commandOperation func(*http.Request) error

type actionOperation[Response any] func(*http.Request) (Response, error)

// transport centralizes typed v2 request and response plumbing.
type transport struct{}

func (transport) Read[Response any](status int, operation readOperation[Response]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		response, err := operation(r)
		if err != nil {
			return err
		}
		return writeDocument(w, status, response)
	}
}

func (transport) Metadata[Response, Meta any](operation metadataOperation[Response, Meta]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		response, meta, err := operation(r)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, metadataDocument[Response, Meta]{Data: response, Meta: meta})
	}
}

func (transport) Page[Response any](operation pageOperation[Response]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		items, page, total, err := operation(r)
		if err != nil {
			return err
		}
		totalPages := 0
		if total > 0 {
			totalPages = (total + page.PageSize - 1) / page.PageSize
		}
		return writeJSON(w, http.StatusOK, pageDocument[Response]{
			Data: items,
			Pagination: paginationDocument{
				Page: page.Page, PageSize: page.PageSize, TotalItems: total, TotalPages: totalPages,
			},
		})
	}
}

func (transport) PageMetadata[Response, Meta any](operation pageMetadataOperation[Response, Meta]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		items, page, total, meta, err := operation(r)
		if err != nil {
			return err
		}
		totalPages := 0
		if total > 0 {
			totalPages = (total + page.PageSize - 1) / page.PageSize
		}
		return writeJSON(w, http.StatusOK, pageMetadataDocument[Response, Meta]{
			Data: items,
			Pagination: paginationDocument{
				Page: page.Page, PageSize: page.PageSize, TotalItems: total, TotalPages: totalPages,
			},
			Meta: meta,
		})
	}
}

func (transport) JSON[Request, Response any](status int, patch bool, operation jsonOperation[Request, Response]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		var request Request
		if err := decodeJSON(w, r, &request, patch); err != nil {
			return err
		}
		response, err := operation(r, request)
		if err != nil {
			return err
		}
		return writeDocument(w, status, response)
	}
}

func (transport) Command(operation commandOperation) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if err := operation(r); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func (transport) Action[Response any](status int, operation actionOperation[Response]) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		response, err := operation(r)
		if err != nil {
			return err
		}
		return writeDocument(w, status, response)
	}
}

func writeDocument[T any](w http.ResponseWriter, status int, value T) error {
	return writeJSON(w, status, document[T]{Data: value})
}

func writeJSON[T any](w http.ResponseWriter, status int, value T) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(value); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
	return nil
}
