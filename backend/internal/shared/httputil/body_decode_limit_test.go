package httputil

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"testing"
)

type zeroBodyReader struct{}

func (zeroBodyReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestDecodedBodyRejectsOverflowWithoutReturningTruncatedSuccess(t *testing.T) {
	body, err := readDecompressedRequestBody(io.LimitReader(zeroBodyReader{}, maxDecompressedBodySize+1))
	var limit *http.MaxBytesError
	if !errors.As(err, &limit) || limit.Limit != maxDecompressedBodySize || body != nil {
		t.Fatalf("overflow accepted: bytes=%d err=%v", len(body), err)
	}
}

func TestCompressedBodyChecksTrailerAtExactDecodedLimit(t *testing.T) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := io.CopyN(writer, zeroBodyReader{}, maxDecompressedBodySize); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buffer.Bytes()
	raw[len(raw)-8] ^= 0xff
	body, err := decompressRequestBody("gzip", raw)
	if err == nil || body != nil {
		t.Fatalf("corrupt trailer accepted at limit: bytes=%d err=%v", len(body), err)
	}
}
