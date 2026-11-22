package mediaprotocol

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeTaskIdentityAndResultValidation(t *testing.T) {
	driver, err := Select("ark")
	require.NoError(t, err)
	id, err := PublicTaskID(driver, 7, "cgt-123")
	require.NoError(t, err)
	got, raw := Lookup(7, id)
	require.Equal(t, driver, got)
	require.Equal(t, "cgt-123", raw)
	for _, raw := range []string{"", "../other", strings.Repeat("t", 201)} {
		_, err := PublicTaskID(driver, 7, raw)
		require.Error(t, err)
	}
	other, err := PublicTaskID(driver, 8, "cgt-123")
	require.NoError(t, err)
	require.NotEqual(t, id, other)
	for _, body := range []string{
		`{"id":"other","status":"running"}`,
		`{"id":"cgt-123","status":"unknown"}`,
		`{"id":"cgt-123","status":"succeeded","content":{"video_url":"https://user:pass@example.org/video"}}`,
		`{"id":"cgt-123","status":"succeeded","content":{}}`,
	} {
		_, err := driver.DecodeTask(VideoContent, []byte(body), "cgt-123")
		require.Error(t, err)
	}
	_, err = driver.DecodeImages([]byte(`{"data":[{"url":"https://cdn.example/one"}]}`), "b64_json")
	require.Error(t, err)
	_, err = driver.DecodeImages([]byte(`{"data":[{"error":"failed"}],"usage":{"generated_images":1}}`), "url")
	require.Error(t, err)
}
