package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/grouproutingscheme"
	"github.com/Wei-Shaw/sub2api/ent/grouproutingschemeitem"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type routingSchemeRepository struct {
	client *dbent.Client
}

func NewRoutingSchemeRepository(client *dbent.Client) service.RoutingSchemeRepository {
	return &routingSchemeRepository{client: client}
}

func (r *routingSchemeRepository) List(ctx context.Context, platform string, includeDisabled bool) ([]service.RoutingScheme, error) {
	query := r.client.GroupRoutingScheme.Query().
		Where(grouproutingscheme.DeletedAtIsNil()).
		WithItems(func(q *dbent.GroupRoutingSchemeItemQuery) {
			q.Order(dbent.Asc(grouproutingschemeitem.FieldPosition), dbent.Asc(grouproutingschemeitem.FieldID))
		}).
		Order(dbent.Asc(grouproutingscheme.FieldSortOrder), dbent.Asc(grouproutingscheme.FieldID))
	if platform != "" {
		query.Where(grouproutingscheme.PlatformEQ(platform))
	}
	if !includeDisabled {
		query.Where(grouproutingscheme.StatusEQ(domain.RoutingSchemeStatusActive))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.RoutingScheme, 0, len(rows))
	for _, row := range rows {
		out = append(out, routingSchemeEntityToService(row))
	}
	return out, nil
}

func (r *routingSchemeRepository) GetByID(ctx context.Context, id int64) (*service.RoutingScheme, error) {
	row, err := r.client.GroupRoutingScheme.Query().
		Where(grouproutingscheme.IDEQ(id), grouproutingscheme.DeletedAtIsNil()).
		WithItems(func(q *dbent.GroupRoutingSchemeItemQuery) {
			q.Order(dbent.Asc(grouproutingschemeitem.FieldPosition), dbent.Asc(grouproutingschemeitem.FieldID))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRoutingSchemeNotFound
		}
		return nil, err
	}
	scheme := routingSchemeEntityToService(row)
	return &scheme, nil
}

func (r *routingSchemeRepository) Create(ctx context.Context, scheme *service.RoutingScheme) error {
	if scheme == nil {
		return fmt.Errorf("routing scheme is nil")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	created, err := tx.GroupRoutingScheme.Create().
		SetName(scheme.Name).
		SetDescription(scheme.Description).
		SetPlatform(scheme.Platform).
		SetStatus(scheme.Status).
		SetSortOrder(scheme.SortOrder).
		SetSortMode(scheme.SortMode).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := createRoutingSchemeItems(ctx, tx, created.ID, scheme.Items); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	scheme.ID = created.ID
	scheme.CreatedAt = created.CreatedAt
	scheme.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *routingSchemeRepository) Update(ctx context.Context, scheme *service.RoutingScheme) error {
	if scheme == nil || scheme.ID <= 0 {
		return service.ErrRoutingSchemeNotFound
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	updated, err := tx.GroupRoutingScheme.UpdateOneID(scheme.ID).
		SetName(scheme.Name).
		SetDescription(scheme.Description).
		SetStatus(scheme.Status).
		SetSortOrder(scheme.SortOrder).
		SetSortMode(scheme.SortMode).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if dbent.IsNotFound(err) {
			return service.ErrRoutingSchemeNotFound
		}
		return err
	}
	if _, err := tx.GroupRoutingSchemeItem.Delete().
		Where(grouproutingschemeitem.SchemeIDEQ(scheme.ID)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := createRoutingSchemeItems(ctx, tx, scheme.ID, scheme.Items); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	scheme.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *routingSchemeRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := tx.GroupRoutingScheme.UpdateOneID(id).
		SetDeletedAt(time.Now()).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		if dbent.IsNotFound(err) {
			return service.ErrRoutingSchemeNotFound
		}
		return err
	}
	// Keys must not keep a dangling scheme reference after the scheme is gone.
	if _, err := tx.APIKey.Update().
		Where(apikey.RoutingSchemeIDEQ(id)).
		ClearRoutingSchemeID().
		Save(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createRoutingSchemeItems(ctx context.Context, tx *dbent.Tx, schemeID int64, items []service.RoutingSchemeItem) error {
	for position, item := range items {
		rules := item.Rules
		if rules == nil {
			rules = []domain.RoutingSchemeRule{}
		}
		if _, err := tx.GroupRoutingSchemeItem.Create().
			SetSchemeID(schemeID).
			SetGroupID(item.GroupID).
			SetWeight(item.Weight).
			SetPosition(position).
			SetRules(rules).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func routingSchemeEntityToService(row *dbent.GroupRoutingScheme) service.RoutingScheme {
	if row == nil {
		return service.RoutingScheme{}
	}
	items := make([]service.RoutingSchemeItem, 0, len(row.Edges.Items))
	for _, item := range row.Edges.Items {
		items = append(items, service.RoutingSchemeItem{
			GroupID:  item.GroupID,
			Weight:   item.Weight,
			Position: item.Position,
			Rules:    item.Rules,
		})
	}
	description := ""
	if row.Description != nil {
		description = *row.Description
	}
	return service.RoutingScheme{
		ID:          row.ID,
		Name:        row.Name,
		Description: description,
		Platform:    row.Platform,
		Status:      row.Status,
		SortOrder:   row.SortOrder,
		SortMode:    row.SortMode,
		Items:       items,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
