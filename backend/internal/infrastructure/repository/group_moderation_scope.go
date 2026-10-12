package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/lib/pq"
)

// Config writers and deleters lock groups first, then the setting row.
func lockLiveGroups(ctx context.Context, client *ent.Client, ids []int64) error {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil
	}
	rows, err := client.QueryContext(ctx, `SELECT id FROM groups WHERE id = ANY($1::bigint[]) AND deleted_at IS NULL ORDER BY id FOR SHARE`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(ids) {
		return service.ErrGroupNotFound
	}
	return nil
}

func cleanupDeletedModerationGroup(ctx context.Context, client *ent.Client, id int64) error {
	var value string
	err := scanSingleRow(ctx, client, `SELECT value FROM settings WHERE key = $1 FOR UPDATE`, []any{service.SettingKeyContentModerationConfig}, &value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Preserve malformed legacy settings; deletion must still succeed.
	if !json.Valid([]byte(value)) {
		return nil
	}
	_, err = client.ExecContext(ctx, `UPDATE settings
	SET value = jsonb_set(value::jsonb, '{group_ids}',
	  CASE WHEN jsonb_typeof(value::jsonb->'group_ids') = 'array' THEN
	    (SELECT COALESCE(jsonb_agg(group_id ORDER BY ord), '[]'::jsonb)
	     FROM jsonb_array_elements(value::jsonb->'group_ids') WITH ORDINALITY AS ids(group_id, ord)
	     WHERE group_id <> to_jsonb($2::bigint))
	  ELSE value::jsonb->'group_ids' END)::text, updated_at = NOW()
	WHERE key = $1 AND jsonb_typeof(value::jsonb->'group_ids') = 'array'
	  AND value::jsonb->'group_ids' @> jsonb_build_array($2::bigint)`, service.SettingKeyContentModerationConfig, id)
	return err
}
