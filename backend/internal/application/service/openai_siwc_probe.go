package service

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

func prepareOpenAISIWCProbe(req *http.Request, account *Account) error {
	if req == nil || !account.IsOpenAISIWC() {
		return nil
	}
	applyOpenAISIWCHeaders(req.Header)
	if req.Method != http.MethodPost || req.Body == nil || !strings.Contains(req.URL.Path, "/responses") {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	_ = req.Body.Close()
	body, err = normalizeOpenAISIWCBody(body, strings.HasSuffix(req.URL.Path, "/compact"), false)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return nil
}
