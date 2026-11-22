package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/mediaprotocol"
	"github.com/gin-gonic/gin"
)

type multimodalUpstream struct {
	driver mediaprotocol.Driver
	url    string
	rawID  string
}

func resolveMultimodalUpstream(account *Account, op MultimodalMediaEndpoint, id string) (multimodalUpstream, error) {
	var route multimodalUpstream
	var err error
	if op.IsVideoLookupRequest() {
		route.driver, route.rawID = mediaprotocol.Lookup(account.ID, id)
	} else {
		route.driver, err = mediaprotocol.Select(account.GetCredential("media_protocol"))
		if err != nil {
			return route, err
		}
	}
	if route.driver == nil {
		route.url, err = buildMultimodalMediaURL(account, op, id)
	} else {
		route.url, err = route.driver.URL(account.GetMultimodalBaseURL(), op, route.rawID)
	}
	return route, err
}

func encodeNativeMediaRequest(driver mediaprotocol.Driver, op MultimodalMediaEndpoint, model string, body []byte, contentType string, info GrokMediaRequestInfo) ([]byte, error) {
	fields, err := customModelJSONAdapterBase(body, contentType)
	if err != nil || fields == nil {
		return nil, fmt.Errorf("%w: expected JSON object or multipart form", mediaprotocol.ErrUnsupported)
	}
	fields["model"] = model
	// A native provider's defaults can differ from the gateway defaults. Send
	// the exact duration and tier already priced by admission, including when
	// the client omitted them. Reject values outside the priceable range.
	if op.IsVideoGenerationRequest() {
		if value, ok := fields["duration"]; ok {
			raw := fmt.Sprint(value)
			seconds, err := strconv.Atoi(raw)
			if err != nil || seconds != info.DurationSeconds || seconds < VideoBillingMinDurationSeconds || seconds > VideoBillingMaxDurationSeconds {
				return nil, fmt.Errorf("%w: duration must be an integer between %d and %d seconds", mediaprotocol.ErrUnsupported, VideoBillingMinDurationSeconds, VideoBillingMaxDurationSeconds)
			}
		}
		if raw, ok := fields["resolution"]; ok {
			tier, valid := ClassifyVideoBillingResolution(fmt.Sprint(raw))
			if !valid || tier != info.Resolution {
				return nil, fmt.Errorf("%w: resolution must match a supported video billing tier", mediaprotocol.ErrUnsupported)
			}
		}
		fields["duration"] = info.DurationSeconds
		fields["resolution"] = info.Resolution
	}
	return driver.Encode(op, fields)
}

func nativeVideoResponse(driver mediaprotocol.Driver, accountID int64, task mediaprotocol.Task, c *gin.Context) ([]byte, error) {
	id, err := mediaprotocol.PublicTaskID(driver, accountID, task.ID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"id": id, "status": task.Status}
	if task.Seconds > 0 {
		payload["seconds"] = task.Seconds
	}
	if task.Resolution != "" {
		payload["size"] = task.Resolution
	}
	if task.CreatedAt > 0 {
		payload["created_at"] = task.CreatedAt
	}
	if task.Error != "" {
		payload["error"] = map[string]string{"message": task.Error}
	}
	if task.Artifact != "" {
		payload["url"] = multimodalMediaContentProxyURL(c, id)
	}
	return json.Marshal(payload)
}

// For providers returning a signed artifact on the status resource, authorize
// and settle the status before revealing that URL. Never send the account key
// to the artifact host.
func (s *OpenAIGatewayService) deliverNativeVideo(ctx context.Context, c *gin.Context, account *Account, id string, task mediaprotocol.Task, body []byte, start time.Time) (*OpenAIForwardResult, error) {
	if _, err := s.applyMultimodalVideoBilling(ctx, c, account, MultimodalMediaEndpointVideoStatus, id, nil, body); err != nil {
		return nil, err
	}
	if task.Status != "completed" {
		writeMultimodalMediaErrorResponse(c, http.StatusConflict, "video_not_ready", "Video content is unavailable for a "+task.Status+" task")
	} else {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Location", task.Artifact)
		c.Status(http.StatusFound)
		c.Writer.WriteHeaderNow()
		MarkResponseCommitted(c)
	}
	return &OpenAIForwardResult{Duration: time.Since(start)}, nil
}
