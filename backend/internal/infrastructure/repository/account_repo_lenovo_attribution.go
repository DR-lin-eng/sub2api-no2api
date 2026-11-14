package repository

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/lenovoattribution"
)

// CompareAndSwapOpenAILenovoAttribution serializes both installation creation
// and network leases across replicas. Unrelated extra fields are merged; the
// result can only be committed by the same credentials, proxy and lease.
func (r *accountRepository) CompareAndSwapOpenAILenovoAttribution(ctx context.Context, account *service.Account, expected any, next *lenovoattribution.State) (bool, error) {
	if account == nil || next == nil {
		return false, service.ErrAccountNilInput
	}
	credentials, err := json.Marshal(account.Credentials)
	if err != nil {
		return false, err
	}
	before, err := json.Marshal(expected)
	if err != nil {
		return false, err
	}
	after, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, `
		UPDATE accounts SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb),
			'{openai_lenovo_attribution}', $4::jsonb), updated_at = NOW()
		WHERE id = $1 AND credentials = $2::jsonb
			AND COALESCE(extra -> 'openai_lenovo_attribution', 'null'::jsonb) = $3::jsonb
			AND proxy_id IS NOT DISTINCT FROM $5
			AND COALESCE(extra -> 'lenovo_device_serial_digest', 'null'::jsonb)
				IS NOT DISTINCT FROM $6::jsonb
			AND platform = 'openai' AND type = 'oauth' AND parent_account_id IS NULL
			AND status = 'active' AND deleted_at IS NULL
	`, account.ID, string(credentials), string(before), string(after), proxyID, lenovoAttributionDigestJSON(account))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func lenovoAttributionDigestJSON(account *service.Account) string {
	raw, _ := json.Marshal(account.Extra[lenovoattribution.DigestExtraKey])
	return string(raw)
}

var _ service.OpenAILenovoAttributionRepository = (*accountRepository)(nil)
