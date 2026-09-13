package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func largeNativeResponsesBody(imageBytes int, invalidID bool) []byte {
	id := "fc_valid"
	if invalidID {
		id = "item_invalid"
	}
	prefix := `{"model":"gpt-5.6-sol","store":false,"stream":true,"input":[{"type":"custom_tool_call","id":"` + id + `","call_id":"call_valid","output":[{"type":"input_image","image_url":"data:image/png;base64,`
	suffix := `"}]}],"opaque":9007199254740993}`
	return []byte(prefix + strings.Repeat("A", imageBytes) + suffix)
}

func TestLargeNativeResponsesNoopKeepsOriginalBytes(t *testing.T) {
	body := largeNativeResponsesBody(2<<20, false)
	out, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)
	require.NoError(t, err)
	require.False(t, changed)
	require.True(t, bytes.Equal(body, out))
}

func TestLargeNativeResponsesRewriteCopiesImageOnce(t *testing.T) {
	body := largeNativeResponsesBody(2<<20, true)
	wantImage := gjson.GetBytes(body, "input.0.output.0.image_url").String()
	out, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gjson.ValidBytes(out))
	require.False(t, gjson.GetBytes(out, "input.0.id").Exists())
	require.Equal(t, wantImage, gjson.GetBytes(out, "input.0.output.0.image_url").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(out, "opaque").Raw)
}

func TestDuplicateInputKeysUseDecoderCompatibilityPath(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[],"input":[{"type":"message","id":"bad","content":"keep"}]}`)
	want, wantChanged, wantErr := sanitizeOpenAIResponsesInputIDsDecoded(body, false)
	got, gotChanged, gotErr := sanitizeOpenAIResponsesInputIDs(body, false)
	if wantErr != nil {
		require.Error(t, gotErr)
		return
	}
	require.NoError(t, gotErr)
	require.Equal(t, wantChanged, gotChanged)
	if wantChanged {
		var wantJSON, gotJSON any
		require.NoError(t, json.Unmarshal(want, &wantJSON))
		require.NoError(t, json.Unmarshal(got, &gotJSON))
		require.Equal(t, wantJSON, gotJSON)
	}
}
