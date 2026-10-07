package requestmodel

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/tidwall/gjson"
)

var ErrDuplicateModelField = errors.New("duplicate model or session fields are not allowed")

// ValidateModelFields rejects model representations on which first-value and
// last-value parsers can disagree. It does not inspect user content or schemas.
func ValidateModelFields(contentType string, body []byte) error {
	if !isMultipartContentType(contentType) {
		return ValidateJSONModelFields(body)
	}
	_, params, err := mime.ParseMediaType(contentType)
	boundary := params["boundary"]
	if err != nil || strings.TrimSpace(boundary) == "" {
		return nil // The protocol handler reports malformed multipart requests.
	}
	// Media handlers trim the boundary, while Live's standard Go form parser
	// preserves it. Check both interpretations before either reaches billing.
	if err := validateMultipartModelFields(body, boundary); err != nil {
		return err
	}
	if trimmed := strings.TrimSpace(boundary); trimmed != boundary {
		return validateMultipartModelFields(body, trimmed)
	}
	return nil
}

func validateMultipartModelFields(body []byte, boundary string) error {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var modelSeen, sessionSeen bool
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil
		}
		if strings.TrimSpace(part.FileName()) != "" {
			continue
		}
		switch strings.TrimSpace(part.FormName()) {
		case "model":
			if modelSeen {
				return ErrDuplicateModelField
			}
			modelSeen = true
		case "session":
			if sessionSeen {
				return ErrDuplicateModelField
			}
			sessionSeen = true
			data, err := io.ReadAll(part)
			if err != nil {
				return err
			}
			if err := ValidateJSONModelFields(data); err != nil {
				return err
			}
		}
	}
}

// ValidateJSONModelFields also protects WS response.create/session.update
// frames, before any model mapping or JSON reserialization can hide duplicates.
func ValidateJSONModelFields(body []byte) error {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	var modelSeen, sessionSeen bool
	var validationErr error
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		switch {
		case strings.EqualFold(key.String(), "model"):
			if modelSeen {
				validationErr = ErrDuplicateModelField
			}
			modelSeen = true
		case strings.EqualFold(key.String(), "session"):
			if sessionSeen {
				validationErr = ErrDuplicateModelField
			} else {
				sessionModelSeen := false
				value.ForEach(func(sessionKey, _ gjson.Result) bool {
					if strings.EqualFold(sessionKey.String(), "model") {
						if sessionModelSeen {
							validationErr = ErrDuplicateModelField
						}
						sessionModelSeen = true
					}
					return validationErr == nil
				})
			}
			sessionSeen = true
		}
		return validationErr == nil
	})
	return validationErr
}
