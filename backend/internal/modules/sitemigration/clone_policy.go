package sitemigration

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/lib/pq"
)

const ClonePolicyHeader = "X-Sub2API-Clone-Policy"
const ClonePolicyVersion = "ordinary-users-v2"

var ErrClonePolicy = errors.New("both sites must support cloning without administrator accounts")

// These settings establish administrative authority or contain account-bound
// state. Keep them out of both the wire snapshot and the restored database.
var cloneLocalSettings = []string{"permission_groups", "legacy_admin_api_key_options"}
var cloneLocalPrefixes = []string{"admin_", "mcp_oauth", "oauth2_provider"}

func cloneSettingAllowed(k string) bool {
	if slices.Contains(cloneLocalSettings, k) {
		return false
	}
	for _, p := range cloneLocalPrefixes {
		if strings.HasPrefix(k, p) {
			return false
		}
	}
	return true
}

// Keep only identifiers of removed parents, never their credentials. Nullable
// historical attribution can be cleared; the owner's entire row is excluded.
func cloneRemovedRef(r ref) string {
	return "EXISTS (SELECT 1 FROM pg_temp.clone_removed rm WHERE rm.t=" + pq.QuoteLiteral(r.parent) + " AND rm.k=" + pq.QuoteLiteral(r.key) + " AND rm.v=src." + pq.QuoteIdentifier(r.col) + "::text)"
}
func cloneRequiresParent(t *table, r ref) bool {
	return !t.nullable[r.col] || r.col == "user_id" || r.col == "used_by" || r.parent == "api_keys" || r.parent == "auth_identities"
}
func cloneQueries(ctx context.Context, tx *sql.Tx, c catalog) (map[string]string, error) {
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE clone_removed(t text,k text,v text,PRIMARY KEY(t,k,v)) ON COMMIT DROP"); err != nil {
		return nil, err
	}
	targets := map[string][]string{}
	for _, t := range c.tables {
		for _, r := range t.refs {
			if c.tables[r.parent] != nil && !slices.Contains(targets[r.parent], r.key) {
				targets[r.parent] = append(targets[r.parent], r.key)
			}
		}
	}
	out := map[string]string{}
	for _, name := range c.order {
		t := c.tables[name]
		var excluded []string
		switch name {
		case "users":
			excluded = append(excluded, "src.role IS DISTINCT FROM 'user'")
		case "settings":
			for _, k := range cloneLocalSettings {
				excluded = append(excluded, "src.key="+pq.QuoteLiteral(k))
			}
			for _, p := range cloneLocalPrefixes {
				excluded = append(excluded, "starts_with(src.key,"+pq.QuoteLiteral(p)+")")
			}
		case "audit_logs":
			excluded = append(excluded, "src.actor_role NOT IN ('','user')")
		}
		nullableRefs := map[string]string{}
		cascade := false
		for _, r := range t.refs {
			if c.tables[r.parent] == nil {
				continue
			}
			if cloneRequiresParent(t, r) {
				excluded = append(excluded, cloneRemovedRef(r))
				if r.parent == name {
					cascade = true
				}
			} else {
				nullableRefs[r.col] = cloneRemovedRef(r)
			}
		}
		where := ""
		if len(excluded) > 0 {
			predicate := strings.Join(excluded, " OR ")
			where = " WHERE NOT (" + predicate + ")"
			for {
				var added int64
				for _, k := range targets[name] {
					res, e := tx.ExecContext(ctx, "INSERT INTO pg_temp.clone_removed SELECT $1::text,$2::text,src."+pq.QuoteIdentifier(k)+"::text FROM "+qt(name)+" src WHERE src."+pq.QuoteIdentifier(k)+" IS NOT NULL AND ("+predicate+") ON CONFLICT DO NOTHING", name, k)
					if e != nil {
						return nil, e
					}
					n, e := res.RowsAffected()
					if e != nil {
						return nil, e
					}
					added += n
				}
				if !cascade || added == 0 {
					break
				}
			}
		}
		cols := make([]string, 0, len(t.cols))
		for _, col := range t.cols {
			v := "src." + pq.QuoteIdentifier(col)
			if test := nullableRefs[col]; test != "" {
				v = "CASE WHEN " + test + " THEN NULL ELSE " + v + " END AS " + pq.QuoteIdentifier(col)
			}
			cols = append(cols, v)
		}
		out[name] = "SELECT row_to_json(x) FROM (SELECT " + strings.Join(cols, ",") + " FROM " + qt(name) + " src" + where + " ORDER BY " + columns(t.pk) + ") x"
	}
	return out, nil
}

func cloneSecrets(name string, r row, key []byte, export bool) error {
	if name != "settings" {
		return portableSecrets(name, r, key, export)
	}
	var field string
	switch r.str("key") {
	case "backup_s3_config", "image_storage_config":
		field = "secret_access_key"
	case "cloudflare_ingress_settings":
		field = "api_token_ciphertext"
	case "prompt_audit_config":
		field = "token_ciphertext"
	default:
		return nil
	}
	raw := r.str("value")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var obj any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&obj) != nil {
		return errors.New("invalid encrypted site configuration")
	}
	if err := rewriteCloneSecret(obj, field, key, export); err != nil {
		return err
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	r.set("value", string(b))
	return nil
}
func rewriteCloneSecret(x any, field string, key []byte, export bool) error {
	switch v := x.(type) {
	case map[string]any:
		for k, val := range v {
			if k == field {
				if val == nil {
					continue
				}
				s, ok := val.(string)
				if !ok {
					return ErrInvalid
				}
				s, e := changeSecret(s, key, export)
				if e != nil {
					return e
				}
				v[k] = s
			} else if err := rewriteCloneSecret(val, field, key, export); err != nil {
				return err
			}
		}
	case []any:
		for _, val := range v {
			if err := rewriteCloneSecret(val, field, key, export); err != nil {
				return err
			}
		}
	}
	return nil
}
func cloneStorageKey(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	// JWT and at-rest encryption roots are local to the new DB. Never seed
	// them from the source (or from the receiving site's live JWT root).
	for _, k := range []string{"jwt_secret", "totp_encryption_key"} {
		var key [32]byte
		if _, e := rand.Read(key[:]); e != nil {
			return nil, e
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO public.security_secrets(key,value,created_at,updated_at) VALUES($1,$2,now(),now()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()", k, hex.EncodeToString(key[:])); e != nil {
			return nil, e
		}
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO public.security_secrets(key,value,created_at,updated_at) VALUES('installation_complete','v1',now(),now()) ON CONFLICT(key) DO NOTHING")
	if err != nil {
		return nil, err
	}
	return storageKey(ctx, tx)
}
func validateCloneRecord(t *table, r row) error {
	switch t.name {
	case "users":
		if !bytes.Equal(r["role"], []byte(`"user"`)) {
			return ErrDenied
		}
	case "settings":
		if !cloneSettingAllowed(r.str("key")) {
			return ErrDenied
		}
	}
	return nil
}
