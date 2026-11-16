package v2

import (
	"net/http"

	"windshift/internal/services"
)

func registerOperatorControlsRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/operator-controls", AuthAuthenticated, []string{"admin:operator-controls:read"}, func(r *http.Request) (services.OperatorControlsStatus, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return services.OperatorControlsStatus{}, err
		}
		if deps.OperatorControls == nil {
			return services.OperatorControlsStatus{}, newError(http.StatusServiceUnavailable, "operator_controls_unavailable", "Operator controls discovery is unavailable")
		}
		status, err := deps.OperatorControls.Status(r.Context())
		if err != nil {
			return services.OperatorControlsStatus{}, internalError(err)
		}
		return status, nil
	})
}
