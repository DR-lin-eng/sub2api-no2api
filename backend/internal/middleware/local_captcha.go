package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	localCaptchaTTL          = 5 * time.Minute
	localCaptchaAnswerLength = 5
	localCaptchaMaxBodyBytes = 64 << 10
	localCaptchaMaxActive    = 10_000 // New challenges retire the oldest entry once this global cap is reached.
	localCaptchaAlphabet     = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

var (
	localCaptchaFontOnce sync.Once
	localCaptchaFont     *opentype.Font
	localCaptchaFontErr  error
)

var localCaptchaIssueScript = redis.NewScript(`
local created = redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2], 'NX')
if not created then
  return 0
end

local sequence = redis.call('INCR', KEYS[3])
redis.call('ZADD', KEYS[2], sequence, ARGV[3])
redis.call('PEXPIRE', KEYS[2], ARGV[2] * 2)

local excess = redis.call('ZCARD', KEYS[2]) - tonumber(ARGV[4])
if excess > 0 then
  local victims = redis.call('ZRANGE', KEYS[2], 0, excess - 1)
  for _, victim in ipairs(victims) do
    redis.call('DEL', ARGV[5] .. victim)
    redis.call('ZREM', KEYS[2], victim)
  end
end

return 1
`)

var localCaptchaConsumeScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value then
  redis.call('DEL', KEYS[1])
end
redis.call('ZREM', KEYS[2], ARGV[1])
return value
`)

// LocalCaptchaPayload contains the challenge fields used by protected auth routes.
type LocalCaptchaPayload struct {
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
	VerifyCode  string `json:"verify_code"`
}

type LocalCaptchaRequireOptions struct {
	Enabled func(context.Context) bool
	Skip    func(context.Context, LocalCaptchaPayload) bool
}

type LocalCaptcha struct {
	redis     *redis.Client
	prefix    string
	maxActive int
}

type LocalCaptchaResponse struct {
	CaptchaID string `json:"captcha_id"`
	ImageData string `json:"image_data"`
	ExpiresIn int    `json:"expires_in"`
}

func NewLocalCaptcha(redisClient *redis.Client) *LocalCaptcha {
	return &LocalCaptcha{
		redis:     redisClient,
		prefix:    "auth_captcha:",
		maxActive: localCaptchaMaxActive,
	}
}

// Generate returns a short-lived image challenge. The answer is never returned
// and Redis stores only an IP-bound digest keyed by a random challenge ID.
func (l *LocalCaptcha) Generate(enabled func(context.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		if enabled == nil || !enabled(c.Request.Context()) {
			response.NotFound(c, "captcha protection is not enabled")
			return
		}
		if l == nil || l.redis == nil {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			return
		}

		captchaID, err := randomHex(16)
		if err != nil {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			return
		}
		answer, err := randomCaptchaText(localCaptchaAnswerLength)
		if err != nil {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			return
		}
		imageData, err := renderLocalCaptcha(answer)
		if err != nil {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			return
		}

		digest := hex.EncodeToString(localCaptchaDigest(captchaID, answer, clientip.GetClientIP(c)))
		created, err := l.issue(c.Request.Context(), captchaID, digest)
		if err != nil || !created {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			return
		}

		response.Success(c, LocalCaptchaResponse{
			CaptchaID: captchaID,
			ImageData: imageData,
			ExpiresIn: int(localCaptchaTTL.Seconds()),
		})
	}
}

// Require consumes and verifies a challenge before allowing the auth handler to
// run. Every challenge is single-use, including failed answer attempts.
func (l *LocalCaptcha) Require(opts LocalCaptchaRequireOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if opts.Enabled == nil || !opts.Enabled(ctx) {
			c.Next()
			return
		}

		payload, ok := readLocalCaptchaPayload(c)
		if !ok {
			return
		}
		if opts.Skip != nil && opts.Skip(ctx, payload) {
			c.Next()
			return
		}
		if strings.TrimSpace(payload.CaptchaID) == "" || strings.TrimSpace(payload.CaptchaCode) == "" {
			abortLocalCaptcha(c, "LOCAL_CAPTCHA_REQUIRED", "captcha verification is required")
			return
		}
		if l == nil || l.redis == nil {
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			c.Abort()
			return
		}

		captchaID := strings.TrimSpace(payload.CaptchaID)
		expectedHex, err := l.consume(ctx, captchaID)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				abortLocalCaptcha(c, "LOCAL_CAPTCHA_EXPIRED", "captcha expired, please refresh and try again")
				return
			}
			response.ErrorWithDetails(c, http.StatusServiceUnavailable, "captcha service is unavailable", "LOCAL_CAPTCHA_UNAVAILABLE", nil)
			c.Abort()
			return
		}

		expected, err := hex.DecodeString(expectedHex)
		actual := localCaptchaDigest(captchaID, payload.CaptchaCode, clientip.GetClientIP(c))
		if err != nil || len(expected) != len(actual) || subtle.ConstantTimeCompare(expected, actual) != 1 {
			abortLocalCaptcha(c, "LOCAL_CAPTCHA_INVALID", "captcha verification failed")
			return
		}

		c.Set("local_captcha_verified", true)
		c.Next()
	}
}

func (l *LocalCaptcha) issue(ctx context.Context, captchaID, digest string) (bool, error) {
	maxActive := l.maxActive
	if maxActive < 1 {
		maxActive = localCaptchaMaxActive
	}
	created, err := localCaptchaIssueScript.Run(
		ctx,
		l.redis,
		[]string{l.prefix + captchaID, l.prefix + "active", l.prefix + "sequence"},
		digest,
		localCaptchaTTL.Milliseconds(),
		captchaID,
		maxActive,
		l.prefix,
	).Int64()
	return created == 1, err
}

func (l *LocalCaptcha) consume(ctx context.Context, captchaID string) (string, error) {
	return localCaptchaConsumeScript.Run(
		ctx,
		l.redis,
		[]string{l.prefix + captchaID, l.prefix + "active"},
		captchaID,
	).Text()
}

func readLocalCaptchaPayload(c *gin.Context) (LocalCaptchaPayload, bool) {
	var payload LocalCaptchaPayload
	if c == nil {
		return payload, false
	}
	if c.Request == nil || c.Request.Body == nil {
		abortLocalCaptcha(c, "LOCAL_CAPTCHA_REQUIRED", "captcha verification is required")
		return payload, false
	}

	originalBody := c.Request.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, localCaptchaMaxBodyBytes+1))
	_ = originalBody.Close()
	if err != nil {
		response.BadRequest(c, "invalid request body")
		c.Abort()
		return payload, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > localCaptchaMaxBodyBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "request body is too large")
		c.Abort()
		return payload, false
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		response.BadRequest(c, "invalid request body")
		c.Abort()
		return payload, false
	}
	return payload, true
}

func abortLocalCaptcha(c *gin.Context, reason, message string) {
	response.ErrorWithDetails(c, http.StatusBadRequest, message, reason, nil)
	c.Abort()
}

func localCaptchaDigest(captchaID, answer, clientIP string) []byte {
	normalized := strings.ToUpper(strings.TrimSpace(answer))
	digest := sha256.Sum256([]byte(captchaID + ":" + normalized + ":" + strings.TrimSpace(clientIP)))
	return digest[:]
}

func randomHex(byteLength int) (string, error) {
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomCaptchaText(length int) (string, error) {
	buf := make([]byte, length)
	for i := range buf {
		index, err := secureRandomInt(len(localCaptchaAlphabet))
		if err != nil {
			return "", err
		}
		buf[i] = localCaptchaAlphabet[index]
	}
	return string(buf), nil
}

func secureRandomInt(max int) (int, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func renderLocalCaptcha(answer string) (string, error) {
	localCaptchaFontOnce.Do(func() {
		localCaptchaFont, localCaptchaFontErr = opentype.Parse(gobold.TTF)
	})
	if localCaptchaFontErr != nil {
		return "", localCaptchaFontErr
	}
	face, err := opentype.NewFace(localCaptchaFont, &opentype.FaceOptions{
		Size:    32,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return "", err
	}
	defer func() { _ = face.Close() }()

	const width, height = 220, 70
	visualRandom := make([]byte, 1024)
	if _, err := rand.Read(visualRandom); err != nil {
		return "", err
	}
	randomOffset := 0
	nextVisualInt := func(max int) int {
		value := int(visualRandom[randomOffset]) % max
		randomOffset++
		return value
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillImage(img, color.RGBA{R: 246, G: 248, B: 250, A: 255})

	for i := 0; i < 180; i++ {
		x := nextVisualInt(width)
		y := nextVisualInt(height)
		shade := nextVisualInt(45)
		img.Set(x, y, color.RGBA{R: uint8(185 + shade), G: uint8(190 + shade), B: uint8(195 + shade), A: 255})
	}
	for i := 0; i < 6; i++ {
		x0 := nextVisualInt(width)
		y0 := nextVisualInt(height)
		x1 := nextVisualInt(width)
		y1 := nextVisualInt(height)
		drawCaptchaLine(img, x0, y0, x1, y1, color.RGBA{R: 145, G: 155, B: 165, A: 150})
	}

	for i, char := range answer {
		jitterX := nextVisualInt(7)
		jitterY := nextVisualInt(13)
		red := nextVisualInt(65)
		green := nextVisualInt(65)
		blue := nextVisualInt(65)
		drawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.RGBA{R: uint8(25 + red), G: uint8(35 + green), B: uint8(45 + blue), A: 255}),
			Face: face,
			Dot:  fixed.P(16+i*39+jitterX, 44+jitterY),
		}
		drawer.DrawString(string(char))
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}

func fillImage(img *image.RGBA, fill color.RGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
}

func drawCaptchaLine(img *image.RGBA, x0, y0, x1, y1 int, lineColor color.RGBA) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if image.Pt(x0, y0).In(img.Bounds()) {
			img.SetRGBA(x0, y0, lineColor)
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
