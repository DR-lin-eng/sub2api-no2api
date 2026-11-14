package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

// PostgreSQL serializes competing first requests on the account row. Every
// replica receives the winning offset, including when its snapshot is stale.
func (r *accountRepository) GetOrCreateOpenAILocationOffset(ctx context.Context, id int64, candidate service.OpenAILocationOffset) (service.OpenAILocationOffset, error) {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return service.OpenAILocationOffset{}, err
	}
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `
		UPDATE accounts SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb),
			'{openai_location_persistent_offset}',
			CASE WHEN
				jsonb_typeof(extra #> '{openai_location_persistent_offset,north_meters}') = 'number'
				AND jsonb_typeof(extra #> '{openai_location_persistent_offset,east_meters}') = 'number'
			THEN CASE WHEN
				power((extra #>> '{openai_location_persistent_offset,north_meters}')::numeric, 2) +
				power((extra #>> '{openai_location_persistent_offset,east_meters}')::numeric, 2) <= 250000
				THEN extra -> 'openai_location_persistent_offset' ELSE $2::jsonb END
			ELSE $2::jsonb END)
		WHERE id = $1 AND platform = 'openai' AND deleted_at IS NULL
		RETURNING extra -> 'openai_location_persistent_offset'
	`, id, string(raw))
	if err != nil {
		return service.OpenAILocationOffset{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return service.OpenAILocationOffset{}, err
		}
		return service.OpenAILocationOffset{}, service.ErrAccountNotFound
	}
	var stored []byte
	if err := rows.Scan(&stored); err != nil {
		return service.OpenAILocationOffset{}, err
	}
	var offset service.OpenAILocationOffset
	if err := json.Unmarshal(stored, &offset); err != nil {
		return offset, fmt.Errorf("decode persisted location offset: %w", err)
	}
	return offset, rows.Err()
}

var _ service.OpenAILocationOffsetRepository = (*accountRepository)(nil)
