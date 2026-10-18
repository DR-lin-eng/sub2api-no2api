package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

// MergeUpstreamModelMapping changes just model_mapping. The row lock preserves
// mappings edited during the probe; a credential/proxy change requires a retry.
// The model update and scheduler outbox insertion commit in one SQL statement.
func (r *accountRepository) MergeUpstreamModelMapping(ctx context.Context, account *service.Account, models []string) (int, error) {
	if account == nil || account.IsCredentialShadow() {
		return 0, service.ErrUpstreamModelSyncAccountChanged
	}
	identity := make(map[string]any, len(account.Credentials))
	for key, value := range account.Credentials {
		if key != "model_mapping" {
			identity[key] = value
		}
	}
	expected, err := json.Marshal(identity)
	if err != nil {
		return 0, err
	}
	mapping := make(map[string]string, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			mapping[model] = model
		}
	}
	if len(mapping) == 0 {
		return 0, errors.New("upstream model mapping is empty")
	}
	payload, err := json.Marshal(mapping)
	if err != nil {
		return 0, err
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH current_account AS MATERIALIZED (
			SELECT id, CASE WHEN jsonb_typeof(credentials->'model_mapping') = 'object'
				THEN credentials->'model_mapping' ELSE '{}'::jsonb END AS mapping
			FROM accounts
			WHERE id = $1 AND deleted_at IS NULL AND parent_account_id IS NULL
				AND platform = $2 AND type = $3
				AND (COALESCE(credentials, '{}'::jsonb) - 'model_mapping') = $4::jsonb
				AND proxy_id IS NOT DISTINCT FROM $5
			FOR NO KEY UPDATE
		), updated AS (
			UPDATE accounts AS a SET credentials = jsonb_set(COALESCE(a.credentials, '{}'::jsonb),
				'{model_mapping}', $6::jsonb || current_account.mapping), updated_at = NOW()
			FROM current_account WHERE a.id = current_account.id
			RETURNING a.id, (SELECT COUNT(*) FROM jsonb_object_keys($6::jsonb) AS model(id)
				WHERE NOT (current_account.mapping ? model.id)) AS added
		), outbox AS (
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
			SELECT $7, id, NULL, NULL FROM updated RETURNING account_id
		)
		SELECT updated.added FROM updated JOIN outbox ON outbox.account_id = updated.id
	`, account.ID, account.Platform, account.Type, string(expected), account.ProxyID,
		string(payload), service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, service.ErrUpstreamModelSyncAccountChanged
	}
	var added int
	if err := rows.Scan(&added); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, account.ID)
	return added, nil
}
