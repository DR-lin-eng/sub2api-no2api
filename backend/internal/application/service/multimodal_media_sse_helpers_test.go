//go:build unit

package service

import "bytes"

func aggregateMultimodalImageSSE(body []byte) ([]byte, bool) {
	collector := newMultimodalImageSSECollector()
	if err := readMultimodalSSEFrames(bytes.NewReader(body), defaultUpstreamResponseReadMaxBytes, collector.addFrame); err != nil {
		return nil, false
	}
	encoded, err := collector.response()
	return encoded, err == nil
}
