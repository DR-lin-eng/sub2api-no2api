// Package qualityrender owns source-code fingerprint matching and the embedded
// local HTML-to-raster worker. It has no API, credentials, settings or listening port.
package qualityrender

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/worker.py
var worker []byte

const MaxHTMLBytes = 1 << 20
const MaxArtifactBytes = 4 << 20
const outputLimit = 12 << 20

// Only one Chromium process at a time per server. A waiting caller
// respects its existing run cancellation; the execution budget starts on entry.
var slot = make(chan struct{}, 1)

type Artifact struct {
	PNG, WebP []byte
}

type Processor struct{}

func New() *Processor { return &Processor{} }

func (p *Processor) Process(ctx context.Context, html string) (*Artifact, error) {
	if strings.TrimSpace(html) == "" || len(html) > MaxHTMLBytes || !containsSVG(html) {
		return nil, errors.New("invalid local render input")
	}
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-slot }()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	python, err := exec.LookPath("python3")
	if err != nil {
		return nil, errors.New("local renderer runtime missing from this installation")
	}
	dir, err := os.MkdirTemp("", "sub2api-quality-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for name, data := range map[string][]byte{"worker.py": worker} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return nil, err
		}
	}
	payload, err := json.Marshal(map[string]string{"html": html})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, python, "-I", filepath.Join(dir, "worker.py")) // #nosec G204 -- executable is python3, script is embedded, HTML only travels on stdin.
	cmd.Dir = dir
	// No database credentials, proxy variables or inherited application secrets.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir, "PYTHONDONTWRITEBYTECODE=1", "PYTHONUNBUFFERED=1"}
	if path := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); path != "" {
		cmd.Env = append(cmd.Env, "PLAYWRIGHT_BROWSERS_PATH="+path)
	}
	cmd.Stdin = bytes.NewReader(payload)
	output := &limitedBuffer{remaining: outputLimit}
	stderr := &limitedBuffer{remaining: 4096}
	cmd.Stdout, cmd.Stderr = output, stderr
	cmd.WaitDelay = 2 * time.Second
	configureProcess(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("local preview rendering failed")
	}
	var wire struct {
		PNG  string `json:"png_base64"`
		WebP string `json:"webp_base64"`
	}
	if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
		return nil, errors.New("invalid local renderer result")
	}
	png, err := base64.StdEncoding.DecodeString(wire.PNG)
	if err != nil || len(png) == 0 || len(png) > MaxArtifactBytes {
		return nil, errors.New("invalid renderer PNG")
	}
	webp, err := base64.StdEncoding.DecodeString(wire.WebP)
	if err != nil || len(webp) == 0 || len(webp) > MaxArtifactBytes {
		return nil, errors.New("invalid renderer WebP")
	}
	return &Artifact{PNG: png, WebP: webp}, nil
}

type limitedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.remaining {
		return 0, errors.New("renderer output limit exceeded")
	}
	b.remaining -= len(p)
	return b.Buffer.Write(p)
}
