package services

// InvalidRequestError marks a client-caused request violation: malformed
// parameters, unknown referenced ids, or target configuration the caller
// must fix. Transport mappers classify it as a 400 without inspecting the
// message text, so internal failures can never masquerade as client errors.
type InvalidRequestError struct {
	Message string
}

func (e *InvalidRequestError) Error() string { return e.Message }
