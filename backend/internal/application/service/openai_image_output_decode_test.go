package service

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestSVGOutputNormalization(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg"><text>向量 🚲</text></svg>`
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, err := w.Write([]byte(raw))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	utf16Bytes := func(order binary.ByteOrder, bom []byte) []byte {
		data := append([]byte(nil), bom...)
		for _, v := range utf16.Encode([]rune(`<?xml version="1.0" encoding="UTF-16"?>` + raw)) {
			var encoded [2]byte
			order.PutUint16(encoded[:], v)
			data = append(data, encoded[:]...)
		}
		return data
	}
	fixtures := map[string]string{
		"standard":         base64.StdEncoding.EncodeToString([]byte(raw)),
		"unpadded":         base64.RawStdEncoding.EncodeToString([]byte(raw)),
		"url safe":         base64.RawURLEncoding.EncodeToString([]byte(raw)),
		"data URL":         "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(raw)),
		"percent data URL": "data:image/svg+xml," + url.PathEscape(raw),
		"raw":              raw,
		"UTF-8 BOM":        base64.StdEncoding.EncodeToString(append([]byte{0xef, 0xbb, 0xbf}, []byte(raw)...)),
		"UTF-16 LE":        base64.StdEncoding.EncodeToString(utf16Bytes(binary.LittleEndian, []byte{0xff, 0xfe})),
		"UTF-16 BE":        base64.StdEncoding.EncodeToString(utf16Bytes(binary.BigEndian, []byte{0xfe, 0xff})),
		"compressed":       base64.StdEncoding.EncodeToString(compressed.Bytes()),
	}
	for name, value := range fixtures {
		t.Run(name, func(t *testing.T) {
			normalized, err := normalizeOpenAIImageOutput(value, "svg")
			require.NoError(t, err)
			decoded, err := base64.StdEncoding.DecodeString(normalized)
			require.NoError(t, err)
			require.True(t, utf8.Valid(decoded))
			require.Contains(t, string(decoded), "向量 🚲")
			require.NoError(t, validateAccountImageTestOutput(normalized, "svg"))
		})
	}
}

func TestSVGOutputRejectsCorruptOrMislabeledData(t *testing.T) {
	for _, data := range []string{
		"<svg>\n<text>\xff</text></svg>", `<svg><path></svg>`, `<html/>`, `<svg/><svg/>`, `<svg/>unrelated text`,
		string([]byte{0xff, 0xfe, 0x3c}),
	} {
		_, err := normalizeOpenAISVGOutput(base64.StdEncoding.EncodeToString([]byte(data)))
		require.Error(t, err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	_, err := normalizeOpenAISVGOutput(base64.StdEncoding.EncodeToString(png))
	require.ErrorContains(t, err, "upstream returned image/png")
	_, err = normalizeOpenAISVGOutput("data:image/svg+xml")
	require.Error(t, err)
}
