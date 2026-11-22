package mediaprotocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
)

// Ark uses a synchronous image API and a polling API for video. A completed
// video lives at the signed HTTPS URL returned by the task status call.
type ark struct{}

func (ark) Name() string { return "ark" }

func (ark) URL(base string, op Operation, id string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid Ark base URL")
	}
	basePath := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v3")
	switch {
	case op.IsImageRequest():
		u.Path = basePath + "/api/v3/images/generations"
	case op == VideoGenerate:
		u.Path = basePath + "/api/v3/contents/generations/tasks"
	case op.IsVideoLookupRequest() && validTaskID(id):
		u.Path = basePath + "/api/v3/contents/generations/tasks/" + id
	default:
		return "", fmt.Errorf("%w: Ark operation %s", ErrUnsupported, op)
	}
	u.RawPath = ""
	return u.String(), nil
}

func unsupported(field string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, field)
}

// Encode receives the mapped model and a fresh gateway request object. Accept
// only fields whose semantics both contracts can express.
func (ark) Encode(op Operation, fields map[string]any) ([]byte, error) {
	prompt, _ := fields["prompt"].(string)
	model, _ := fields["model"].(string)
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(model) == "" {
		return nil, unsupported("prompt and model are required")
	}
	var refs []string
	for _, field := range []string{"image", "images", "reference_images"} {
		value, ok := fields[field]
		if !ok {
			continue
		}
		var err error
		refs, err = appendImageRefs(refs, value)
		if err != nil {
			return nil, err
		}
	}
	out := map[string]any{"model": model}
	switch {
	case op.IsImageRequest():
		out["prompt"] = prompt
		// Ask stream-capable Seedream generations for SSE even for a JSON
		// client. Opaque endpoint IDs can return ordinary Images JSON.
		if strings.HasPrefix(model, "doubao-seedream-4-") || strings.HasPrefix(model, "doubao-seedream-5-0-lite-") {
			out["stream"] = true
		}
		if op == ImageEdit && len(refs) == 0 {
			return nil, unsupported("image is required for edits")
		}
		if len(refs) != 0 {
			out["image"] = refs
		}
	case op == VideoGenerate:
		content := []map[string]any{{"type": "text", "text": prompt}}
		if len(refs) > 1 {
			return nil, unsupported("this video adapter accepts one first-frame image")
		}
		for _, ref := range refs {
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": ref}, "role": "first_frame"})
		}
		out["content"] = content
	default:
		return nil, unsupported(string(op))
	}
	for field, value := range fields {
		switch field {
		case "model", "prompt", "image", "images", "reference_images":
			continue
		case "stream":
			if _, ok := value.(bool); !ok {
				return nil, unsupported("stream must be a boolean")
			}
		case "n":
			if !op.IsImageRequest() || (value != 1 && value != float64(1)) {
				return nil, unsupported("n must be 1 for image generation")
			}
		case "response_format":
			if !op.IsImageRequest() || (value != "url" && value != "b64_json") {
				return nil, unsupported("response_format")
			}
			out[field] = value
		case "size", "watermark":
			if !op.IsImageRequest() {
				return nil, unsupported(field)
			}
			out[field] = value
		case "quality":
			if !op.IsImageRequest() || value != "auto" {
				return nil, unsupported("quality")
			}
		case "duration", "resolution", "aspect_ratio":
			if op != VideoGenerate {
				return nil, unsupported(field)
			}
			if field == "aspect_ratio" {
				field = "ratio"
			}
			out[field] = value
		default:
			return nil, unsupported(field)
		}
	}
	return json.Marshal(out)
}

func appendImageRefs(refs []string, value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		ref := strings.TrimSpace(v)
		switch {
		case strings.HasPrefix(ref, "data:image/") && strings.Contains(ref, ";base64,"):
		case strings.HasPrefix(ref, "https://"), strings.HasPrefix(ref, "http://"):
			u, err := url.Parse(ref)
			if err != nil || u.Hostname() == "" || u.User != nil {
				return nil, unsupported("invalid image URL")
			}
		default:
			return nil, unsupported("expected an image URL or data URL")
		}
		return append(refs, ref), nil
	case []any:
		for _, item := range v {
			ref, ok := item.(string)
			if !ok {
				return nil, unsupported("expected an image URL")
			}
			var err error
			refs, err = appendImageRefs(refs, ref)
			if err != nil {
				return nil, err
			}
		}
	case []string: // Decoded multipart file fields.
		for _, ref := range v {
			var err error
			refs, err = appendImageRefs(refs, ref)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, unsupported("image")
	}
	return refs, nil
}

// The Ark image envelope already matches Images. Keep its exact usage and
// bytes, but require an actual image of the requested representation.
func (ark) DecodeImages(body []byte, format string) ([]byte, error) {
	if format == "" {
		format = "url"
	}
	items := gjson.GetBytes(body, "data")
	count := 0
	valid := items.IsArray() && gjson.ValidBytes(body)
	items.ForEach(func(_, item gjson.Result) bool {
		image := item.Get(format)
		valid = valid && image.Type == gjson.String && strings.TrimSpace(image.Str) != ""
		count++
		return valid
	})
	if count == 0 || !valid {
		return nil, fmt.Errorf("missing image data in the requested %s format in Ark response", format)
	}
	return body, nil
}

func (ark) DecodeTask(op Operation, body []byte, expectedID string) (Task, error) {
	var resp struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Duration   int    `json:"duration"`
		Resolution string `json:"resolution"`
		CreatedAt  int64  `json:"created_at"`
		Content    struct {
			URL string `json:"video_url"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || !validTaskID(resp.ID) || (expectedID != "" && resp.ID != expectedID) {
		return Task{}, errors.New("invalid Ark video task response")
	}
	task := Task{ID: resp.ID, Seconds: resp.Duration, Resolution: resp.Resolution, CreatedAt: resp.CreatedAt}
	switch resp.Status {
	case "":
		if op != VideoGenerate {
			return Task{}, errors.New("missing video status in Ark response")
		}
		task.Status = "queued"
	case "queued":
		task.Status = "queued"
	case "running":
		task.Status = "in_progress"
	case "succeeded":
		if !validArtifactURL(resp.Content.URL) {
			return Task{}, errors.New("invalid Ark video artifact URL")
		}
		task.Status, task.Artifact = "completed", resp.Content.URL
	case "failed", "expired", "cancelled":
		task.Status, task.Error = "failed", resp.Error.Message
	default:
		return Task{}, errors.New("unknown Ark video status")
	}
	return task, nil
}
