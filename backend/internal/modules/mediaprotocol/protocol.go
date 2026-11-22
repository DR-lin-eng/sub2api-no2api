// Package mediaprotocol translates gateway media operations to native provider protocols.
package mediaprotocol

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Operation string

const (
	ImageGenerate Operation = "images_generations"
	ImageEdit     Operation = "images_edits"
	VideoGenerate Operation = "videos_generations"
	VideoEdit     Operation = "videos_edits"
	VideoExtend   Operation = "videos_extensions"
	VideoStatus   Operation = "video_status"
	VideoContent  Operation = "video_content"
)

func (o Operation) IsImageRequest() bool { return o == ImageGenerate || o == ImageEdit }

func (o Operation) IsVideoGenerationRequest() bool {
	return o == VideoGenerate || o == VideoEdit || o == VideoExtend
}

func (o Operation) IsVideoLookupRequest() bool { return o == VideoStatus || o == VideoContent }
func (o Operation) IsGenerationRequest() bool {
	return o.IsImageRequest() || o.IsVideoGenerationRequest()
}
func (o Operation) RequiresRequestBody() bool { return o.IsGenerationRequest() }

// Driver translates a selected account's media protocol. The gateway retains
// ownership of network access, authorization, model mapping and settlement.
type Driver interface {
	Name() string
	URL(base string, operation Operation, upstreamID string) (string, error)
	Encode(operation Operation, fields map[string]any) ([]byte, error)
	DecodeImages(body []byte, format string) ([]byte, error)
	DecodeTask(operation Operation, body []byte, expectedID string) (Task, error)
}

var ErrUnsupported = errors.New("unsupported media request")

type Task struct {
	ID         string
	Status     string
	Seconds    int
	Resolution string
	CreatedAt  int64
	Error      string
	Artifact   string
}

func IsRelay(name string) bool {
	name = strings.TrimSpace(name)
	return name == "" || name == "relay"
}

// Select returns nil for the existing compatible relay.
func Select(name string) (Driver, error) {
	if IsRelay(name) {
		return nil, nil
	}
	if strings.TrimSpace(name) == "ark" {
		return ark{}, nil
	}
	return nil, fmt.Errorf("unknown media protocol %q", name)
}

// The key/hold binding is the authority. The tag distinguishes equal upstream
// IDs on different accounts and preserves the submit protocol across edits.
func PublicTaskID(d Driver, accountID int64, raw string) (string, error) {
	if accountID <= 0 || !validTaskID(raw) {
		return "", errors.New("invalid upstream video task ID")
	}
	scope := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", d.Name(), accountID, raw)))
	return fmt.Sprintf("s2m1.%s.%x.%s", d.Name(), scope[:8], raw), nil
}

// Lookup is called only after resolving the original account. Untagged IDs
// (including pre-upgrade work on an account now using a driver) use the relay.
func Lookup(accountID int64, id string) (Driver, string) {
	if !strings.HasPrefix(id, "s2m1.") {
		return nil, id
	}
	parts := strings.SplitN(id, ".", 4)
	if len(parts) == 4 && parts[0] == "s2m1" {
		if driver, err := Select(parts[1]); err == nil && driver != nil {
			if encoded, err := PublicTaskID(driver, accountID, parts[3]); err == nil && encoded == id {
				return driver, parts[3]
			}
		}
	}
	return nil, id
}

func validTaskID(id string) bool {
	if len(id) == 0 || len(id) > 200 {
		return false
	}
	for _, c := range id {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if !letter && !digit && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func validArtifactURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" &&
		!strings.ContainsAny(raw, "\r\n\\")
}
