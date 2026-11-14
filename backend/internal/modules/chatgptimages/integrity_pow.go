package chatgptimages

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"
	"unicode/utf16"
)

// proof reuses the encoded environment and buffers. Only slots 3 and 9 change
// in the nonce loop, avoiding JSON encoding and allocations per candidate.
func (i *Integrity) proof(ctx context.Context, seed, difficulty string, limit int) (string, error) {
	start := time.Now()
	env, err := i.environment()
	if err != nil {
		return "", err
	}
	parts := [3][]byte{[]byte{'['}, {','}, {','}}
	for n, field := range env {
		if n == 3 || n == 9 {
			continue
		}
		part := 0
		if n > 3 {
			part = 1
		}
		if n > 9 {
			part = 2
		}
		if n != 0 && n != 4 && n != 10 {
			parts[part] = append(parts[part], ',')
		}
		data, err := integrityJSON(field)
		if err != nil {
			return "", err
		}
		parts[part] = append(parts[part], data...)
	}
	parts[0] = append(parts[0], ',')
	parts[1] = append(parts[1], ',')
	parts[2] = append(parts[2], ']')
	capacity := len(parts[0]) + len(parts[1]) + len(parts[2]) + 32
	candidate := make([]byte, 0, capacity)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(capacity))
	seedHash := uint32(0x811c9dc5)
	for _, c := range utf16.Encode([]rune(seed)) {
		seedHash = (seedHash ^ uint32(c)) * 0x1000193
	}
	digits := min(len(utf16.Encode([]rune(difficulty))), 8)
	const hex = "0123456789abcdef"
	for nonce := 0; nonce < limit; nonce++ {
		if nonce%256 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		candidate = append(candidate[:0], parts[0]...)
		candidate = strconv.AppendInt(candidate, int64(nonce), 10)
		candidate = append(candidate, parts[1]...)
		candidate = strconv.AppendInt(candidate, (time.Since(start).Nanoseconds()+500000)/1000000, 10)
		candidate = append(candidate, parts[2]...)
		n := base64.StdEncoding.EncodedLen(len(candidate))
		base64.StdEncoding.Encode(encoded[:n], candidate)
		h := seedHash
		for _, c := range encoded[:n] {
			h = (h ^ uint32(c)) * 0x1000193
		}
		h = finishProofHash(h)
		var sum [8]byte
		for j := range sum {
			sum[j] = hex[h>>uint(28-j*4)&15]
		}
		if string(sum[:digits]) <= difficulty {
			return "gAAAAAB" + string(encoded[:n]) + "~S", nil
		}
	}
	return "", ErrProofExhausted
}

func finishProofHash(h uint32) uint32 {
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	return h ^ h>>16
}
