package chatgptimages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
)

const MaxPayloadBytes = 96 << 20
const MaxImages = 16

type Image struct {
	Value  string
	Format string
	Prompt string
	ID     string
	Width  int
	Height int
}

type Handoff struct {
	ResumeURL string
	Topic     string
}

type Decoder struct {
	totalBytes            int
	lastPath              string
	lastOperation         string
	lastChannel           int
	channels              map[int]any
	ConversationID        string
	Model                 string
	Usage                 json.RawMessage
	Images                []Image
	Handoff               Handoff
	Done                  bool
	MessageStreamComplete bool
	Async                 bool
	PollIntervalMS        int64
	FreshnessMinutes      int64
	Offset                int
}

func (d *Decoder) Feed(payload []byte) error {
	d.totalBytes += len(payload)
	if d.totalBytes > 256<<20 {
		return fmt.Errorf("ChatGPT image stream exceeds total size limit")
	}
	if len(payload) > MaxPayloadBytes {
		return fmt.Errorf("ChatGPT image event exceeds size limit")
	}
	if string(bytes.TrimSpace(payload)) == "[DONE]" {
		d.Done = true
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(payload), []byte("v1")) || bytes.Equal(bytes.TrimSpace(payload), []byte(`"v1"`)) {
		d.channels = make(map[int]any)
		d.lastChannel = 0
		d.lastPath = ""
		d.lastOperation = "add"

		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid ChatGPT image event")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return fmt.Errorf("invalid trailing ChatGPT image event data")
	}
	if m, ok := value.(map[string]any); ok {
		if errValue := m["error"]; errValue != nil && errValue != false {
			return fmt.Errorf("ChatGPT image stream returned an error")
		}
		if str(m["type"]) == "stream_handoff" {
			for _, v := range array(m["options"]) {
				o, _ := v.(map[string]any)
				switch str(o["type"]) {
				case "resume_sse_endpoint":
					d.Handoff.ResumeURL = "/f/conversation/resume"
				case "subscribe_ws_topic":
					d.Handoff.Topic = str(o["topic_id"])
				}
			}
		}
		_, shortValue := m["v"]
		_, longValue := m["value"]
		_, shortOp := m["o"]
		_, longOp := m["op"]
		if str(m["type"]) != "" && !shortOp && !longOp {
			shortValue = false
			longValue = false
		}
		if shortValue || longValue || shortOp || longOp {
			operation := normalizedDelta(m)
			channel := d.lastChannel
			if raw, ok := operation["c"]; ok {
				n, ok := raw.(json.Number)
				if !ok {
					return fmt.Errorf("invalid image delta channel")
				}
				parsed, err := n.Int64()
				if err != nil || parsed < 0 || parsed > 9007199254740991 {
					return fmt.Errorf("invalid image delta channel")
				}
				channel = int(parsed)
			}
			if _, ok := operation["p"]; !ok {
				operation["p"] = d.lastPath
			}
			if _, ok := operation["o"]; !ok {
				operation["o"] = d.lastOperation
				if d.lastOperation == "" {
					operation["o"] = "add"
				}
			}
			if d.channels == nil {
				d.channels = make(map[int]any)
			}
			if _, exists := d.channels[channel]; !exists && len(d.channels) >= 64 {
				return fmt.Errorf("image delta channel limit exceeded")
			}
			root, err := apply(d.channels[channel], operation, 0)
			if err != nil {
				return err
			}
			d.channels[channel] = root
			d.lastChannel, d.lastPath, d.lastOperation = channel, str(operation["p"]), str(operation["o"])
			value = root
		}

	}
	if err := d.walk(value, 0, false, "", "", ""); err != nil {
		return err
	}
	d.Offset++
	return nil
}

func (d *Decoder) walk(v any, depth int, generated bool, prompt, messageID, path string) error {
	if depth > 64 {
		return fmt.Errorf("ChatGPT image nesting exceeds limit")
	}
	switch m := v.(type) {
	case map[string]any:
		if id := str(m["conversation_id"]); id != "" {
			d.ConversationID = id
		}
		if author, ok := m["author"].(map[string]any); ok {
			role := str(author["role"])
			if role == "user" || role == "system" {
				return nil
			}
			generated = role == "assistant" || role == "tool"
			if generated {
				messageID = first(str(m["id"]), str(m["message_id"]), messageID)
				path = ""
			}
		}
		if str(m["type"]) == "message_stream_complete" {
			d.MessageStreamComplete = true
		}
		if meta, ok := m["metadata"].(map[string]any); ok {
			if generation, ok := meta["generation"].(map[string]any); ok {
				if height, exists := generation["height"]; exists {
					if content, ok := m["content"].(map[string]any); ok {
						for _, part := range array(content["parts"]) {
							if image, ok := part.(map[string]any); ok && str(image["content_type"]) == "image_asset_pointer" && number(image["height"]) > number(height) {
								d.Async = true
								return nil
							}
						}
					}
				}
			}

			if model := str(meta["model_slug"]); model != "" {
				d.Model = model
			}
			if meta["image_gen_async"] == true || meta["image_gen_task_id"] != nil {
				d.Async = true
			}
			if n := number(meta["poll_interval_ms"]); n > 0 {
				d.PollIntervalMS = n
			}
			if n := number(meta["poll_freshness_max_mins"]); n > 0 {
				d.FreshnessMinutes = n
			}
		}
		if meta, ok := m["metadata"].(map[string]any); ok && str(meta["image_gen_task_status"]) == "intermediate" {
			d.Async = true
			return nil
		}
		if u, ok := m["usage"].(map[string]any); ok {
			d.Usage, _ = json.Marshal(u)
		}
		prompt = first(str(m["revised_prompt"]), str(m["prompt"]), prompt)
		status := str(m["status"])
		if str(m["type"]) == "image_generation_call" {
			if status == "in_progress" || status == "inProgress" || status == "queued" {
				d.Async = true
				return nil
			}
			if status != "" && status != "completed" && status != "success" {
				return nil
			}
			if value := str(m["result"]); value != "" {
				return d.add(Image{Value: value, Format: str(m["output_format"]), Prompt: prompt, ID: first(str(m["id"]), fmt.Sprintf("%s:%d:%s", messageID, d.lastChannel, path))})
			}
		}
		if generated && str(m["content_type"]) == "image_asset_pointer" {
			if pointer := str(m["asset_pointer"]); pointer != "" {
				return d.add(Image{Value: pointer, Width: int(number(m["width"])), Height: int(number(m["height"])), Prompt: prompt, ID: first(str(m["generation_id"]), pointer)})
			}
		}
		for k, child := range m {
			if k == "metadata" || k == "author" || k == "usage" {
				continue
			}
			if err := d.walk(child, depth+1, generated, prompt, messageID, path+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range m {
			if err := d.walk(child, depth+1, generated, prompt, messageID, path+"/"+strconv.Itoa(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Decoder) add(image Image) error {
	if !plausibleImageValue(image.Value) {
		return nil
	}
	for i, old := range d.Images {
		if image.ID != "" && old.ID == image.ID || (image.ID == "" || old.ID == "") && old.Value == image.Value {
			d.Images[i] = image
			return nil
		}
	}
	if len(d.Images) >= MaxImages {
		return fmt.Errorf("ChatGPT image count exceeds limit")
	}
	d.Images = append(d.Images, image)
	return nil
}

func plausibleImageValue(value string) bool {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"data:image/", "file-service://", "sediment://", "iVBORw0KGgo", "/9j/", "R0lGOD", "UklGR"} {
		if strings.HasPrefix(value, prefix) && len(value) > len(prefix) {
			return true
		}
	}
	return value == "iVBORw0KGgo"
}

func normalizedDelta(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, v := range value {
		result[key] = v
	}
	for _, pair := range [][2]string{{"channel", "c"}, {"path", "p"}, {"op", "o"}, {"value", "v"}} {
		if v, ok := value[pair[0]]; ok {
			result[pair[1]] = v
		}
	}
	return result
}

func apply(root any, op map[string]any, depth int) (any, error) {
	op = normalizedDelta(op)
	path := str(op["p"])
	if depth > 64 {
		return nil, fmt.Errorf("image patch nesting exceeds limit")
	}
	if str(op["o"]) == "patch" {
		ops, ok := op["v"].([]any)
		if !ok || len(ops) > 4096 {
			return nil, fmt.Errorf("invalid image patch batch")
		}
		for _, raw := range ops {
			child, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid image patch item")
			}
			copy := make(map[string]any, len(child))
			for k, v := range child {
				copy[k] = v
			}
			child = normalizedDelta(child)
			copy = normalizedDelta(child)
			childPath := str(child["p"])
			if path != "" && childPath != "" && !strings.HasPrefix(childPath, "/") {
				childPath = "/" + childPath
			}
			copy["p"] = path + childPath
			var err error
			root, err = apply(root, copy, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return root, nil
	}

	if path == "" {
		return patchValue(root, str(op["o"]), op["v"])
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 64 {
		return nil, fmt.Errorf("image delta path exceeds limit")
	}
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
		if parts[i] == "__proto__" || parts[i] == "constructor" || parts[i] == "prototype" {
			return nil, fmt.Errorf("unsafe image delta path")
		}
	}
	return patchAt(root, parts, str(op["o"]), op["v"], depth)
}

func patchAt(root any, path []string, operation string, value any, depth int) (any, error) {
	if len(path) == 0 {
		return patchValue(root, operation, value)
	}
	if depth > 64 {
		return nil, fmt.Errorf("image delta nesting exceeds limit")
	}
	key := path[0]
	switch v := root.(type) {
	case map[string]any:
		if len(path) == 1 && operation == "remove" {
			delete(v, key)
			return v, nil
		}
		child, err := patchAt(v[key], path[1:], operation, value, depth+1)
		if err != nil {
			return nil, err
		}
		v[key] = child
		return v, nil
	case []any:
		if key == "-" && len(path) == 1 && operation == "add" {
			if len(v) >= 4096 {
				return nil, fmt.Errorf("image delta array exceeds limit")
			}
			return append(v, value), nil
		}
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i > len(v) || i >= 4096 {
			return nil, fmt.Errorf("invalid image delta index")
		}
		if i == len(v) {
			if len(path) == 1 && operation == "add" {
				return append(v, value), nil
			}
			if len(path) == 1 && operation == "remove" {
				return v, nil
			}
			v = append(v, nil)
		}
		if len(path) == 1 && operation == "add" {
			if len(v) >= 4096 {
				return nil, fmt.Errorf("image delta array exceeds limit")
			}
			v = append(v, nil)
			copy(v[i+1:], v[i:])
			v[i] = value
			return v, nil
		}
		if len(path) == 1 && operation == "remove" {
			return append(v[:i], v[i+1:]...), nil
		}
		child, err := patchAt(v[i], path[1:], operation, value, depth+1)
		if err != nil {
			return nil, err
		}
		v[i] = child
		return v, nil
	case nil:
		if _, err := strconv.Atoi(key); err == nil || key == "-" {
			return patchAt([]any{}, path, operation, value, depth+1)
		}
		return patchAt(map[string]any{}, path, operation, value, depth+1)
	default:
		return nil, fmt.Errorf("image delta parent is not a container")
	}
}

func patchValue(old any, operation string, value any) (any, error) {
	switch operation {
	case "add", "replace":
		return value, nil
	case "append":
		if text, ok := old.(string); ok {
			if part, ok := value.(string); ok && len(text)+len(part) <= MaxPayloadBytes {
				return text + part, nil
			}
		}
		if list, ok := old.([]any); ok {
			parts, ok := value.([]any)
			if !ok {
				parts = []any{value}
			}
			if len(list)+len(parts) > 4096 {
				return nil, fmt.Errorf("image append exceeds limit")
			}
			return append(list, parts...), nil
		}
		if old == nil {
			return value, nil
		}
		if object, ok := old.(map[string]any); ok {
			if fields, ok := value.(map[string]any); ok {
				if len(object)+len(fields) > 4096 {
					return nil, fmt.Errorf("image delta object exceeds limit")
				}
				for key, v := range fields {
					if key == "__proto__" || key == "constructor" || key == "prototype" {
						return nil, fmt.Errorf("unsafe image delta key")
					}
					object[key] = v
				}
				return object, nil
			}
		}
		return nil, fmt.Errorf("invalid image append delta")
	case "truncate":
		n := number(value)
		if n < 0 {
			n = 0
		}
		if text, ok := old.(string); ok {
			runes := utf16.Encode([]rune(text))
			if n < int64(len(runes)) {
				runes = runes[:n]
			}
			return string(utf16.Decode(runes)), nil
		}
		if values, ok := old.([]any); ok {
			if n > 4096 {
				return nil, fmt.Errorf("image truncate exceeds limit")
			}
			if int(n) > len(values) {
				values = append(values, make([]any, int(n)-len(values))...)
			}
			return values[:n], nil
		}
		return old, nil
	case "remove":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported image delta operation")
	}
}

func str(v any) string  { s, _ := v.(string); return s }
func array(v any) []any { s, _ := v.([]any); return s }
func number(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
func first(values ...string) string {
	for _, s := range values {
		if s != "" {
			return s
		}
	}
	return ""
}
