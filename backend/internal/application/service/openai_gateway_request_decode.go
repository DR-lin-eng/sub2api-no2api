package service

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func getOpenAIRequestBodyMap(_ *gin.Context, body []byte) (map[string]any, error) {
	var reqBody map[string]any
	if err := decodeOpenAIRequestObject(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}
	return reqBody, nil
}

// Keep the existing decoded types used by request transforms, while preserving
// numeric metadata before those transforms marshal the whole request again.
func decodeOpenAIRequestObject(body []byte, reqBody *map[string]any) error {
	if err := json.Unmarshal(body, reqBody); err != nil {
		return err
	}
	if raw := gjson.GetBytes(body, "client_metadata"); raw.IsObject() {
		metadata, err := decodeCodexMetadataObject(raw.Raw)
		if err != nil {
			return err
		}
		(*reqBody)["client_metadata"] = metadata
	}
	return nil
}
