package requestmodel

import (
	"errors"
	"strings"
)

var ErrDuplicateModelField = errors.New("duplicate model or session fields are not allowed")
var errConflictingRequestFields = errors.New("duplicate or ambiguous request fields are not allowed")

// ValidateModelFields is shared by all HTTP gateway aliases before routing and billing.
func ValidateModelFields(contentType string, body []byte) error {
	field, conflict := ConflictingField(contentType, body)
	if !conflict {
		return nil
	}
	if strings.EqualFold(field, "model") || strings.EqualFold(field, "session") || strings.EqualFold(field, "session.model") {
		return ErrDuplicateModelField
	}
	return errConflictingRequestFields
}

// ValidateJSONModelFields validates the first and subsequent WebSocket frames.
func ValidateJSONModelFields(body []byte) error { return ValidateModelFields("", body) }
