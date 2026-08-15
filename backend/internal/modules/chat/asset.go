package chat

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"
)

const (
	MaxAssetBytes        = 5 << 20
	MaxCatalogPageSize   = 100
	assetCleanupBatch    = 100
	assetCleanupMaxAge   = time.Hour
	assetCleanupInterval = 5 * time.Minute
)

var errAssetRepositoryUnavailable = errors.New("chat asset repository is unavailable")
var nextAssetCleanupUnix atomic.Int64

type AssetUpload struct {
	Name       string
	MIMEType   string
	Data       []byte
	Collection string
}

func (s *Service) UploadAssetForUser(ctx context.Context, userID int64, input AssetUpload) (*Asset, error) {
	conv, err := s.conversationRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.createAsset(ctx, AssetScopeMessage, &conv.ID, userID, input)
}

func (s *Service) UploadAssetForAdmin(ctx context.Context, conversationID, adminID int64, input AssetUpload) (*Asset, error) {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.createAsset(ctx, AssetScopeMessage, &conv.ID, adminID, input)
}

func (s *Service) CreateCatalogAsset(ctx context.Context, adminID int64, scope AssetScope, input AssetUpload) (*Asset, error) {
	if scope != AssetScopeLibrary && scope != AssetScopeSticker {
		return nil, ErrMessageAssetsInvalid
	}
	return s.createAsset(ctx, scope, nil, adminID, input)
}

func (s *Service) createAsset(
	ctx context.Context,
	scope AssetScope,
	conversationID *int64,
	uploaderID int64,
	input AssetUpload,
) (*Asset, error) {
	if s.assetRepo == nil {
		return nil, errAssetRepositoryUnavailable
	}
	if len(input.Data) == 0 || len(input.Data) > MaxAssetBytes || input.MIMEType == "" {
		return nil, ErrMessageAssetsInvalid
	}
	uploadedBy := uploaderID
	asset := &Asset{
		Scope:          scope,
		ConversationID: conversationID,
		UploadedBy:     &uploadedBy,
		Name:           trimRunes(input.Name, 255),
		MIMEType:       input.MIMEType,
		Size:           len(input.Data),
		Data:           input.Data,
		Collection:     trimRunes(input.Collection, 100),
		CatalogVisible: scope != AssetScopeMessage,
	}
	if asset.Name == "" {
		asset.Name = "image"
	}
	if err := s.assetRepo.Create(ctx, asset); err != nil {
		return nil, err
	}
	// Keep cleanup bounded and off the steady-state upload path. At most one
	// request per process performs the indexed cleanup in each interval.
	s.maybeCleanupAbandonedAssets(ctx)
	asset.Data = nil
	return asset, nil
}

func (s *Service) maybeCleanupAbandonedAssets(ctx context.Context) {
	now := time.Now()
	next := nextAssetCleanupUnix.Load()
	if next > now.Unix() || !nextAssetCleanupUnix.CompareAndSwap(next, now.Add(assetCleanupInterval).Unix()) {
		return
	}
	_, _ = s.assetRepo.DeleteUnattachedBefore(ctx, now.Add(-assetCleanupMaxAge), assetCleanupBatch)
}

func (s *Service) ListCatalogAssets(ctx context.Context, scope AssetScope, limit int) ([]Asset, error) {
	if s.assetRepo == nil {
		return nil, errAssetRepositoryUnavailable
	}
	if scope != AssetScopeLibrary && scope != AssetScopeSticker {
		return nil, ErrMessageAssetsInvalid
	}
	if limit <= 0 || limit > MaxCatalogPageSize {
		limit = MaxCatalogPageSize
	}
	return s.assetRepo.ListCatalog(ctx, scope, limit)
}

func (s *Service) HideCatalogAsset(ctx context.Context, scope AssetScope, id int64) error {
	if s.assetRepo == nil {
		return errAssetRepositoryUnavailable
	}
	if id <= 0 || (scope != AssetScopeLibrary && scope != AssetScopeSticker) {
		return ErrAssetNotFound
	}
	return s.assetRepo.HideCatalog(ctx, scope, id)
}

func (s *Service) GetAssetForUser(ctx context.Context, id, userID int64) (*Asset, error) {
	if s.assetRepo == nil {
		return nil, errAssetRepositoryUnavailable
	}
	conv, err := s.conversationRepo.GetByUserID(ctx, userID)
	if errors.Is(err, ErrConversationNotFound) {
		return nil, ErrAssetNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.assetRepo.GetForUser(ctx, id, userID, conv.ID)
}

func (s *Service) GetAssetForAdmin(ctx context.Context, id, adminID int64) (*Asset, error) {
	if s.assetRepo == nil {
		return nil, errAssetRepositoryUnavailable
	}
	if id <= 0 || adminID <= 0 {
		return nil, ErrAssetNotFound
	}
	return s.assetRepo.GetForAdmin(ctx, id, adminID)
}

func NormalizeAssetCollection(value string) string {
	return trimRunes(strings.TrimSpace(value), 100)
}

func NormalizeAssetName(value, fallback string) string {
	value = trimRunes(value, 120)
	if value == "" {
		return fallback
	}
	return value
}
