package v2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"windshift/internal/models"
)

// Customer login reuses portal HTTP handlers; keep their legacy errors behind
// the v2 transport boundary while forwarding cookies and direct JSON success bodies.
type customerAccessResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *customerAccessResponse) Header() http.Header { return w.header }
func (w *customerAccessResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *customerAccessResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

type customerAccessHTTP interface {
	Status(http.ResponseWriter, *http.Request)
	GetPolicy(http.ResponseWriter, *http.Request)
	PutPolicy(http.ResponseWriter, *http.Request)
	RequestMagicLink(http.ResponseWriter, *http.Request)
	VerifyMagicLink(http.ResponseWriter, *http.Request)
	StartPasskey(http.ResponseWriter, *http.Request)
	CompletePasskey(http.ResponseWriter, *http.Request)
	Logout(http.ResponseWriter, *http.Request)
}

func registerCustomerAccessRoutes(b *routeBuilder, deps Deps) {
	wrap := func(operation func(customerAccessHTTP, http.ResponseWriter, *http.Request)) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			if deps.CustomerAccess == nil {
				return newError(http.StatusNotFound, "not_found", "Customer login is unavailable")
			}
			response := &customerAccessResponse{header: make(http.Header)}
			operation(deps.CustomerAccess, response, r)
			for key, values := range response.header {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.Header().Set("Cache-Control", "private, no-store")
			if response.status >= 400 {
				var body struct {
					Error   string `json:"error"`
					Message string `json:"message"`
					Code    string `json:"code"`
				}
				if err := json.Unmarshal(response.body.Bytes(), &body); err != nil {
					return internalError(err)
				}
				if body.Error == "" {
					body.Error = body.Message
				}
				if body.Code == "" {
					body.Code = "customer_login_failed"
				}
				return newError(response.status, strings.ToLower(body.Code), body.Error)
			}
			if response.status == 0 {
				response.status = http.StatusOK
			}
			w.WriteHeader(response.status)
			_, err := w.Write(response.body.Bytes())
			return err
		}
	}
	b.sessionOnly(func() {
		b.RawResponse[models.CustomerAccessPolicy](http.MethodGet, "/customer-access/{kind}/{id}/policy", http.StatusOK, "application/json", AuthAuthenticated, nil, wrap(func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.GetPolicy(w, r) }))
		b.RawResponse[models.CustomerAccessPolicy](http.MethodPut, "/customer-access/{kind}/{id}/policy", http.StatusOK, "application/json", AuthAuthenticated, nil, wrap(func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.PutPolicy(w, r) }))
		b.routes[len(b.routes)-1].RequestType = reflect.TypeFor[models.CustomerAccessPolicy]()
		b.routes[len(b.routes)-1].RequestMediaType = "application/json"
		b.RawResponse[models.CustomerAccessStatus](http.MethodGet, "/customer-access/{kind}/{slug}/status", http.StatusOK, "application/json", AuthPublic, nil, wrap(func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.Status(w, r) }))
		for _, entry := range []struct {
			path string
			call func(customerAccessHTTP, http.ResponseWriter, *http.Request)
		}{
			{"request", func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.RequestMagicLink(w, r) }},
			{"verify", func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.VerifyMagicLink(w, r) }},
			{"passkey/start", func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.StartPasskey(w, r) }},
			{"passkey/complete", func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.CompletePasskey(w, r) }},
			{"logout", func(h customerAccessHTTP, w http.ResponseWriter, r *http.Request) { h.Logout(w, r) }},
		} {
			b.RawResponse[models.CustomerLoginResponse](http.MethodPost, "/customer-access/{kind}/{slug}/auth/"+entry.path, http.StatusOK, "application/json", AuthPublic, nil, wrap(entry.call))
			b.routes[len(b.routes)-1].RequestType = reflect.TypeFor[models.CustomerLoginRequest]()
			b.routes[len(b.routes)-1].RequestMediaType = "application/json"
		}
	})
}
