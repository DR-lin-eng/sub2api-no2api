package service

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Keep GIF frame scans streaming. Inspect the alphabet without decoding a
// potentially large animation twice or allocating pixel buffers.
func openAIImageBase64Payload(value string) (string, *base64.Encoding, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		meta, payload, ok := strings.Cut(value, ",")
		if !ok || !strings.HasSuffix(strings.ToLower(meta), ";base64") {
			return "", nil, fmt.Errorf("image data URL must contain Base64")
		}
		value = strings.TrimSpace(payload)
	}
	if len(value) == 0 || len(value) > base64.StdEncoding.EncodedLen(openAIImageMaxDownloadBytes) {
		return "", nil, fmt.Errorf("image output is empty or exceeds the size limit")
	}
	value = strings.TrimRight(value, "=")
	if value == "" {
		return "", nil, fmt.Errorf("image output is empty")
	}
	encoding := base64.RawStdEncoding
	if strings.ContainsAny(value, "-_") {
		encoding = base64.RawURLEncoding
	}
	return value, encoding, nil
}

func normalizeOpenAIImageOutput(value, format string) (string, error) {
	if format == "svg" {
		return normalizeOpenAISVGOutput(value)
	}
	payload, encoding, err := openAIImageBase64Payload(value)
	if err != nil {
		return "", err
	}
	data, err := encoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("invalid image Base64: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Normalize compatible SVG representations before validation and delivery.
// Raster bytes or malformed XML are errors; never relabel them as SVG.
func normalizeOpenAISVGOutput(value string) (string, error) {
	value = strings.TrimSpace(value)
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(value, "<") || strings.HasPrefix(value, "\ufeff<"):
		if len(value) > openAIImageMaxDownloadBytes {
			return "", fmt.Errorf("SVG exceeds the size limit")
		}
		data = []byte(value)
	case strings.HasPrefix(strings.ToLower(value), "data:") && !strings.HasSuffix(strings.ToLower(strings.SplitN(value, ",", 2)[0]), ";base64"):
		_, payload, ok := strings.Cut(value, ",")
		if !ok || len(payload) > 3*openAIImageMaxDownloadBytes {
			return "", fmt.Errorf("invalid SVG data URL")
		}
		decoded, decodeErr := url.PathUnescape(payload)
		if decodeErr != nil {
			return "", fmt.Errorf("invalid SVG data URL: %w", decodeErr)
		}
		data = []byte(decoded)
	default:
		payload, encoding, parseErr := openAIImageBase64Payload(value)
		if parseErr != nil {
			return "", parseErr
		}
		data, err = encoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("invalid SVG Base64: %w", err)
		}
	}
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		r, openErr := gzip.NewReader(bytes.NewReader(data))
		if openErr != nil {
			return "", fmt.Errorf("invalid compressed SVG: %w", openErr)
		}
		data, err = io.ReadAll(io.LimitReader(r, openAIImageMaxDownloadBytes+1))
		closeErr := r.Close()
		if err != nil {
			return "", fmt.Errorf("invalid compressed SVG: %w", err)
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	if len(data) > openAIImageMaxDownloadBytes {
		return "", fmt.Errorf("SVG exceeds the size limit")
	}
	if kind := http.DetectContentType(data); strings.HasPrefix(kind, "image/") && kind != "image/svg+xml" {
		return "", fmt.Errorf("SVG was requested but upstream returned %s", kind)
	}
	if len(data) >= 2 && (bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff})) {
		var order binary.ByteOrder = binary.BigEndian
		if data[0] == 0xff {
			order = binary.LittleEndian
		}
		if len(data)%2 != 0 {
			return "", fmt.Errorf("invalid UTF-16 SVG length")
		}
		var text strings.Builder
		for i := 2; i < len(data); i += 2 {
			v := rune(order.Uint16(data[i:]))
			if utf16.IsSurrogate(v) {
				if i+3 >= len(data) {
					return "", fmt.Errorf("invalid UTF-16 SVG surrogate")
				}
				v = utf16.DecodeRune(v, rune(order.Uint16(data[i+2:])))
				if v == utf8.RuneError {
					return "", fmt.Errorf("invalid UTF-16 SVG surrogate")
				}
				i += 2
			}
			_, _ = text.WriteRune(v)
		}
		data = []byte(text.String())
		// The bytes are now UTF-8. The declaration is optional in XML.
		if bytes.HasPrefix(data, []byte("<?xml")) {
			if end := bytes.Index(data, []byte("?>")); end >= 0 {
				data = data[end+2:]
			}
		}
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if len(data) > openAIImageMaxDownloadBytes {
		return "", fmt.Errorf("SVG exceeds the size limit")
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("upstream SVG contains invalid UTF-8 bytes")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for {
		token, decodeErr := decoder.Token()
		if decodeErr == io.EOF {
			break
		}
		if decodeErr != nil {
			return "", fmt.Errorf("upstream did not return a valid SVG file: %w", decodeErr)
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || element.Name.Local != "svg" {
					return "", fmt.Errorf("upstream did not return an SVG document")
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return "", fmt.Errorf("unexpected text outside SVG document")
			}
		}
	}
	if roots != 1 {
		return "", fmt.Errorf("SVG output is empty")
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
