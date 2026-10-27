package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
)

const UpstreamModelMetadataExtraKey = modelcatalog.ExtraKey

var ErrUpstreamModelCatalogChanged = errors.New("upstream model catalog or account identity changed during refresh")

type upstreamModelCatalogRepository interface {
	UpdateUpstreamModelCatalog(context.Context, *Account, *modelcatalog.Snapshot) error
}
type upstreamModelCatalogFetcher interface {
	FetchUpstreamModelCatalog(context.Context, *Account) (*modelcatalog.Catalog, error)
}
type modelCatalogCaptureKey struct{}
type modelCatalogCapture struct {
	metadata map[string]modelcatalog.Descriptor
	scope    string
}

func captureUpstreamModelCatalog(ctx context.Context, body []byte, ids []string) error {
	capture, _ := ctx.Value(modelCatalogCaptureKey{}).(*modelCatalogCapture)
	if capture == nil {
		return nil
	}
	metadata, err := modelcatalog.Parse(body, ids)
	if err != nil {
		return newUpstreamModelSyncUpstreamError("Upstream model metadata is invalid or incomplete", err)
	}
	for id, d := range metadata {
		if _, duplicate := capture.metadata[id]; duplicate {
			return newUpstreamModelSyncUpstreamError("Upstream model catalog contains duplicate models", nil)
		}
		capture.metadata[id] = d
	}
	return nil
}

func (s *AccountTestService) FetchUpstreamSupportedModels(ctx context.Context, account *Account) ([]string, error) {
	catalog, err := s.FetchUpstreamModelCatalog(ctx, account)
	if err != nil {
		return nil, err
	}
	return catalog.Models, nil
}

// FetchUpstreamModelCatalog reuses the selected account's transport and reads
// metadata from the same bounded response. It has no persistence side effects.
func (s *AccountTestService) FetchUpstreamModelCatalog(ctx context.Context, account *Account) (*modelcatalog.Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	capture := &modelCatalogCapture{metadata: map[string]modelcatalog.Descriptor{}}
	ids, err := s.fetchUpstreamSupportedModels(context.WithValue(ctx, modelCatalogCaptureKey{}, capture), account)
	if err != nil {
		return nil, err
	}
	catalog := &modelcatalog.Catalog{Models: ids, Metadata: capture.metadata, DiscoveryScope: capture.scope}
	if _, err := catalog.Snapshot("upstream", time.Now()); err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Upstream model metadata exceeds limits", err)
	}
	return catalog, nil
}

// SyncUpstreamModelCatalog is the manual admin application use case. Preview
// uses Fetch only. Background refresh persists through the same narrow port.
func (s *AccountTestService) SyncUpstreamModelCatalog(ctx context.Context, account *Account) (*modelcatalog.Catalog, error) {
	started := time.Now()
	catalog, err := s.FetchUpstreamModelCatalog(ctx, account)
	if err != nil {
		return nil, err
	}
	if account.ID > 0 && s.accountRepo != nil {
		repo, ok := s.accountRepo.(upstreamModelCatalogRepository)
		if !ok {
			return nil, fmt.Errorf("model catalog persistence is not configured")
		}
		snapshot, err := catalog.Snapshot(account.Platform, started)
		if err != nil {
			return nil, err
		}
		if err := repo.UpdateUpstreamModelCatalog(ctx, account, snapshot); err != nil {
			return nil, err
		}
		if account.IsGrok() {
			extra := make(map[string]any, len(account.Extra)+1)
			for key, value := range account.Extra {
				extra[key] = value
			}
			extra[modelcatalog.ExtraKey] = snapshot
			account.Extra = extra
		}
	}
	return catalog, nil
}
