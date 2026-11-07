package chatgptimages

import "time"

type Attachment struct {
	ID     string
	Width  int
	Height int
	Bytes  int
}

// Conversation matches the Images page's picture_v2 path. Native Images API
// model/quality/animation fields do not belong to this protocol.
func Conversation(prompt, model, parentID, messageID, sendID, timezone string, attachments []Attachment) map[string]any {
	_, offset := time.Now().Zone()
	if location, err := time.LoadLocation(timezone); err == nil {
		_, offset = time.Now().In(location).Zone()
	}
	parts := make([]any, 0, len(attachments)+1)
	for _, image := range attachments {
		prefix := "file-service://"
		if len(image.ID) >= 5 && image.ID[:5] == "file_" {
			prefix = "sediment://"
		}
		parts = append(parts, map[string]any{"content_type": "image_asset_pointer", "asset_pointer": prefix + image.ID, "width": image.Width, "height": image.Height, "size_bytes": image.Bytes})
	}
	parts = append(parts, prompt)
	contentType := "text"
	if len(attachments) > 0 {
		contentType = "multimodal_text"
	}
	return map[string]any{
		"action": "next", "model": model, "parent_message_id": parentID,
		"system_hints": []string{"picture_v2"}, "supported_encodings": []string{"v1"},
		"timezone": timezone, "timezone_offset_min": -offset / 60, "client_prepare_state": "none",
		"messages": []any{map[string]any{
			"id": messageID, "author": map[string]any{"role": "user"}, "create_time": float64(time.Now().UnixMilli()) / 1000,
			"content":  map[string]any{"content_type": contentType, "parts": parts},
			"metadata": map[string]any{"system_hints": []string{"picture_v2"}, "image_send_uuid": sendID},
		}},
	}
}
