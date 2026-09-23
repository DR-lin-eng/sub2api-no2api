package service

import (
	"context"
	"time"
)

// QualityArtifact is the immutable result of rendering an account's generated
// SVG/HTML. The quality service assigns the verdict from source-code matching.
type QualityArtifact struct {
	Label        string
	Confidence   float64
	ModelVersion string
	PNG          []byte
	WebP         []byte
}

type AccountQualityArtifactProcessor interface {
	Process(context.Context, string) (*QualityArtifact, error)
}

type AccountQualityRun struct {
	ID           string
	AccountID    int64
	Model        string
	Effort       string
	Status       string
	Label        string
	Confidence   float64
	StartedAt    time.Time
	FinishedAt   time.Time
	LatencyMs    int64
	ModelVersion string
	PNG          []byte
	WebP         []byte
	Error        string
	Details      AccountQualityProbeDetails
	HasPreview   bool
}

type AccountQualityArtifactRepository interface {
	Save(context.Context, *AccountQualityRun) error
	ListPublic(context.Context, time.Time, int) ([]AccountQualityRun, error)
	GetPublicImage(context.Context, string, string) ([]byte, string, error)
}
