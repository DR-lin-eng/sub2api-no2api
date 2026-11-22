//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/mediaprotocol"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type mediaProtocolHTTP struct {
	HTTPUpstream
	client *http.Client
}

func (u mediaProtocolHTTP) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func TestNativeMediaThroughGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const rawID = "cgt-demo123"
	var status atomic.Value
	status.Store("running")
	var wirePath, wireBody string
	var count int
	type requestSnapshot struct{ path, body string }
	received := make(chan requestSnapshot, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		data, _ := io.ReadAll(r.Body)
		received <- requestSnapshot{path, string(data)}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && path == "/api/v3/images/generations":
			fmt.Fprint(w, `{"created":42,"data":[{"b64_json":"aW1hZ2U=","size":"1024x1024"}]}`)
		case r.Method == http.MethodPost && path == "/api/v3/contents/generations/tasks":
			fmt.Fprintf(w, `{"id":%q}`, rawID)
		case r.Method == http.MethodGet && path == "/api/v3/contents/generations/tasks/"+rawID:
			fmt.Fprintf(w, `{"id":%q,"status":%q,"duration":4,"content":{"video_url":"https://cdn.example/result.mp4"}}`, rawID, status.Load())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(up.Close)
	account := &Account{ID: 99, Platform: PlatformMultimodal, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": up.URL + "/api/v3", "api_key": "fixture-key", "media_protocol": "ark",
			"model_mapping": map[string]any{"my-image": "seedream-fixture", "my-video": "seedance-fixture"}}}
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{cfg: &config.Config{}, cache: &reproGatewayCache{}, usageBillingRepo: repo,
		httpUpstream: mediaProtocolHTTP{client: up.Client()}}
	group := Group{ID: 3, Platform: PlatformMultimodal}
	key := &APIKey{ID: 7, UserID: 42, GroupID: &group.ID, Group: &group}
	call := func(op MultimodalMediaEndpoint, id string, body []byte, typ string, hold *BalanceHoldRecord) (*httptest.ResponseRecorder, *OpenAIForwardResult, error) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/"+id, bytes.NewReader(body))
		c.Set("api_key", key)
		result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, op, id, body, typ, hold)
		select {
		case request := <-received:
			count++
			wirePath, wireBody = request.path, request.body
		default:
		}
		return w, result, err
	}
	edit := new(bytes.Buffer)
	form := multipart.NewWriter(edit)
	require.NoError(t, form.WriteField("model", "my-image"))
	require.NoError(t, form.WriteField("prompt", "edit"))
	require.NoError(t, form.WriteField("response_format", "b64_json"))
	part, err := form.CreateFormFile("image[]", "ref.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, form.Close())
	for _, tc := range []struct {
		op   MultimodalMediaEndpoint
		body []byte
		typ  string
	}{
		{mediaprotocol.ImageGenerate, []byte(`{"model":"my-image","prompt":"draw","response_format":"b64_json","n":1}`), "application/json"},
		{mediaprotocol.ImageEdit, edit.Bytes(), form.FormDataContentType()},
	} {
		w, result, err := call(tc.op, "", tc.body, tc.typ, nil)
		require.NoError(t, err)
		require.Equal(t, "seedream-fixture", gjson.Get(wireBody, "model").String())
		require.False(t, gjson.Get(wireBody, "stream").Exists())
		require.Equal(t, 1, result.ImageCount)
		require.Equal(t, "my-image", result.BillingModel)
		require.Equal(t, "aW1hZ2U=", gjson.Get(w.Body.String(), "data.0.b64_json").String())
		if tc.op == mediaprotocol.ImageEdit {
			require.Equal(t, "data:image/png;base64,iVBORw0KGgo=", gjson.Get(wireBody, "image.0").String())
		}
	}
	before := count
	for _, extra := range []string{`"stream":"invalid"`, `"mask":"input.png"`, `"n":2`} {
		_, _, err = call(mediaprotocol.ImageGenerate, "", []byte(`{"model":"my-image","prompt":"draw",`+extra+`}`), "application/json", nil)
		require.ErrorIs(t, err, mediaprotocol.ErrUnsupported)
	}
	_, _, err = call(mediaprotocol.VideoGenerate, "", []byte(`{"model":"my-video","prompt":"go","duration":26}`), "application/json", nil)
	require.ErrorIs(t, err, mediaprotocol.ErrUnsupported)
	require.Equal(t, before, count)

	hold := reproExpiredHold(repo, "native-complete", 1)
	body := []byte(`{"model":"my-video","prompt":"pan","images":["https://ref.example/frame.jpg"],"duration":8,"resolution":"720p","aspect_ratio":"16:9"}`)
	w, result, err := call(mediaprotocol.VideoGenerate, "", body, "application/json", hold)
	require.NoError(t, err)
	require.Equal(t, "seedance-fixture", gjson.Get(wireBody, "model").String())
	require.Equal(t, "first_frame", gjson.Get(wireBody, "content.1.role").String())
	require.Equal(t, "pan", gjson.Get(wireBody, "content.0.text").String())
	require.Equal(t, "16:9", gjson.Get(wireBody, "ratio").String())
	require.False(t, gjson.Get(wireBody, "prompt").Exists())
	id := result.ResponseID
	require.NotEqual(t, rawID, id)
	require.Equal(t, id, gjson.Get(w.Body.String(), "id").String())
	require.Equal(t, id, hold.UpstreamRequestID)
	// Reload with a new service/cache and change the account's submit protocol.
	// The persisted hold and task ID still recover the original native route.
	svc = &OpenAIGatewayService{cfg: svc.cfg, usageBillingRepo: repo, httpUpstream: svc.httpUpstream}
	delete(account.Credentials, "media_protocol")
	owner, err := svc.ResolveMultimodalVideoTaskOwner(context.Background(), key, id)
	require.NoError(t, err)
	require.Equal(t, account.ID, owner)
	other := *key
	other.ID++
	owner, err = svc.ResolveMultimodalVideoTaskOwner(context.Background(), &other, id)
	require.NoError(t, err)
	require.Zero(t, owner)
	w, _, err = call(mediaprotocol.VideoContent, id, nil, "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Empty(t, repo.captureCalls)
	status.Store("succeeded")
	w, _, err = call(mediaprotocol.VideoContent, id, nil, "", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, w.Code)
	require.Equal(t, "https://cdn.example/result.mp4", w.Header().Get("Location"))
	w, _, err = call(mediaprotocol.VideoStatus, id, nil, "", nil)
	require.NoError(t, err)
	require.Equal(t, "completed", gjson.Get(w.Body.String(), "status").String())
	require.NotContains(t, w.Body.String(), "cdn.example")
	require.Equal(t, "/v1/videos/"+id+"/content", gjson.Get(w.Body.String(), "url").String())
	require.Len(t, repo.captureCalls, 1)
	require.InDelta(t, .5, repo.captureCalls[0].ActualAmount, 1e-9)
	// A second fixture task fails. A direct content read also releases its hold.
	account.Credentials["media_protocol"] = "ark"
	delete(repo.holds, hold.HoldKey)
	failed := reproExpiredHold(repo, "native-failure", 1)
	_, _, err = call(mediaprotocol.VideoGenerate, "", body, "application/json", failed)
	require.NoError(t, err)
	status.Store("failed")
	repo.releaseErr = fmt.Errorf("fixture storage unavailable")
	w, _, err = call(mediaprotocol.VideoContent, id, nil, "", nil)
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Empty(t, w.Header().Get("Location"))
	require.Equal(t, BalanceHoldStatusPending, failed.Status)
	repo.releaseErr = nil
	w, _, err = call(mediaprotocol.VideoContent, id, nil, "", nil)
	require.NoError(t, err)
	require.Empty(t, w.Header().Get("Location"))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Len(t, repo.releaseCalls, 2)
	require.Equal(t, repo.releaseCalls[0].RequestID, repo.releaseCalls[1].RequestID)
	require.Len(t, repo.captureCalls, 1)
	// A pre-upgrade ID on this migrated account uses the old path.
	_, _, err = call(mediaprotocol.VideoStatus, "old-task", nil, "", nil)
	require.Equal(t, "/api/v3/videos/old-task", wirePath)
	require.Error(t, err)
}
