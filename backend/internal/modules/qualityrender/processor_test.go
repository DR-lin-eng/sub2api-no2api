package qualityrender

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmbeddedCheckpoint(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(checkpoint)); got != "96bc1abf360ffba879310a0c5d4b4d9d70027083358999ed9fd3daba84fae2b6" {
		t.Fatalf("checkpoint: %s", got)
	}
}
func TestRejectsInvalidInputAndCancellation(t *testing.T) {
	p := New()
	if _, err := p.Process(context.Background(), ""); err == nil {
		t.Fatal("empty HTML accepted")
	}
	if _, err := p.Process(context.Background(), string(bytes.Repeat([]byte("x"), MaxHTMLBytes+1))); err == nil {
		t.Fatal("oversize accepted")
	}
	slot <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Process(ctx, "<svg></svg>")
	<-slot
	if err == nil {
		t.Fatal("cancel ignored while queued")
	}
}
func TestLocalRenderRealModel(t *testing.T) {
	if os.Getenv("QUALITY_RENDER_INTEGRATION") != "1" {
		t.Skip("real runtime test runs inside packaged Docker image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	html := `<!doctype html><html><body><svg xmlns="http://www.w3.org/2000/svg" width="896" height="672" viewBox="0 0 896 672"><rect width="896" height="672" fill="#e0f2fe"/><circle cx="240" cy="440" r="85" fill="none" stroke="#0f172a" stroke-width="8"/><circle cx="630" cy="440" r="85" fill="none" stroke="#0f172a" stroke-width="8"/><path d="M240 440L365 300L440 440L240 440M365 300L595 300L440 440M595 300L630 440" fill="none" stroke="#e11d48" stroke-width="10"/><ellipse cx="445" cy="255" rx="100" ry="60" fill="white" stroke="#334155" stroke-width="4"/><path d="M490 220Q540 100 570 165L610 180L575 192Q545 170 540 255" fill="white" stroke="#334155" stroke-width="4"/><path d="M585 172L700 200L588 198Z" fill="#fbbf24"/><circle cx="577" cy="165" r="5"/><circle cx="440" cy="440" r="16" fill="#334155"><animate attributeName="r" values="16;22;16" dur="1s" repeatCount="indefinite"/></circle></svg></body></html>`
	artifact, err := New().Process(ctx, html)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ModelVersion != "8.4.14" || (artifact.Label != "normal" && artifact.Label != "unnormal") {
		t.Fatalf("invalid model result: %+v", artifact)
	}
	config, err := png.DecodeConfig(bytes.NewReader(artifact.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 896 || config.Height != 672 {
		t.Fatalf("dimensions %dx%d", config.Width, config.Height)
	}
	if len(artifact.WebP) < 12 || string(artifact.WebP[8:12]) != "WEBP" {
		t.Fatal("missing WebP")
	}
	if dir := os.Getenv("QUALITY_RENDER_TEST_OUTPUT"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "preview.png"), artifact.PNG, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "preview.webp"), artifact.WebP, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("model=%s label=%s confidence=%.6f png=%d webp=%d", artifact.ModelVersion, artifact.Label, artifact.Confidence, len(artifact.PNG), len(artifact.WebP))
	if _, err := New().Process(ctx, "<html>not an SVG</html>"); err == nil {
		t.Fatal("invalid drawing was classified")
	}
}
