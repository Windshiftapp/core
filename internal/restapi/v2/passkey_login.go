package v2

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/go-webauthn/webauthn/protocol"

	"windshift/internal/models"
)

type passkeyLoginHTTP interface {
	StartPasskeyLogin(http.ResponseWriter, *http.Request)
	CompleteFIDOLoginNew(http.ResponseWriter, *http.Request)
}

type passkeyStartRequest struct {
	EmailOrUsername string `json:"email_or_username,omitempty"`
}

type passkeyStartResponse struct {
	PublicKey protocol.PublicKeyCredentialRequestOptions `json:"publicKey"`
	SessionID string                                     `json:"sessionId"`
}

type passkeyCompleteRequest struct {
	SessionID  string          `json:"sessionId"`
	Response   json.RawMessage `json:"response"`
	RememberMe bool            `json:"remember_me,omitempty"`
}

type passkeyCompleteResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	User    models.User `json:"user"`
}

func registerPasskeyLoginRoutes(b *routeBuilder, deps Deps) {
	wrap := func(complete bool) Handler {
		handler := Handler(func(w http.ResponseWriter, r *http.Request) error {
			if deps.PasskeyLogin == nil {
				return newError(http.StatusNotFound, "not_found", "Passkey login is unavailable")
			}
			response := &loginHTTPResponse{header: make(http.Header)}
			if complete {
				deps.PasskeyLogin.CompleteFIDOLoginNew(response, r)
			} else {
				deps.PasskeyLogin.StartPasskeyLogin(response, r)
			}
			w.Header().Set("Cache-Control", "private, no-store")
			if response.status >= 400 {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.body.Bytes(), &body); err != nil {
					return internalError(err)
				}
				return newError(response.status, "passkey_login_failed", body.Error)
			}
			for key, values := range response.header {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(response.body.Bytes())
			return err
		})
		if deps.PasskeyLoginLimiter != nil {
			handler = limitRequests(deps.PasskeyLoginLimiter)(handler)
		}
		if deps.CSRF != nil {
			handler = requireCSRF(deps.CSRF)(handler)
		}
		return handler
	}
	b.sessionOnly(func() {
		b.RawResponse[passkeyStartResponse](http.MethodPost, "/auth/passkey/start", http.StatusOK, "application/json", AuthPublic, nil, wrap(false))
		b.routes[len(b.routes)-1].RequestType = reflect.TypeFor[passkeyStartRequest]()
		b.routes[len(b.routes)-1].RequestMediaType = "application/json"
		b.RawResponse[passkeyCompleteResponse](http.MethodPost, "/auth/passkey/complete", http.StatusOK, "application/json", AuthPublic, nil, wrap(true))
		b.routes[len(b.routes)-1].RequestType = reflect.TypeFor[passkeyCompleteRequest]()
		b.routes[len(b.routes)-1].RequestMediaType = "application/json"
	})
}
