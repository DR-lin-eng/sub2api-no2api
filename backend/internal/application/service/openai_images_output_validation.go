package service

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Read GIF blocks without decoding pixel buffers, so checking animations does
// not allocate width*height*frame_count memory.
type openAIImagesGIFInfo struct {
	FrameCount int
	DelaysMS   [16]int
}

func openAIImagesGIFFrames(image string) (int, error) {
	metadata, err := openAIImagesGIFMetadata(image)
	return metadata.FrameCount, err
}

func openAIImagesGIFMetadata(image string) (openAIImagesGIFInfo, error) {
	metadata := openAIImagesGIFInfo{}
	payload, encoding, err := openAIImageBase64Payload(image)
	if err != nil {
		return metadata, err
	}
	r := base64.NewDecoder(encoding, strings.NewReader(payload))
	header := make([]byte, 13)
	if _, err := io.ReadFull(r, header); err != nil {
		return metadata, err
	}
	if string(header[:6]) != "GIF87a" && string(header[:6]) != "GIF89a" {
		return metadata, fmt.Errorf("not a GIF file")
	}
	skipPalette := func(flags byte) error {
		if flags&0x80 == 0 {
			return nil
		}
		_, err := io.CopyN(io.Discard, r, int64(3*(1<<((flags&7)+1))))
		return err
	}
	if err := skipPalette(header[10]); err != nil {
		return metadata, err
	}
	readByte := func() (byte, error) { var b [1]byte; _, err := io.ReadFull(r, b[:]); return b[0], err }
	skipBlocks := func() error {
		for {
			n, err := readByte()
			if err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
			if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
				return err
			}
		}
	}
	delayMS := 0
	for {
		marker, err := readByte()
		if err != nil {
			return metadata, err
		}
		switch marker {
		case 0x3b:
			if metadata.FrameCount == 0 {
				return metadata, fmt.Errorf("GIF contains no frames")
			}
			return metadata, nil
		case 0x21:
			label, err := readByte()
			if err != nil {
				return metadata, err
			}
			if label == 0xf9 {
				var control [6]byte
				if _, err := io.ReadFull(r, control[:]); err != nil {
					return metadata, err
				}
				if control[0] != 4 || control[5] != 0 {
					return metadata, fmt.Errorf("invalid GIF control block")
				}
				delayMS = int(binary.LittleEndian.Uint16(control[2:4])) * 10
				continue
			}
			if err := skipBlocks(); err != nil {
				return metadata, err
			}
		case 0x2c:
			var descriptor [9]byte
			if _, err := io.ReadFull(r, descriptor[:]); err != nil {
				return metadata, err
			}
			if err := skipPalette(descriptor[8]); err != nil {
				return metadata, err
			}
			if _, err := readByte(); err != nil {
				return metadata, err
			}
			if err := skipBlocks(); err != nil {
				return metadata, err
			}
			if metadata.FrameCount >= len(metadata.DelaysMS) {
				return metadata, fmt.Errorf("GIF exceeds 16 frames")
			}
			metadata.DelaysMS[metadata.FrameCount] = delayMS
			metadata.FrameCount++
			delayMS = 0
		default:
			return metadata, fmt.Errorf("invalid GIF block")
		}
	}
}

func validateAccountImageTestOutput(image, format string) error {
	switch format {
	case "gif":
		frames, err := openAIImagesGIFFrames(image)
		if err != nil || frames < 2 {
			return fmt.Errorf("upstream did not return a valid animated GIF file")
		}
	case "svg":
		_, err := normalizeOpenAISVGOutput(image)
		return err
	}
	return nil
}
