package chatgptimages

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRequirements   = errors.New("invalid ChatGPT requirements response")
	ErrForceLogin     = errors.New("ChatGPT requires the selected account to sign in again")
	ErrChallenge      = errors.New("invalid ChatGPT verification challenge")
	ErrProofExhausted = errors.New("ChatGPT proof of work exhausted its search budget")
	ErrVMBudget       = errors.New("ChatGPT verification VM exceeded its execution budget")
	ErrVMRejected     = errors.New("ChatGPT verification VM rejected the challenge")
)

// Integrity implements the Linux/browser wire protocol without launching a
// browser. Its small environment is a protocol model, not device attestation.
// Each instance belongs to exactly one prepare and its subsequent conversation.
type Integrity struct {
	key     string
	profile integrityProfile
	started time.Time
}

type integrityProfile struct {
	UserAgent   string `json:"userAgent"`
	Language    string `json:"language"`
	Platform    string `json:"platform"`
	Concurrency int    `json:"concurrency"`
	TimeOrigin  int64  `json:"timeOrigin"`
	DeviceID    string `json:"deviceID"`
}

var navigatorProperties = []string{"userAgent", "language", "languages", "hardwareConcurrency", "platform"}
var documentProperties = []string{"scripts", "documentElement", "location"}
var windowProperties = []string{"window", "self", "navigator", "screen", "performance", "document", "location", "localStorage", "Math", "JSON", "Object", "Array", "String", "Number", "Boolean", "Date", "Promise", "atob", "btoa", "crypto"}

func NewIntegrity(userAgent, language, deviceID string) (*Integrity, error) {
	language = strings.TrimSpace(strings.Split(strings.Split(language, ",")[0], ";")[0])
	if language == "" || language == "*" || len(language) > 64 {
		language = "en"
	}
	started := time.Now()
	i := &Integrity{started: started, profile: integrityProfile{UserAgent: userAgent, Language: language, Platform: runtime.GOOS, Concurrency: runtime.NumCPU(), TimeOrigin: started.UnixMilli(), DeviceID: deviceID}}
	env, err := i.environment()
	if err != nil {
		return nil, err
	}
	env[3], env[9] = 1, float64(time.Since(started))/float64(time.Millisecond)
	data, err := integrityJSON(env)
	if err != nil {
		return nil, err
	}
	i.key = "gAAAAAC" + base64.StdEncoding.EncodeToString(data)
	return i, nil
}

func (i *Integrity) RequirementsKey() string { return i.key }

func (i *Integrity) environment() ([25]any, error) {
	var env [25]any
	id, err := uuid.NewRandom()
	if err != nil {
		return env, err
	}
	var choices [3]byte
	if _, err := rand.Read(choices[:]); err != nil {
		return env, err
	}
	navigatorValues := []string{i.profile.UserAgent, i.profile.Language, i.profile.Language, fmt.Sprint(i.profile.Concurrency), i.profile.Platform}
	n := int(choices[0]) % len(navigatorProperties)
	// The protocol model has no display, DOM scripts or JS heap measurement.
	// Do not invent App build IDs, script URLs, screen size or Apple tokens.
	env = [25]any{0, time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)"), nil, 0,
		i.profile.UserAgent, nil, nil, i.profile.Language, i.profile.Language, 0,
		navigatorProperties[n] + "−" + navigatorValues[n], documentProperties[int(choices[1])%len(documentProperties)], windowProperties[int(choices[2])%len(windowProperties)],
		float64(time.Since(i.started)) / float64(time.Millisecond), id.String(), "", i.profile.Concurrency, i.profile.TimeOrigin, 0, 0, 0, 0, 0, 0, 0}
	return env, nil
}

// Headers consumes only this prepare's response; tokens are never cached.
func (i *Integrity) Headers(ctx context.Context, body []byte) (http.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var r struct {
		Token        string `json:"token"`
		PrepareToken string `json:"prepare_token"`
		ForceLogin   bool   `json:"force_login"`
		Proof        struct {
			Required   bool   `json:"required"`
			Seed       string `json:"seed"`
			Difficulty string `json:"difficulty"`
		} `json:"proofofwork"`
		Turnstile struct {
			Required bool   `json:"required"`
			DX       string `json:"dx"`
		} `json:"turnstile"`
	}
	if len(body) > 1<<20 || len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' || json.Unmarshal(body, &r) != nil {
		return nil, ErrRequirements
	}
	if r.ForceLogin {
		return nil, ErrForceLogin
	}
	headers := make(http.Header)
	if r.Token != "" {
		headers.Set("OpenAI-Sentinel-Chat-Requirements-Token", r.Token)
	} else if r.PrepareToken != "" {
		headers.Set("OpenAI-Sentinel-Chat-Requirements-Prepare-Token", r.PrepareToken)
	}
	for _, values := range headers {
		for _, value := range values {
			if len(value) > 256<<10 || strings.ContainsAny(value, "\r\n\x00") {
				return nil, ErrRequirements
			}
		}
	}
	if !r.Proof.Required && !r.Turnstile.Required {
		return headers, nil
	}
	if r.Proof.Required && (r.Proof.Seed == "" || r.Proof.Difficulty == "") || r.Turnstile.Required && r.Turnstile.DX == "" {
		return nil, ErrChallenge
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case integritySlots <- struct{}{}:
		defer func() { <-integritySlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if r.Proof.Required {
		proof, err := i.proof(ctx, r.Proof.Seed, r.Proof.Difficulty, 500000)
		if err != nil {
			return nil, err
		}
		headers.Set("OpenAI-Sentinel-Proof-Token", proof)
	}
	if r.Turnstile.Required {
		token, err := i.solveVM(ctx, r.Turnstile.DX)
		if err != nil {
			return nil, err
		}
		if token != "" {
			headers.Set("OpenAI-Sentinel-Turnstile-Token", token)
		}
	}
	for _, values := range headers {
		for _, value := range values {
			if len(value) > 256<<10 || strings.ContainsAny(value, "\r\n\x00") {
				return nil, ErrChallenge
			}
		}
	}
	return headers, nil
}

var integritySlots = make(chan struct{}, max(1, min(4, runtime.GOMAXPROCS(0))))

func integrityJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	// JSON.stringify emits these Unicode characters literally.
	data := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	if !bytes.Contains(data, []byte(`\u2028`)) && !bytes.Contains(data, []byte(`\u2029`)) {
		return data, nil
	}
	result := make([]byte, 0, len(data))
	for n := 0; n < len(data); {
		if data[n] != '\\' {
			result = append(result, data[n])
			n++
			continue
		}
		end := n
		for end < len(data) && data[end] == '\\' {
			end++
		}
		// An even run encodes literal backslashes, including a literal
		// backslash followed by u2028/u2029. Preserve it exactly.
		if (end-n)%2 == 1 && end+5 <= len(data) && (string(data[end:end+5]) == "u2028" || string(data[end:end+5]) == "u2029") {
			result = append(result, data[n:end-1]...)
			if data[end+4] == '8' {
				result = append(result, '\xe2', '\x80', '\xa8')
			} else {
				result = append(result, '\xe2', '\x80', '\xa9')
			}
			n = end + 5
		} else {
			result = append(result, data[n:end]...)
			n = end
		}
	}
	return result, nil
}
