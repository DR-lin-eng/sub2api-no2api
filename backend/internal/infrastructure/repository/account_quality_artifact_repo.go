package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
)

const qualityArtifactMaxHTMLBytes = 1 << 20
const qualityArtifactMaxBytes = 4 << 20

// HTTPAccountQualityArtifactProcessor keeps generated HTML in a private,
// network-isolated renderer. The renderer returns only raster artifacts and a
// classifier decision; raw HTML never reaches the public API.
type httpAccountQualityArtifactProcessor struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewAccountQualityArtifactProcessor() service.AccountQualityArtifactProcessor {
	return &httpAccountQualityArtifactProcessor{
		endpoint: strings.TrimRight(strings.TrimSpace(os.Getenv("ACCOUNT_QUALITY_RENDERER_URL")), "/"),
		token:    strings.TrimSpace(os.Getenv("ACCOUNT_QUALITY_RENDERER_TOKEN")),
		client:   &http.Client{Timeout: 50 * time.Second},
	}
}

func (p *httpAccountQualityArtifactProcessor) Process(ctx context.Context, html string) (*service.QualityArtifact, error) {
	if p == nil || p.endpoint == "" {
		return nil, errors.New("account quality renderer is not configured")
	}
	if len([]byte(html)) > qualityArtifactMaxHTMLBytes {
		return nil, errors.New("generated HTML exceeds renderer limit")
	}
	body, err := json.Marshal(map[string]string{"html": html})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/v1/process", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quality renderer returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Label        string  `json:"label"`
		Confidence   float64 `json:"confidence"`
		ModelVersion string  `json:"model_version"`
		PNG          string  `json:"png_base64"`
		WebP         string  `json:"webp_base64"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	png, err := base64.StdEncoding.DecodeString(payload.PNG)
	if err != nil || len(png) == 0 {
		return nil, errors.New("quality renderer returned invalid PNG")
	}
	webp, err := base64.StdEncoding.DecodeString(payload.WebP)
	if err != nil || len(webp) == 0 {
		return nil, errors.New("quality renderer returned invalid WebP")
	}
	return &service.QualityArtifact{Label: payload.Label, Confidence: payload.Confidence, ModelVersion: payload.ModelVersion, PNG: png, WebP: webp}, nil
}

type accountQualityArtifactRepository struct{ db *sql.DB }

func NewAccountQualityArtifactRepository(db *sql.DB) service.AccountQualityArtifactRepository {
	return &accountQualityArtifactRepository{db: db}
}

func (r *accountQualityArtifactRepository) Save(ctx context.Context, run *service.AccountQualityRun) error {
	if r == nil || r.db == nil || run == nil {
		return errors.New("quality artifact repository unavailable")
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status != "ready" && run.Status != "wrong" && run.Status != "uncertain" && run.Status != "error" {
		return errors.New("invalid quality artifact status")
	}
	if run.Confidence < 0 || run.Confidence > 1 {
		return errors.New("invalid quality artifact confidence")
	}
	if len(run.PNG) > qualityArtifactMaxBytes || len(run.WebP) > qualityArtifactMaxBytes {
		return errors.New("quality artifact exceeds size limit")
	}
	details, err := json.Marshal(run.Details)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO account_quality_runs
		(id, account_id, model, effort, status, label, confidence, started_at, finished_at, latency_ms, model_version, png, webp, error_message, probe_details)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb)
	`, run.ID, run.AccountID, run.Model, run.Effort, run.Status, run.Label, run.Confidence,
		run.StartedAt, run.FinishedAt, run.LatencyMs, run.ModelVersion, run.PNG, run.WebP, run.Error, string(details))
	if err != nil {
		return err
	}
	_, _ = r.db.ExecContext(ctx, `DELETE FROM account_quality_runs WHERE finished_at < NOW() - INTERVAL '24 hours'`)
	// Keep the public gallery bounded like the reference page; aggregate state
	// remains in accounts.extra and is not affected by artifact pruning.
	_, _ = r.db.ExecContext(ctx, `DELETE FROM account_quality_runs WHERE id IN (SELECT id FROM account_quality_runs ORDER BY finished_at DESC OFFSET 200)`)
	return nil
}

func (r *accountQualityArtifactRepository) ListPublic(ctx context.Context, since time.Time, limit int) ([]service.AccountQualityRun, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("quality artifact repository unavailable")
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, account_id, model, effort, status, label, confidence, started_at, finished_at, latency_ms, model_version, error_message, probe_details, COALESCE(octet_length(webp),0) > 0
		FROM account_quality_runs WHERE finished_at >= $1 ORDER BY finished_at DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.AccountQualityRun, 0, limit)
	for rows.Next() {
		var run service.AccountQualityRun
		var details []byte
		if err := rows.Scan(&run.ID, &run.AccountID, &run.Model, &run.Effort, &run.Status, &run.Label, &run.Confidence, &run.StartedAt, &run.FinishedAt, &run.LatencyMs, &run.ModelVersion, &run.Error, &details, &run.HasPreview); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(details, &run.Details); err != nil {
			return nil, fmt.Errorf("decode quality details: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *accountQualityArtifactRepository) GetPublicImage(ctx context.Context, id, format string) ([]byte, string, error) {
	if r == nil || r.db == nil {
		return nil, "", errors.New("quality artifact repository unavailable")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	column := "png"
	contentType := "image/png"
	if format == "webp" {
		column, contentType = "webp", "image/webp"
	}
	if format != "png" && format != "webp" {
		return nil, "", errors.New("unsupported quality image format")
	}
	var data []byte
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM account_quality_runs WHERE id = $1::uuid AND finished_at >= NOW() - INTERVAL '24 hours' AND octet_length(%s) > 0", column, column), id).Scan(&data); err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}
