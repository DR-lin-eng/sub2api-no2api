package service

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Read GIF blocks without decoding pixel buffers, so checking animations does
// not allocate width*height*frame_count memory.
func openAIImagesGIFFrames(image string) (int, error) {
	r := base64.NewDecoder(base64.StdEncoding, strings.NewReader(image))
	header := make([]byte, 13)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, err
	}
	if string(header[:6]) != "GIF87a" && string(header[:6]) != "GIF89a" {
		return 0, fmt.Errorf("not a GIF file")
	}
	skipPalette := func(flags byte) error {
		if flags&0x80 == 0 {
			return nil
		}
		_, err := io.CopyN(io.Discard, r, int64(3*(1<<((flags&7)+1))))
		return err
	}
	if err := skipPalette(header[10]); err != nil {
		return 0, err
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
	frames := 0
	for {
		marker, err := readByte()
		if err != nil {
			return 0, err
		}
		switch marker {
		case 0x3b:
			if frames == 0 {
				return 0, fmt.Errorf("GIF contains no frames")
			}
			return frames, nil
		case 0x21:
			if _, err := readByte(); err != nil {
				return 0, err
			}
			if err := skipBlocks(); err != nil {
				return 0, err
			}
		case 0x2c:
			var descriptor [9]byte
			if _, err := io.ReadFull(r, descriptor[:]); err != nil {
				return 0, err
			}
			if err := skipPalette(descriptor[8]); err != nil {
				return 0, err
			}
			if _, err := readByte(); err != nil {
				return 0, err
			}
			if err := skipBlocks(); err != nil {
				return 0, err
			}
			frames++
			if frames > 16 {
				return 0, fmt.Errorf("GIF exceeds 16 frames")
			}
		default:
			return 0, fmt.Errorf("invalid GIF block")
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
		decoder := xml.NewDecoder(base64.NewDecoder(base64.StdEncoding, strings.NewReader(image)))
		found := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				if found {
					return nil
				}
				return fmt.Errorf("SVG output is empty")
			}
			if err != nil {
				return fmt.Errorf("upstream did not return a valid SVG file: %w", err)
			}
			if element, ok := token.(xml.StartElement); ok && !found {
				if element.Name.Local != "svg" {
					return fmt.Errorf("upstream did not return an SVG document")
				}
				found = true
			}
		}
	}
	return nil
}
