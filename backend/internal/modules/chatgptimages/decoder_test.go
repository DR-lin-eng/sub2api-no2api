package chatgptimages

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPictureV2PayloadAndReferencePointer(t *testing.T) {
	payload := Conversation("unchanged prompt", "selected-chat-model", "parent", "message", "send", "Asia/Shanghai", []Attachment{{ID: "file_reference", Width: 4, Height: 3, Bytes: 100}, {ID: "legacy-reference", Width: 2, Height: 2, Bytes: 40}})
	encoded, _ := json.Marshal(payload)
	s := string(encoded)
	for _, expected := range []string{`"system_hints":["picture_v2"]`, `"model":"selected-chat-model"`, `"asset_pointer":"sediment://file_reference"`, `"asset_pointer":"file-service://legacy-reference"`, `"supported_encodings":["v1"]`, `"timezone_offset_min":-480`} {
		if !strings.Contains(s, expected) {
			t.Fatalf("missing %s", expected)
		}
	}
	for _, field := range []string{`"quality"`, `"image_generation_settings"`, `"concurrentRequests"`} {
		if strings.Contains(s, field) {
			t.Fatalf("foreign field %s", field)
		}
	}
}

func TestDeltaImageDecodingAndInheritedOperations(t *testing.T) {
	d := &Decoder{}
	for _, payload := range []string{
		`{"o":"add","p":"","v":{"conversation_id":"conv-a","message":{"author":{"role":"assistant"},"content":{"content_type":"multimodal_text","parts":[]}}}}`,
		`{"p":"/message/content/parts","o":"append","v":[{"type":"image_generation_call","id":"image-a","status":"in_progress","result":""}]}`,
		`{"p":"/message/content/parts/0/result","o":"append","v":"data:image/png;base64,"}`,
		`{"v":"iVBORw0KGgo="}`,
		`{"p":"/message/content/parts/0","o":"patch","v":[{"p":"/status","o":"replace","v":"completed"}]}`,
		`[DONE]`,
	} {
		if err := d.Feed([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if !d.Done || d.ConversationID != "conv-a" || len(d.Images) != 1 || d.Images[0].Value != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("unexpected image state: %+v", d)
	}
}

func TestUserReferencesAndPlaceholdersAreNotGeneratedOutput(t *testing.T) {
	d := &Decoder{}
	if err := d.Feed([]byte(`{"conversation_id":"c","mapping":{"user":{"message":{"author":{"role":"user"},"content":{"parts":[{"content_type":"image_asset_pointer","asset_pointer":"sediment://input"}]}}},"pending":{"message":{"author":{"role":"assistant"},"metadata":{"image_gen_async":true},"output":[{"type":"image_generation_call","status":"in_progress","result":"unfinished"}]}}}}`)); err != nil {
		t.Fatal(err)
	}
	if len(d.Images) != 0 || !d.Async {
		t.Fatalf("input or placeholder counted: %+v", d.Images)
	}
}

func TestMessageIDFallbackDeduplicatesImageItemsAndRejectsText(t *testing.T) {
	d := &Decoder{}
	for _, value := range []string{"data:image/png;base64,AAAA", "data:image/png;base64,BBBB"} {
		payload, _ := json.Marshal(map[string]any{"message": map[string]any{"id": "message-a", "author": map[string]any{"role": "assistant"}, "output": []any{map[string]any{"type": "image_generation_call", "status": "completed", "result": value}}}})
		if err := d.Feed(payload); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.Images) != 1 || d.Images[0].Value != "data:image/png;base64,BBBB" {
		t.Fatal("message fallback duplicated an image")
	}
	text := &Decoder{}
	if err := text.Feed([]byte(`{"output":[{"type":"image_generation_call","status":"completed","result":"The request was refused"}]}`)); err != nil {
		t.Fatal(err)
	}
	if len(text.Images) != 0 {
		t.Fatal("text was counted as an image")
	}
}

func TestDistinctImageIDsAndSlotsRetainIdenticalOutputs(t *testing.T) {
	for _, ids := range []bool{false, true} {
		d := &Decoder{}
		first := map[string]any{"type": "image_generation_call", "status": "completed", "result": "data:image/png;base64,AAAA"}
		second := map[string]any{"type": "image_generation_call", "status": "completed", "result": "data:image/png;base64,AAAA"}
		if ids {
			first["id"] = "image-1"
			second["id"] = "image-2"
		}
		payload, _ := json.Marshal(map[string]any{"message": map[string]any{"id": "message", "author": map[string]any{"role": "assistant"}, "output": []any{first, second}}})
		if err := d.Feed(payload); err != nil {
			t.Fatal(err)
		}
		if len(d.Images) != 2 {
			t.Fatalf("two completed image slots collapsed: %+v", d.Images)
		}
	}
}

func TestImageResultsDeduplicateAcrossSnapshots(t *testing.T) {
	d := &Decoder{}
	payload := []byte(`{"message":{"author":{"role":"tool"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://output"}]}}}`)
	for i := 0; i < 3; i++ {
		if err := d.Feed(payload); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.Images) != 1 {
		t.Fatalf("duplicate output count %d", len(d.Images))
	}
}

func TestMalformedDeltaAndForeignErrorsAreRejected(t *testing.T) {
	for _, payload := range []string{`{"o":"add","p":"/constructor/key","v":"text"}`, `{"c":-1,"o":"add","p":"","v":1}`, `{"error":{"code":"blocked"}}`, `{"o":"patch","v":"bad"}`} {
		if err := (&Decoder{}).Feed([]byte(payload)); err == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
}

func TestCurrentDeltaChannelsAndArrayOperations(t *testing.T) {
	d := &Decoder{}
	for _, payload := range []string{
		`v1`,
		`{"v":{"conversation_id":"conv","message":{"author":{"role":"assistant"},"output":[]}}}`,
		`{"c":1,"p":"","v":{"unrelated":"a"}}`,
		`{"c":0,"p":"/message/output","o":"append","v":{"type":"image_generation_call","id":"a","status":"in_progress","result":""}}`,
		`{"p":"/message/output/0/result","v":"iVBORw0KGgo","o":"append"}`,
		`{"v":"-part"}`,
		`{"o":"truncate","v":11}`,
		`{"c":1,"p":"/unrelated","o":"append","v":"b"}`,
		`{"c":0,"p":"/message/output/0/status","o":"replace","v":"completed"}`,
	} {
		if err := d.Feed([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.Images) != 1 || d.Images[0].Value != "iVBORw0KGgo" {
		t.Fatalf("channels mixed: %+v", d.Images)
	}
	channel, ok := d.channels[1].(map[string]any)
	if !ok || channel["unrelated"] != "ab" {
		t.Fatal("independent channel overwritten")
	}
}

func TestIntermediatePixelHeightIsNotFinishedImage(t *testing.T) {
	d := &Decoder{}
	if err := d.Feed([]byte(`{"message":{"author":{"role":"tool"},"metadata":{"generation":{"height":128}},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"sediment://progress","height":1024}]}}}`)); err != nil {
		t.Fatal(err)
	}
	if len(d.Images) != 0 || !d.Async {
		t.Fatal("partial image was treated as complete")
	}
}
