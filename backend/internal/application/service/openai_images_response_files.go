package service

import "encoding/json"

// Both OAuth and API-key JSON responses project compatible SVG encodings into
// the public Images contract: b64_json contains UTF-8 SVG bytes, never a data URL.
func normalizeOpenAIImagesResponseFiles(body []byte, parsed *OpenAIImagesRequest) ([]byte, error) {
	if parsed == nil || parsed.OutputFormat != "svg" {
		return body, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	items := []map[string]json.RawMessage{root}
	if data, ok := root["data"]; ok {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
	}
	changed := false
	for _, item := range items {
		var value string
		if err := json.Unmarshal(item["b64_json"], &value); err != nil || value == "" {
			continue
		}
		normalized, err := normalizeOpenAISVGOutput(value)
		if err != nil {
			return nil, err
		}
		item["b64_json"], _ = json.Marshal(normalized)
		item["output_format"] = json.RawMessage(`"svg"`)
		changed = true
	}
	if !changed {
		return body, nil
	}
	if _, ok := root["data"]; ok {
		var err error
		root["data"], err = json.Marshal(items)
		if err != nil {
			return nil, err
		}
	}
	root["output_format"] = json.RawMessage(`"svg"`)
	return json.Marshal(root)
}
